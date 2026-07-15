<template>
  <div class="servers-page">
    <div class="page-header" style="margin-bottom: var(--space-5)">
      <span class="page-title">服务器状态</span>
      <el-button size="small" @click="fetchData" :loading="loading">
        <el-icon><Refresh /></el-icon>刷新
      </el-button>
    </div>

    <div v-loading="loading" class="server-grid">
      <el-card v-for="s in servers" :key="s.id" class="server-card animate-in">
        <!-- Header: name + region + status dot -->
        <div class="card-top">
          <div class="server-name-row">
            <span class="status-dot" :class="s.latency > 0 ? 'dot-online' : 'dot-offline'"></span>
            <span class="server-name">{{ s.name }}</span>
            <el-tag size="small" type="info" v-if="s.region">{{ s.region }}</el-tag>
          </div>
          <div class="server-version">frps {{ s.frp_version || '-' }}</div>
        </div>

        <!-- Latency prominently displayed -->
        <div class="latency-section">
          <div class="latency-ring" :class="latencyClass(s.latency)">
            <span class="latency-value">{{ s.latency > 0 ? s.latency : '-' }}</span>
            <span class="latency-unit" v-if="s.latency > 0">ms</span>
          </div>
          <div class="latency-label">{{ latencyLabel(s.latency) }}</div>
        </div>

        <!-- Connection stats -->
        <div class="stat-row">
          <div class="stat-item">
            <div class="stat-num">{{ s.client_count || 0 }}</div>
            <div class="stat-label">客户端</div>
          </div>
          <div class="stat-divider"></div>
          <div class="stat-item">
            <div class="stat-num">{{ s.proxy_count || 0 }}</div>
            <div class="stat-label">代理数</div>
          </div>
          <div class="stat-divider"></div>
          <div class="stat-item">
            <div class="stat-num text-mono" style="font-size: 13px">{{ s.vhost_http_port || '-' }}</div>
            <div class="stat-label">HTTP</div>
          </div>
          <div class="stat-divider"></div>
          <div class="stat-item">
            <div class="stat-num text-mono" style="font-size: 13px">{{ s.vhost_https_port || '-' }}</div>
            <div class="stat-label">HTTPS</div>
          </div>
        </div>

        <!-- Metrics (if agent installed) -->
        <div v-if="s.metrics" class="metrics-section">
          <div class="metric-bar">
            <div class="metric-bar-header">
              <span class="metric-bar-label">CPU</span>
              <span class="metric-bar-value">{{ s.metrics.cpu_usage?.toFixed(1) || '0.0' }}%</span>
            </div>
            <el-progress :percentage="s.metrics.cpu_usage || 0" :stroke-width="6" :show-text="false"
              :color="progressColor(s.metrics.cpu_usage)" />
          </div>
          <div class="metric-bar">
            <div class="metric-bar-header">
              <span class="metric-bar-label">内存</span>
              <span class="metric-bar-value">{{ formatBytes(s.metrics.memory_used) }} / {{ formatBytes(s.metrics.memory_total) }}</span>
            </div>
            <el-progress :percentage="memPercent(s.metrics)" :stroke-width="6" :show-text="false"
              :color="progressColor(memPercent(s.metrics))" />
          </div>
          <div class="metric-bar">
            <div class="metric-bar-header">
              <span class="metric-bar-label">磁盘</span>
              <span class="metric-bar-value">{{ formatBytes(s.metrics.disk_used) }} / {{ formatBytes(s.metrics.disk_total) }}</span>
            </div>
            <el-progress :percentage="diskPercent(s.metrics)" :stroke-width="6" :show-text="false"
              :color="progressColor(diskPercent(s.metrics))" />
          </div>
          <div class="metric-extras">
            <span>网络 ↓{{ formatBytes(s.metrics.net_in) }}/s ↑{{ formatBytes(s.metrics.net_out) }}/s</span>
            <span>负载 {{ s.metrics.load_avg_1?.toFixed(2) || '0.00' }}</span>
            <span>连接 {{ s.metrics.connections || 0 }}</span>
          </div>
        </div>

        <!-- IP footer -->
        <div class="card-footer">
          <span class="text-mono" style="font-size: 12px; color: var(--color-text-muted)">{{ s.ip }}:{{ s.bind_port }}</span>
        </div>
      </el-card>
    </div>

    <el-empty v-if="!loading && servers.length === 0" description="暂无可用服务器" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { getAvailableServers } from '../api'

const servers = ref<any[]>([])
const loading = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  fetchData()
  timer = setInterval(fetchData, 10000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})

async function fetchData() {
  loading.value = true
  try {
    const res = await getAvailableServers()
    servers.value = res.data || []
  } finally {
    loading.value = false
  }
}

function latencyClass(ms: number): string {
  if (ms <= 0) return 'latency-bad'
  if (ms < 50) return 'latency-excellent'
  if (ms < 100) return 'latency-good'
  if (ms < 200) return 'latency-ok'
  return 'latency-bad'
}

function latencyLabel(ms: number): string {
  if (ms <= 0) return '不可达'
  if (ms < 50) return '极快'
  if (ms < 100) return '良好'
  if (ms < 200) return '一般'
  return '较慢'
}

function memPercent(m: any): number {
  if (!m?.memory_total) return 0
  return Math.round((m.memory_used / m.memory_total) * 100)
}

function diskPercent(m: any): number {
  if (!m?.disk_total) return 0
  return Math.round((m.disk_used / m.disk_total) * 100)
}

function progressColor(pct: number): string {
  if (pct < 60) return '#1a7f37'
  if (pct < 85) return '#9a6700'
  return '#cf222e'
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
}
</script>

<style scoped>
.servers-page {
  max-width: 100%;
}

.server-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: var(--space-4);
}

.server-card {
  transition: box-shadow 0.2s;
}

.server-card:hover {
  box-shadow: var(--shadow-lg);
}

/* ---- Top section ---- */
.card-top {
  margin-bottom: var(--space-4);
}

.server-name-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: 4px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.dot-online {
  background: #1a7f37;
  box-shadow: 0 0 6px rgba(26, 127, 55, 0.4);
}

.dot-offline {
  background: #cf222e;
  box-shadow: 0 0 6px rgba(207, 34, 46, 0.4);
}

.server-name {
  font-size: var(--text-md);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}

.server-version {
  font-size: 12px;
  color: var(--color-text-muted);
  font-family: var(--font-mono);
}

/* ---- Latency ring ---- */
.latency-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--space-4) 0;
}

.latency-ring {
  width: 80px;
  height: 80px;
  border-radius: 50%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  border: 3px solid;
  position: relative;
}

.latency-excellent { border-color: #1a7f37; background: rgba(26, 127, 55, 0.06); }
.latency-good { border-color: #2da44e; background: rgba(45, 164, 78, 0.06); }
.latency-ok { border-color: #9a6700; background: rgba(154, 103, 0, 0.06); }
.latency-bad { border-color: #cf222e; background: rgba(207, 34, 46, 0.06); }

.latency-value {
  font-size: 22px;
  font-weight: 800;
  font-family: var(--font-mono);
  line-height: 1;
}

.latency-excellent .latency-value { color: #1a7f37; }
.latency-good .latency-value { color: #2da44e; }
.latency-ok .latency-value { color: #9a6700; }
.latency-bad .latency-value { color: #cf222e; }

.latency-unit {
  font-size: 11px;
  color: var(--color-text-muted);
  margin-top: 2px;
}

.latency-label {
  font-size: 12px;
  font-weight: var(--font-semibold);
  margin-top: var(--space-2);
}

.latency-excellent + .latency-label { color: #1a7f37; }

/* ---- Stat row ---- */
.stat-row {
  display: flex;
  align-items: center;
  justify-content: space-around;
  padding: var(--space-3) 0;
  border-top: 1px solid var(--color-border-light);
  border-bottom: 1px solid var(--color-border-light);
  margin-bottom: var(--space-3);
}

.stat-item {
  text-align: center;
  flex: 1;
}

.stat-num {
  font-size: 16px;
  font-weight: var(--font-bold);
  color: var(--color-text);
  font-family: var(--font-mono);
}

.stat-label {
  font-size: 11px;
  color: var(--color-text-muted);
  margin-top: 2px;
}

.stat-divider {
  width: 1px;
  height: 24px;
  background: var(--color-border-light);
}

/* ---- Metrics ---- */
.metrics-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.metric-bar {
  padding: var(--space-1) 0;
}

.metric-bar-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}

.metric-bar-label {
  font-size: 12px;
  font-weight: var(--font-semibold);
  color: var(--color-text-secondary);
}

.metric-bar-value {
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--color-text-muted);
}

.metric-extras {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--color-text-muted);
  padding-top: var(--space-2);
  border-top: 1px solid var(--color-border-light);
  margin-top: var(--space-1);
  font-family: var(--font-mono);
}

/* ---- Footer ---- */
.card-footer {
  margin-top: var(--space-3);
  text-align: center;
}
</style>
