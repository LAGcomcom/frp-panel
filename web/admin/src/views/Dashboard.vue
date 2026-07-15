<template>
  <div class="dashboard">
    <!-- Stat Cards -->
    <div class="stat-grid">
      <div class="stat-card animate-in">
        <div class="stat-icon stat-icon--blue">
          <el-icon :size="22"><Monitor /></el-icon>
        </div>
        <div class="stat-body">
          <div class="stat-label">服务器</div>
          <div class="stat-value">{{ data.servers?.total ?? '--' }}</div>
          <div class="stat-detail">在线 {{ data.servers?.online ?? 0 }}</div>
        </div>
      </div>

      <div class="stat-card animate-in animate-in-delay-1">
        <div class="stat-icon stat-icon--green">
          <el-icon :size="22"><User /></el-icon>
        </div>
        <div class="stat-body">
          <div class="stat-label">用户总数</div>
          <div class="stat-value">{{ data.users?.total ?? '--' }}</div>
          <div class="stat-detail">今日新增 {{ data.users?.today ?? 0 }}</div>
        </div>
      </div>

      <div class="stat-card animate-in animate-in-delay-2">
        <div class="stat-icon stat-icon--purple">
          <el-icon :size="22"><Connection /></el-icon>
        </div>
        <div class="stat-body">
          <div class="stat-label">代理隧道</div>
          <div class="stat-value">{{ data.proxies?.total ?? '--' }}</div>
          <div class="stat-detail">运行中 {{ data.proxies?.running ?? 0 }}</div>
        </div>
      </div>

      <div class="stat-card animate-in animate-in-delay-3">
        <div class="stat-icon stat-icon--amber">
          <el-icon :size="22"><Coin /></el-icon>
        </div>
        <div class="stat-body">
          <div class="stat-label">本月收入</div>
          <div class="stat-value">&yen;{{ (data.revenue?.month ?? 0).toFixed(2) }}</div>
          <div class="stat-detail">总计 &yen;{{ (data.revenue?.total ?? 0).toFixed(2) }}</div>
        </div>
      </div>
    </div>

    <!-- Info Row -->
    <div class="info-grid">
      <!-- Proxy Distribution -->
      <el-card class="animate-in animate-in-delay-3">
        <template #header>
          <div class="card-header">
            <span>代理类型分布</span>
          </div>
        </template>
        <div v-if="data.proxies?.by_type?.length" class="distribution-list">
          <div v-for="item in data.proxies.by_type" :key="item.type" class="distribution-item">
            <div class="distribution-label">
              <el-tag :type="getTypeTag(item.type)" size="small">{{ item.type }}</el-tag>
            </div>
            <div class="distribution-bar">
              <div class="distribution-bar__fill" :style="{ width: getBarWidth(item.count) + '%' }"></div>
            </div>
            <div class="distribution-value">{{ item.count }}</div>
          </div>
        </div>
        <el-empty v-else description="暂无数据" :image-size="48" />
      </el-card>

      <!-- Traffic Today -->
      <el-card class="animate-in animate-in-delay-4">
        <template #header>
          <div class="card-header">
            <span>今日流量</span>
          </div>
        </template>
        <div class="traffic-stats">
          <div class="traffic-item">
            <span class="traffic-label">入站流量</span>
            <span class="traffic-value">{{ formatBytes(data.traffic?.today_in ?? 0) }}</span>
          </div>
          <div class="traffic-item">
            <span class="traffic-label">出站流量</span>
            <span class="traffic-value">{{ formatBytes(data.traffic?.today_out ?? 0) }}</span>
          </div>
          <div class="traffic-divider"></div>
          <div class="traffic-item traffic-item--total">
            <span class="traffic-label">总流量</span>
            <span class="traffic-value">{{ formatBytes((data.traffic?.today_in ?? 0) + (data.traffic?.today_out ?? 0)) }}</span>
          </div>
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getDashboard } from '../api'

const data = ref<any>({})

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

function getTypeTag(type: string): '' | 'success' | 'warning' | 'danger' | 'info' {
  const map: Record<string, '' | 'success' | 'warning' | 'danger' | 'info'> = {
    tcp: '', udp: 'success', http: 'warning', https: 'danger', stcp: 'info', xtcp: 'info',
  }
  return map[type] ?? 'info'
}

function getBarWidth(count: number): number {
  const max = Math.max(...(data.value.proxies?.by_type || []).map((i: any) => i.count), 1)
  return (count / max) * 100
}

onMounted(async () => {
  try {
    const res = await getDashboard()
    data.value = res.data
  } catch {}
})
</script>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

/* ---- Stat Cards ---- */
.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-4);
}

/* ---- Info Grid ---- */
.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-4);
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

/* ---- Distribution ---- */
.distribution-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.distribution-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.distribution-label {
  width: 60px;
  flex-shrink: 0;
}

.distribution-bar {
  flex: 1;
  height: 6px;
  background: var(--color-bg);
  border-radius: var(--radius-full);
  overflow: hidden;
}

.distribution-bar__fill {
  height: 100%;
  background: var(--color-primary);
  border-radius: var(--radius-full);
  transition: width var(--transition-normal);
}

.distribution-value {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text);
  width: 40px;
  text-align: right;
  font-family: var(--font-mono);
}

/* ---- Traffic ---- */
.traffic-stats {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.traffic-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.traffic-item--total {
  font-weight: var(--font-semibold);
}

.traffic-label {
  font-size: var(--text-sm);
  color: var(--color-text-secondary);
}

.traffic-value {
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  color: var(--color-text);
  font-family: var(--font-mono);
}

.traffic-divider {
  height: 1px;
  background: var(--color-border-light);
}
</style>
