// VPNGate 节点及地区查询路由接口
// 中文注释，用于前端做节点选择和地区概览

const express = require('express');
const vpngateFetcher = require('../services/vpngate-fetcher');

const router = express.Router();

/**
 * GET /api/vpngate/nodes
 * 获取 VPNGate 活跃节点列表（限 30 个），提供预览
 */
router.get('/nodes', async (req, res) => {
  try {
    const nodes = await vpngateFetcher.fetchNodes();
    // 过滤掉基本数据不全的，并截取前 30 个高分节点
    const previewNodes = nodes
      .filter(n => n.ip && n.countryShort)
      .slice(0, 30)
      .map(n => ({
        hostname: n.hostname,
        ip: n.ip,
        ping: n.ping,
        speed: n.speed,
        country: n.countryLong,
        region: n.countryShort,
        score: n.score,
        sessions: n.numSessions
      }));
    
    res.json(previewNodes);
  } catch (error) {
    console.error('[-] 获取 VPNGate 节点预览失败:', error);
    res.status(500).json({ error: error.message });
  }
});

/**
 * GET /api/vpngate/regions
 * 获取当前可选的国家/地区简称及其节点数，用于前端下拉选择框
 */
router.get('/regions', async (req, res) => {
  try {
    const nodes = await vpngateFetcher.fetchNodes();
    
    // 统计各国家的节点数
    const regionMap = {};
    for (const node of nodes) {
      const code = node.countryShort.toUpperCase();
      const name = node.countryLong;
      if (!regionMap[code]) {
        regionMap[code] = {
          code,
          name,
          count: 0
        };
      }
      regionMap[code].count++;
    }

    // 转换为数组并按节点数量降序排列
    const regions = Object.values(regionMap).sort((a, b) => b.count - a.count);
    
    res.json(regions);
  } catch (error) {
    console.error('[-] 获取 VPNGate 地区列表失败:', error);
    res.status(500).json({ error: error.message });
  }
});

module.exports = router;
