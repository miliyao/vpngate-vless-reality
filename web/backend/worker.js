const selfHealing = require('./services/self-healing');
const db = require('./models/db');
const egressOps = require('./services/egress-ops');

console.log('[*] worker process starting');
selfHealing.start();

async function processJobs() {
  const jobs = db.listJobs(20).filter(job => job.status === 'queued');
  for (const job of jobs) {
    db.updateJob(job.id, { status: 'running', startedAt: Date.now(), error: '' });
    try {
      let result = {};
      if (job.type === 'create') {
        result = await egressOps.createEgress(job.payload);
      } else if (job.type === 'create-all') {
        result = await egressOps.createAll(job.payload.regions, job.payload.uuid);
      } else if (job.type === 'delete') {
        result = await egressOps.deleteEgress(job.payload.name);
      } else if (job.type === 'delete-all') {
        result = { deleted: await egressOps.deleteAll() };
      } else if (job.type === 'rebuild') {
        result = await egressOps.rebuildEgress(job.payload.name);
      } else if (job.type === 'rebuild-all') {
        result = { results: await egressOps.rebuildAll() };
      } else {
        throw new Error(`未知任务类型: ${job.type}`);
      }
      db.updateJob(job.id, { status: 'done', result, finishedAt: Date.now(), error: '' });
    } catch (err) {
      db.updateJob(job.id, { status: 'failed', error: err.message, finishedAt: Date.now(), result: {} });
    }
  }
}

// 使用递归 setTimeout 代替 setInterval，确保上一轮任务处理完毕后
// 再安排下一次轮询，彻底消除因任务耗时超过轮询间隔导致的双重执行竞态。
async function runLoop() {
  await processJobs().catch(err => console.error('[-] Job 处理出错:', err.message));
  setTimeout(runLoop, Number(process.env.JOB_POLL_INTERVAL || 2000));
}
runLoop();

process.on('SIGINT', () => process.exit(0));
process.on('SIGTERM', () => process.exit(0));
