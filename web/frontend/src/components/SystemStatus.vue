<script setup>
import { computed } from 'vue';

// 简体中文注释：接收父组件传入的系统状态与加载状态
const props = defineProps({
  systemStatus: {
    type: Object,
    default: null
  },
  loadingSystemStatus: {
    type: Boolean,
    default: false
  }
});

const emit = defineEmits(['refresh']);

// 简体中文注释：计算系统卡片展示项
const systemCards = computed(() => {
  const stats = props.systemStatus?.jobStatusCounts || {};
  const totalJobs = Object.values(stats).reduce((a, b) => a + b, 0);
  return [
    { label: '版本', value: props.systemStatus?.version || '-' },
    { label: '认证', value: props.systemStatus?.authEnabled ? '已启用' : '已关闭' },
    { label: '出口', value: props.systemStatus?.egressCount ?? '-' },
    { label: '任务', value: props.systemStatus ? totalJobs : '-' }
  ];
});
</script>

<template>
  <div class="section-head">
    <div>
      <h2>系统状态</h2>
      <p class="text-secondary">面板、任务队列和健康检查</p>
    </div>
    <button class="btn btn-sm" @click="emit('refresh')" :disabled="loadingSystemStatus">
      {{ loadingSystemStatus ? '...' : '刷新' }}
    </button>
  </div>
  <div class="status-grid">
    <div v-for="card in systemCards" :key="card.label" class="status-card">
      <span class="status-label">{{ card.label }}</span>
      <span class="status-value">{{ card.value }}</span>
    </div>
  </div>
  <div class="status-note" v-if="systemStatus">
    <span>健康检查</span>
    <strong :style="{ color: systemStatus.ok ? '#00e676' : '#ff5252' }">
      {{ systemStatus.ok ? '正常' : '异常' }}
    </strong>
    <span>最近任务</span>
    <strong>{{ systemStatus.latestJob ? `#${systemStatus.latestJob.id}` : '-' }}</strong>
  </div>
</template>
