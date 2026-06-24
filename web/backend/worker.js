const selfHealing = require('./services/self-healing');
const db = require('./models/db');
const egressOps = require('./services/egress-ops');

console.log('[*] worker process starting');
selfHealing.start();

// 简体中文注释：限制异步任务并发执行的辅助函数
async function limitConcurrency(tasks, limit) {
  const results = [];
  const executing = [];
  for (const task of tasks) {
    const p = Promise.resolve().then(() => task());
    results.push(p);
    if (limit <= tasks.length) {
      const e = p.then(() => executing.splice(executing.indexOf(e), 1));
      executing.push(e);
      if (executing.length >= limit) {
        await Promise.race(executing);
      }
    }
  }
  return Promise.all(results);
}

async function processJobs() {
  const jobs = db.listJobs(20).filter(job => job.status === 'queued');
  if (jobs.length === 0) return;

  const toRun = [];
  const activeTargets = new Set();

  // 简体中文注释：同一轮轮询中，对具有相同 target（同一出口）的多个任务进行互斥过滤，防止并发引起的 Docker 容器竞态
  for (const job of jobs) {
    if (job.target && activeTargets.has(job.target)) {
      // 这一轮先不执行，延迟到下一轮轮询处理
      continue;
    }
    if (job.target) {
      activeTargets.add(job.target);
    }
    toRun.push(job);
  }

  if (toRun.length === 0) return;

  const tasks = toRun.map(job => async () => {
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
  });

  // 限制最大并发数为 2，防止高并发导致 VPS 主机性能超载与网络拥堵
  await limitConcurrency(tasks, 2);
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
