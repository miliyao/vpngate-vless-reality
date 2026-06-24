// 抓取并解析 VPNGate 节点服务
// 使用 Node 18+ 内置 fetch，移除 axios 外部依赖
// 中文注释，保持代码的高鲁棒性和降级机制

const VPNGATE_API_URLS = [
  'https://www.vpngate.net/api/iphone/',
  'http://www2.vpngate.net/api/iphone/',
  'http://www.vpngate.net/api/iphone/'
];
const TIMEOUT_MS = 12000; // 单次抓取限制 12 秒超时，防止顺次尝试总耗时过长
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

  let lastError = null;
  let data = null;

  // 简体中文注释：顺次循环拉取配置的多源镜像源，直到成功或全部失败
  for (let idx = 0; idx < VPNGATE_API_URLS.length; idx++) {
    const url = VPNGATE_API_URLS[idx];
    try {
      console.log(`[*] 正在从 ${url} 获取 VPNGate 节点列表... (${idx + 1}/${VPNGATE_API_URLS.length})`);
      const response = await fetch(url, {
        signal: AbortSignal.timeout(TIMEOUT_MS),
        headers: {
          'User-Agent': 'Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X)'
        }
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status} ${response.statusText}`);
      }

      data = await response.text();
      if (data && data.includes('*vpn_servers')) {
        // 数据包完整，退出多源轮询
        break;
      } else {
        throw new Error('获取到的数据格式不正确');
      }
    } catch (err) {
      console.warn(`[!] 从源 ${url} 获取节点失败:`, err.message);
      lastError = err;
    }
  }

  // 简体中文注释：若所有源抓取全部报错，则启用本地缓存降级或抛出异常
  if (!data) {
    console.error('[-] 所有配置的 VPNGate 镜像源均已尝试拉取失败');
    if (nodeCache.length > 0) {
      console.log('[*] 启用降级方案：使用本地内存中的缓存节点数据（已过期但可用）');
      return nodeCache;
    }
    throw new Error(`所有 VPNGate 镜像源拉取失败，最后一次错误: ${lastError ? lastError.message : '未知'}`);
  }

  try {
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
    console.error('[-] 解析 VPNGate 缓存数据出错:', error.message);
    if (nodeCache.length > 0) {
      console.log('[*] 启用降级方案：使用本地内存中的缓存数据兜底');
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
  let filtered = nodes.filter(
    n => n.countryShort.toLowerCase() === region.toLowerCase() &&
         (!excludeIps || (excludeIps instanceof Set ? !excludeIps.has(n.ip) : !excludeIps.includes(n.ip)))
  );

  // 简体中文注释：退化降级处理。如果排除失效节点后无匹配节点，但该地区实际上是有节点的，则清空排除条件重新筛选，
  // 允许尝试之前连过的节点，防止仅有一两个节点或全部被排除导致自愈死锁、报错中断。
  if (filtered.length === 0 && excludeIps) {
    const hasExclude = excludeIps instanceof Set ? excludeIps.size > 0 : excludeIps.length > 0;
    if (hasExclude) {
      const fallbackNodes = nodes.filter(
        n => n.countryShort.toLowerCase() === region.toLowerCase()
      );
      if (fallbackNodes.length > 0) {
        console.warn(`[!] 地区 ${region} 过滤排除 IP 后无可用节点，降级为不进行 IP 排除过滤（可用节点数: ${fallbackNodes.length}）`);
        filtered = fallbackNodes;
      }
    }
  }

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
