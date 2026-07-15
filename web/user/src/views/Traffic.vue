<template>
  <div class="traffic-page">
    <div class="traffic-grid">
      <el-card class="animate-in">
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
        <el-progress
          v-if="stats.plan?.max_traffic"
          :percentage="usagePercent"
          :color="usagePercent > 80 ? '#cf222e' : '#2563eb'"
          style="margin-top: 12px"
        />
      </el-card>

      <el-card class="animate-in animate-in-delay-1">
        <template #header>套餐信息</template>
        <div v-if="stats.plan">
          <div class="plan-name-row">
            <span class="plan-name-tag">{{ stats.plan.name }}</span>
          </div>
          <div class="divider"></div>
          <div class="kv-row">
            <span class="kv-label">代理数量</span>
            <span class="kv-value text-mono">{{ stats.plan.max_proxies }} 个</span>
          </div>
          <div class="kv-row">
            <span class="kv-label">带宽限制</span>
            <span class="kv-value text-mono">{{ formatBytes(stats.plan.max_bandwidth) }}/s</span>
          </div>
          <div class="kv-row">
            <span class="kv-label">流量限额</span>
            <span class="kv-value text-mono">{{ formatBytes(stats.plan.max_traffic) }}</span>
          </div>
          <div class="kv-row">
            <span class="kv-label">端口数量</span>
            <span class="kv-value text-mono">{{ stats.plan.max_ports }} 个</span>
          </div>
          <div class="divider"></div>
          <div class="kv-row">
            <span class="kv-label">到期时间</span>
            <span class="kv-value" :class="{ 'text-danger': isExpired }">{{ formatDate(stats.plan.expires_at) }}</span>
          </div>
        </div>
        <div v-else style="text-align: center; padding: 20px 0">
          <el-text type="info">暂未购买套餐</el-text>
        </div>
      </el-card>
    </div>

    <el-card style="margin-top: var(--space-4)" class="animate-in animate-in-delay-2">
      <template #header>代理流量明细</template>
      <el-table :data="stats.per_proxy || []" stripe>
        <el-table-column prop="proxy_name" label="代理名称" min-width="150" show-overflow-tooltip />
        <el-table-column label="入站" width="120">
          <template #default="{ row }"><span class="text-mono">{{ formatBytes(row.traffic_in) }}</span></template>
        </el-table-column>
        <el-table-column label="出站" width="120">
          <template #default="{ row }"><span class="text-mono">{{ formatBytes(row.traffic_out) }}</span></template>
        </el-table-column>
        <el-table-column label="合计" width="120">
          <template #default="{ row }"><span class="text-mono">{{ formatBytes(row.traffic_in + row.traffic_out) }}</span></template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!stats.per_proxy?.length" description="暂无代理" :image-size="48" />
    </el-card>

    <el-card style="margin-top: var(--space-4)" class="animate-in animate-in-delay-3">
      <template #header>流量日志</template>
      <el-table :data="logs" stripe>
        <el-table-column prop="date" label="日期" width="120" />
        <el-table-column label="入站" width="120">
          <template #default="{ row }"><span class="text-mono">{{ formatBytes(row.traffic_in) }}</span></template>
        </el-table-column>
        <el-table-column label="出站" width="120">
          <template #default="{ row }"><span class="text-mono">{{ formatBytes(row.traffic_out) }}</span></template>
        </el-table-column>
        <el-table-column label="合计">
          <template #default="{ row }"><span class="text-mono">{{ formatBytes(row.traffic_in + row.traffic_out) }}</span></template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-model:current-page="page"
        :page-size="20"
        :total="total"
        layout="prev, pager, next"
        @current-change="loadLogs"
      />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { getTrafficStats, getTrafficLogs } from '../api'

const stats = ref<any>({})
const logs = ref<any[]>([])
const page = ref(1)
const total = ref(0)

const usagePercent = computed(() => {
  if (!stats.value.plan?.max_traffic) return 0
  return Math.min(100, Math.round((stats.value.monthly?.total || 0) / stats.value.plan.max_traffic * 100))
})

const isExpired = computed(() => {
  if (!stats.value.plan?.expires_at) return false
  return new Date(stats.value.plan.expires_at) < new Date()
})

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const formatDate = (date: string) => new Date(date).toLocaleDateString('zh-CN')

const loadLogs = async () => {
  const res = await getTrafficLogs({ page: page.value, size: 20, days: 30 })
  logs.value = res.data.list
  total.value = res.data.total
}

onMounted(async () => {
  const res = await getTrafficStats()
  stats.value = res.data
  await loadLogs()
})
</script>

<style scoped>
.traffic-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-4);
}

.plan-name-row {
  text-align: center;
  padding: 4px 0 8px;
}

.plan-name-tag {
  display: inline-block;
  font-size: var(--text-lg);
  font-weight: var(--font-bold);
  color: var(--color-primary);
}

.text-danger {
  color: var(--color-danger);
}
</style>
