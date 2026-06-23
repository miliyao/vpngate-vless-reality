// 出口管理路由接口
// 中文注释，确保高内聚和代码鲁棒性

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

// 默认混淆域名配置（可通过环境变量重写，默认 www.asus.com）
const SERVER_NAME = process.env.RE_DOMAINS || 'www.asus.com';
const DEST_DOMAIN = `${SERVER_NAME}:443`;

// 起步暴露端口
const START_PORT = 44301;
const END_PORT = 44400;
const EGRESS_NAME_RE = /^[a-zA-Z0-9_-]+$/;

function validateEgressName(name) {
  return typeof name === 'string' && EGRESS_NAME_RE.test(name);
}

/**
 * 助手函数：为新出口分配可用端口
 * 从 docker-compose 暴露的固定端口池中分配未使用端口
 */
function allocatePort() {
  const egresses = db.getEgresses();
  const usedPorts = new Set(egresses.map(e => Number(e.port)).filter(Boolean));
  for (let port = START_PORT; port <= END_PORT; port++) {
    if (!usedPorts.has(port)) {
      return port;
    }
  }
  throw new Error(`可用端口已耗尽，请扩展 ${START_PORT}-${END_PORT} 端口映射范围`);
}

/**
 * POST /api/egress/create
 * 新增并启动一个出口
 */
router.post('/create', async (req, res) => {
  try {
    const { name, region, uuid } = req.body;

    if (!validateEgressName(name)) {
      return res.status(400).json({ error: '出口名称无效，仅允许字母、数字、下划线和连字符' });
    }
    if (!region || region.length !== 2) {
      return res.status(400).json({ error: '国家/地区代码无效，必须为两位字母（如 JP, US）' });
    }
    if (uuid && !validateUuid(uuid)) {
      return res.status(400).json({ error: '自定义 UUID 格式无效' });
    }

    // 1. 获取 VPNGate 最优节点
    console.log(`[*] 正在为出口 ${name} 获取 ${region} 的最优 OpenVPN 节点...`);
    const node = await vpngateFetcher.getBestNode(region);

    // 2. 生成 Reality 密钥对
    const keys = realityKeygen.generateRealityKeys();

    // 3. 分配宿主机映射端口
    const port = allocatePort();

    // 4. 生成 Xray 配置
    const clientUuid = uuid || uuidv4();
    const egressDir = path.join(__dirname, '..', 'data', 'egress', name);
    const xrayConfigPath = path.join(egressDir, 'config.json');

    xrayConfigGen.generateConfig(
      {
        uuid: clientUuid,
        privateKey: keys.privateKey,
        shortId: keys.shortId,
        destDomain: DEST_DOMAIN,
        serverName: SERVER_NAME
      },
      xrayConfigPath
    );

    // 5. 保存 OpenVPN 配置文件
    if (!fs.existsSync(egressDir)) {
      fs.mkdirSync(egressDir, { recursive: true });
    }
    fs.writeFileSync(path.join(egressDir, 'client.ovpn'), node.ovpnConfig, 'utf-8');

    // 6. 保存元数据到数据库
    const newEgress = {
      name,
      region: region.toUpperCase(),
      port,
      uuid: clientUuid,
      privateKey: keys.privateKey,
      publicKey: keys.publicKey,
      shortId: keys.shortId,
      nodeIp: node.ip,
      nodeHostname: node.hostname,
      status: 'starting',
      error: '',
      currentEgressIp: '',
      containerId: '',
      createdAt: Date.now(),
      updatedAt: Date.now(),
      lastCheckTime: Date.now(),
      latency: node.ping,
      failureCount: 0
    };

    db.addEgress(newEgress);

    // 7. 异步在 Docker 中启动容器，避免接口挂起超时
    dockerService.startEgressContainer(newEgress)
      .then(containerId => {
        db.updateEgress(name, { containerId, status: 'running', error: '', updatedAt: Date.now() });
        console.log(`[+] 出口 ${name} 启动流程全部就绪`);
      })
      .catch(err => {
        db.updateEgress(name, { status: 'error', error: err.message, updatedAt: Date.now() });
        console.error(`[-] 出口 ${name} 容器启动失败:`, err.message);
      });

    res.json({
      success: true,
      message: `出口 ${name} 已创建，正在后台初始化容器...`,
      data: { name, port, uuid: clientUuid }
    });

  } catch (error) {
    console.error('[-] 创建出口失败:', error);
    res.status(500).json({ error: error.message });
  }
});

/**
 * GET /api/egress/list
 * 获取出口列表及实时健康状态
 */
router.get('/list', async (req, res) => {
  try {
    const list = db.getEgresses();
    const result = [];

    for (let egress of list) {
      // 实时获取 Docker 状态和当前出口 IP
      const dockerStatus = await dockerService.getContainerStatusAndIp(egress.name);
      
      // 更新数据库
      const shouldKeepTransientStatus = ['starting', 'rebuilding'].includes(egress.status)
        && dockerStatus.status === 'offline';
      const resolvedStatus = dockerStatus.ip === 'error' ? 'error' : dockerStatus.status;

      const updates = {
        status: shouldKeepTransientStatus ? egress.status : resolvedStatus,
        error: dockerStatus.error || '',
        lastCheckTime: Date.now()
      };
      if (dockerStatus.ip && dockerStatus.ip !== 'offline' && dockerStatus.ip !== 'error') {
        updates.currentEgressIp = dockerStatus.ip;
      }
      
      const updatedItem = db.updateEgress(egress.name, updates);
      result.push(updatedItem || egress);
    }

    res.json(result);
  } catch (error) {
    console.error('[-] 获取出口列表失败:', error);
    res.status(500).json({ error: error.message });
  }
});

/**
 * POST /api/egress/delete
 * 停止、删除容器并清理数据
 */
router.post('/delete', async (req, res) => {
  try {
    const { name } = req.body;
    if (!validateEgressName(name)) {
      return res.status(400).json({ error: '出口名称无效' });
    }

    const egress = db.getEgress(name);
    if (!egress) {
      return res.status(404).json({ error: '出口未找到' });
    }

    console.log(`[*] 正在删除出口 ${name}...`);

    // 1. 停止并删除 Docker 容器
    await dockerService.stopAndRemoveContainer(name);

    // 2. 清理磁盘物理文件
    const egressBaseDir = path.resolve(__dirname, '..', 'data', 'egress');
    const egressDir = path.resolve(egressBaseDir, name);
    if (!egressDir.startsWith(egressBaseDir + path.sep)) {
      return res.status(400).json({ error: '出口目录无效' });
    }
    if (fs.existsSync(egressDir)) {
      fs.rmSync(egressDir, { recursive: true, force: true });
    }

    // 3. 从数据库中删除记录
    db.deleteEgress(name);

    res.json({ success: true, message: `出口 ${name} 已成功清理` });
  } catch (error) {
    console.error('[-] 删除出口失败:', error);
    res.status(500).json({ error: error.message });
  }
});

/**
 * POST /api/egress/rebuild
 * 一键重建/漂移出口容器（保留原端口、UUID和密钥，只更新 VPNGate 节点）
 */
router.post('/rebuild', async (req, res) => {
  try {
    const { name } = req.body;
    if (!validateEgressName(name)) {
      return res.status(400).json({ error: '出口名称无效' });
    }
    const egress = db.getEgress(name);

    if (!egress) {
      return res.status(404).json({ error: '出口未找到' });
    }

    console.log(`[*] 正在为出口 ${name} 执行一键漂移自愈...`);
    db.updateEgress(name, { status: 'rebuilding', error: '', updatedAt: Date.now() });

    // 1. 重新拉取对应地区最优节点
    const node = await vpngateFetcher.getBestNode(egress.region);

    // 2. 覆盖写入新的 client.ovpn 配置文件
    const egressDir = path.join(__dirname, '..', 'data', 'egress', name);
    if (!fs.existsSync(egressDir)) {
      fs.mkdirSync(egressDir, { recursive: true });
    }
    fs.writeFileSync(path.join(egressDir, 'client.ovpn'), node.ovpnConfig, 'utf-8');

    // 3. 更新数据库信息
    db.updateEgress(name, {
      nodeIp: node.ip,
      nodeHostname: node.hostname,
      latency: node.ping,
      updatedAt: Date.now()
    });

    // 4. 重启 Docker 容器（传入最新的 egress 对象，而非漂移前的旧数据）
    const updatedEgress = db.getEgress(name);
    dockerService.startEgressContainer(updatedEgress)
      .then(containerId => {
        db.updateEgress(name, { containerId, status: 'running', error: '', failureCount: 0, updatedAt: Date.now() });
        console.log(`[+] 出口 ${name} 漂移自愈已完成`);
      })
      .catch(err => {
        db.updateEgress(name, { status: 'error', error: err.message, updatedAt: Date.now() });
        console.error(`[-] 出口 ${name} 漂移重建失败:`, err.message);
      });

    res.json({ success: true, message: `出口 ${name} 重建漂移任务已下发` });
  } catch (error) {
    console.error('[-] 重建出口失败:', error);
    res.status(500).json({ error: error.message });
  }
});

/**
 * GET /api/egress/:name/link
 * 获取特定出口的 VLESS 订阅连接
 */
router.get('/:name/link', (req, res) => {
  try {
    if (!validateEgressName(req.params.name)) {
      return res.status(400).json({ error: '出口名称无效' });
    }
    const egress = db.getEgress(req.params.name);
    if (!egress) {
      return res.status(404).json({ error: '出口未找到' });
    }

    // 获取客户端用于连接 VPS 的 IP 或域名
    // 默认拆分访问面板时使用的 Hostname，也可以通过环境变量覆盖
    let vpsHost = process.env.VPS_ADDRESS;
    if (!vpsHost && req.headers.host) {
      vpsHost = req.headers.host.split(':')[0];
    }
    vpsHost = vpsHost || 'your_vps_ip';

    // 拼接 VLESS Reality 标准连接协议
    const link = `vless://${egress.uuid}@${vpsHost}:${egress.port}?encryption=none&type=tcp&security=reality&flow=xtls-rprx-vision&pbk=${egress.publicKey}&sid=${egress.shortId}&sni=${SERVER_NAME}&fp=chrome&spx=%2F&headerType=none#${egress.name}`;

    res.json({ link });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

/**
 * GET /api/egress/:name/logs
 * 获取特定出口容器最近日志
 */
router.get('/:name/logs', async (req, res) => {
  try {
    if (!validateEgressName(req.params.name)) {
      return res.status(400).json({ error: '出口名称无效' });
    }

    const egress = db.getEgress(req.params.name);
    if (!egress) {
      return res.status(404).json({ error: '出口未找到' });
    }

    const tail = Math.min(Math.max(parseInt(req.query.tail, 10) || 120, 20), 500);
    const logs = await dockerService.getContainerLogs(req.params.name, tail);
    res.json({ logs });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

module.exports = router;
