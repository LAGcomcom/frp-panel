<template>
  <div v-loading="loading">
    <div class="detail-header">
      <el-button text @click="$router.back()">
        <el-icon><ArrowLeft /></el-icon>返回
      </el-button>
      <span class="detail-title">{{ server.name }}</span>
      <el-tag :type="server.status === 'running' ? 'success' : server.status === 'error' ? 'danger' : 'warning'" size="small" style="margin-left: 8px">
        {{ statusMap[server.status] || server.status }}
      </el-tag>
      <div class="header-actions">
        <el-button size="small" @click="handleDeploy" :disabled="server.status === 'installing'">
          {{ server.status === 'pending' ? '部署' : '重装' }}
        </el-button>
        <el-button size="small" @click="handleRestart" :disabled="server.status !== 'running'">重启</el-button>
        <el-button size="small" @click="handleStop" :disabled="server.status !== 'running'">停止</el-button>
        <el-button size="small" v-if="server.status === 'running' && !server.agent_installed" @click="handleInstallAgent">
          安装监控
        </el-button>
        <el-button size="small" type="danger" @click="handleDelete">删除</el-button>
      </div>
    </div>

    <!-- Info cards row -->
    <div class="info-row">
      <el-card class="animate-in info-card">
        <div class="info-label">IP 地址</div>
        <div class="info-value text-mono">{{ server.ip }}</div>
      </el-card>
      <el-card class="animate-in info-card">
        <div class="info-label">地区</div>
        <div class="info-value">{{ server.region || '-' }}</div>
      </el-card>
      <el-card class="animate-in info-card">
        <div class="info-label">延迟</div>
        <div class="info-value text-mono" :style="{ color: server.latency < 100 ? '#1a7f37' : server.latency < 300 ? '#9a6700' : '#cf222e' }">{{ server.latency > 0 ? server.latency + 'ms' : '-' }}</div>
      </el-card>
      <el-card class="animate-in info-card">
        <div class="info-label">客户端 / 代理</div>
        <div class="info-value">{{ server.client_count || 0 }} / {{ server.proxy_count || 0 }}</div>
      </el-card>
    </div>

    <div class="detail-grid">
      <!-- Ports card -->
      <el-card class="animate-in animate-in-delay-1">
        <template #header>端口配置</template>
        <el-descriptions :column="1" border size="small">
          <el-descriptions-item label="绑定端口">
            <span class="text-mono">{{ server.bind_port }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="面板端口">
            <span class="text-mono">{{ server.dashboard_port }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="HTTP 端口">
            <span class="text-mono">{{ server.vhost_http_port || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="HTTPS 端口">
            <span class="text-mono">{{ server.vhost_https_port || '-' }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <!-- Metrics card -->
      <el-card v-if="server.agent_installed" class="animate-in animate-in-delay-1">
        <template #header>服务器监控</template>
        <div class="metrics-grid">
          <div class="metric-item">
            <div class="metric-label">CPU</div>
            <div class="metric-value">{{ metrics.cpu_usage?.toFixed(1) || 0 }}%</div>
            <el-progress :percentage="metrics.cpu_usage || 0" :stroke-width="6" :show-text="false" />
          </div>
          <div class="metric-item">
            <div class="metric-label">内存</div>
            <div class="metric-value">{{ formatBytes(metrics.memory_usage) }} / {{ formatBytes(metrics.memory_total) }}</div>
            <el-progress :percentage="memoryPercent" :stroke-width="6" :show-text="false" />
          </div>
          <div class="metric-item">
            <div class="metric-label">磁盘</div>
            <div class="metric-value">{{ formatBytes(metrics.disk_usage) }} / {{ formatBytes(metrics.disk_total) }}</div>
            <el-progress :percentage="diskPercent" :stroke-width="6" :show-text="false" />
          </div>
          <div class="metric-item">
            <div class="metric-label">网络</div>
            <div class="metric-value">↓{{ formatBytes(metrics.net_in) }}/s ↑{{ formatBytes(metrics.net_out) }}/s</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">负载</div>
            <div class="metric-value">{{ metrics.load_avg_1?.toFixed(2) || '0.00' }} / {{ metrics.load_avg_5?.toFixed(2) || '0.00' }} / {{ metrics.load_avg_15?.toFixed(2) || '0.00' }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">连接数</div>
            <div class="metric-value">{{ metrics.connections || 0 }}</div>
          </div>
        </div>
      </el-card>

      <!-- Proxy list - full width -->
      <el-card class="animate-in animate-in-delay-2 proxy-card">
        <template #header>代理列表 ({{ proxies.length }})</template>
        <el-table :data="proxies" stripe size="small">
          <el-table-column prop="name" label="名称" />
          <el-table-column prop="type" label="类型" width="80">
            <template #default="{ row }"><el-tag size="small">{{ row.type }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="80">
            <template #default="{ row }">
              <el-tag :type="row.status === 'running' ? 'success' : 'info'" size="small">
                {{ row.status === 'running' ? '运行中' : row.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="remote_addr" label="远程地址" />
          <el-table-column label="流量" width="160">
            <template #default="{ row }">
              <span class="text-mono" style="font-size: 12px">↓{{ formatBytes(row.traffic_in) }} ↑{{ formatBytes(row.traffic_out) }}</span>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="proxies.length === 0" description="暂无代理" :image-size="48" />
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getServer, getServerProxies, getServerMetrics, installAgent, deployServer, restartServer, stopServer, deleteServer } from '../../api'

const route = useRoute()
const router = useRouter()
const server = ref<any>({})
const proxies = ref<any[]>([])
const metrics = ref<any>({})
const loading = ref(false)
let metricsTimer: ReturnType<typeof setInterval> | null = null

const statusMap: Record<string, string> = {
  pending: '待部署', installing: '安装中', running: '运行中', stopped: '已停止', error: '异常',
}

const memoryPercent = computed(() => {
  if (!metrics.value.memory_total) return 0
  return Math.round((metrics.value.memory_usage / metrics.value.memory_total) * 100)
})

const diskPercent = computed(() => {
  if (!metrics.value.disk_total) return 0
  return Math.round((metrics.value.disk_usage / metrics.value.disk_total) * 100)
})

onMounted(async () => {
  loading.value = true
  try {
    const id = route.params.id
    const [serverRes, proxiesRes] = await Promise.all([
      getServer(Number(id)),
      getServerProxies(Number(id)),
    ])
    server.value = serverRes.data.server
    proxies.value = proxiesRes.data

    if (server.value.agent_installed) {
      fetchMetrics()
      metricsTimer = setInterval(fetchMetrics, 5000)
    }
  } finally {
    loading.value = false
  }
})

onUnmounted(() => {
  if (metricsTimer) clearInterval(metricsTimer)
})

async function fetchMetrics() {
  try {
    const id = Number(route.params.id)
    const res = await getServerMetrics(id, 1)
    if (res.data?.current) {
      metrics.value = res.data.current
    }
  } catch (e) {
    // ignore
  }
}

async function handleInstallAgent() {
  try {
    const id = Number(route.params.id)
    await installAgent(id)
    ElMessage.info('监控 Agent 正在安装')
    for (let attempt = 0; attempt < 30; attempt++) {
      await new Promise(resolve => setTimeout(resolve, 2000))
      const res = await getServer(id)
      server.value = res.data.server
      if (server.value.agent_installed) {
        ElMessage.success('监控 Agent 安装成功')
        await fetchMetrics()
        if (metricsTimer) clearInterval(metricsTimer)
        metricsTimer = setInterval(fetchMetrics, 5000)
        return
      }
      if (server.value.error_msg?.startsWith('[Agent]')) {
        throw new Error(server.value.error_msg)
      }
    }
    ElMessage.warning('Agent 安装仍在进行，请稍后刷新')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || e.message || '安装失败')
  }
}

async function handleDeploy() {
  await ElMessageBox.confirm(`确认在 ${server.value.name} (${server.value.ip}) 上部署 frps？`, '确认部署')
  await deployServer(server.value.id)
  ElMessage.success('部署任务已提交')
  server.value.status = 'installing'
}

async function handleRestart() {
  await ElMessageBox.confirm(`确认重启 ${server.value.name} 上的 frps？`, '确认重启')
  await restartServer(server.value.id)
  ElMessage.success('重启指令已发送')
}

async function handleStop() {
  await ElMessageBox.confirm(`确认停止 ${server.value.name} 上的 frps？`, '确认停止')
  await stopServer(server.value.id)
  ElMessage.success('停止指令已发送')
  server.value.status = 'stopped'
}

async function handleDelete() {
  await ElMessageBox.confirm(`确认删除服务器 ${server.value.name}？此操作不可恢复。`, '确认删除', { type: 'warning' })
  await deleteServer(server.value.id)
  ElMessage.success('服务器已删除')
  router.push('/servers')
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}
</script>

<style scoped>
.detail-header {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-5);
}
.detail-title {
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}
.header-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}
.info-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-4);
  margin-bottom: var(--space-5);
}
.info-card {
  text-align: center;
}
.info-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 4px;
}
.info-value {
  font-size: 18px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.detail-grid {
  display: grid;
  grid-template-columns: 320px 1fr;
  gap: var(--space-4);
}
.proxy-card {
  grid-column: 1 / -1;
}
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}
.metric-item {
  padding: 12px;
  background: var(--el-fill-color-lighter);
  border-radius: 8px;
}
.metric-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 4px;
}
.metric-value {
  font-size: 16px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin-bottom: 8px;
  font-family: monospace;
}
</style>
