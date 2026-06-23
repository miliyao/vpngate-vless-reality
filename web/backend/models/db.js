// SQLite 数据库驱动，替代原 JSON 文件存储

const fs = require('fs');
const path = require('path');
const Database = require('better-sqlite3');

const DATA_DIR = path.join(__dirname, '..', 'data');
const DB_FILE = path.join(DATA_DIR, 'app.sqlite3');

function ensureDirExists() {
  if (!fs.existsSync(DATA_DIR)) {
    fs.mkdirSync(DATA_DIR, { recursive: true });
  }
}

ensureDirExists();

const db = new Database(DB_FILE);
db.pragma('journal_mode = WAL');
db.pragma('foreign_keys = ON');

db.exec(`
CREATE TABLE IF NOT EXISTS egresses (
  name TEXT PRIMARY KEY,
  region TEXT NOT NULL,
  port INTEGER NOT NULL,
  uuid TEXT NOT NULL,
  privateKey TEXT NOT NULL,
  publicKey TEXT NOT NULL,
  shortId TEXT NOT NULL,
  nodeIp TEXT NOT NULL,
  nodeHostname TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  currentEgressIp TEXT NOT NULL DEFAULT '',
  containerId TEXT NOT NULL DEFAULT '',
  createdAt INTEGER NOT NULL,
  updatedAt INTEGER NOT NULL,
  lastCheckTime INTEGER NOT NULL,
  latency INTEGER NOT NULL DEFAULT 0,
  failureCount INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  target TEXT NOT NULL DEFAULT '',
  payload TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'queued',
  result TEXT NOT NULL DEFAULT '{}',
  error TEXT NOT NULL DEFAULT '',
  createdAt INTEGER NOT NULL,
  updatedAt INTEGER NOT NULL,
  startedAt INTEGER,
  finishedAt INTEGER
);
`);

const legacyJsonFile = path.join(DATA_DIR, 'db.json');
if (stmtCountEgresses() === 0 && fs.existsSync(legacyJsonFile)) {
  try {
    const legacy = JSON.parse(fs.readFileSync(legacyJsonFile, 'utf-8'));
    if (Array.isArray(legacy.egresses)) {
      const insertLegacy = db.transaction((egresses) => {
        for (const e of egresses) {
          if (!e || !e.name) continue;
          db.prepare(`INSERT OR IGNORE INTO egresses (
            name, region, port, uuid, privateKey, publicKey, shortId,
            nodeIp, nodeHostname, status, error, currentEgressIp, containerId,
            createdAt, updatedAt, lastCheckTime, latency, failureCount
          ) VALUES (
            @name, @region, @port, @uuid, @privateKey, @publicKey, @shortId,
            @nodeIp, @nodeHostname, @status, @error, @currentEgressIp, @containerId,
            @createdAt, @updatedAt, @lastCheckTime, @latency, @failureCount
          )`).run({
            name: e.name,
            region: e.region || '',
            port: Number(e.port || 0),
            uuid: e.uuid || '',
            privateKey: e.privateKey || '',
            publicKey: e.publicKey || '',
            shortId: e.shortId || '',
            nodeIp: e.nodeIp || '',
            nodeHostname: e.nodeHostname || '',
            status: e.status || 'offline',
            error: e.error || '',
            currentEgressIp: e.currentEgressIp || '',
            containerId: e.containerId || '',
            createdAt: Number(e.createdAt || Date.now()),
            updatedAt: Number(e.updatedAt || Date.now()),
            lastCheckTime: Number(e.lastCheckTime || Date.now()),
            latency: Number(e.latency || 0),
            failureCount: Number(e.failureCount || 0)
          });
        }
      });
      insertLegacy(legacy.egresses);
      console.log(`[+] 已从旧 JSON 数据库迁移 ${legacy.egresses.length} 条出口记录`);
    }
  } catch (err) {
    console.warn('[!] 旧 JSON 数据库迁移失败:', err.message);
  }
}

function stmtCountEgresses() {
  return db.prepare('SELECT COUNT(1) AS c FROM egresses').get().c;
}

function now() {
  return Date.now();
}

const updateEgressStmtCache = {};

function getUpdateEgressStmt(fields) {
  const cacheKey = fields.sort().join(',');
  if (!updateEgressStmtCache[cacheKey]) {
    const setClause = fields.map(f => `${f}=@${f}`).join(', ');
    updateEgressStmtCache[cacheKey] = db.prepare(`UPDATE egresses SET ${setClause}, updatedAt=@updatedAt WHERE name=@name`);
  }
  return updateEgressStmtCache[cacheKey];
}

const stmt = {
  getAllEgresses: db.prepare('SELECT * FROM egresses ORDER BY createdAt ASC'),
  getEgress: db.prepare('SELECT * FROM egresses WHERE name = ? LIMIT 1'),
  insertEgress: db.prepare(`INSERT INTO egresses (
    name, region, port, uuid, privateKey, publicKey, shortId,
    nodeIp, nodeHostname, status, error, currentEgressIp, containerId,
    createdAt, updatedAt, lastCheckTime, latency, failureCount
  ) VALUES (
    @name, @region, @port, @uuid, @privateKey, @publicKey, @shortId,
    @nodeIp, @nodeHostname, @status, @error, @currentEgressIp, @containerId,
    @createdAt, @updatedAt, @lastCheckTime, @latency, @failureCount
  )`),
  deleteEgress: db.prepare('DELETE FROM egresses WHERE name = ?'),
  countEgress: db.prepare('SELECT COUNT(1) AS c FROM egresses'),
  insertJob: db.prepare(`INSERT INTO jobs (type, target, payload, status, result, error, createdAt, updatedAt, startedAt, finishedAt)
    VALUES (@type, @target, @payload, @status, @result, @error, @createdAt, @updatedAt, @startedAt, @finishedAt)`),
  updateJob: db.prepare(`UPDATE jobs SET status=@status, result=@result, error=@error, updatedAt=@updatedAt, startedAt=@startedAt, finishedAt=@finishedAt WHERE id=@id`),
  getJob: db.prepare('SELECT * FROM jobs WHERE id = ? LIMIT 1'),
  listJobs: db.prepare('SELECT * FROM jobs ORDER BY id DESC LIMIT ?')
  ,
  jobStatusCounts: db.prepare('SELECT status, COUNT(1) AS count FROM jobs GROUP BY status'),
  latestJob: db.prepare('SELECT * FROM jobs ORDER BY id DESC LIMIT 1')
};

function normalizeEgress(egress) {
  return {
    ...egress,
    port: Number(egress.port),
    createdAt: Number(egress.createdAt),
    updatedAt: Number(egress.updatedAt),
    lastCheckTime: Number(egress.lastCheckTime),
    latency: Number(egress.latency || 0),
    failureCount: Number(egress.failureCount || 0)
  };
}

module.exports = {
  getEgresses() {
    return stmt.getAllEgresses.all().map(normalizeEgress);
  },

  getEgress(name) {
    const row = stmt.getEgress.get(name);
    return row ? normalizeEgress(row) : null;
  },

  addEgress(egress) {
    stmt.insertEgress.run(egress);
  },

  updateEgress(name, updates) {
    const fields = Object.keys(updates).filter(k => k !== 'name' && k !== 'updatedAt');
    if (fields.length === 0) return this.getEgress(name);

    const stmtParams = { ...updates, name, updatedAt: updates.updatedAt || now() };
    const stmtToUse = getUpdateEgressStmt(fields);
    const info = stmtToUse.run(stmtParams);
    if (info.changes === 0) return null;
    return this.getEgress(name);
  },

  deleteEgress(name) {
    stmt.deleteEgress.run(name);
  },

  countEgress() {
    return stmt.countEgress.get().c;
  },

  jobStatusCounts() {
    return stmt.jobStatusCounts.all().reduce((acc, row) => {
      acc[row.status] = Number(row.count);
      return acc;
    }, {});
  },

  latestJob() {
    const job = stmt.latestJob.get();
    if (!job) return null;
    return {
      ...job,
      payload: safeJsonParse(job.payload),
      result: safeJsonParse(job.result)
    };
  },

  createJob({ type, target = '', payload = {} }) {
    const ts = now();
    const result = stmt.insertJob.run({
      type,
      target,
      payload: JSON.stringify(payload),
      status: 'queued',
      result: '{}',
      error: '',
      createdAt: ts,
      updatedAt: ts,
      startedAt: null,
      finishedAt: null
    });
    return this.getJob(result.lastInsertRowid);
  },

  listJobs(limit = 100) {
    return stmt.listJobs.all(limit).map(job => ({
      ...job,
      payload: safeJsonParse(job.payload),
      result: safeJsonParse(job.result)
    }));
  },

  getJob(id) {
    const job = stmt.getJob.get(id);
    if (!job) return null;
    return {
      ...job,
      payload: safeJsonParse(job.payload),
      result: safeJsonParse(job.result)
    };
  },

  updateJob(id, updates = {}) {
    const current = stmt.getJob.get(id);
    if (!current) return null;
    const merged = {
      ...current,
      ...updates,
      id,
      updatedAt: updates.updatedAt || now()
    };
    stmt.updateJob.run({
      id,
      status: merged.status,
      result: JSON.stringify(merged.result || {}),
      error: merged.error || '',
      updatedAt: merged.updatedAt,
      startedAt: merged.startedAt || current.startedAt || null,
      finishedAt: merged.finishedAt || current.finishedAt || null
    });
    return this.getJob(id);
  }
};

function safeJsonParse(v) {
  try {
    return JSON.parse(v || '{}');
  } catch {
    return {};
  }
}
