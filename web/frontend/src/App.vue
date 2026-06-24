<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue';
import SystemStatus from './components/SystemStatus.vue';
import CreateEgressForm from './components/CreateEgressForm.vue';
import EgressCard from './components/EgressCard.vue';
import JobList from './components/JobList.vue';

const API_BASE = '';
const egressList = ref([]);
const vpnRegions = ref([]);
const jobs = ref([]);
const systemStatus = ref(null);
const loadingRegions = ref(false);
const loadingList = ref(false);
const loadingJobs = ref(false);
const loadingSystemStatus = ref(false);
const creatingEgress = ref(false);
const creatingAll = ref(false);

const toastMessage = ref('');
const toastType = ref('success');
let toastTimeout = null;

const logModalVisible = ref(false);
const logModalTitle = ref('');
const logContent = ref('');
const loadingLogs = ref(false);

const linkModalVisible = ref(false);
const linkModalName = ref('');
const linkModalLink = ref('');
const linkModalLoading = ref(false);
const linkCopied = ref(false);

const subModalVisible = ref(false);
const subModalLinks = ref([]);
const subModalB64 = ref('');
const subUrl = ref('');
const subCopied = ref(false);

const jobDetailVisible = ref(false);
const selectedJob = ref(null);
const vpsAddress = ref('');

function showToast(msg, type = 'success') {
  toastMessage.value = msg;
  toastType.value = type;
  if (toastTimeout) clearTimeout(toastTimeout);
  toastTimeout = setTimeout(() => { toastMessage.value = ''; }, 4000);
}

// 简体中文注释：统一封装 API 请求函数，包含通用的错误提示
async function apiFetch(path, options = {}) {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
  });
  let data;
  try {
    data = await res.json();
  } catch (err) {
    // 无法解析 JSON
  }
  if (!res.ok) {
    if (res.status === 401) {
      throw new Error('未授权，请重新登录');
    }
    throw new Error(data?.error || `请求失败 (状态码: ${res.status})`);
  }
  return data;
}

async function fetchEgressList() {
  loadingList.value = true;
  try {
    egressList.value = await apiFetch('/api/egress/list');
    if (!vpsAddress.value) vpsAddress.value = window.location.hostname || 'your_vps_ip';
  } catch (err) { showToast(err.message, 'error'); }
  finally { loadingList.value = false; }
}

async function fetchRegions() {
  loadingRegions.value = true;
  try {
    vpnRegions.value = await apiFetch('/api/vpngate/regions');
  } catch (err) { showToast(err.message, 'error'); }
  finally { loadingRegions.value = false; }
}

async function fetchJobs() {
  loadingJobs.value = true;
  try {
    jobs.value = await apiFetch('/api/egress/jobs?limit=20');
  } catch (err) { showToast(err.message, 'error'); }
  finally { loadingJobs.value = false; }
}

async function fetchSystemStatus() {
  loadingSystemStatus.value = true;
  try {
    systemStatus.value = await apiFetch('/api/system/status');
  } catch (err) { showToast(err.message, 'error'); }
  finally { loadingSystemStatus.value = false; }
}

function updateJobHint(res, fallback) {
  if (res && res.jobId) showToast(`${fallback}，任务 #${res.jobId}`, 'success');
  else showToast(fallback, 'success');
}

async function createEgress(formValues) {
  creatingEgress.value = true;
  try {
    const r = await apiFetch('/api/egress/create', {
      method: 'POST',
      body: JSON.stringify({ name: formValues.name, region: formValues.region, uuid: formValues.uuid })
    });
    updateJobHint(r, r.message || '创建任务已提交');
    setTimeout(fetchEgressList, 2000);
    setTimeout(fetchJobs, 1000);
  } catch (err) { showToast(err.message, 'error'); }
  finally { creatingEgress.value = false; }
}

async function createAllRegions(formValues) {
  creatingAll.value = true;
  try {
    const r = await apiFetch('/api/egress/create-all', {
      method: 'POST',
      body: JSON.stringify({ uuid: formValues.uuid })
    });
    updateJobHint(r, r.message || '批量创建任务已提交');
    setTimeout(fetchEgressList, 3000);
    setTimeout(fetchJobs, 1000);
  } catch (err) { showToast(err.message, 'error'); }
  finally { creatingAll.value = false; }
}

async function rebuildEgress(name) {
  try {
    const r = await apiFetch('/api/egress/rebuild', {
      method: 'POST',
      body: JSON.stringify({ name })
    });
    updateJobHint(r, `${name} 正在漂移`);
    setTimeout(fetchEgressList, 2000);
    setTimeout(fetchJobs, 1000);
  } catch (err) { showToast(err.message, 'error'); }
}

async function rebuildAll() {
  try {
    const r = await apiFetch('/api/egress/rebuild-all', { method: 'POST' });
    updateJobHint(r, r.message || '批量漂移任务已提交');
    setTimeout(fetchEgressList, 3000);
    setTimeout(fetchJobs, 1000);
  } catch (err) { showToast(err.message, 'error'); }
}

async function deleteEgress(name) {
  if (!confirm(`确定删除出口 ${name}？`)) return;
  try {
    const r = await apiFetch('/api/egress/delete', {
      method: 'POST',
      body: JSON.stringify({ name })
    });
    updateJobHint(r, `${name} 已删除`);
    fetchEgressList();
    fetchJobs();
  } catch (err) { showToast(err.message, 'error'); }
}

async function deleteAll() {
  try {
    const r = await apiFetch('/api/egress/delete-all', { method: 'POST' });
    updateJobHint(r, r.message || '删除全部任务已提交');
    fetchEgressList();
    fetchJobs();
  } catch (err) { showToast(err.message, 'error'); }
}

async function openLinkModal(name) {
  linkModalVisible.value = true; linkModalName.value = name;
  linkModalLink.value = ''; linkModalLoading.value = true; linkCopied.value = false;
  try {
    const r = await apiFetch(`/api/egress/${name}/link`);
    linkModalLink.value = r.link;
  } catch (err) { linkModalLink.value = '获取失败'; showToast(err.message, 'error'); }
  finally { linkModalLoading.value = false; }
}

async function openSubModal() {
  subModalVisible.value = true; subModalLinks.value = []; subModalB64.value = ''; subUrl.value = ''; subCopied.value = false;
  try {
    const r = await apiFetch('/api/egress/subscription');
    subModalLinks.value = r.links; subModalB64.value = r.subscription;
    subUrl.value = `${window.location.origin}/api/egress/subscription.txt`;
  } catch (err) { showToast(err.message, 'error'); }
}

async function viewLogs(name) {
  logModalVisible.value = true; logModalTitle.value = name;
  logContent.value = ''; loadingLogs.value = true;
  try {
    const r = await apiFetch(`/api/egress/${name}/logs?tail=160`);
    logContent.value = r.logs || '暂无日志';
  } catch (err) { logContent.value = '读取失败: ' + err.message; }
  finally { loadingLogs.value = false; }
}

function openJobDetail(job) {
  selectedJob.value = job;
  jobDetailVisible.value = true;
}

function prettyJson(value) {
  return JSON.stringify(value || {}, null, 2);
}

function parseLink(link) {
  try {
    const u = new URL(link); const sp = u.searchParams;
    return { uuid: u.username, host: u.hostname, port: u.port, sni: sp.get('sni')||'-', pbk: sp.get('pbk')||'-', sid: sp.get('sid')||'-', fp: sp.get('fp')||'-' };
  } catch { return null; }
}

async function copyText(text, successMsg) {
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      const ta = document.createElement('textarea');
      ta.value = text; ta.style.cssText = 'position:fixed;opacity:0';
      document.body.appendChild(ta); ta.select(); document.execCommand('copy');
      document.body.removeChild(ta);
    }
    showToast(successMsg || '已复制', 'success');
    return true;
  } catch (err) { showToast('复制失败', 'error'); return false; }
}

async function copyAllLinks() {
  await copyText(subModalLinks.value.join('\n'), '全部链接已复制');
}

const stats = computed(() => ({
  total: egressList.value.length,
  running: egressList.value.filter(e => e.status === 'running').length,
  error: egressList.value.filter(e => e.status === 'error').length
}));

// 简体中文注释：定时调度逻辑与页面可见性心跳管理
let egressTimer = null;
let jobsTimer = null;
let systemTimer = null;

function scheduleEgress() {
  if (document.hidden) return;
  egressTimer = setTimeout(async () => {
    await fetchEgressList();
    scheduleEgress();
  }, 15000);
}

function scheduleJobs() {
  if (document.hidden) return;
  jobsTimer = setTimeout(async () => {
    await fetchJobs();
    scheduleJobs();
  }, 5000);
}

function scheduleSystem() {
  if (document.hidden) return;
  systemTimer = setTimeout(async () => {
    await fetchSystemStatus();
    scheduleSystem();
  }, 10000);
}

function startPolling() {
  stopPolling();
  scheduleEgress();
  scheduleJobs();
  scheduleSystem();
}

function stopPolling() {
  if (egressTimer) clearTimeout(egressTimer);
  if (jobsTimer) clearTimeout(jobsTimer);
  if (systemTimer) clearTimeout(systemTimer);
}

async function handleVisibilityChange() {
  if (document.hidden) {
    stopPolling();
  } else {
    await Promise.allSettled([
      fetchEgressList(),
      fetchJobs(),
      fetchSystemStatus()
    ]);
    startPolling();
  }
}

onMounted(() => {
  fetchEgressList();
  fetchRegions();
  fetchJobs();
  fetchSystemStatus();
  startPolling();
  document.addEventListener('visibilitychange', handleVisibilityChange);
});

onUnmounted(() => {
  stopPolling();
  document.removeEventListener('visibilitychange', handleVisibilityChange);
});
</script>

<template>
  <div class="container">
    <Transition name="toast">
      <div v-if="toastMessage" :class="['toast', `toast-${toastType}`]">
        <span v-if="toastType==='success'">&#10003;</span>
        <span v-else-if="toastType==='error'">&#10007;</span>
        <span v-else>&#9432;</span>
        {{ toastMessage }}
      </div>
    </Transition>

    <Transition name="fade">
      <div v-if="linkModalVisible" class="modal-bg" @click.self="linkModalVisible=false">
        <div class="modal glass-panel">
          <div class="modal-head"><h3>{{ linkModalName }} 订阅链接</h3><button class="btn btn-sm" @click="linkModalVisible=false">&times;</button></div>
          <div v-if="linkModalLoading" class="modal-loading">获取中...</div>
          <template v-else>
            <div class="link-breakdown" v-if="parseLink(linkModalLink)">
              <div class="kv" v-for="(v,k) in parseLink(linkModalLink)" :key="k"><span class="kv-k">{{ k }}</span><span class="kv-v mono">{{ v }}</span></div>
            </div>
            <div class="link-box"><code>{{ linkModalLink }}</code></div>
            <div class="modal-actions"><button class="btn btn-primary" @click="copyLinkFromModal">{{ linkCopied ? '已复制' : '复制链接' }}</button></div>
          </template>
        </div>
      </div>
    </Transition>

    <Transition name="fade">
      <div v-if="subModalVisible" class="modal-bg" @click.self="subModalVisible=false">
        <div class="modal modal-wide glass-panel">
          <div class="modal-head">
            <div><h3>订阅管理</h3><p class="text-secondary">{{ subModalLinks.length }} 个出口 · 可导入 v2rayN / Clash Meta / Sing-box</p></div>
            <button class="btn btn-sm" @click="subModalVisible=false">&times;</button>
          </div>
          <div class="sub-actions">
            <button class="btn btn-success" @click="copySubUrl">复制订阅 URL</button>
            <button class="btn btn-primary" @click="copySubB64">{{ subCopied ? '已复制' : '复制订阅 Base64' }}</button>
            <button class="btn" @click="copyAllLinks">复制全部链接</button>
          </div>
          <div class="link-box"><code>{{ subUrl }}</code></div>
          <div class="sub-links">
            <div v-for="link in subModalLinks" :key="link" class="sub-link-row" @click="copyText(link, '已复制单条链接')"><code>{{ link }}</code></div>
            <div v-if="subModalLinks.length===0" class="text-secondary" style="text-align:center;padding:24px">暂无出口</div>
          </div>
        </div>
      </div>
    </Transition>

    <Transition name="fade">
      <div v-if="logModalVisible" class="modal-bg" @click.self="logModalVisible=false">
        <div class="modal modal-wide glass-panel">
          <div class="modal-head"><div><h3>{{ logModalTitle }} 日志</h3></div><button class="btn btn-sm" @click="logModalVisible=false">&times;</button></div>
          <pre class="log-output">{{ loadingLogs ? '读取中...' : logContent }}</pre>
        </div>
      </div>
    </Transition>

    <Transition name="fade">
      <div v-if="jobDetailVisible" class="modal-bg" @click.self="jobDetailVisible=false">
        <div class="modal modal-wide glass-panel">
          <div class="modal-head">
            <div>
              <h3>任务 #{{ selectedJob?.id }} 详情</h3>
              <p class="text-secondary">{{ selectedJob?.type }} / {{ selectedJob?.target || '-' }}</p>
            </div>
            <button class="btn btn-sm" @click="jobDetailVisible=false">&times;</button>
          </div>
          <div v-if="selectedJob" class="job-detail-grid">
            <div class="job-detail-item">
              <span class="kv-k">状态</span>
              <span>{{ selectedJob.status }}</span>
            </div>
            <div class="job-detail-item">
              <span class="kv-k">更新时间</span>
              <span>{{ selectedJob.updatedAt }}</span>
            </div>
          </div>
          <div v-if="selectedJob?.error" class="detail-error">{{ selectedJob.error }}</div>
          <h4 class="detail-title">请求参数</h4>
          <pre class="detail-json">{{ prettyJson(selectedJob?.payload) }}</pre>
          <h4 class="detail-title">执行结果</h4>
          <pre class="detail-json">{{ prettyJson(selectedJob?.result) }}</pre>
        </div>
      </div>
    </Transition>

    <header class="header">
      <div class="header-left">
        <h1>VLESS Reality 控制台</h1>
        <p class="text-secondary">多出口透明自愈代理</p>
      </div>
      <div class="header-stats">
        <div class="stat"><span class="stat-val">{{ stats.total }}</span><span class="stat-lbl">出口</span></div>
        <div class="stat"><span class="stat-val text-success">{{ stats.running }}</span><span class="stat-lbl">正常</span></div>
        <div class="stat" v-if="stats.error"><span class="stat-val text-danger">{{ stats.error }}</span><span class="stat-lbl">异常</span></div>
      </div>
    </header>

    <div class="main-grid">
      <aside class="panel glass-panel">
        <SystemStatus
          :systemStatus="systemStatus"
          :loadingSystemStatus="loadingSystemStatus"
          @refresh="fetchSystemStatus"
        />

        <div class="divider"></div>

        <CreateEgressForm
          :vpnRegions="vpnRegions"
          :loadingRegions="loadingRegions"
          :creatingEgress="creatingEgress"
          :creatingAll="creatingAll"
          :egressCount="egressList.length"
          @create-single="createEgress"
          @create-all="createAllRegions"
          @rebuild-all="rebuildAll"
          @delete-all="deleteAll"
          @open-sub="openSubModal"
        />

        <div class="divider"></div>
        <h2>备份恢复</h2>
        <p class="text-secondary mb-16">包含 `.env`、SQLite 数据和出口配置</p>
        <div class="ops-box">
          <code>./scripts/backup.sh</code>
          <code>./scripts/restore.sh ./backups/vless-reality-backup-YYYYmmdd-HHMMSS.tar.gz</code>
        </div>
      </aside>

      <main>
        <div class="list-head">
          <h2>出口管理</h2>
          <button class="btn btn-sm" @click="fetchEgressList" :disabled="loadingList">{{ loadingList ? '...' : '刷新' }}</button>
        </div>

        <div v-if="egressList.length === 0" class="empty glass-panel">
          <p>暂无出口，从左侧创建</p>
        </div>

        <div class="egress-grid" v-else>
          <EgressCard
            v-for="eg in egressList"
            :key="eg.name"
            :eg="eg"
            :vpsAddress="vpsAddress"
            @open-link="openLinkModal"
            @rebuild="rebuildEgress"
            @view-logs="viewLogs"
            @delete="deleteEgress"
          />
        </div>

        <JobList
          :jobs="jobs"
          :loadingJobs="loadingJobs"
          @refresh="fetchJobs"
          @open-detail="openJobDetail"
        />
      </main>
    </div>
  </div>
</template>

<style scoped>
/* 吐司弹窗美化 */
.toast {
  position: fixed;
  top: 24px;
  right: 24px;
  z-index: 9999;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 24px;
  border-radius: 16px;
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  font-weight: 600;
  font-size: 14px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.5), 0 0 20px 0 rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.08);
}
.toast-success {
  background: rgba(16, 185, 129, 0.12);
  border-color: rgba(16, 185, 129, 0.25);
  color: #34d399;
  box-shadow: 0 10px 30px -10px rgba(16, 185, 129, 0.25);
}
.toast-error {
  background: rgba(239, 68, 68, 0.12);
  border-color: rgba(239, 68, 68, 0.25);
  color: #f87171;
  box-shadow: 0 10px 30px -10px rgba(239, 68, 68, 0.25);
}
.toast-enter-active, .toast-leave-active {
  transition: all 0.4s cubic-bezier(0.16, 1, 0.3, 1);
}
.toast-enter-from {
  opacity: 0;
  transform: translateY(-20px) scale(0.95);
}
.toast-leave-to {
  opacity: 0;
  transform: translateY(-10px) scale(0.98);
}

/* 模态框背景与毛玻璃 */
.modal-bg {
  position: fixed;
  inset: 0;
  z-index: 990;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgba(3, 3, 7, 0.75);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}

.modal {
  width: min(580px, 100%);
  max-height: 85vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 20px;
  background: rgba(15, 22, 42, 0.75);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 30px 60px -15px rgba(0, 0, 0, 0.8), 0 0 50px 0 rgba(99, 102, 241, 0.05);
}
.modal-wide {
  width: min(880px, 100%);
}
.modal-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 16px;
}
.modal-head h3 {
  font-size: 18px;
  font-weight: 800;
  background: linear-gradient(135deg, #fff 0%, #cbd5e1 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}
.modal-loading {
  padding: 40px 0;
  text-align: center;
  color: var(--text-secondary);
  font-weight: 500;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 8px;
}
.fade-enter-active, .fade-leave-active {
  transition: opacity 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.fade-enter-from, .fade-leave-to {
  opacity: 0;
}

/* 链接信息网格展示 */
.link-breakdown {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
  background: rgba(5, 5, 12, 0.3);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 16px;
}
.kv {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.kv-k {
  font-size: 9px;
  text-transform: uppercase;
  color: var(--text-secondary);
  letter-spacing: 1px;
  font-weight: 700;
}
.kv-v {
  font-size: 13px;
  word-break: break-all;
  color: var(--text-primary);
  font-weight: 500;
}
.link-box {
  background: rgba(5, 5, 12, 0.45);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 16px;
  font-size: 12px;
  word-break: break-all;
  line-height: 1.6;
  font-family: 'JetBrains Mono', monospace;
  color: #818cf8;
  max-height: 120px;
  overflow-y: auto;
}

.sub-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.sub-links {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 40vh;
  overflow-y: auto;
  padding-right: 4px;
}
.sub-link-row {
  background: rgba(5, 5, 12, 0.2);
  border: 1px solid var(--border-color);
  border-radius: 10px;
  padding: 10px 14px;
  font-size: 12px;
  cursor: pointer;
  transition: var(--transition-smooth);
  word-break: break-all;
  font-family: 'JetBrains Mono', monospace;
  color: #a5b4fc;
}
.sub-link-row:hover {
  border-color: var(--accent-color);
  background: rgba(99, 102, 241, 0.05);
}

.log-output {
  min-height: 250px;
  max-height: 55vh;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
  background: rgba(5, 5, 12, 0.55);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 16px;
  font-size: 12px;
  line-height: 1.6;
  font-family: 'JetBrains Mono', monospace;
  color: #cbd5e1;
}

/* 顶部大标题与统称卡 */
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 36px;
  flex-wrap: wrap;
  gap: 20px;
}
.header h1 {
  font-size: 28px;
  font-weight: 800;
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, #ffffff 0%, #cbd5e1 50%, #818cf8 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  filter: drop-shadow(0 4px 12px rgba(99, 102, 241, 0.15));
}
.header-stats {
  display: flex;
  gap: 16px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--border-color);
  padding: 8px 20px;
  border-radius: 16px;
  backdrop-filter: blur(8px);
}
.stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 60px;
}
.stat-val {
  font-size: 22px;
  font-weight: 800;
  line-height: 1.2;
}
.stat-lbl {
  font-size: 9px;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.8px;
  font-weight: 600;
  margin-top: 2px;
}

/* 布局分布 */
.main-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 24px;
}
@media(min-width:992px){
  .main-grid {
    grid-template-columns: 330px 1fr;
  }
}

.panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  background: rgba(11, 12, 22, 0.5);
  height: fit-content;
}
.panel h2 {
  font-size: 15px;
  font-weight: 800;
  margin-bottom: 2px;
  background: linear-gradient(90deg, #fff 0%, #94a3b8 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}
.section-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 16px;
}
.section-head h2 {
  font-size: 15px;
  font-weight: 800;
  margin-bottom: 2px;
}

.w-full {
  width: 100%;
}
.mb-16 {
  margin-bottom: 16px;
}
.divider {
  height: 1px;
  background: linear-gradient(90deg, rgba(255,255,255,0.06) 0%, transparent 100%);
  margin: 12px 0;
}

/* 终端指令备份展示框 */
.ops-box {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.ops-box code {
  display: block;
  background: rgba(5, 5, 12, 0.45);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 12px 14px;
  font-size: 11px;
  line-height: 1.5;
  color: #818cf8;
  word-break: break-all;
  font-family: 'JetBrains Mono', monospace;
  position: relative;
}
.ops-box code::before {
  content: '$';
  color: #ec4899;
  margin-right: 8px;
  font-weight: bold;
}

/* 列表部分标题 */
.list-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}
.list-head h2 {
  font-size: 18px;
  font-weight: 800;
  background: linear-gradient(90deg, #fff 0%, #cbd5e1 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}
.empty {
  text-align: center;
  padding: 60px 20px;
  color: var(--text-secondary);
  font-weight: 500;
  background: rgba(15, 23, 42, 0.15);
  border: 1px dashed rgba(255, 255, 255, 0.05);
}

.egress-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 20px;
}
@media(min-width:768px){
  .egress-grid {
    grid-template-columns: 1fr 1fr;
  }
}

/* 任务详情弹窗细节 */
.job-detail-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.job-detail-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  background: rgba(5, 5, 12, 0.25);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 12px 16px;
}
.detail-title {
  font-size: 12px;
  font-weight: 800;
  color: var(--text-secondary);
  margin-top: 14px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.detail-json {
  max-height: 200px;
  overflow: auto;
  background: rgba(5, 5, 12, 0.45);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 14px;
  font-size: 12px;
  line-height: 1.6;
  font-family: 'JetBrains Mono', monospace;
  color: #a5b4fc;
}
.detail-error {
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.2);
  border-radius: 12px;
  padding: 12px 16px;
  color: #f87171;
  font-size: 13px;
  line-height: 1.5;
}

.text-secondary {
  color: var(--text-secondary);
}
.text-success {
  color: #34d399;
}
.text-danger {
  color: #f87171;
}
.mono {
  font-family: 'JetBrains Mono', monospace;
}
.btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
  transform: none !important;
  box-shadow: none !important;
}
.btn-sm {
  font-size: 12px;
  padding: 8px 14px;
  border-radius: 8px;
}

.spinner {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.2);
  border-radius: 50%;
  border-top-color: white;
  animation: spin 0.8s linear infinite;
  display: inline-block;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
