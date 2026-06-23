// 出口管理路由接口

const express = require('express');
const fs = require('fs');
const path = require('path');
const { v4: uuidv4, validate: validateUuid } = require('uuid');

const db = require('../models/db');
const dockerService = require('../services/docker');
const vpngateFetcher = require('../services/vpngate-fetcher');
const xrayConfigGen = require('../services/xray-config-gen');
const realityKeygen = require('../services/reality-keygen');

const router = express.Router();

const SERVER_NAME = process.env.RE_DOMAINS || 'www.amd.com';
const DEST_DOMAIN = `${SERVER_NAME}:443`;
const START_PORT = 44301;
const END_PORT = 44400;
const EGRESS_NAME_RE = /^[a-zA-Z0-9_-]+$/;

function validateEgressName(name) {
  return typeof name === 'string' && EGRESS_NAME_RE.test(name);
}

function allocatePort() {
  const egresses = db.getEgresses();
  const usedPorts = new Set(egresses.map(e => Number(e.port)).filter(Boolean));
  for (let port = START_PORT; port <= END_PORT; port++) {
    if (!usedPorts.has(port)) return port;
  }
  throw new Error(`端口已耗尽 (${START_PORT}-${END_PORT})`);
}

function getVpsHost(req) {
  if (process.env.VPS_ADDRESS) return process.env.VPS_ADDRESS;
  if (req.headers.host) return req.headers.host.split(':')[0];
  return 'your_vps_ip';
}

function buildLink(egress, vpsHost) {
  return `vless://${egress.uuid}@${vpsHost}:${egress.port}?encryption=none&type=tcp&security=reality&flow=xtls-rprx-vision&pbk=${egress.publicKey}&sid=${egress.shortId}&sni=${SERVER_NAME}&fp=chrome&spx=%2F&headerType=none#${egress.name}`;
}

function buildSubscriptionPayload(req) {
  const list = db.getEgresses();
  const vpsHost = getVpsHost(req);
  const links = list.map(e => buildLink(e, vpsHost));
  const subscription = Buffer.from(links.join('\n'), 'utf-8').toString('base64');
  return { list, vpsHost, links, subscription };
}

async function createOneEgress(name, region, clientUuid) {
  const node = await vpngateFetcher.getBestNode(region);
  const keys = realityKeygen.generateRealityKeys();
  const port = allocatePort();

  const egressDir = path.join(__dirname, '..', 'data', 'egress', name);
  if (!fs.existsSync(egressDir)) fs.mkdirSync(egressDir, { recursive: true });

  xrayConfigGen.generateConfig({
    uuid: clientUuid, privateKey: keys.privateKey, shortId: keys.shortId,
    destDomain: DEST_DOMAIN, serverName: SERVER_NAME
  }, path.join(egressDir, 'config.json'));

  fs.writeFileSync(path.join(egressDir, 'client.ovpn'), node.ovpnConfig, 'utf-8');

  const egress = {
    name, region: region.toUpperCase(), port, uuid: clientUuid,
    privateKey: keys.privateKey, publicKey: keys.publicKey, shortId: keys.shortId,
    nodeIp: node.ip, nodeHostname: node.hostname,
    status: 'starting', error: '', currentEgressIp: '', containerId: '',
    createdAt: Date.now(), updatedAt: Date.now(), lastCheckTime: Date.now(),
    latency: node.ping, failureCount: 0
  };

  db.addEgress(egress);

  dockerService.startEgressContainer(egress)
    .then(cid => {
      db.updateEgress(name, { containerId: cid, status: 'running', error: '', updatedAt: Date.now() });
      console.log(`[+] 出口 ${name} 启动完成`);
    })
    .catch(err => {
      db.updateEgress(name, { status: 'error', error: err.message, updatedAt: Date.now() });
      console.error(`[-] 出口 ${name} 启动失败:`, err.message);
    });

  return { name, region: region.toUpperCase(), port, uuid: clientUuid };
}

// POST /api/egress/create
router.post('/create', async (req, res) => {
  try {
    const { name, region, uuid } = req.body;
    if (!validateEgressName(name)) return res.status(400).json({ error: '名称无效' });
    if (!region || region.length !== 2) return res.status(400).json({ error: '地区代码无效' });
    if (uuid && !validateUuid(uuid)) return res.status(400).json({ error: 'UUID 格式无效' });

    const result = await createOneEgress(name, region.toUpperCase(), uuid || uuidv4());
    res.json({ success: true, message: `出口 ${name} 已创建，容器初始化中...`, data: result });
  } catch (error) {
    console.error('[-] 创建出口失败:', error);
    res.status(500).json({ error: error.message });
  }
});

// POST /api/egress/create-all
router.post('/create-all', async (req, res) => {
  try {
    const { regions, uuid } = req.body || {};
    const clientUuid = (uuid && validateUuid(uuid)) ? uuid : uuidv4();

    const nodes = await vpngateFetcher.fetchNodes();
    const existingRegions = new Set(db.getEgresses().map(e => e.region));

    // 每个地区取最高分节点
    const regionMap = {};
    for (const n of nodes) {
      if (!n.countryShort || n.countryShort.length !== 2) continue;
      const code = n.countryShort.toUpperCase();
      if (!regionMap[code] || n.score > regionMap[code].score) regionMap[code] = n;
    }

    let targetRegions = Object.keys(regionMap);
    if (regions && Array.isArray(regions) && regions.length > 0) {
      targetRegions = regions.map(r => r.toUpperCase()).filter(r => regionMap[r]);
    }

    const newRegions = targetRegions.filter(r => !existingRegions.has(r));
    if (newRegions.length === 0) {
      return res.json({ success: true, message: '所有地区已有出口', created: [], skipped: targetRegions });
    }

    const created = [];
    const errors = [];

    for (const region of newRegions) {
      try {
        const result = await createOneEgress(region.toLowerCase(), region, clientUuid);
        created.push(result);
      } catch (err) {
        errors.push({ region, error: err.message });
        console.error(`[-] [批量] ${region} 失败:`, err.message);
      }
    }

    res.json({
      success: true,
      message: `已提交 ${created.length} 个出口`,
      created, errors,
      skipped: targetRegions.filter(r => existingRegions.has(r))
    });
  } catch (error) {
    console.error('[-] 批量创建失败:', error);
    res.status(500).json({ error: error.message });
  }
});

// GET /api/egress/list
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

// POST /api/egress/delete
router.post('/delete', async (req, res) => {
  try {
    const { name } = req.body;
    if (!validateEgressName(name)) return res.status(400).json({ error: '名称无效' });
    const egress = db.getEgress(name);
    if (!egress) return res.status(404).json({ error: '未找到' });

    await dockerService.stopAndRemoveContainer(name);
    const egressBaseDir = path.resolve(__dirname, '..', 'data', 'egress');
    const egressDir = path.resolve(egressBaseDir, name);
    if (egressDir.startsWith(egressBaseDir + path.sep) && fs.existsSync(egressDir)) {
      fs.rmSync(egressDir, { recursive: true, force: true });
    }
    db.deleteEgress(name);
    res.json({ success: true, message: `出口 ${name} 已删除` });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// POST /api/egress/delete-all
router.post('/delete-all', async (req, res) => {
  try {
    const list = db.getEgresses();
    const deleted = [];
    for (const e of list) {
      try {
        await dockerService.stopAndRemoveContainer(e.name);
        const egressDir = path.resolve(__dirname, '..', 'data', 'egress', e.name);
        if (fs.existsSync(egressDir)) fs.rmSync(egressDir, { recursive: true, force: true });
        deleted.push(e.name);
      } catch (err) {
        console.error(`[-] 删除 ${e.name} 失败:`, err.message);
      }
    }
    for (const name of deleted) db.deleteEgress(name);
    res.json({ success: true, message: `已删除 ${deleted.length} 个出口`, deleted });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// POST /api/egress/rebuild
router.post('/rebuild', async (req, res) => {
  try {
    const { name } = req.body;
    if (!validateEgressName(name)) return res.status(400).json({ error: '名称无效' });
    const egress = db.getEgress(name);
    if (!egress) return res.status(404).json({ error: '未找到' });

    db.updateEgress(name, { status: 'rebuilding', error: '', updatedAt: Date.now() });
    const node = await vpngateFetcher.getBestNode(egress.region);
    const egressDir = path.join(__dirname, '..', 'data', 'egress', name);
    if (!fs.existsSync(egressDir)) fs.mkdirSync(egressDir, { recursive: true });
    fs.writeFileSync(path.join(egressDir, 'client.ovpn'), node.ovpnConfig, 'utf-8');
    db.updateEgress(name, { nodeIp: node.ip, nodeHostname: node.hostname, latency: node.ping, updatedAt: Date.now() });

    dockerService.startEgressContainer(db.getEgress(name))
      .then(cid => {
        db.updateEgress(name, { containerId: cid, status: 'running', error: '', failureCount: 0, updatedAt: Date.now() });
      })
      .catch(err => {
        db.updateEgress(name, { status: 'error', error: err.message, updatedAt: Date.now() });
      });

    res.json({ success: true, message: `出口 ${name} 漂移任务已下发` });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// POST /api/egress/rebuild-all
router.post('/rebuild-all', async (req, res) => {
  try {
    const list = db.getEgresses();
    const results = [];
    for (const e of list) {
      try {
        db.updateEgress(e.name, { status: 'rebuilding', error: '', updatedAt: Date.now() });
        const node = await vpngateFetcher.getBestNode(e.region);
        const egressDir = path.join(__dirname, '..', 'data', 'egress', e.name);
        if (!fs.existsSync(egressDir)) fs.mkdirSync(egressDir, { recursive: true });
        fs.writeFileSync(path.join(egressDir, 'client.ovpn'), node.ovpnConfig, 'utf-8');
        db.updateEgress(e.name, { nodeIp: node.ip, nodeHostname: node.hostname, latency: node.ping, updatedAt: Date.now() });
        dockerService.startEgressContainer(db.getEgress(e.name))
          .then(cid => db.updateEgress(e.name, { containerId: cid, status: 'running', error: '', failureCount: 0, updatedAt: Date.now() }))
          .catch(err => db.updateEgress(e.name, { status: 'error', error: err.message, updatedAt: Date.now() }));
        results.push({ name: e.name, region: e.region, status: 'drifting' });
      } catch (err) {
        results.push({ name: e.name, region: e.region, status: 'failed', error: err.message });
      }
    }
    res.json({ success: true, message: `已对 ${results.length} 个出口下发漂移`, results });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// GET /api/egress/subscription  —  路径必须在 /:name 之前
router.get('/subscription', (req, res) => {
  try {
    const { links, subscription } = buildSubscriptionPayload(req);
    res.json({ subscription, links, count: links.length });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// GET /api/egress/subscription.txt
router.get('/subscription.txt', (req, res) => {
  try {
    const { subscription } = buildSubscriptionPayload(req);
    res.type('text/plain; charset=utf-8').send(subscription);
  } catch (error) {
    res.status(500).type('text/plain; charset=utf-8').send(error.message);
  }
});

// GET /api/egress/:name/link
router.get('/:name/link', (req, res) => {
  try {
    if (!validateEgressName(req.params.name)) return res.status(400).json({ error: '名称无效' });
    const egress = db.getEgress(req.params.name);
    if (!egress) return res.status(404).json({ error: '未找到' });
    res.json({ link: buildLink(egress, getVpsHost(req)) });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// GET /api/egress/:name/logs
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
