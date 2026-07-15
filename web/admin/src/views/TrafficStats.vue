<template>
  <div class="traffic-page">
    <div class="traffic-grid">
      <el-card class="animate-in">
        <template #header>今日流量</template>
        <div class="kv-row">
          <span class="kv-label">入站流量</span>
          <span class="kv-value text-mono">{{ formatBytes(stats.today?.traffic_in || 0) }}</span>
        </div>
        <div class="kv-row">
          <span class="kv-label">出站流量</span>
          <span class="kv-value text-mono">{{ formatBytes(stats.today?.traffic_out || 0) }}</span>
        </div>
        <div class="divider"></div>
        <div class="kv-row" style="font-weight: var(--font-semibold)">
          <span class="kv-label" style="font-weight: var(--font-semibold)">总流量</span>
          <span class="kv-value text-mono">{{ formatBytes(stats.today?.total || 0) }}</span>
        </div>
      </el-card>

      <el-card class="animate-in animate-in-delay-1">
        <template #header>本月流量</template>
        <div class="kv-row">
          <span class="kv-label">入站流量</span>
          <span class="kv-value text-mono">{{ formatBytes(stats.monthly?.traffic_in || 0) }}</span>
        </div>
        <div class="kv-row">
          <span class="kv-label">出站流量</span>
          <span class="kv-value text-mono">{{ formatBytes(stats.monthly?.traffic_out || 0) }}</span>
        </div>
        <div class="divider"></div>
        <div class="kv-row" style="font-weight: var(--font-semibold)">
          <span class="kv-label" style="font-weight: var(--font-semibold)">总流量</span>
          <span class="kv-value text-mono">{{ formatBytes(stats.monthly?.total || 0) }}</span>
        </div>
      </el-card>

      <el-card class="animate-in animate-in-delay-2">
        <template #header>流量排行</template>
        <div v-if="stats.top_users?.length">
          <div v-for="(user, idx) in stats.top_users.slice(0, 5)" :key="user.user_id" class="rank-item">
            <span class="rank-num" :class="{ 'rank-num--top': idx < 3 }">{{ idx + 1 }}</span>
            <span class="rank-name">{{ user.email }}</span>
            <span class="rank-value text-mono">{{ formatBytes(user.traffic_in + user.traffic_out) }}</span>
          </div>
        </div>
        <el-empty v-else description="暂无数据" :image-size="48" />
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getTrafficStats } from '../api'
import { ElMessage } from 'element-plus'

const stats = ref<any>({})

function formatBytes(bytes: number) {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

onMounted(async () => {
  try {
    const res = await getTrafficStats()
    stats.value = res.data
  } catch (e: any) {
    ElMessage.error(e?.message || '加载流量数据失败')
  }
})
</script>

<style scoped>
.traffic-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-4);
}

.rank-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) 0;
}

.rank-num {
  width: 22px;
  height: 22px;
  background: var(--color-bg);
  border-radius: var(--radius-full);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--text-xs);
  font-weight: var(--font-semibold);
  color: var(--color-text-secondary);
  flex-shrink: 0;
}

.rank-num--top {
  background: var(--color-primary-light);
  color: var(--color-primary);
}

.rank-name {
  flex: 1;
  font-size: var(--text-sm);
  color: var(--color-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rank-value {
  font-size: var(--text-sm);
  color: var(--color-text-secondary);
  flex-shrink: 0;
}
</style>
