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

// 简体中文注释：计算系统卡片展示项并关联图标
const systemCards = computed(() => {
  const stats = props.systemStatus?.jobStatusCounts || {};
  const totalJobs = Object.values(stats).reduce((a, b) => a + b, 0);
  return [
    { label: '系统版本', value: props.systemStatus?.version || '-', icon: '⚡' },
    { label: '安全认证', value: props.systemStatus?.authEnabled ? '已启用' : '未开启', icon: '🛡️' },
    { label: '活跃出口', value: props.systemStatus?.egressCount ?? '-', icon: '🌐' },
    { label: '任务总数', value: props.systemStatus ? totalJobs : '-', icon: '📊' }
  ];
});
</script>

<template>
  <div class="section-head">
    <div>
      <h2>系统状态</h2>
      <p class="text-secondary">实时控制台及任务队列诊断</p>
    </div>
    <button class="btn btn-sm refresh-btn" @click="emit('refresh')" :disabled="loadingSystemStatus">
      <span v-if="loadingSystemStatus" class="spinner"></span>
      <span v-else>刷新</span>
    </button>
  </div>
  <div class="status-grid">
    <div v-for="card in systemCards" :key="card.label" class="status-card-neon">
      <div class="status-card-header">
        <span class="status-icon">{{ card.icon }}</span>
        <span class="status-label">{{ card.label }}</span>
      </div>
      <span class="status-value">{{ card.value }}</span>
    </div>
  </div>
  <div class="status-note-neon" v-if="systemStatus">
    <div class="note-item">
      <span class="note-label">健康状态</span>
      <span class="note-value">
        <span :class="['pulse-dot', systemStatus.ok ? 'pulse-dot-success' : 'pulse-dot-danger']"></span>
        <span :style="{ color: systemStatus.ok ? '#10b981' : '#ef4444', fontWeight: 'bold' }">
          {{ systemStatus.ok ? '正常' : '故障' }}
        </span>
      </span>
    </div>
    <div class="note-item">
      <span class="note-label">最新任务</span>
      <span class="note-value mono highlight">
        {{ systemStatus.latestJob ? `#${systemStatus.latestJob.id}` : '无' }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.refresh-btn {
  background: rgba(99, 102, 241, 0.08);
  border: 1px solid rgba(99, 102, 241, 0.2);
  color: #a5b4fc;
}
.refresh-btn:hover {
  background: var(--accent-gradient);
  border-color: transparent;
  color: white;
}
.status-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-bottom: 16px;
}
.status-card-neon {
  background: rgba(255, 255, 255, 0.015);
  border: 1px solid rgba(255, 255, 255, 0.04);
  border-radius: 14px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  transition: var(--transition-smooth);
}
.status-card-neon:hover {
  border-color: rgba(99, 102, 241, 0.2);
  background: rgba(99, 102, 241, 0.02);
  transform: translateY(-1px);
}
.status-card-header {
  display: flex;
  align-items: center;
  gap: 6px;
}
.status-icon {
  font-size: 13px;
  opacity: 0.8;
}
.status-label {
  font-size: 11px;
  color: var(--text-secondary);
  font-weight: 500;
}
.status-value {
  font-size: 16px;
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.5px;
}
.status-note-neon {
  background: rgba(5, 5, 12, 0.35);
  border: 1px solid rgba(255, 255, 255, 0.04);
  border-radius: 14px;
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.note-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
}
.note-label {
  color: var(--text-secondary);
}
.note-value {
  display: flex;
  align-items: center;
  gap: 8px;
}
.highlight {
  color: #a855f7;
  font-weight: 600;
}
</style>
