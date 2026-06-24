<script setup>
import { computed } from 'vue';

// 简体中文注释：单个出口卡片组件
const props = defineProps({
  eg: { type: Object, required: true },
  vpsAddress: { type: String, default: '' }
});

const emit = defineEmits(['open-link', 'rebuild', 'view-logs', 'delete']);

const REGION_CN = {
  JP: '日本', KR: '韩国', TH: '泰国', RU: '俄罗斯', RO: '罗马尼亚', VN: '越南', US: '美国',
  HR: '克罗地亚', CN: '中国', HK: '中国香港', TW: '中国台湾', SG: '新加坡', DE: '德国',
  FR: '法国', GB: '英国', CA: '加拿大', AU: '澳大利亚', NL: '荷兰', PL: '波兰', BR: '巴西',
  IN: '印度', ID: '印度尼西亚', MY: '马来西亚', PH: '菲律宾'
};

function regionName(code, fallback = '') {
  if (!code) return fallback || '-';
  return REGION_CN[String(code).toUpperCase()] || fallback || String(code).toUpperCase();
}

function getFlagEmoji(c) {
  if (!c) return '';
  try { return String.fromCodePoint(...c.toUpperCase().split('').map(x => 127397 + x.charCodeAt(0))); } catch { return c; }
}

function getStatusLabel(s) {
  return { running: '运行中', starting: '启动中', rebuilding: '漂移中', error: '异常', offline: '离线' }[s] || s || '未知';
}

function getStatusClass(s) {
  return { running: 'success', starting: 'warning', rebuilding: 'warning', error: 'danger', offline: 'muted' }[s] || 'muted';
}

// 简体中文注释：根据中转延迟动态计算颜色值
const latencyColor = computed(() => {
  const lat = props.eg.latency;
  if (!lat) return 'var(--text-muted)';
  if (lat < 100) return '#34d399';
  if (lat < 300) return '#fbbf24';
  return '#f87171';
});

function timeAgo(ts) {
  if (!ts) return '-';
  const s = Math.floor((Date.now() - ts) / 1000);
  if (s < 60) return s + '秒前';
  if (s < 3600) return Math.floor(s / 60) + '分钟前';
  return Math.floor(s / 3600) + '小时前';
}
</script>

<template>
  <div :class="['eg-card', 'glass-panel', `glow-${getStatusClass(eg.status)}`]">
    <div class="eg-top">
      <div class="eg-identity">
        <span class="eg-flag">{{ getFlagEmoji(eg.region) }}</span>
        <div>
          <div class="eg-name">{{ eg.name }}</div>
          <div class="text-secondary region-text">{{ regionName(eg.region) }} · {{ eg.region }}</div>
        </div>
      </div>
      <div class="status-indicator">
        <span :class="['pulse-dot', `pulse-dot-${getStatusClass(eg.status)}`]"></span>
        <span :class="['status-label-text', `color-${getStatusClass(eg.status)}`]">{{ getStatusLabel(eg.status) }}</span>
      </div>
    </div>
    <div class="eg-conn">
      <div class="conn-row"><span class="conn-label">面板订阅端口</span><span class="conn-val mono highlight-port">{{ vpsAddress }}:{{ eg.port }}</span></div>
      <div class="conn-row"><span class="conn-label">当前上游 VPN IP</span><span class="conn-val mono">{{ eg.nodeIp || '未分配' }}</span></div>
      <div class="conn-row">
        <span class="conn-label">出口外网 IP</span>
        <span :class="['conn-val', 'mono', eg.currentEgressIp && eg.currentEgressIp !== 'error' && eg.currentEgressIp !== 'offline' ? 'text-success' : 'text-danger']">
          {{ eg.currentEgressIp || '检测中...' }}
        </span>
      </div>
      <div class="conn-row">
        <span class="conn-label">中转延迟</span>
        <span class="conn-val mono" :style="{ color: latencyColor, fontWeight: 'bold' }">
          {{ eg.latency ? `${eg.latency}ms` : '测速中...' }}
        </span>
      </div>
      <div class="conn-row"><span class="conn-label">最近状态同步</span><span class="conn-val">{{ timeAgo(eg.lastCheckTime) }}</span></div>
    </div>
    <div v-if="eg.error" class="eg-error">{{ eg.error }}</div>
    <div class="eg-actions">
      <button class="btn btn-success btn-sm flex1" @click="emit('open-link', eg.name)">🔑 提取链接</button>
      <button class="btn btn-sm btn-rebuild flex1" @click="emit('rebuild', eg.name)" :disabled="eg.status === 'starting' || eg.status === 'rebuilding'">
        <span v-if="eg.status === 'rebuilding'" class="spinner"></span>
        <span v-else>🔄 故障漂移</span>
      </button>
      <button class="btn btn-sm btn-secondary ico" @click="emit('view-logs', eg.name)" title="查看日志">LOG</button>
      <button class="btn btn-danger btn-sm ico" @click="emit('delete', eg.name)" title="删除出口">DEL</button>
    </div>
  </div>
</template>

<style scoped>
.eg-card {
  border-top: 4px solid var(--border-color);
  display: flex;
  flex-direction: column;
  gap: 16px;
  background: rgba(15, 23, 42, 0.35);
}
.glow-success {
  border-top-color: var(--success-color);
}
.glow-success:hover {
  box-shadow: 0 16px 36px -12px rgba(16, 185, 129, 0.15), 0 0 24px 0 rgba(16, 185, 129, 0.05);
  border-color: rgba(16, 185, 129, 0.3);
}
.glow-warning {
  border-top-color: var(--warning-color);
}
.glow-warning:hover {
  box-shadow: 0 16px 36px -12px rgba(245, 158, 11, 0.15), 0 0 24px 0 rgba(245, 158, 11, 0.05);
  border-color: rgba(245, 158, 11, 0.3);
}
.glow-danger {
  border-top-color: var(--danger-color);
}
.glow-danger:hover {
  box-shadow: 0 16px 36px -12px rgba(239, 68, 68, 0.15), 0 0 24px 0 rgba(239, 68, 68, 0.05);
  border-color: rgba(239, 68, 68, 0.3);
}
.glow-muted {
  border-top-color: var(--text-muted);
}
.eg-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.eg-identity {
  display: flex;
  align-items: center;
  gap: 12px;
}
.eg-flag {
  font-size: 28px;
  filter: drop-shadow(0 4px 8px rgba(0,0,0,0.25));
}
.eg-name {
  font-size: 16px;
  font-weight: 800;
  color: var(--text-primary);
  line-height: 1.2;
}
.region-text {
  font-size: 11px;
  margin-top: 2px;
}
.status-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(5, 5, 12, 0.45);
  padding: 4px 12px;
  border-radius: 99px;
  border: 1px solid rgba(255, 255, 255, 0.03);
}
.status-label-text {
  font-size: 11px;
  font-weight: 700;
}
.color-success { color: #34d399; }
.color-warning { color: #fbbf24; }
.color-danger { color: #f87171; }
.color-muted { color: var(--text-secondary); }

.eg-conn {
  background: rgba(5, 5, 12, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.03);
  border-radius: 12px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.conn-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
}
.conn-label {
  color: var(--text-secondary);
}
.conn-val {
  font-weight: 600;
  color: var(--text-primary);
}
.highlight-port {
  color: #a5b4fc;
}
.eg-error {
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.2);
  border-radius: 10px;
  padding: 8px 12px;
  font-size: 12px;
  color: #f87171;
  word-break: break-all;
  line-height: 1.5;
}
.eg-actions {
  display: flex;
  gap: 8px;
}
.flex1 {
  flex: 1;
}
.btn-rebuild {
  background: rgba(99, 102, 241, 0.08);
  border-color: rgba(99, 102, 241, 0.2);
  color: #a5b4fc;
}
.btn-rebuild:hover {
  background: rgba(99, 102, 241, 0.18);
  border-color: rgba(99, 102, 241, 0.4);
}
.ico {
  min-width: 44px;
  font-size: 11px;
}
</style>
