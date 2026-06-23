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

setInterval(processJobs, Number(process.env.JOB_POLL_INTERVAL || 2000));
processJobs().catch(err => console.error('job processing error:', err.message));

process.on('SIGINT', () => process.exit(0));
process.on('SIGTERM', () => process.exit(0));
