<script setup>
// 前端仪表盘主页面（Vue 3 Composition API）
// 中文注释，保证极致用户体验与清晰的数据双向绑定

import { ref, onMounted, computed } from 'vue';

// 接口基地址（开发环境下代理生效，生产环境下同域名托管）
const API_BASE = '';

// 状态定义
const egressList = ref([]);
const vpnRegions = ref([]);
const vpnNodes = ref([]);

const loadingRegions = ref(false);
const loadingList = ref(false);
const loadingNodes = ref(false);
const creatingEgress = ref(false);

// 新增出口表单
const formName = ref('');
const formRegion = ref('');
const formUuid = ref('');

// 提示消息通知
const toastMessage = ref('');
const toastType = ref('success'); // success, error, info
let toastTimeout = null;

function showToast(msg, type = 'success') {
  toastMessage.value = msg;
  toastType.value = type;
  if (toastTimeout) clearTimeout(toastTimeout);
  toastTimeout = setTimeout(() => {
    toastMessage.value = '';
  }, 4000);
}

// 1. 获取出口列表
async function fetchEgressList() {
  loadingList.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/egress/list`);
    if (!res.ok) throw new Error('拉取出口列表失败');
    egressList.value = await res.json();
  } catch (err) {
    showToast(err.message, 'error');
  } finally {
    loadingList.value = false;
  }
}

// 2. 获取可选地区列表
async function fetchRegions() {
  loadingRegions.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/vpngate/regions`);
    if (!res.ok) throw new Error('拉取地区列表失败');
    vpnRegions.value = await res.json();
    if (vpnRegions.value.length > 0 && !formRegion.value) {
      formRegion.value = vpnRegions.value[0].code;
    }
  } catch (err) {
    showToast(err.message, 'error');
  } finally {
    loadingRegions.value = false;
  }
}

// 3. 获取 VPNGate 节点预览
async function fetchVpnNodes() {
  loadingNodes.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/vpngate/nodes`);
    if (!res.ok) throw new Error('拉取节点预览失败');
    vpnNodes.value = await res.json();
  } catch (err) {
    showToast(err.message, 'error');
  } finally {
    loadingNodes.value = false;
  }
}

// 4. 创建新出口
async function createEgress() {
  if (!formName.value || !formRegion.value) {
    showToast('请输入出口名称并选择国家地区', 'error');
    return;
  }
  
  creatingEgress.value = true;
  try {
    const res = await fetch(`${API_BASE}/api/egress/create`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: formName.value.trim(),
        region: formRegion.value,
        uuid: formUuid.value.trim() || undefined
      })
    });

    const result = await res.json();
    if (!res.ok) throw new Error(result.error || '创建出口失败');

    showToast(`出口 ${formName.value} 部署指令已下发，正在初始化...`, 'success');
    
    // 重置表单
    formName.value = '';
    formUuid.value = '';
    
    // 延时刷新列表
    setTimeout(fetchEgressList, 1500);
  } catch (err) {
    showToast(err.message, 'error');
  } finally {
    creatingEgress.value = false;
  }
}

// 5. 手动重建/漂移出口
async function rebuildEgress(name) {
  try {
    const res = await fetch(`${API_BASE}/api/egress/rebuild`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    });
    
    const result = await res.json();
    if (!res.ok) throw new Error(result.error || '重建失败');
    
    showToast(`已成功为 ${name} 启动一键透明漂移...`, 'success');
    fetchEgressList();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// 6. 删除出口
async function deleteEgress(name) {
  if (!confirm(`确定要删除并下线出口 ${name} 吗？`)) return;
  
  try {
    const res = await fetch(`${API_BASE}/api/egress/delete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    });
    
    const result = await res.json();
    if (!res.ok) throw new Error(result.error || '删除失败');
    
    showToast(`出口 ${name} 已彻底删除并清理`, 'success');
    fetchEgressList();
  } catch (err) {
    showToast(err.message, 'error');
  }
}

// 7. 获取并复制订阅链接
async function copyLink(name) {
  try {
    const res = await fetch(`${API_BASE}/api/egress/${name}/link`);
    if (!res.ok) throw new Error('拉取订阅链接失败');
    
    const result = await res.json();
    
    // 兼容 HTTP 非安全上下文的复制方式
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(result.link);
    } else {
      // 降级复制方案
      const textArea = document.createElement("textarea");
      textArea.value = result.link;
      textArea.style.position = "fixed";
      textArea.style.opacity = "0";
      document.body.appendChild(textArea);
      textArea.focus();
      textArea.select();
      const successful = document.execCommand("copy");
      document.body.removeChild(textArea);
      if (!successful) throw new Error("浏览器不支持复制操作");
    }
    
    showToast(`已成功复制 ${name} 的 VLESS 订阅链接！`, 'success');
  } catch (err) {
    showToast(`复制失败: ${err.message}`, 'error');
    console.error('复制出错：', err);
  }
}

// 辅助状态翻译与颜色
function getStatusLabel(status) {
  switch (status) {
    case 'running': return '运行中';
    case 'starting': return '自愈/启动中';
    case 'error': return '链路异常';
    case 'offline': return '已离线';
    default: return status || '未知';
  }
}

// 获取国旗 Emoji
function getFlagEmoji(countryCode) {
  if (!countryCode) return '🏳️';
  const codePoints = countryCode
    .toUpperCase()
    .split('')
    .map(char =>  127397 + char.charCodeAt(0));
  try {
    return String.fromCodePoint(...codePoints);
  } catch {
    return countryCode;
  }
}

// 页面加载初始化
onMounted(() => {
  fetchEgressList();
  fetchRegions();
  fetchVpnNodes();
  
  // 每 20 秒自动更新一次出口状态
  setInterval(fetchEgressList, 20000);
});

// 计算统计指标
const stats = computed(() => {
  const total = egressList.value.length;
  const running = egressList.value.filter(e => e.status === 'running').length;
  return { total, running };
});
</script>

<template>
  <div class="container">
    <!-- 提示通知弹窗 -->
    <Transition name="toast">
      <div v-if="toastMessage" :class="['toast', `toast-${toastType}`]">
        <span class="toast-icon">
          <span v-if="toastType === 'success'">✓</span>
          <span v-else-if="toastType === 'error'">✕</span>
          <span v-else>ℹ</span>
        </span>
        {{ toastMessage }}
      </div>
    </Transition>

    <!-- 顶部页眉 -->
    <header class="header-section">
      <div class="header-logo">
        <span class="logo-emoji">🛡️</span>
        <div>
          <h1>VLESS Reality 控制台</h1>
          <p>基于 VPNGate 与 Docker 容器的多出口透明自愈代理方案</p>
        </div>
      </div>
      
      <!-- 汇总状态卡 -->
      <div class="status-summary">
        <div class="summary-item">
          <div class="val">{{ stats.total }}</div>
          <div class="lbl">已建出口</div>
        </div>
        <div class="summary-divider"></div>
        <div class="summary-item">
          <div class="val text-success">{{ stats.running }}</div>
          <div class="lbl">正常运行</div>
        </div>
      </div>
    </header>

    <!-- 主面板网格布局 -->
    <div class="grid-cols-3">
      <!-- 左栏：新增出口表单 -->
      <div class="glass-panel col-span-1">
        <h2 class="panel-title">🚀 快捷创建出口</h2>
        <p class="panel-subtitle">后端将自动获取指定地区的高分可用节点建立 VPN 桥接</p>
        
        <form @submit.prevent="createEgress" class="form-container">
          <div class="form-group">
            <label class="form-label">出口唯一标识名称 (仅英文数字)</label>
            <input 
              v-model="formName" 
              type="text" 
              class="input-field" 
              placeholder="例如: jp-tokyo-01" 
              required
              :disabled="creatingEgress"
            />
          </div>

          <div class="form-group">
            <label class="form-label">目标国家/地区 (基于 VPNGate 实测节点数)</label>
            <div class="select-wrapper">
              <select 
                v-model="formRegion" 
                class="input-field select-field" 
                :disabled="creatingEgress || loadingRegions"
              >
                <option v-if="loadingRegions" value="">正在拉取国家列表...</option>
                <option 
                  v-for="reg in vpnRegions" 
                  :key="reg.code" 
                  :value="reg.code"
                >
                  {{ getFlagEmoji(reg.code) }} {{ reg.name }} ({{ reg.count }} 节点可用)
                </option>
              </select>
            </div>
          </div>

          <div class="form-group">
            <label class="form-label">自定义客户端 UUID (留空自动生成)</label>
            <input 
              v-model="formUuid" 
              type="text" 
              class="input-field" 
              placeholder="格式: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              :disabled="creatingEgress"
            />
          </div>

          <button 
            type="submit" 
            class="btn btn-primary w-full" 
            :disabled="creatingEgress"
          >
            <span v-if="creatingEgress" class="spinner"></span>
            {{ creatingEgress ? '正在拉取节点并部署容器...' : '一键构建出口' }}
          </button>
        </form>

        <!-- 透明自愈技术原理科普 -->
        <div class="tech-card">
          <h4>💡 出口稳定性保障原理：</h4>
          <ul>
            <li>容器内建网络自愈，当 VPNGate 节点因波动掉线，OpenVPN 会不断尝试重连。</li>
            <li>若出口彻底损坏，后端主控在 60s 内自动触发<strong>“透明漂移”</strong>。</li>
            <li>系统将拉取该地区最新最佳节点并重建容器，由于 <strong>Xray 侦听端口、UUID 与 Reality 密钥绝不改变</strong>，因此您的客户端<strong>无需任何修改</strong>，即可透明漂移。</li>
          </ul>
        </div>
      </div>

      <!-- 右栏（合并两列宽）：出口卡片列表 -->
      <div class="col-span-2">
        <div class="list-header">
          <h2 class="panel-title">🛡️ 活跃出口容器管理</h2>
          <button @click="fetchEgressList" class="btn btn-icon" :disabled="loadingList">
            <span :class="['refresh-icon', { 'spinning': loadingList }]">🔄</span> 刷新状态
          </button>
        </div>

        <div v-if="egressList.length === 0" class="empty-state glass-panel">
          <span class="empty-emoji">🌐</span>
          <h3>暂无已创建的代理出口</h3>
          <p>请在左侧面板选择国家/地区，快速部署您的第一个 VLESS 出口。</p>
        </div>

        <div class="egress-grid" v-else>
          <div 
            v-for="egress in egressList" 
            :key="egress.name" 
            :class="['egress-card', 'glass-panel', `status-border-${egress.status}`]"
          >
            <!-- 卡片头部 -->
            <div class="card-head">
              <div class="card-title-group">
                <span class="card-flag">{{ getFlagEmoji(egress.region) }}</span>
                <div>
                  <h3 class="card-name">{{ egress.name }}</h3>
                  <span class="card-region-tag">{{ egress.region }} 出口</span>
                </div>
              </div>
              
              <span :class="['badge', `badge-${egress.status}`]">
                <span class="badge-dot"></span>
                {{ getStatusLabel(egress.status) }}
              </span>
            </div>

            <!-- 卡片信息区 -->
            <div class="card-info">
              <div class="info-row">
                <span class="info-label">入站地址:</span>
                <span class="info-value text-glow">{{ egress.port }} (VLESS+TCP)</span>
              </div>
              <div class="info-row">
                <span class="info-label">VPN 节点:</span>
                <span class="info-value font-mono truncate" :title="egress.nodeHostname">
                  {{ egress.nodeIp || '获取中...' }}
                </span>
              </div>
              <div class="info-row">
                <span class="info-label">真实出口 IP:</span>
                <span 
                  :class="[
                    'info-value', 'font-bold',
                    egress.currentEgressIp === 'error' || egress.currentEgressIp === 'offline' 
                      ? 'text-danger' 
                      : 'text-success'
                  ]"
                >
                  {{ egress.currentEgressIp || '检测中...' }}
                </span>
              </div>
              <div class="info-row">
                <span class="info-label">健康判定:</span>
                <span class="info-value font-mono text-secondary">
                  延迟 {{ egress.latency || 0 }}ms | 检测于 {{ new Date(egress.lastCheckTime).toLocaleTimeString() }}
                </span>
              </div>
            </div>

            <!-- 卡片操作区 -->
            <div class="card-actions">
              <button 
                @click="copyLink(egress.name)" 
                class="btn btn-success flex-1"
              >
                📋 复制 VLESS 订阅
              </button>
              
              <button 
                @click="rebuildEgress(egress.name)" 
                class="btn flex-1"
                title="保持入站配置不变，自动优选并替换底层 VPN 出口节点"
                :disabled="egress.status === 'starting'"
              >
                ⚡ 透明漂移
              </button>

              <button 
                @click="deleteEgress(egress.name)" 
                class="btn btn-danger btn-icon"
                title="删除下线"
              >
                🗑️
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部：VPNGate 高分节点监控预览 -->
    <section class="nodes-section glass-panel">
      <div class="section-head">
        <div>
          <h3>🌐 VPNGate 节点监控看板</h3>
          <p>当前拉取到的最优可用公共 VPN 节点预览，作为漂移池候选</p>
        </div>
        <button @click="fetchVpnNodes" class="btn" :disabled="loadingNodes">
          {{ loadingNodes ? '正在更新...' : '拉取最新数据' }}
        </button>
      </div>

      <div class="table-container">
        <table class="nodes-table">
          <thead>
            <tr>
              <th>国家地区</th>
              <th>IP / Hostname</th>
              <th>综合评分</th>
              <th>响应延迟</th>
              <th>活跃会话</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="node in vpnNodes" :key="node.ip + node.hostname">
              <td>
                <span class="table-flag">{{ getFlagEmoji(node.region) }}</span>
                {{ node.country }} ({{ node.region }})
              </td>
              <td class="font-mono">{{ node.ip }}</td>
              <td class="text-success font-bold">{{ node.score.toLocaleString() }}</td>
              <td>
                <span 
                  :class="[
                    'badge', 
                    node.ping < 100 ? 'badge-success' : node.ping < 250 ? 'badge-starting' : 'badge-error'
                  ]"
                >
                  {{ node.ping }} ms
                </span>
              </td>
              <td>👥 {{ node.sessions }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* 局部样式，与全局精美科技感融合 */

.toast {
  position: fixed;
  top: 24px;
  right: 24px;
  z-index: 1000;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 24px;
  border-radius: 12px;
  backdrop-filter: blur(20px);
  box-shadow: 0 10px 30px rgba(0,0,0,0.5);
  font-weight: 500;
  animation: toast-in 0.3s cubic-bezier(0.175, 0.885, 0.32, 1.275);
}

.toast-success {
  background: rgba(0, 230, 118, 0.15);
  border: 1px solid rgba(0, 230, 118, 0.3);
  color: var(--success-color);
}

.toast-error {
  background: rgba(255, 23, 68, 0.15);
  border: 1px solid rgba(255, 23, 68, 0.3);
  color: #ff5252;
}

@keyframes toast-in {
  from { transform: translateY(-20px) scale(0.9); opacity: 0; }
  to { transform: translateY(0) scale(1); opacity: 1; }
}

/* Transitions */
.toast-enter-active, .toast-leave-active {
  transition: all 0.3s;
}
.toast-enter-from, .toast-leave-to {
  opacity: 0;
  transform: translateY(-20px);
}

/* 页眉 */
.header-section {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 40px;
  gap: 20px;
}

@media (min-width: 768px) {
  .header-section {
    flex-direction: row;
    align-items: center;
  }
}

.header-logo {
  display: flex;
  align-items: center;
  gap: 16px;
}

.logo-emoji {
  font-size: 40px;
}

.header-logo h1 {
  font-size: 28px;
  font-weight: 800;
  background: linear-gradient(135deg, #fff 0%, #a5b4fc 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 4px;
}

.header-logo p {
  color: var(--text-secondary);
  font-size: 14px;
}

/* 汇总指标卡 */
.status-summary {
  display: flex;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 12px 24px;
  align-items: center;
}

.summary-item {
  text-align: center;
}

.summary-item .val {
  font-size: 24px;
  font-weight: 800;
  line-height: 1.2;
}

.summary-item .lbl {
  font-size: 11px;
  color: var(--text-secondary);
  text-transform: uppercase;
}

.summary-divider {
  width: 1px;
  height: 24px;
  background: var(--border-color);
  margin: 0 20px;
}

/* 表单与卡片 */
.col-span-1 { grid-column: span 1; }
.col-span-2 { grid-column: span 1; }

@media (min-width: 992px) {
  .col-span-2 { grid-column: span 2; }
}

.panel-title {
  font-size: 18px;
  font-weight: 700;
  margin-bottom: 6px;
}

.panel-subtitle {
  color: var(--text-secondary);
  font-size: 13px;
  margin-bottom: 24px;
}

.form-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.select-wrapper {
  position: relative;
}

.select-field {
  appearance: none;
  background-image: url("data:image/svg+xml;charset=UTF-8,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%238b9bb4' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3e%3cpolyline points='6 9 12 15 18 9'%3e%3c/polyline%3e%3c/svg%3e");
  background-repeat: no-repeat;
  background-position: right 16px center;
  background-size: 16px;
  padding-right: 40px;
}

.w-full { width: 100%; }

.tech-card {
  margin-top: 24px;
  background: rgba(88, 101, 242, 0.05);
  border: 1px dashed rgba(88, 101, 242, 0.2);
  border-radius: 12px;
  padding: 16px;
}

.tech-card h4 {
  font-size: 13px;
  color: #a5b4fc;
  margin-bottom: 8px;
}

.tech-card ul {
  padding-left: 16px;
  font-size: 12px;
  color: var(--text-secondary);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* 列表区域 */
.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.btn-icon {
  padding: 8px 12px;
}

.refresh-icon {
  font-size: 12px;
  transition: transform 0.5s ease;
}

.spinning {
  animation: rotate 1s linear infinite;
}

@keyframes rotate {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 40px;
  text-align: center;
}

.empty-emoji {
  font-size: 64px;
  margin-bottom: 20px;
  opacity: 0.7;
}

.empty-state h3 {
  font-size: 20px;
  margin-bottom: 8px;
}

.empty-state p {
  color: var(--text-secondary);
  font-size: 14px;
  max-width: 400px;
}

/* 出口网格与卡片 */
.egress-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 20px;
}

@media (min-width: 768px) {
  .egress-grid {
    grid-template-columns: 1fr 1fr;
  }
}

.egress-card {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  border-left-width: 4px;
}

.status-border-running { border-left-color: var(--success-color); }
.status-border-starting { border-left-color: var(--warning-color); }
.status-border-error { border-left-color: var(--danger-color); }
.status-border-offline { border-left-color: var(--text-secondary); }

.card-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
}

.card-title-group {
  display: flex;
  align-items: center;
  gap: 12px;
}

.card-flag {
  font-size: 28px;
  background: rgba(255,255,255,0.05);
  padding: 4px;
  border-radius: 8px;
}

.card-name {
  font-size: 16px;
  font-weight: 700;
}

.card-region-tag {
  font-size: 11px;
  color: var(--text-secondary);
}

.badge-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.badge-success .badge-dot {
  box-shadow: 0 0 8px var(--success-color);
  animation: pulse-glow 1.5s infinite alternate;
}

@keyframes pulse-glow {
  from { opacity: 0.5; }
  to { opacity: 1; }
}

.card-info {
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: rgba(0, 0, 0, 0.15);
  border-radius: 8px;
  padding: 12px;
  margin-bottom: 20px;
}

.info-row {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
}

.info-label {
  color: var(--text-secondary);
}

.info-value {
  color: var(--text-primary);
  max-width: 160px;
}

.truncate {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.text-glow {
  color: #a5b4fc;
  font-weight: 600;
}

.font-mono {
  font-family: monospace;
}

.card-actions {
  display: flex;
  gap: 8px;
}

.flex-1 { flex: 1; }

/* 底部监控表格 */
.nodes-section {
  margin-top: 40px;
}

.section-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.section-head h3 {
  font-size: 18px;
  font-weight: 700;
}

.section-head p {
  color: var(--text-secondary);
  font-size: 12px;
}

.table-container {
  overflow-x: auto;
  border-radius: 8px;
  border: 1px solid var(--border-color);
}

.nodes-table {
  width: 100%;
  border-collapse: collapse;
  text-align: left;
  font-size: 13px;
}

.nodes-table th, .nodes-table td {
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-color);
}

.nodes-table th {
  background: rgba(255, 255, 255, 0.02);
  color: var(--text-secondary);
  font-weight: 600;
}

.nodes-table tr:last-child td {
  border-bottom: none;
}

.nodes-table tr:hover td {
  background: rgba(255, 255, 255, 0.01);
}

.table-flag {
  font-size: 16px;
  margin-right: 6px;
}

.font-bold { font-weight: 700; }

/* 旋转加载动画 */
.spinner {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255,255,255,0.3);
  border-radius: 50%;
  border-top-color: white;
  animation: spin 1s linear infinite;
  display: inline-block;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
