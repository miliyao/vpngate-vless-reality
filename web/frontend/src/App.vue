<script setup>
import { ref, onMounted, computed } from 'vue';

const API_BASE = '';
const egressList = ref([]);
const vpnRegions = ref([]);
const jobs = ref([]);
const loadingRegions = ref(false);
const loadingList = ref(false);
const loadingJobs = ref(false);
const creatingEgress = ref(false);
const creatingAll = ref(false);
const formName = ref('');
const formRegion = ref('');
const formUuid = ref('');

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

const REGION_CN = {
  JP: '日本',
  KR: '韩国',
  TH: '泰国',
  RU: '俄罗斯',
  RO: '罗马尼亚',
  VN: '越南',
  US: '美国',
  HR: '克罗地亚',
  CN: '中国',
  HK: '中国香港',
  TW: '中国台湾',
  SG: '新加坡',
  DE: '德国',
  FR: '法国',
  GB: '英国',
  CA: '加拿大',
  AU: '澳大利亚',
  NL: '荷兰',
  PL: '波兰',
  BR: '巴西',
  IN: '印度',
  ID: '印度尼西亚',
  MY: '马来西亚',
  PH: '菲律宾'
};

function showToast(msg, type = 'success') {
  toastMessage.value = msg;
  toastType.value = type;
  if (toastTimeout) clearTimeout(toastTimeout);
  toastTimeout = setTimeout(() => { toastMessage.value = ''; }, 4000);
}

async function fetchEgressList() {
  loadingList.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/egress/list`);
    if (!res.ok) throw new Error('拉取出口列表失败');
    egressList.value = await res.json();
    if (!vpsAddress.value) vpsAddress.value = window.location.hostname || 'your_vps_ip';
  } catch (err) { showToast(err.message, 'error'); }
  finally { loadingList.value = false; }
}

async function fetchRegions() {
  loadingRegions.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/vpngate/regions`);
    if (!res.ok) throw new Error('拉取地区列表失败');
    vpnRegions.value = await res.json();
    if (vpnRegions.value.length > 0 && !formRegion.value) formRegion.value = vpnRegions.value[0].code;
  } catch (err) { showToast(err.message, 'error'); }
  finally { loadingRegions.value = false; }
}

async function fetchJobs() {
  loadingJobs.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/egress/jobs?limit=20`);
    if (!res.ok) throw new Error('拉取任务列表失败');
    jobs.value = await res.json();
  } catch (err) { showToast(err.message, 'error'); }
  finally { loadingJobs.value = false; }
}

function regionName(code, fallback = '') {
  if (!code) return fallback || '-';
  return REGION_CN[String(code).toUpperCase()] || fallback || String(code).toUpperCase();
}

function jobBadge(job) {
  return {
    queued: 'job-queued',
    running: 'job-running',
    done: 'job-done',
    failed: 'job-failed'
  }[job.status] || 'job-queued';
}

function jobLabel(job) {
  return {
    queued: '排队中',
    running: '执行中',
    done: '已完成',
    failed: '失败'
  }[job.status] || job.status;
}

function updateJobHint(res, fallback) {
  if (res && res.jobId) showToast(`${fallback}，任务 #${res.jobId}`, 'success');
  else showToast(fallback, 'success');
}

function openJobDetail(job) {
  selectedJob.value = job;
  jobDetailVisible.value = true;
}

function prettyJson(value) {
  return JSON.stringify(value || {}, null, 2);
}

async function createEgress() {
  if (!formName.value || !formRegion.value) { showToast('请输入名称并选择地区', 'error'); return; }
  creatingEgress.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/egress/create`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: formName.value.trim(), region: formRegion.value, uuid: formUuid.value.trim() || undefined })
    });
    const r = await res.json();
    if (!res.ok) throw new Error(r.error || '创建失败');
    formName.value = ''; formUuid.value = '';
    updateJobHint(r, r.message || '创建任务已提交');
    setTimeout(fetchEgressList, 2000);
    setTimeout(fetchJobs, 1000);
  } catch (err) { showToast(err.message, 'error'); }
  finally { creatingEgress.value = false; }
}

async function createAllRegions() {
  if (!confirm('将为所有 VPNGate 可用地区各创建一个出口，确认？')) return;
  creatingAll.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/egress/create-all`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ uuid: formUuid.value.trim() || undefined })
    });
    const r = await res.json();
    if (!res.ok) throw new Error(r.error || '批量创建失败');
    updateJobHint(r, r.message || '批量创建任务已提交');
    setTimeout(fetchEgressList, 3000);
    setTimeout(fetchJobs, 1000);
  } catch (err) { showToast(err.message, 'error'); }
  finally { creatingAll.value = false; }
}

async function rebuildEgress(name) {
  try {
    const res = await fetch(`${API_BASE}/api/egress/rebuild`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    });
    const r = await res.json();
    if (!res.ok) throw new Error(r.error || '重建失败');
    updateJobHint(r, `${name} 正在漂移`);
    setTimeout(fetchEgressList, 2000);
    setTimeout(fetchJobs, 1000);
  } catch (err) { showToast(err.message, 'error'); }
}

async function rebuildAll() {
  if (!confirm('将对所有出口执行透明漂移，确认？')) return;
  try {
    const res = await fetch(`${API_BASE}/api/egress/rebuild-all`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }
    });
    const r = await res.json();
    if (!res.ok) throw new Error(r.error || '批量漂移失败');
    updateJobHint(r, r.message || '批量漂移任务已提交');
    setTimeout(fetchEgressList, 3000);
    setTimeout(fetchJobs, 1000);
  } catch (err) { showToast(err.message, 'error'); }
}

async function deleteEgress(name) {
  if (!confirm(`确定删除出口 ${name}？`)) return;
  try {
    const res = await fetch(`${API_BASE}/api/egress/delete`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    });
    const r = await res.json();
    if (!res.ok) throw new Error(r.error || '删除失败');
    updateJobHint(r, `${name} 已删除`);
    fetchEgressList();
    fetchJobs();
  } catch (err) { showToast(err.message, 'error'); }
}

async function deleteAll() {
  if (!confirm('确定删除全部出口？此操作不可撤销！')) return;
  try {
    const res = await fetch(`${API_BASE}/api/egress/delete-all`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }
    });
    const r = await res.json();
    if (!res.ok) throw new Error(r.error || '删除失败');
    updateJobHint(r, r.message || '删除全部任务已提交');
    fetchEgressList();
    fetchJobs();
  } catch (err) { showToast(err.message, 'error'); }
}

async function openLinkModal(name) {
  linkModalVisible.value = true; linkModalName.value = name;
  linkModalLink.value = ''; linkModalLoading.value = true; linkCopied.value = false;
  try {
    const res = await fetch(`${API_BASE}/api/egress/${name}/link`);
    const r = await res.json();
    if (!res.ok) throw new Error(r.error || '获取链接失败');
    linkModalLink.value = r.link;
  } catch (err) { linkModalLink.value = '获取失败'; showToast(err.message, 'error'); }
  finally { linkModalLoading.value = false; }
}

async function openSubModal() {
  subModalVisible.value = true; subModalLinks.value = []; subModalB64.value = ''; subUrl.value = ''; subCopied.value = false;
  try {
    const res = await fetch(`${API_BASE}/api/egress/subscription`);
    const r = await res.json();
    if (!res.ok) throw new Error(r.error || '获取订阅失败');
    subModalLinks.value = r.links; subModalB64.value = r.subscription;
    subUrl.value = `${window.location.origin}/api/egress/subscription.txt`;
  } catch (err) { showToast(err.message, 'error'); }
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

async function copyLinkFromModal() {
  if (await copyText(linkModalLink.value, '链接已复制')) { linkCopied.value = true; setTimeout(() => linkCopied.value = false, 3000); }
}

async function copySubB64() {
  if (await copyText(subModalB64.value, '订阅 Base64 已复制，可导入客户端')) { subCopied.value = true; setTimeout(() => subCopied.value = false, 3000); }
}

async function copySubUrl() {
  if (await copyText(subUrl.value, '订阅 URL 已复制')) { subCopied.value = true; setTimeout(() => subCopied.value = false, 3000); }
}

async function copyAllLinks() {
  await copyText(subModalLinks.value.join('\n'), '全部链接已复制');
}

async function viewLogs(name) {
  logModalVisible.value = true; logModalTitle.value = name;
  logContent.value = ''; loadingLogs.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/egress/${name}/logs?tail=160`);
    const r = await res.json();
    if (!res.ok) throw new Error(r.error || '读取日志失败');
    logContent.value = r.logs || '暂无日志';
  } catch (err) { logContent.value = '读取失败: ' + err.message; }
  finally { loadingLogs.value = false; }
}

function getStatusLabel(s) {
  return { running:'运行中', starting:'启动中', rebuilding:'漂移中', error:'异常', offline:'离线' }[s] || s || '未知';
}
function getStatusClass(s) {
  return { running:'success', starting:'warning', rebuilding:'warning', error:'danger', offline:'muted' }[s] || 'muted';
}
function getFlagEmoji(c) {
  if (!c) return '';
  try { return String.fromCodePoint(...c.toUpperCase().split('').map(x => 127397 + x.charCodeAt(0))); } catch { return c; }
}
function timeAgo(ts) {
  if (!ts) return '-';
  const s = Math.floor((Date.now() - ts) / 1000);
  if (s < 60) return s + '秒前';
  if (s < 3600) return Math.floor(s / 60) + '分钟前';
  return Math.floor(s / 3600) + '小时前';
}
function parseLink(link) {
  try {
    const u = new URL(link); const sp = u.searchParams;
    return { uuid: u.username, host: u.hostname, port: u.port, sni: sp.get('sni')||'-', pbk: sp.get('pbk')||'-', sid: sp.get('sid')||'-', fp: sp.get('fp')||'-' };
  } catch { return null; }
}

const stats = computed(() => ({
  total: egressList.value.length,
  running: egressList.value.filter(e => e.status === 'running').length,
  error: egressList.value.filter(e => e.status === 'error').length
}));

onMounted(() => {
  fetchEgressList();
  fetchRegions();
  fetchJobs();
  setInterval(fetchEgressList, 15000);
  setInterval(fetchJobs, 3000);
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
          <div class="link-box"><code>{{ subUrl || `${window.location.origin}/api/egress/subscription.txt` }}</code></div>
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
              <span :class="['job-pill', jobBadge(selectedJob)]">{{ jobLabel(selectedJob) }}</span>
            </div>
            <div class="job-detail-item">
              <span class="kv-k">更新时间</span>
              <span>{{ timeAgo(selectedJob.updatedAt) }}</span>
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
        <h2>创建出口</h2>
        <p class="text-secondary mb-16">自动获取 VPNGate 最优节点</p>
        <form @submit.prevent="createEgress" class="form">
          <label class="label">名称</label>
          <input v-model="formName" type="text" class="input" placeholder="如 jp-01" required :disabled="creatingEgress" />
          <label class="label">地区</label>
          <select v-model="formRegion" class="input" :disabled="creatingEgress || loadingRegions">
            <option v-if="loadingRegions" value="">加载中...</option>
            <option v-for="r in vpnRegions" :key="r.code" :value="r.code">{{ getFlagEmoji(r.code) }} {{ regionName(r.code, r.name) }} ({{ r.count }})</option>
          </select>
          <label class="label">UUID <span class="text-secondary">(可选，留空自动生成)</span></label>
          <input v-model="formUuid" type="text" class="input" placeholder="留空则自动生成" :disabled="creatingEgress" />
          <button type="submit" class="btn btn-primary w-full" :disabled="creatingEgress">
            <span v-if="creatingEgress" class="spinner"></span>
            {{ creatingEgress ? '部署中...' : '构建单个出口' }}
          </button>
        </form>

        <div class="divider"></div>
        <h2>批量操作</h2>
        <p class="text-secondary mb-16">一键管理全部地区出口</p>
        <div class="batch-btns">
          <button class="btn btn-primary w-full" @click="createAllRegions" :disabled="creatingAll">
            <span v-if="creatingAll" class="spinner"></span>
            {{ creatingAll ? '正在创建...' : '一键创建全部地区' }}
          </button>
          <button class="btn w-full" @click="openSubModal">查看/复制订阅</button>
          <button class="btn w-full" @click="rebuildAll" :disabled="egressList.length===0">全部漂移</button>
          <button class="btn btn-danger w-full" @click="deleteAll" :disabled="egressList.length===0">删除全部</button>
        </div>

        <div class="hint-box">
          <p><strong>自愈</strong>：出口连续 2 次失败后自动漂移至同地区新节点，客户端配置不变。</p>
        </div>
      </aside>

      <main>
        <div class="list-head">
          <h2>出口管理</h2>
          <button class="btn btn-sm" @click="fetchEgressList" :disabled="loadingList">{{ loadingList ? '...' : '刷新' }}</button>
        </div>

        <div v-if="egressList.length===0" class="empty glass-panel">
          <p>暂无出口，从左侧创建</p>
        </div>

        <div class="egress-grid" v-else>
          <div v-for="eg in egressList" :key="eg.name" :class="['eg-card','glass-panel',`border-${getStatusClass(eg.status)}`]">
            <div class="eg-top">
              <div class="eg-identity"><span class="eg-flag">{{ getFlagEmoji(eg.region) }}</span><div><div class="eg-name">{{ eg.name }}</div><div class="text-secondary">{{ regionName(eg.region) }} / {{ eg.region }}</div></div></div>
              <span :class="['pill',`pill-${getStatusClass(eg.status)}`]">{{ getStatusLabel(eg.status) }}</span>
            </div>
            <div class="eg-conn">
              <div class="conn-row"><span class="conn-label">入口</span><span class="conn-val mono">{{ vpsAddress }}:{{ eg.port }}</span></div>
              <div class="conn-row"><span class="conn-label">VPN</span><span class="conn-val mono">{{ eg.nodeIp || '...' }}</span></div>
              <div class="conn-row"><span class="conn-label">出口IP</span><span :class="['conn-val','mono',eg.currentEgressIp&&eg.currentEgressIp!=='error'&&eg.currentEgressIp!=='offline'?'text-success':'text-danger']">{{ eg.currentEgressIp || '...' }}</span></div>
              <div class="conn-row"><span class="conn-label">延迟</span><span class="conn-val">{{ eg.latency||'-' }}ms</span></div>
              <div class="conn-row"><span class="conn-label">检测</span><span class="conn-val">{{ timeAgo(eg.lastCheckTime) }}</span></div>
            </div>
            <div v-if="eg.error" class="eg-error">{{ eg.error }}</div>
            <div class="eg-actions">
              <button class="btn btn-success btn-sm flex1" @click="openLinkModal(eg.name)">订阅</button>
              <button class="btn btn-sm flex1" @click="rebuildEgress(eg.name)" :disabled="eg.status==='starting'">漂移</button>
              <button class="btn btn-sm ico" @click="viewLogs(eg.name)">LOG</button>
              <button class="btn btn-danger btn-sm ico" @click="deleteEgress(eg.name)">DEL</button>
            </div>
          </div>
        </div>

        <section class="jobs-section glass-panel">
          <div class="list-head">
            <div>
              <h2>任务队列</h2>
              <p class="text-secondary">最近 20 条任务</p>
            </div>
            <button class="btn btn-sm" @click="fetchJobs" :disabled="loadingJobs">{{ loadingJobs ? '...' : '刷新任务' }}</button>
          </div>
          <div class="jobs-list">
            <div v-for="job in jobs" :key="job.id" class="job-row" @click="openJobDetail(job)">
              <div class="job-main">
                <span :class="['job-pill', jobBadge(job)]">{{ jobLabel(job) }}</span>
                <span class="job-type">#{{ job.id }} {{ job.type }} / {{ job.target || '-' }}</span>
                <span class="job-time">{{ timeAgo(job.updatedAt) }}</span>
              </div>
              <div class="job-error" v-if="job.error">{{ job.error }}</div>
            </div>
            <div v-if="jobs.length===0" class="text-secondary" style="padding:12px 0">暂无任务</div>
          </div>
        </section>
      </main>
    </div>
  </div>
</template>

<style scoped>
.toast{position:fixed;top:16px;right:16px;z-index:9999;display:flex;align-items:center;gap:10px;padding:12px 20px;border-radius:10px;backdrop-filter:blur(16px);font-weight:500;font-size:14px;box-shadow:0 8px 24px rgba(0,0,0,.4)}
.toast-success{background:rgba(0,230,118,.15);border:1px solid rgba(0,230,118,.3);color:var(--success-color)}
.toast-error{background:rgba(255,23,68,.15);border:1px solid rgba(255,23,68,.3);color:#ff5252}
.toast-enter-active,.toast-leave-active{transition:all .3s}
.toast-enter-from,.toast-leave-to{opacity:0;transform:translateY(-12px)}

.modal-bg{position:fixed;inset:0;z-index:900;display:flex;align-items:center;justify-content:center;padding:16px;background:rgba(0,0,0,.6);backdrop-filter:blur(6px)}
.modal{width:min(560px,100%);max-height:85vh;overflow-y:auto;display:flex;flex-direction:column;gap:16px}
.modal-wide{width:min(860px,100%)}
.modal-head{display:flex;justify-content:space-between;align-items:flex-start}
.modal-head h3{font-size:17px;font-weight:700}
.modal-loading{padding:24px 0;text-align:center;color:var(--text-secondary)}
.modal-actions{display:flex;justify-content:flex-end;gap:8px}
.fade-enter-active,.fade-leave-active{transition:opacity .25s}
.fade-enter-from,.fade-leave-to{opacity:0}

.link-breakdown{display:grid;grid-template-columns:1fr 1fr;gap:8px;background:rgba(0,0,0,.15);border-radius:8px;padding:12px}
.kv{display:flex;flex-direction:column;gap:2px}
.kv-k{font-size:10px;text-transform:uppercase;color:var(--text-secondary);letter-spacing:.5px}
.kv-v{font-size:13px;word-break:break-all}
.link-box{background:rgba(0,0,0,.25);border:1px solid var(--border-color);border-radius:8px;padding:12px;font-size:12px;word-break:break-all;line-height:1.6;font-family:monospace;color:#a5b4fc}

.sub-actions{display:flex;gap:8px;flex-wrap:wrap}
.sub-links{display:flex;flex-direction:column;gap:4px;max-height:45vh;overflow-y:auto}
.sub-link-row{background:rgba(0,0,0,.15);border:1px solid var(--border-color);border-radius:6px;padding:8px 12px;font-size:11px;cursor:pointer;transition:border-color .2s;word-break:break-all;font-family:monospace;color:#a5b4fc}
.sub-link-row:hover{border-color:var(--accent-color)}

.log-output{min-height:200px;max-height:55vh;overflow:auto;white-space:pre-wrap;word-break:break-word;background:rgba(0,0,0,.3);border:1px solid var(--border-color);border-radius:8px;padding:14px;font-size:12px;line-height:1.5;color:#d7e2f0}

.header{display:flex;justify-content:space-between;align-items:center;margin-bottom:28px;flex-wrap:wrap;gap:16px}
.header h1{font-size:24px;font-weight:800;background:linear-gradient(135deg,#fff 0%,#a5b4fc 100%);-webkit-background-clip:text;-webkit-text-fill-color:transparent}
.header-stats{display:flex;gap:16px}
.stat{display:flex;flex-direction:column;align-items:center;min-width:48px}
.stat-val{font-size:22px;font-weight:800;line-height:1.2}
.stat-lbl{font-size:10px;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px}

.main-grid{display:grid;grid-template-columns:1fr;gap:20px}
@media(min-width:992px){.main-grid{grid-template-columns:320px 1fr}}

.panel h2{font-size:16px;font-weight:700;margin-bottom:4px}
.form{display:flex;flex-direction:column;gap:10px}
.label{font-size:12px;font-weight:600;color:var(--text-secondary)}
.input{background:rgba(0,0,0,.2);border:1px solid var(--border-color);border-radius:8px;color:var(--text-primary);font-family:inherit;font-size:14px;padding:10px 14px;width:100%;outline:none;transition:border-color .2s,box-shadow .2s}
.input:focus{border-color:var(--accent-color);box-shadow:0 0 0 2px var(--accent-glow)}
.w-full{width:100%}
.mb-16{margin-bottom:16px}

.divider{height:1px;background:var(--border-color);margin:20px 0}
.batch-btns{display:flex;flex-direction:column;gap:8px}
.hint-box{margin-top:16px;background:rgba(88,101,242,.05);border:1px dashed rgba(88,101,242,.2);border-radius:8px;padding:12px;font-size:12px;color:var(--text-secondary);line-height:1.6}

.list-head{display:flex;justify-content:space-between;align-items:center;margin-bottom:16px}
.list-head h2{font-size:17px;font-weight:700}
.empty{text-align:center;padding:48px;color:var(--text-secondary)}
.egress-grid{display:grid;grid-template-columns:1fr;gap:16px}
@media(min-width:768px){.egress-grid{grid-template-columns:1fr 1fr}}
.eg-card{display:flex;flex-direction:column;gap:14px;border-left:4px solid var(--border-color)}
.border-success{border-left-color:var(--success-color)}
.border-warning{border-left-color:var(--warning-color)}
.border-danger{border-left-color:var(--danger-color)}
.border-muted{border-left-color:var(--text-secondary)}
.eg-top{display:flex;justify-content:space-between;align-items:center}
.eg-identity{display:flex;align-items:center;gap:10px}
.eg-flag{font-size:24px}
.eg-name{font-size:15px;font-weight:700}
.eg-conn{background:rgba(0,0,0,.15);border-radius:8px;padding:10px 12px;display:flex;flex-direction:column;gap:6px}
.conn-row{display:flex;justify-content:space-between;align-items:center;font-size:12px}
.conn-label{color:var(--text-secondary)}
.conn-val{font-weight:500}
.eg-error{background:rgba(255,23,68,.08);border:1px solid rgba(255,23,68,.2);border-radius:6px;padding:6px 10px;font-size:12px;color:#ff5252}
.eg-actions{display:flex;gap:6px;flex-wrap:wrap}
.flex1{flex:1;min-width:0}
.ico{min-width:42px;font-size:11px;font-weight:700}

.jobs-section{margin-top:20px}
.jobs-list{display:flex;flex-direction:column;gap:8px}
.job-row{display:flex;flex-direction:column;gap:4px;padding:10px 12px;background:rgba(0,0,0,.12);border:1px solid var(--border-color);border-radius:8px}
.job-row{cursor:pointer;transition:border-color .2s,background .2s}
.job-row:hover{background:rgba(255,255,255,.03);border-color:rgba(88,101,242,.35)}
.job-main{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.job-pill{display:inline-flex;align-items:center;padding:3px 8px;border-radius:999px;font-size:11px;font-weight:700}
.job-queued{background:rgba(255,145,0,.12);color:var(--warning-color)}
.job-running{background:rgba(88,101,242,.12);color:#a5b4fc}
.job-done{background:rgba(0,230,118,.12);color:var(--success-color)}
.job-failed{background:rgba(255,23,68,.12);color:#ff5252}
.job-type,.job-time{font-size:12px;color:var(--text-secondary)}
.job-error{font-size:12px;color:#ff5252;word-break:break-word}
.job-detail-grid{display:grid;grid-template-columns:1fr 1fr;gap:10px}
.job-detail-item{display:flex;flex-direction:column;gap:6px;background:rgba(0,0,0,.14);border:1px solid var(--border-color);border-radius:8px;padding:10px}
.detail-title{font-size:13px;font-weight:700;color:var(--text-secondary);margin-top:4px}
.detail-json{max-height:220px;overflow:auto;background:rgba(0,0,0,.28);border:1px solid var(--border-color);border-radius:8px;padding:12px;font-size:12px;line-height:1.5;color:#d7e2f0}
.detail-error{background:rgba(255,23,68,.08);border:1px solid rgba(255,23,68,.2);border-radius:8px;padding:10px;color:#ff5252;font-size:12px}

.text-secondary{color:var(--text-secondary)}
.text-success{color:var(--success-color)}
.text-danger{color:#ff5252}
.mono{font-family:monospace}

.btn{background:rgba(255,255,255,.05);border:1px solid var(--border-color);border-radius:8px;color:var(--text-primary);cursor:pointer;display:inline-flex;align-items:center;justify-content:center;font-family:inherit;font-weight:500;font-size:13px;padding:8px 16px;gap:6px;transition:all .2s;outline:none}
.btn:hover{background:rgba(255,255,255,.1)}
.btn:disabled{opacity:.5;cursor:not-allowed}
.btn-sm{font-size:12px;padding:6px 12px}
.btn-primary{background:var(--accent-color);border-color:transparent}
.btn-primary:hover{background:#4752c4}
.btn-success{background:rgba(0,230,118,.1);border-color:rgba(0,230,118,.3);color:var(--success-color)}
.btn-success:hover{background:var(--success-color);color:#0a0c10}
.btn-danger{background:rgba(255,23,68,.1);border-color:rgba(255,23,68,.3);color:#ff5252}
.btn-danger:hover{background:#ff1744;color:white}

.pill{display:inline-flex;align-items:center;font-size:12px;font-weight:600;padding:3px 10px;border-radius:20px}
.pill-success{background:rgba(0,230,118,.1);color:var(--success-color);border:1px solid rgba(0,230,118,.2)}
.pill-warning{background:rgba(255,145,0,.1);color:var(--warning-color);border:1px solid rgba(255,145,0,.2)}
.pill-danger{background:rgba(255,23,68,.1);color:#ff5252;border:1px solid rgba(255,23,68,.2)}
.pill-muted{background:rgba(139,155,180,.1);color:var(--text-secondary);border:1px solid rgba(139,155,180,.2)}
.spinner{width:14px;height:14px;border:2px solid rgba(255,255,255,.3);border-radius:50%;border-top-color:white;animation:spin 1s linear infinite;display:inline-block}
@keyframes spin{to{transform:rotate(360deg)}}
.glass-panel{background:var(--glass-bg);backdrop-filter:var(--glass-blur);-webkit-backdrop-filter:var(--glass-blur);border:1px solid var(--border-color);border-radius:12px;padding:20px;box-shadow:0 8px 32px rgba(0,0,0,.3)}
.container{max-width:1200px;margin:0 auto;padding:24px 16px}
@media(min-width:768px){.container{padding:32px 24px}}
</style>
