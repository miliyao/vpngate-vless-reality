const fs = require('fs');
const path = require('path');
const { v4: uuidv4 } = require('uuid');

const db = require('../models/db');
const dockerService = require('./docker');
const vpngateFetcher = require('./vpngate-fetcher');
const xrayConfigGen = require('./xray-config-gen');
const realityKeygen = require('./reality-keygen');

const SERVER_NAME = process.env.RE_DOMAINS || 'www.amd.com';
const DEST_DOMAIN = `${SERVER_NAME}:443`;

const START_PORT = 44301;
const END_PORT = 44400;

function allocatePort() {
  const usedPorts = new Set(db.getEgresses().map(e => Number(e.port)).filter(Boolean));
  for (let port = START_PORT; port <= END_PORT; port++) {
    if (!usedPorts.has(port)) return port;
  }
  throw new Error(`端口已耗尽 (${START_PORT}-${END_PORT})`);
}

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

function egressDir(name) {
  return path.resolve(__dirname, '..', 'data', 'egress', name);
}

function ensureDir(name) {
  const dir = egressDir(name);
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
  return dir;
}

async function createEgress({ name, region, uuid }) {
  const node = await vpngateFetcher.getBestNode(region);
  const keys = realityKeygen.generateRealityKeys();
  const port = allocatePort();
  const clientUuid = uuid || uuidv4();
  const dir = ensureDir(name);

  xrayConfigGen.generateConfig({
    uuid: clientUuid,
    privateKey: keys.privateKey,
    shortId: keys.shortId,
    destDomain: DEST_DOMAIN,
    serverName: SERVER_NAME
  }, path.join(dir, 'config.json'));

  fs.writeFileSync(path.join(dir, 'client.ovpn'), node.ovpnConfig, 'utf-8');

  const egress = {
    name,
    region: region.toUpperCase(),
    port,
    uuid: clientUuid,
    privateKey: keys.privateKey,
    publicKey: keys.publicKey,
    shortId: keys.shortId,
    nodeIp: node.ip,
    nodeHostname: node.hostname,
    status: 'starting',
    error: '',
    currentEgressIp: '',
    containerId: '',
    createdAt: Date.now(),
    updatedAt: Date.now(),
    lastCheckTime: Date.now(),
    latency: node.ping,
    failureCount: 0
  };

  db.addEgress(egress);
  const containerId = await dockerService.startEgressContainer(egress);
  db.updateEgress(name, { containerId, status: 'running', error: '', updatedAt: Date.now() });
  return { name, region: region.toUpperCase(), port, uuid: clientUuid };
}

async function rebuildEgress(name) {
  const egress = db.getEgress(name);
  if (!egress) throw new Error('出口未找到');
  db.updateEgress(name, { status: 'rebuilding', error: '', updatedAt: Date.now() });
  
  // 简体中文注释：提取刚才失效连接的 IP 强行予以剔除，确保换成新节点重试
  const lastFailedIp = egress.nodeIp;
  const excludeSet = new Set();
  if (lastFailedIp) excludeSet.add(lastFailedIp);

  const node = await vpngateFetcher.getBestNode(egress.region, excludeSet);
  const dir = ensureDir(name);
  fs.writeFileSync(path.join(dir, 'client.ovpn'), node.ovpnConfig, 'utf-8');
  db.updateEgress(name, { nodeIp: node.ip, nodeHostname: node.hostname, latency: node.ping, updatedAt: Date.now() });
  const containerId = await dockerService.startEgressContainer(db.getEgress(name));
  db.updateEgress(name, { containerId, status: 'running', error: '', failureCount: 0, updatedAt: Date.now() });
  return { name, region: egress.region, port: egress.port };
}

async function deleteEgress(name) {
  await dockerService.stopAndRemoveContainer(name);
  const dir = egressDir(name);
  const targetParent = path.resolve(__dirname, '..', 'data', 'egress');
  if (dir.startsWith(targetParent) && dir !== targetParent && fs.existsSync(dir)) {
    fs.rmSync(dir, { recursive: true, force: true });
  }
  db.deleteEgress(name);
  return { name };
}

async function createAll(regions, uuid) {
  const clientUuid = uuid || uuidv4();
  const nodes = await vpngateFetcher.fetchNodes();
  const existingRegions = new Set(db.getEgresses().map(e => e.region));
  const regionMap = {};
  for (const n of nodes) {
    if (!n.countryShort || n.countryShort.length !== 2) continue;
    const code = n.countryShort.toUpperCase();
    if (!regionMap[code] || n.score > regionMap[code].score) regionMap[code] = n;
  }
  let targetRegions = Object.keys(regionMap);
  if (regions && Array.isArray(regions) && regions.length > 0) {
    targetRegions = regions.map(r => r.toUpperCase()).filter(r => regionMap[r]);
  }
  const newRegions = targetRegions.filter(r => !existingRegions.has(r));
  const created = [];
  const errors = [];
  
  const tasks = newRegions.map(region => async () => {
    try {
      const name = region.toLowerCase();
      const res = await createEgress({ name, region, uuid: clientUuid });
      created.push(res);
    } catch (err) {
      errors.push({ region, error: err.message });
    }
  });
  await limitConcurrency(tasks, 3);
  
  return { created, errors, skipped: targetRegions.filter(r => existingRegions.has(r)) };
}

async function rebuildAll() {
  const results = [];
  const list = db.getEgresses();
  const tasks = list.map(e => async () => {
    try {
      const res = await rebuildEgress(e.name);
      results.push(res);
    } catch (err) {
      results.push({ name: e.name, region: e.region, error: err.message });
    }
  });
  await limitConcurrency(tasks, 3);
  return results;
}

async function deleteAll() {
  const deleted = [];
  const tasks = db.getEgresses().map(e => async () => {
    try {
      await deleteEgress(e.name);
      deleted.push(e.name);
    } catch (err) {
      deleted.push({ name: e.name, error: err.message });
    }
  });
  await limitConcurrency(tasks, 3);
  return deleted;
}

function buildLink(egress, vpsHost) {
  return `vless://${egress.uuid}@${vpsHost}:${egress.port}?encryption=none&type=tcp&security=reality&flow=xtls-rprx-vision&pbk=${egress.publicKey}&sid=${egress.shortId}&sni=${SERVER_NAME}&fp=chrome&spx=%2F&headerType=none#${egress.name}`;
}

module.exports = {
  createEgress,
  rebuildEgress,
  deleteEgress,
  createAll,
  rebuildAll,
  deleteAll,
  buildLink,
  serverName: SERVER_NAME
};
