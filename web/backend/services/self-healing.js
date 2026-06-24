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
    // 简体中文注释：筛选可检测的活跃出口
    const targets = egresses.filter(e => e.status !== 'starting' && e.status !== 'rebuilding');
    if (targets.length === 0) return;

    // 简体中文注释：并发向 Docker 发起状态与IP测活探测
    const settled = await Promise.allSettled(
      targets.map(e => dockerService.getContainerStatusAndIp(e.name))
    );

    for (let i = 0; i < targets.length; i++) {
      const egress = targets[i];
      const name = egress.name;
      const result = settled[i];

      // 提取并发检测结果
      const statusReport = result.status === 'fulfilled'
        ? result.value
        : { status: 'error', ip: 'error', error: result.reason?.message || '容器检测异常' };

      try {
        if (statusReport.status !== 'running' || statusReport.ip === 'error') {
          const newCount = (egress.failureCount || 0) + 1;
          db.updateEgress(name, {
            failureCount: newCount,
            error: statusReport.error || '连通性断开',
            lastCheckTime: Date.now(),
            updatedAt: Date.now()
          });
          console.warn(`[!] 出口 ${name} 检测异常 (${newCount}/${MAX_FAILURES})`);

          if (newCount >= MAX_FAILURES) {
            console.error(`[!] 触发自愈：出口 ${name} 提交漂移任务...`);
            // 立即标记为 rebuild，防在 Job 执行前被下一次心跳重复扫到
            db.updateEgress(name, { status: 'rebuilding', failureCount: 0, error: '', updatedAt: Date.now() });

            try {
              // 简体中文注释：将耗时的重建操作提交至 SQLite 任务队列异步处理，释放心跳主流程
              db.createJob({
                type: 'rebuild',
                target: name,
                payload: { name }
              });
              console.log(`[+] [自愈已委派] 出口 ${name} 重置漂移任务已提交至队列`);
            } catch (driftErr) {
              db.updateEgress(name, { status: 'error', error: `自愈入队失败: ${driftErr.message}`, updatedAt: Date.now() });
              console.error(`[-] [自愈入队失败] ${name}:`, driftErr.message);
            }
          }
        } else {
          // 恢复正常，清零计数
          db.updateEgress(name, { failureCount: 0, error: '', lastCheckTime: Date.now(), updatedAt: Date.now() });
        }
      } catch (err) {
        console.error(`[-] 处理检测出口 ${name} 状态出错:`, err.message);
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
