<script setup>
import { ref, watch } from 'vue';

// 简体中文注释：定义接收属性与触发事件
const props = defineProps({
  vpnRegions: { type: Array, default: () => [] },
  loadingRegions: { type: Boolean, default: false },
  creatingEgress: { type: Boolean, default: false },
  creatingAll: { type: Boolean, default: false },
  egressCount: { type: Number, default: 0 }
});

const emit = defineEmits(['create-single', 'create-all', 'rebuild-all', 'delete-all', 'open-sub']);

const formName = ref('');
const formRegion = ref('');
const formUuid = ref('');

// 简体中文注释：监听 vpnRegions 加载，并设置默认地区
watch(() => props.vpnRegions, (newVal) => {
  if (newVal && newVal.length > 0 && !formRegion.value) {
    formRegion.value = newVal[0].code;
  }
}, { immediate: true });

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

function submitSingle() {
  if (!formName.value || !formRegion.value) return;
  emit('create-single', {
    name: formName.value.trim(),
    region: formRegion.value,
    uuid: formUuid.value.trim() || undefined
  });
  formName.value = '';
  formUuid.value = '';
}

function submitCreateAll() {
  if (!confirm('将为所有 VPNGate 可用地区各创建一个出口，确认？')) return;
  emit('create-all', { uuid: formUuid.value.trim() || undefined });
}

function handleRebuildAll() {
  if (!confirm('将对所有出口执行透明漂移，确认？')) return;
  emit('rebuild-all');
}

function handleDeleteAll() {
  if (!confirm('确定删除全部出口？此操作不可撤销！')) return;
  emit('delete-all');
}
</script>

<template>
  <div>
    <h2>部署出口</h2>
    <p class="text-secondary mb-16">动态连通最优 VPNGate 实效节点</p>
    <form @submit.prevent="submitSingle" class="form">
      <div class="input-group">
        <label class="label">出口代号</label>
        <input v-model="formName" type="text" class="input" placeholder="例如：jp-01" required :disabled="creatingEgress" />
      </div>
      
      <div class="input-group">
        <label class="label">目标区域</label>
        <div class="select-wrapper">
          <select v-model="formRegion" class="input select-input" :disabled="creatingEgress || loadingRegions">
            <option v-if="loadingRegions" value="">正在检索可用节点...</option>
            <option v-for="r in vpnRegions" :key="r.code" :value="r.code">
              {{ getFlagEmoji(r.code) }} {{ regionName(r.code, r.name) }} ({{ r.count }} 节点)
            </option>
          </select>
        </div>
      </div>

      <div class="input-group">
        <label class="label">UUID 密钥 <span class="label-hint">(可选，留空将自动生成)</span></label>
        <input v-model="formUuid" type="text" class="input" placeholder="保持默认自动随机生成" :disabled="creatingEgress" />
      </div>

      <button type="submit" class="btn btn-primary w-full submit-btn" :disabled="creatingEgress">
        <span v-if="creatingEgress" class="spinner"></span>
        <span v-else>💡 快速构建单个出口</span>
      </button>
    </form>

    <div class="divider"></div>
    <h2>批量运维</h2>
    <p class="text-secondary mb-16">全局管理当前网络出口集群</p>
    <div class="batch-btns">
      <button class="btn btn-primary w-full pulse-button" @click="submitCreateAll" :disabled="creatingAll">
        <span v-if="creatingAll" class="spinner"></span>
        <span v-else>⚡ 一键部署全可用地区</span>
      </button>
      <button class="btn btn-secondary w-full" @click="emit('open-sub')">📂 导出节点订阅配置</button>
      <button class="btn btn-rebuild w-full" @click="handleRebuildAll" :disabled="egressCount === 0">🔄 全网出口透明漂移</button>
      <button class="btn btn-danger w-full" @click="handleDeleteAll" :disabled="egressCount === 0">🚨 彻底清空出口节点</button>
    </div>
  </div>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.input-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.label {
  font-size: 11px;
  font-weight: 700;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.label-hint {
  font-weight: normal;
  text-transform: none;
  color: var(--text-muted);
}
.select-wrapper {
  position: relative;
  width: 100%;
}
.select-input {
  appearance: none;
  -webkit-appearance: none;
  cursor: pointer;
  padding-right: 36px !important;
}
.select-wrapper::after {
  content: '▼';
  font-size: 8px;
  color: var(--text-secondary);
  position: absolute;
  right: 14px;
  top: 50%;
  transform: translateY(-50%);
  pointer-events: none;
}
.submit-btn {
  font-size: 14px;
  padding: 12px;
  margin-top: 4px;
}
.pulse-button {
  background: linear-gradient(135deg, #a855f7 0%, #ec4899 100%);
  box-shadow: 0 4px 14px 0 rgba(168, 85, 247, 0.4);
}
.pulse-button:hover {
  background: linear-gradient(135deg, #a855f7 0%, #ec4899 100%);
  box-shadow: 0 6px 20px 0 rgba(236, 72, 153, 0.6);
}
.btn-secondary {
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.08);
}
.btn-secondary:hover {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(99, 102, 241, 0.3);
}
.btn-rebuild {
  background: rgba(245, 158, 11, 0.08);
  border-color: rgba(245, 158, 11, 0.2);
  color: #fbbf24;
}
.btn-rebuild:hover {
  background: var(--warning-gradient);
  border-color: transparent;
  color: white;
  box-shadow: 0 4px 14px 0 rgba(245, 158, 11, 0.35);
}
.batch-btns {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
</style>
