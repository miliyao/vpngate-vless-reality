// 出口管理路由接口
// 中文注释，确保高内聚和代码鲁棒性

const express = require('express');
const fs = require('fs');
const path = require('path');
const { v4: uuidv4 } = require('uuid');

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

/**
 * 助手函数：为新出口分配可用端口
 * 为避免端口复用冲突，始终使用当前最大端口 + 1 的策略
 * 即使中间端口被删除也不会回收复用，确保永不冲突
 */
function allocatePort() {
  const egresses = db.getEgresses();
  if (egresses.length === 0) {
    return START_PORT;
  }
  // 取当前已分配最大端口号 + 1，而非简单计数，避免删除中间出口后端口复用冲突
  const maxPort = Math.max(...egresses.map(e => e.port));
  return maxPort + 1;
}

/**
 * POST /api/egress/create
 * 新增并启动一个出口
 */
router.post('/create', async (req, res) => {
  try {
    const { name, region, uuid } = req.body;

    if (!name || !/^[a-zA-Z0-9_-]+$/.test(name)) {
      return res.status(400).json({ error: '出口名称无效，仅允许字母、数字、下划线和连字符' });
    }
    if (!region || region.length !== 2) {
      return res.status(400).json({ error: '国家/地区代码无效，必须为两位字母（如 JP, US）' });
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
      lastCheckTime: Date.now(),
      latency: node.ping
    };

    db.addEgress(newEgress);

    // 7. 异步在 Docker 中启动容器，避免接口挂起超时
    dockerService.startEgressContainer(newEgress)
      .then(containerId => {
        db.updateEgress(name, { containerId, status: 'running' });
        console.log(`[+] 出口 ${name} 启动流程全部就绪`);
      })
      .catch(err => {
        db.updateEgress(name, { status: 'error', error: err.message });
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
      const updates = {
        status: dockerStatus.status,
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
    if (!name) {
      return res.status(400).json({ error: '缺少出口名称' });
    }

    console.log(`[*] 正在删除出口 ${name}...`);

    // 1. 停止并删除 Docker 容器
    await dockerService.stopAndRemoveContainer(name);

    // 2. 清理磁盘物理文件
    const egressDir = path.join(__dirname, '..', 'data', 'egress', name);
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
    const egress = db.getEgress(name);

    if (!egress) {
      return res.status(404).json({ error: '出口未找到' });
    }

    console.log(`[*] 正在为出口 ${name} 执行一键漂移自愈...`);
    db.updateEgress(name, { status: 'starting' });

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
      latency: node.ping
    });

    // 4. 重启 Docker 容器（传入最新的 egress 对象，而非漂移前的旧数据）
    const updatedEgress = db.getEgress(name);
    dockerService.startEgressContainer(updatedEgress)
      .then(containerId => {
        db.updateEgress(name, { containerId, status: 'running' });
        console.log(`[+] 出口 ${name} 漂移自愈已完成`);
      })
      .catch(err => {
        db.updateEgress(name, { status: 'error', error: err.message });
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
    const link = `vless://${egress.uuid}@${vpsHost}:${egress.port}?type=tcp&security=reality&flow=xtls-rprx-vision&pbk=${egress.publicKey}&sid=${egress.shortId}&sni=${SERVER_NAME}#${egress.name}`;

    res.json({ link });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

module.exports = router;
