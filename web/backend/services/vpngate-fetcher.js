// 抓取并解析 VPNGate 节点服务
// 使用 Node 18+ 内置 fetch，移除 axios 外部依赖
// 中文注释，保持代码的高鲁棒性和降级机制

const VPNGATE_API_URL = 'https://www.vpngate.net/api/iphone/';
const TIMEOUT_MS = 15000; // 15 秒超时
const CACHE_TTL_MS = 3600000; // 节点缓存有效期：1 小时

// 内存中的最后一次节点缓存及其时间戳，用于拉取失败时的降级和 TTL 控制
let nodeCache = [];
let nodeCacheTime = 0;

/**
 * 从 VPNGate 获取所有活跃节点并解析为 JSON
 * 若缓存未过期（TTL 1小时内）则直接返回缓存，避免频繁请求上游
 */
async function fetchNodes() {
  // 缓存命中检查：未过期直接返回
  if (nodeCache.length > 0 && Date.now() - nodeCacheTime < CACHE_TTL_MS) {
    console.log(`[*] VPNGate 节点缓存命中（剩余有效期 ${Math.round((CACHE_TTL_MS - (Date.now() - nodeCacheTime)) / 60000)} 分钟）`);
    return nodeCache;
  }

  try {
    console.log(`[*] 正在从 ${VPNGATE_API_URL} 获取 VPNGate 节点列表...`);

    // 使用 Node 18+ 内置 fetch，通过 AbortSignal.timeout 实现超时控制
    const response = await fetch(VPNGATE_API_URL, {
      signal: AbortSignal.timeout(TIMEOUT_MS),
      headers: {
        'User-Agent': 'Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X)'
      }
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status} ${response.statusText}`);
    }

    const data = await response.text();
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

      // 数据段结束标记
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
    // 更新缓存及时间戳
    nodeCache = nodes;
    nodeCacheTime = Date.now();
    return nodes;
  } catch (error) {
    console.error('[-] 拉取 VPNGate 接口失败:', error.message);
    if (nodeCache.length > 0) {
      console.log('[*] 启用降级方案：使用本地内存中的缓存节点数据（已过期但可用）');
      return nodeCache;
    }
    throw error;
  }
}

/**
 * 根据指定国家/地区，获取综合得分最高（或延迟最低）的最佳节点，支持剔除失效节点
 * @param {string} region - 国家简写，如 "JP", "US", "KR"
 * @param {Set|Array} excludeIps - 需要排除的 IP 列表
 */
async function getBestNode(region, excludeIps) {
  const nodes = await fetchNodes();
  
  // 筛选对应国家的节点，并自动排除不可用的失效 IP
  const filtered = nodes.filter(
    n => n.countryShort.toLowerCase() === region.toLowerCase() &&
         (!excludeIps || (excludeIps instanceof Set ? !excludeIps.has(n.ip) : !excludeIps.includes(n.ip)))
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
