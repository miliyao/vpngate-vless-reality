const db = require('../models/db');
const dockerService = require('./docker');
const egressOps = require('./egress-ops');

const HEALTH_CHECK_INTERVAL = Number(process.env.HEALTH_CHECK_INTERVAL || 60000);
const MAX_FAILURES = Number(process.env.MAX_FAILURES || 2);
// 注：不再使用进程内 failureTracker，改为直接读写 DB 的 failureCount 字段，
// 确保 worker 重启后失败计数仍能持续累积，不会被意外清零。
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
          // 直接从 DB 读取当前失败计数并累加，跨重启持久化
          const newCount = (egress.failureCount || 0) + 1;
          db.updateEgress(name, {
            failureCount: newCount,
            error: statusReport.error || '连通性断开',
            lastCheckTime: Date.now(),
            updatedAt: Date.now()
          });
          console.warn(`[!] 出口 ${name} 检测异常 (${newCount}/${MAX_FAILURES})`);

          if (newCount >= MAX_FAILURES) {
            console.error(`[!] 触发自愈：出口 ${name} 开始漂移...`);
            // 重置 DB 计数，标记重建中
            db.updateEgress(name, { status: 'rebuilding', failureCount: 0, error: '', updatedAt: Date.now() });

            try {
              await egressOps.rebuildEgress(name);
              console.log(`[+] [自愈成功] ${name} 完成漂移`);
            } catch (driftErr) {
              db.updateEgress(name, { status: 'error', error: `自愈失败: ${driftErr.message}`, updatedAt: Date.now() });
              console.error(`[-] [自愈失败] ${name}:`, driftErr.message);
            }
          }
        } else {
          // 恢复正常，清零 DB 中的失败计数
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
