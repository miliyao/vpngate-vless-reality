<script setup>
// 简体中文注释：后台 Job 任务监视器组件
const props = defineProps({
  jobs: { type: Array, default: () => [] },
  loadingJobs: { type: Boolean, default: false }
});

const emit = defineEmits(['refresh', 'open-detail']);

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

function getStatusClass(s) {
  return { done: 'success', running: 'accent', queued: 'warning', failed: 'danger' }[s] || 'muted';
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
  <section class="jobs-section glass-panel">
    <div class="list-head">
      <div>
        <h2>任务队列</h2>
        <p class="text-secondary">实时操作与自愈任务审计日志</p>
      </div>
      <button class="btn btn-sm refresh-jobs-btn" @click="emit('refresh')" :disabled="loadingJobs">
        <span v-if="loadingJobs" class="spinner"></span>
        <span v-else>🔄 刷新队列</span>
      </button>
    </div>
    <div class="jobs-list">
      <div v-for="job in jobs" :key="job.id" :class="['job-row-neon', `job-status-${getStatusClass(job.status)}`]" @click="emit('open-detail', job)">
        <div class="job-main">
          <span :class="['job-pill', jobBadge(job)]">
            <span :class="['pulse-dot', `pulse-dot-${getStatusClass(job.status)}` ]" style="margin-right:2px"></span>
            {{ jobLabel(job) }}
          </span>
          <span class="job-type">
            <span class="job-id">#{{ job.id }}</span>
            <span class="job-name">{{ job.type }} / {{ job.target || '全局' }}</span>
          </span>
          <span class="job-time">{{ timeAgo(job.updatedAt) }}</span>
        </div>
        <div class="job-error" v-if="job.error">⚠️ {{ job.error }}</div>
      </div>
      <div v-if="jobs.length === 0" class="empty-jobs text-secondary">暂无操作任务</div>
    </div>
  </section>
</template>

<style scoped>
.jobs-section {
  background: rgba(15, 23, 42, 0.25);
  margin-top: 24px;
}
.refresh-jobs-btn {
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.08);
}
.refresh-jobs-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(99, 102, 241, 0.3);
}
.jobs-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.job-row-neon {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 16px;
  background: rgba(5, 5, 12, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.03);
  border-left: 4px solid var(--border-color);
  border-radius: 12px;
  cursor: pointer;
  transition: var(--transition-smooth);
}
.job-row-neon:hover {
  background: rgba(255, 255, 255, 0.02);
  transform: translateX(3px);
}

.job-status-success { border-left-color: var(--success-color); }
.job-status-success:hover { border-color: rgba(16, 185, 129, 0.2) rgba(16, 185, 129, 0.2) rgba(16, 185, 129, 0.2) var(--success-color); }

.job-status-accent { border-left-color: var(--accent-color); }
.job-status-accent:hover { border-color: rgba(99, 102, 241, 0.2) rgba(99, 102, 241, 0.2) rgba(99, 102, 241, 0.2) var(--accent-color); }

.job-status-warning { border-left-color: var(--warning-color); }
.job-status-warning:hover { border-color: rgba(245, 158, 11, 0.2) rgba(245, 158, 11, 0.2) rgba(245, 158, 11, 0.2) var(--warning-color); }

.job-status-danger { border-left-color: var(--danger-color); }
.job-status-danger:hover { border-color: rgba(239, 68, 68, 0.2) rgba(239, 68, 68, 0.2) rgba(239, 68, 68, 0.2) var(--danger-color); }

.pulse-dot-accent { background-color: var(--accent-color); }
.pulse-dot-accent::after {
  content: '';
  position: absolute;
  inset: -4px;
  border-radius: 50%;
  background-color: var(--accent-color);
  opacity: 0.4;
  animation: pulse-ring 2s cubic-bezier(0.215, 0.610, 0.355, 1) infinite;
}

.job-main {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.job-pill {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 99px;
  font-size: 11px;
  font-weight: 700;
  gap: 6px;
}
.job-queued { background: rgba(245, 158, 11, 0.1); color: #fbbf24; border: 1px solid rgba(245, 158, 11, 0.15); }
.job-running { background: rgba(99, 102, 241, 0.1); color: #a5b4fc; border: 1px solid rgba(99, 102, 241, 0.15); }
.job-done { background: rgba(16, 185, 129, 0.1); color: #34d399; border: 1px solid rgba(16, 185, 129, 0.15); }
.job-failed { background: rgba(239, 68, 68, 0.1); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.15); }

.job-type {
  font-size: 13px;
  color: var(--text-primary);
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.job-id {
  color: var(--text-muted);
  font-weight: 700;
  font-family: monospace;
}
.job-name {
  font-weight: 600;
}
.job-time {
  margin-left: auto;
  font-size: 11px;
  color: var(--text-secondary);
}
.job-error {
  font-size: 12px;
  color: #f87171;
  word-break: break-all;
  background: rgba(239, 68, 68, 0.05);
  border: 1px dashed rgba(239, 68, 68, 0.2);
  border-radius: 6px;
  padding: 6px 10px;
}
.empty-jobs {
  text-align: center;
  padding: 24px 0;
  font-size: 13px;
}
</style>
