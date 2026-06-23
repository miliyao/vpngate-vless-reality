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
    <h2>创建出口</h2>
    <p class="text-secondary mb-16">自动获取 VPNGate 最优节点</p>
    <form @submit.prevent="submitSingle" class="form">
      <label class="label">名称</label>
      <input v-model="formName" type="text" class="input" placeholder="如 jp-01" required :disabled="creatingEgress" />
      <label class="label">地区</label>
      <select v-model="formRegion" class="input" :disabled="creatingEgress || loadingRegions">
        <option v-if="loadingRegions" value="">加载中...</option>
        <option v-for="r in vpnRegions" :key="r.code" :value="r.code">
          {{ getFlagEmoji(r.code) }} {{ regionName(r.code, r.name) }} ({{ r.count }})
        </option>
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
      <button class="btn btn-primary w-full" @click="submitCreateAll" :disabled="creatingAll">
        <span v-if="creatingAll" class="spinner"></span>
        {{ creatingAll ? '正在创建...' : '一键创建全部地区' }}
      </button>
      <button class="btn w-full" @click="emit('open-sub')">查看/复制订阅</button>
      <button class="btn w-full" @click="handleRebuildAll" :disabled="egressCount === 0">全部漂移</button>
      <button class="btn btn-danger w-full" @click="handleDeleteAll" :disabled="egressCount === 0">删除全部</button>
    </div>
  </div>
</template>
