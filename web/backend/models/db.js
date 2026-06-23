// 极简 JSON 文件数据库驱动，规避 sqlite3 二进制编译的平台依赖性问题
// 中文注释，保证高可维护性

const fs = require('fs');
const path = require('path');

const DB_FILE = path.join(__dirname, '..', 'data', 'db.json');
const DB_TMP_FILE = DB_FILE + '.tmp';

// 内存缓存层，避免每次健康检查都做完整的文件读写
let dbCache = null;

// 确保数据目录存在
function ensureDirExists() {
  const dir = path.dirname(DB_FILE);
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
}

// 初始化数据库
function initDb() {
  ensureDirExists();
  if (!fs.existsSync(DB_FILE)) {
    const defaultData = {
      egresses: [],
      settings: {
        lastFetchedTime: 0
      }
    };
    fs.writeFileSync(DB_FILE, JSON.stringify(defaultData, null, 2), 'utf-8');
  }
}

// 读取数据（优先从内存缓存中读取，减少磁盘 IO）
function readDb() {
  if (dbCache) return dbCache;
  initDb();
  try {
    const content = fs.readFileSync(DB_FILE, 'utf-8');
    dbCache = JSON.parse(content);
    return dbCache;
  } catch (error) {
    console.error('读取数据库失败，尝试重建:', error);
    dbCache = { egresses: [], settings: { lastFetchedTime: 0 } };
    return dbCache;
  }
}

// 原子写入数据（先写临时文件再 rename，防止写入中断导致 JSON 损坏）
function writeDb(data) {
  ensureDirExists();
  dbCache = data; // 同步更新内存缓存
  try {
    fs.writeFileSync(DB_TMP_FILE, JSON.stringify(data, null, 2), 'utf-8');
    fs.renameSync(DB_TMP_FILE, DB_FILE);
  } catch (err) {
    // rename 失败时回退到直接写入
    console.error('原子写入失败，回退到直接写入:', err.message);
    fs.writeFileSync(DB_FILE, JSON.stringify(data, null, 2), 'utf-8');
  }
}

module.exports = {
  // 获取所有出口
  getEgresses() {
    const db = readDb();
    return db.egresses;
  },

  // 获取单个出口
  getEgress(name) {
    const db = readDb();
    return db.egresses.find(e => e.name === name);
  },

  // 添加出口
  addEgress(egress) {
    const db = readDb();
    // 检查重名
    if (db.egresses.some(e => e.name === egress.name)) {
      throw new Error(`已存在名为 ${egress.name} 的出口`);
    }
    db.egresses.push(egress);
    writeDb(db);
  },

  // 更新出口状态或属性
  updateEgress(name, updates) {
    const db = readDb();
    const index = db.egresses.findIndex(e => e.name === name);
    if (index !== -1) {
      db.egresses[index] = { ...db.egresses[index], ...updates, updatedAt: updates.updatedAt || Date.now() };
      writeDb(db);
      return db.egresses[index];
    }
    return null;
  },

  // 删除出口
  deleteEgress(name) {
    const db = readDb();
    const filtered = db.egresses.filter(e => e.name !== name);
    db.egresses = filtered;
    writeDb(db);
  },

  // 统计出口数量，以便分配端口
  countEgress() {
    const db = readDb();
    return db.egresses.length;
  }
};
