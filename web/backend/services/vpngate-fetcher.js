// 抓取并解析 VPNGate 节点服务
// 中文注释，保持代码的高鲁棒性和降级机制

const axios = require('axios');

const VPNGATE_API_URL = 'https://www.vpngate.net/api/iphone/';
const TIMEOUT = 15000; // 15秒超时

// 内存中的最后一次节点缓存，用于拉取失败时的降级方案
let nodeCache = [];

/**
 * 从 VPNGate 获取所有活跃节点并解析为 JSON
 */
async function fetchNodes() {
  try {
    console.log(`[*] 正在从 ${VPNGATE_API_URL} 获取 VPNGate 节点列表...`);
    const response = await axios.get(VPNGATE_API_URL, {
      timeout: TIMEOUT,
      headers: {
        'User-Agent': 'Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X)'
      }
    });

    const data = response.data;
    if (!data || !data.includes('*vpn_servers')) {
      throw new Error('获取到的数据格式不正确');
    }

    const lines = data.split('\n');
    const nodes = [];

    // 跳过前面的说明信息，定位到标题行和数据行
    let isDataSection = false;
    for (let line of lines) {
      line = line.trim();
      if (!line) continue;

      if (line.startsWith('*vpn_servers')) {
        isDataSection = true;
        continue;
      }

      // 如果数据段已经结束
      if (line.startsWith('*') && isDataSection) {
        break;
      }

      if (isDataSection) {
        // 标题行以 '#' 开头，跳过
        if (line.startsWith('#')) {
          continue;
        }

        const cols = line.split(',');
        if (cols.length < 15) continue;

        try {
          const node = {
            hostname: cols[0],
            ip: cols[1],
            score: parseInt(cols[2], 10) || 0,
            ping: parseInt(cols[3], 10) || 999,
            speed: parseInt(cols[4], 10) || 0,
            countryLong: cols[5],
            countryShort: cols[6],
            numSessions: parseInt(cols[7], 10) || 0,
            uptime: parseInt(cols[8], 10) || 0,
            totalUsers: parseInt(cols[9], 10) || 0,
            totalTraffic: cols[10],
            logType: cols[11],
            operator: cols[12],
            message: cols[13],
            ovpnB64: cols[14] // Base64 编码的 .ovpn 配置
          };
          nodes.push(node);
        } catch (err) {
          // 容错处理，忽略单行解析错误
        }
      }
    }

    console.log(`[+] 成功拉取并解析了 ${nodes.length} 个 VPNGate 节点`);
    nodeCache = nodes; // 更新缓存
    return nodes;
  } catch (error) {
    console.error('[-] 拉取 VPNGate 接口失败:', error.message);
    if (nodeCache.length > 0) {
      console.log('[*] 启用降级方案：使用本地内存中的缓存节点数据');
      return nodeCache;
    }
    throw error;
  }
}

/**
 * 根据指定国家/地区，获取综合得分最高（或延迟最低）的最佳节点
 * @param {string} region - 国家简写，如 "JP", "US", "KR"
 */
async function getBestNode(region) {
  const nodes = await fetchNodes();
  
  // 筛选对应国家的节点
  const filtered = nodes.filter(
    n => n.countryShort.toLowerCase() === region.toLowerCase()
  );

  if (filtered.length === 0) {
    throw new Error(`未在 VPNGate 中找到国家/地区为 ${region} 的可用节点`);
  }

  // 排序算法：综合 Score（降序）与 Ping（升序）
  // 优先按 Score 得分高排序，如果得分一致，则按 Ping 延迟低排序
  filtered.sort((a, b) => {
    if (b.score !== a.score) {
      return b.score - a.score;
    }
    return a.ping - b.ping;
  });

  const bestNode = filtered[0];
  
  // 解码 OpenVPN 配置
  try {
    const ovpnConfig = Buffer.from(bestNode.ovpnB64, 'base64').toString('utf-8');
    
    // 给 OpenVPN 配置文件注入稳定性优化参数
    let optimizedConfig = ovpnConfig;
    if (!optimizedConfig.includes('keepalive')) {
      optimizedConfig += '\nkeepalive 10 60\n';
    }
    // 强制断线重连，防止由于 DNS 解析失效导致永久掉线
    if (!optimizedConfig.includes('resolv-retry')) {
      optimizedConfig += '\nresolv-retry infinite\n';
    }
    // 避免证书报错卡住
    if (!optimizedConfig.includes('auth-nocache')) {
      optimizedConfig += '\nauth-nocache\n';
    }

    return {
      hostname: bestNode.hostname,
      ip: bestNode.ip,
      country: bestNode.countryShort,
      ping: bestNode.ping,
      speed: bestNode.speed,
      ovpnConfig: optimizedConfig
    };
  } catch (err) {
    console.error('[-] 解码 OpenVPN 配置数据失败:', err);
    throw new Error('解析最优节点的 OpenVPN 配置出错');
  }
}

module.exports = {
  fetchNodes,
  getBestNode
};
