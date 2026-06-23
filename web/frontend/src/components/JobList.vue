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
        <p class="text-secondary">最近 20 条任务</p>
      </div>
      <button class="btn btn-sm" @click="emit('refresh')" :disabled="loadingJobs">
        {{ loadingJobs ? '...' : '刷新任务' }}
      </button>
    </div>
    <div class="jobs-list">
      <div v-for="job in jobs" :key="job.id" class="job-row" @click="emit('open-detail', job)">
        <div class="job-main">
          <span :class="['job-pill', jobBadge(job)]">{{ jobLabel(job) }}</span>
          <span class="job-type">#{{ job.id }} {{ job.type }} / {{ job.target || '-' }}</span>
          <span class="job-time">{{ timeAgo(job.updatedAt) }}</span>
        </div>
        <div class="job-error" v-if="job.error">{{ job.error }}</div>
      </div>
      <div v-if="jobs.length === 0" class="text-secondary" style="padding:12px 0">暂无任务</div>
    </div>
  </section>
</template>
