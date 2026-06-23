const express = require('express');
const fs = require('fs');
const path = require('path');
const { v4: uuidv4, validate: validateUuid } = require('uuid');

const db = require('../models/db');
const dockerService = require('../services/docker');
const egressOps = require('../services/egress-ops');

const router = express.Router();
const EGRESS_NAME_RE = /^[a-zA-Z0-9_-]+$/;

function validateEgressName(name) {
  return typeof name === 'string' && EGRESS_NAME_RE.test(name);
}

function getVpsHost(req) {
  if (process.env.VPS_ADDRESS) return process.env.VPS_ADDRESS;
  if (req.headers.host) return req.headers.host.split(':')[0];
  return 'your_vps_ip';
}

function buildSubscriptionPayload(req) {
  const list = db.getEgresses();
  const vpsHost = getVpsHost(req);
  const links = list.map(e => egressOps.buildLink(e, vpsHost));
  const subscription = Buffer.from(links.join('\n'), 'utf-8').toString('base64');
  return { list, vpsHost, links, subscription };
}

async function enqueueJob(type, target, payload) {
  const job = db.createJob({ type, target, payload });
  return job;
}

router.post('/create', async (req, res) => {
  try {
    const { name, region, uuid } = req.body;
    if (!validateEgressName(name)) return res.status(400).json({ error: '名称无效' });
    if (!region || region.length !== 2) return res.status(400).json({ error: '地区代码无效' });
    if (uuid && !validateUuid(uuid)) return res.status(400).json({ error: 'UUID 格式无效' });

    const job = await enqueueJob('create', name, { name, region, uuid: uuid || uuidv4() });

    res.json({ success: true, jobId: job.id, message: `出口 ${name} 创建任务已提交` });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.post('/create-all', async (req, res) => {
  try {
    const { regions, uuid } = req.body || {};
    const job = await enqueueJob('create-all', 'all', { regions, uuid });
    res.json({ success: true, jobId: job.id, message: '批量创建任务已提交' });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.get('/list', async (req, res) => {
  try {
    const list = db.getEgresses();
    const result = [];
    for (let egress of list) {
      const dockerStatus = await dockerService.getContainerStatusAndIp(egress.name);
      const shouldKeep = ['starting', 'rebuilding'].includes(egress.status) && dockerStatus.status === 'offline';
      const resolvedStatus = dockerStatus.ip === 'error' ? 'error' : dockerStatus.status;
      const updates = {
        status: shouldKeep ? egress.status : resolvedStatus,
        error: dockerStatus.error || '',
        lastCheckTime: Date.now()
      };
      if (dockerStatus.ip && dockerStatus.ip !== 'offline' && dockerStatus.ip !== 'error') {
        updates.currentEgressIp = dockerStatus.ip;
      }
      result.push(db.updateEgress(egress.name, updates) || egress);
    }
    res.json(result);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.post('/delete', async (req, res) => {
  try {
    const { name } = req.body;
    if (!validateEgressName(name)) return res.status(400).json({ error: '名称无效' });
    const job = await enqueueJob('delete', name, { name });
    res.json({ success: true, jobId: job.id, message: `出口 ${name} 删除任务已提交` });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.post('/delete-all', async (req, res) => {
  try {
    const job = await enqueueJob('delete-all', 'all', {});
    res.json({ success: true, jobId: job.id, message: '删除全部任务已提交' });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.post('/rebuild', async (req, res) => {
  try {
    const { name } = req.body;
    if (!validateEgressName(name)) return res.status(400).json({ error: '名称无效' });
    const job = await enqueueJob('rebuild', name, { name });
    res.json({ success: true, jobId: job.id, message: `出口 ${name} 漂移任务已提交` });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.post('/rebuild-all', async (req, res) => {
  try {
    const job = await enqueueJob('rebuild-all', 'all', {});
    res.json({ success: true, jobId: job.id, message: '批量漂移任务已提交' });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.get('/subscription', (req, res) => {
  try {
    const { links, subscription } = buildSubscriptionPayload(req);
    res.json({ subscription, links, count: links.length });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.get('/subscription.txt', (req, res) => {
  try {
    const { subscription } = buildSubscriptionPayload(req);
    res.type('text/plain; charset=utf-8').send(subscription);
  } catch (error) {
    res.status(500).type('text/plain; charset=utf-8').send(error.message);
  }
});

router.get('/jobs', (req, res) => {
  try {
    const limit = Math.min(Math.max(parseInt(req.query.limit, 10) || 50, 1), 200);
    res.json(db.listJobs(limit));
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.get('/:name/link', (req, res) => {
  try {
    if (!validateEgressName(req.params.name)) return res.status(400).json({ error: '名称无效' });
    const egress = db.getEgress(req.params.name);
    if (!egress) return res.status(404).json({ error: '未找到' });
    res.json({ link: egressOps.buildLink(egress, getVpsHost(req)) });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

router.get('/:name/logs', async (req, res) => {
  try {
    if (!validateEgressName(req.params.name)) return res.status(400).json({ error: '名称无效' });
    if (!db.getEgress(req.params.name)) return res.status(404).json({ error: '未找到' });
    const tail = Math.min(Math.max(parseInt(req.query.tail, 10) || 120, 20), 500);
    res.json({ logs: await dockerService.getContainerLogs(req.params.name, tail) });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

module.exports = router;
