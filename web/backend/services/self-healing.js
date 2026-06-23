const fs = require('fs');
const path = require('path');
const db = require('../models/db');
const dockerService = require('./docker');
const vpngateFetcher = require('./vpngate-fetcher');

const HEALTH_CHECK_INTERVAL = Number(process.env.HEALTH_CHECK_INTERVAL || 60000);
const MAX_FAILURES = Number(process.env.MAX_FAILURES || 2);
const failureTracker = {};
let timer = null;

function start() {
  if (timer) return timer;
  console.log('[*] 自愈 worker 已启动...');

  timer = setInterval(async () => {
    const egresses = db.getEgresses();

    for (const egress of egresses) {
      const name = egress.name;
      if (egress.status === 'starting' || egress.status === 'rebuilding') continue;

      try {
        const statusReport = await dockerService.getContainerStatusAndIp(name);
        if (statusReport.status !== 'running' || statusReport.ip === 'error') {
          failureTracker[name] = (failureTracker[name] || 0) + 1;
          db.updateEgress(name, {
            failureCount: failureTracker[name],
            error: statusReport.error || '连通性断开',
            lastCheckTime: Date.now(),
            updatedAt: Date.now()
          });
          console.warn(`[!] 出口 ${name} 检测异常 (${failureTracker[name]}/${MAX_FAILURES})`);

          if (failureTracker[name] >= MAX_FAILURES) {
            console.error(`[!] 触发自愈：出口 ${name} 开始漂移...`);
            failureTracker[name] = 0;
            db.updateEgress(name, { status: 'rebuilding', failureCount: 0, error: '', updatedAt: Date.now() });

            try {
              const bestNode = await vpngateFetcher.getBestNode(egress.region);
              const egressDir = path.join(__dirname, '..', 'data', 'egress', name);
              if (!fs.existsSync(egressDir)) fs.mkdirSync(egressDir, { recursive: true });
              fs.writeFileSync(path.join(egressDir, 'client.ovpn'), bestNode.ovpnConfig, 'utf-8');
              db.updateEgress(name, {
                nodeIp: bestNode.ip,
                nodeHostname: bestNode.hostname,
                latency: bestNode.ping,
                updatedAt: Date.now()
              });

              const updatedEgress = db.getEgress(name) || egress;
              const containerId = await dockerService.startEgressContainer(updatedEgress);
              db.updateEgress(name, { containerId, status: 'running', error: '', failureCount: 0, updatedAt: Date.now() });
              console.log(`[+] [自愈成功] ${name} -> ${bestNode.ip}`);
            } catch (driftErr) {
              db.updateEgress(name, { status: 'error', error: `自愈失败: ${driftErr.message}`, updatedAt: Date.now() });
              console.error(`[-] [自愈失败] ${name}:`, driftErr.message);
            }
          }
        } else {
          failureTracker[name] = 0;
          db.updateEgress(name, { failureCount: 0, error: '', lastCheckTime: Date.now(), updatedAt: Date.now() });
        }
      } catch (err) {
        console.error(`[-] 定时检测出口 ${name} 出错:`, err.message);
      }
    }
  }, HEALTH_CHECK_INTERVAL);

  return timer;
}

function stop() {
  if (!timer) return;
  clearInterval(timer);
  timer = null;
}

module.exports = { start, stop };
