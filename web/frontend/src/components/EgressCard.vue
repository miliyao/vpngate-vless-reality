<script setup>
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

function timeAgo(ts) {
  if (!ts) return '-';
  const s = Math.floor((Date.now() - ts) / 1000);
  if (s < 60) return s + '秒前';
  if (s < 3600) return Math.floor(s / 60) + '分钟前';
  return Math.floor(s / 3600) + '小时前';
}
</script>

<template>
  <div :class="['eg-card', 'glass-panel', `border-${getStatusClass(eg.status)}`]">
    <div class="eg-top">
      <div class="eg-identity">
        <span class="eg-flag">{{ getFlagEmoji(eg.region) }}</span>
        <div>
          <div class="eg-name">{{ eg.name }}</div>
          <div class="text-secondary">{{ regionName(eg.region) }} / {{ eg.region }}</div>
        </div>
      </div>
      <span :class="['pill', `pill-${getStatusClass(eg.status)}`]">{{ getStatusLabel(eg.status) }}</span>
    </div>
    <div class="eg-conn">
      <div class="conn-row"><span class="conn-label">入口</span><span class="conn-val mono">{{ vpsAddress }}:{{ eg.port }}</span></div>
      <div class="conn-row"><span class="conn-label">VPN</span><span class="conn-val mono">{{ eg.nodeIp || '...' }}</span></div>
      <div class="conn-row">
        <span class="conn-label">出口IP</span>
        <span :class="['conn-val', 'mono', eg.currentEgressIp && eg.currentEgressIp !== 'error' && eg.currentEgressIp !== 'offline' ? 'text-success' : 'text-danger']">
          {{ eg.currentEgressIp || '...' }}
        </span>
      </div>
      <div class="conn-row"><span class="conn-label">延迟</span><span class="conn-val">{{ eg.latency || '-' }}ms</span></div>
      <div class="conn-row"><span class="conn-label">检测</span><span class="conn-val">{{ timeAgo(eg.lastCheckTime) }}</span></div>
    </div>
    <div v-if="eg.error" class="eg-error">{{ eg.error }}</div>
    <div class="eg-actions">
      <button class="btn btn-success btn-sm flex1" @click="emit('open-link', eg.name)">订阅</button>
      <button class="btn btn-sm flex1" @click="emit('rebuild', eg.name)" :disabled="eg.status === 'starting'">漂移</button>
      <button class="btn btn-sm ico" @click="emit('view-logs', eg.name)">LOG</button>
      <button class="btn btn-danger btn-sm ico" @click="emit('delete', eg.name)">DEL</button>
    </div>
  </div>
</template>
