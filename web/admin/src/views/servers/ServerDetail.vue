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
        <el-button size="small" @click="handleDeploy" :loading="deployLoading" :disabled="server.status === 'installing'">
          {{ server.status === 'pending' ? '部署' : '重装' }}
        </el-button>
        <el-button size="small" @click="handleRestart" :loading="restartLoading" :disabled="server.status !== 'running'">重启</el-button>
        <el-button size="small" @click="handleStop" :loading="stopLoading" :disabled="server.status !== 'running'">停止</el-button>
        <el-button size="small" v-if="server.status === 'running' && !server.agent_installed" @click="handleInstallAgent">
          安装监控
        </el-button>
		<el-button size="small" @click="openEdit">编辑</el-button>
		<el-dropdown trigger="click" @command="handleMoreCommand">
		  <el-button size="small">更多<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button>
		  <template #dropdown>
			<el-dropdown-menu>
			  <el-dropdown-item command="clients" :disabled="server.status !== 'running'">客户端</el-dropdown-item>
			  <el-dropdown-item command="config" :disabled="server.status === 'pending' || server.status === 'installing'">FRPS 配置</el-dropdown-item>
			  <el-dropdown-item command="logs" :disabled="server.status === 'pending' || server.status === 'installing'">运行日志</el-dropdown-item>
			  <el-dropdown-item command="uninstall" divided :disabled="server.status === 'pending' || server.status === 'installing'">卸载 FRPS</el-dropdown-item>
			</el-dropdown-menu>
		  </template>
		</el-dropdown>
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

	<el-dialog v-model="showEdit" title="编辑节点" width="520" append-to-body>
	  <el-form :model="editForm" label-width="120px">
		<el-form-item label="名称" required><el-input v-model="editForm.name" /></el-form-item>
		<el-form-item label="地区"><el-input v-model="editForm.region" /></el-form-item>
		<el-form-item label="最大用户数"><el-input-number v-model="editForm.max_users" :min="0" /></el-form-item>
		<el-form-item label="绑定端口"><el-input-number v-model="editForm.bind_port" :min="1" :max="65535" /></el-form-item>
		<el-form-item label="面板端口"><el-input-number v-model="editForm.dashboard_port" :min="1" :max="65535" /></el-form-item>
		<el-form-item label="HTTP 端口"><el-input-number v-model="editForm.vhost_http_port" :min="1" :max="65535" /></el-form-item>
		<el-form-item label="HTTPS 端口"><el-input-number v-model="editForm.vhost_https_port" :min="1" :max="65535" /></el-form-item>
	  </el-form>
	  <template #footer>
		<el-button @click="showEdit = false">取消</el-button>
		<el-button type="primary" :loading="savingEdit" @click="saveEdit">保存</el-button>
	  </template>
	</el-dialog>

	<el-dialog v-model="showConfig" title="FRPS 配置" width="760" append-to-body>
	  <el-alert type="warning" :closable="false" show-icon title="手动保存配置后，节点安全认证会暂停；请重新部署节点以恢复安全模式。" />
	  <el-input v-model="configContent" type="textarea" :rows="20" class="config-editor" v-loading="configLoading" />
	  <template #footer>
		<el-button @click="showConfig = false">取消</el-button>
		<el-button type="primary" :loading="configSaving" @click="saveConfig">保存并重启</el-button>
	  </template>
	</el-dialog>

	<el-dialog v-model="showLogs" title="FRPS 运行日志" width="820" append-to-body>
	  <div class="dialog-toolbar">
		<el-input-number v-model="logLines" :min="50" :max="2000" :step="50" />
		<el-button :loading="logsLoading" @click="loadLogs">刷新</el-button>
	  </div>
	  <pre class="log-view">{{ logsContent || '暂无日志' }}</pre>
	</el-dialog>

	<el-dialog v-model="showClients" title="已连接客户端" width="820" append-to-body>
	  <el-table :data="clients" v-loading="clientsLoading" stripe>
		<el-table-column prop="user" label="用户" min-width="130" />
		<el-table-column prop="hostname" label="主机名" min-width="130" />
		<el-table-column prop="clientIP" label="客户端 IP" min-width="140" />
		<el-table-column prop="version" label="版本" width="100" />
		<el-table-column prop="online" label="状态" width="90">
		  <template #default="{ row }"><el-tag :type="row.online ? 'success' : 'info'" size="small">{{ row.online ? '在线' : '离线' }}</el-tag></template>
		</el-table-column>
	  </el-table>
	  <el-empty v-if="!clientsLoading && clients.length === 0" description="暂无客户端" :image-size="48" />
	</el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  deleteServer, deployServer, getServer, getServerClients, getServerConfig, getServerLogs,
  getServerMetrics, getServerProxies, installAgent, restartServer, stopServer, uninstallServer,
  updateServer, updateServerConfig,
} from '../../api'

const route = useRoute()
const router = useRouter()
const server = ref<any>({})
const proxies = ref<any[]>([])
const metrics = ref<any>({})
const loading = ref(false)
let metricsTimer: ReturnType<typeof setInterval> | null = null
const showEdit = ref(false)
const savingEdit = ref(false)
const editForm = reactive({ name: '', region: '', max_users: 0, bind_port: 7000, dashboard_port: 7500, vhost_http_port: 80, vhost_https_port: 443 })
const showConfig = ref(false)
const configContent = ref('')
const configLoading = ref(false)
const configSaving = ref(false)
const showLogs = ref(false)
const logsContent = ref('')
const logsLoading = ref(false)
const logLines = ref(200)
const showClients = ref(false)
const clients = ref<any[]>([])
const clientsLoading = ref(false)
const deployLoading = ref(false)
const restartLoading = ref(false)
const stopLoading = ref(false)

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

onMounted(loadDetail)

async function loadDetail() {
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
      if (!metricsTimer) metricsTimer = setInterval(fetchMetrics, 5000)
    } else if (metricsTimer) {
      clearInterval(metricsTimer)
      metricsTimer = null
    }
  } finally {
    loading.value = false
  }
}

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
  try {
    await ElMessageBox.confirm(`确认在 ${server.value.name} (${server.value.ip}) 上部署 frps？`, '确认部署')
  } catch {
    return
  }
  deployLoading.value = true
  try {
    await deployServer(server.value.id)
    ElMessage.success('部署任务已提交，请稍后刷新查看状态')
    server.value.status = 'installing'
    setTimeout(loadDetail, 1200)
  } catch (e: any) {
    ElMessage.error(e.message || '部署任务提交失败')
  } finally {
    deployLoading.value = false
  }
}

async function handleRestart() {
  try {
    await ElMessageBox.confirm(`确认重启 ${server.value.name} 上的 frps？`, '确认重启')
  } catch {
    return
  }
  restartLoading.value = true
  try {
    await restartServer(server.value.id)
    ElMessage.success('重启成功')
    await loadDetail()
  } catch (e: any) {
    ElMessage.error(e.message || '重启失败')
  } finally {
    restartLoading.value = false
  }
}

async function handleStop() {
  try {
    await ElMessageBox.confirm(`确认停止 ${server.value.name} 上的 frps？`, '确认停止')
  } catch {
    return
  }
  stopLoading.value = true
  try {
    await stopServer(server.value.id)
    ElMessage.success('停止成功')
    server.value.status = 'stopped'
    await loadDetail()
  } catch (e: any) {
    ElMessage.error(e.message || '停止失败')
  } finally {
    stopLoading.value = false
  }
}

async function handleDelete() {
  await ElMessageBox.confirm(`确认删除服务器 ${server.value.name}？此操作不可恢复。`, '确认删除', { type: 'warning' })
  await deleteServer(server.value.id)
  ElMessage.success('服务器已删除')
  router.push('/servers')
}

function openEdit() {
	Object.assign(editForm, {
	  name: server.value.name || '', region: server.value.region || '', max_users: server.value.max_users || 0,
	  bind_port: server.value.bind_port || 7000, dashboard_port: server.value.dashboard_port || 7500,
	  vhost_http_port: server.value.vhost_http_port || 80, vhost_https_port: server.value.vhost_https_port || 443,
	})
	showEdit.value = true
}

async function saveEdit() {
	if (!editForm.name.trim()) {
	  ElMessage.warning('请填写节点名称')
	  return
	}
	savingEdit.value = true
	try {
	  await updateServer(server.value.id, editForm)
	  Object.assign(server.value, editForm)
	  showEdit.value = false
	  ElMessage.success('节点信息已更新')
	} finally {
	  savingEdit.value = false
	}
}

async function handleMoreCommand(command: string) {
	if (command === 'clients') await openClients()
	if (command === 'config') await openConfig()
	if (command === 'logs') await openLogs()
	if (command === 'uninstall') await handleUninstall()
}

async function openClients() {
	showClients.value = true
	clientsLoading.value = true
	try {
	  const res = await getServerClients(server.value.id)
	  clients.value = res.data?.clients || []
	} finally {
	  clientsLoading.value = false
	}
}

async function openConfig() {
	showConfig.value = true
	configLoading.value = true
	try {
	  const res = await getServerConfig(server.value.id)
	  configContent.value = res.data?.config || ''
	} finally {
	  configLoading.value = false
	}
}

async function saveConfig() {
	await ElMessageBox.confirm('保存后 FRPS 会重启，安全认证需重新部署节点才能恢复。确认继续？', '确认保存', { type: 'warning' })
	configSaving.value = true
	try {
	  await updateServerConfig(server.value.id, configContent.value)
	  server.value.plugin_auth_enabled = false
	  showConfig.value = false
	  ElMessage.success('配置已保存，建议立即重新部署节点')
	} finally {
	  configSaving.value = false
	}
}

async function openLogs() {
	showLogs.value = true
	await loadLogs()
}

async function loadLogs() {
	logsLoading.value = true
	try {
	  const res = await getServerLogs(server.value.id, logLines.value)
	  logsContent.value = res.data?.logs || ''
	} finally {
	  logsLoading.value = false
	}
}

async function handleUninstall() {
	await ElMessageBox.confirm(`确认从 ${server.value.name} 卸载 FRPS？节点上的隧道会立即中断。`, '确认卸载', { type: 'warning' })
	await uninstallServer(server.value.id)
	server.value.status = 'pending'
	server.value.plugin_auth_enabled = false
	server.value.agent_installed = false
	ElMessage.success('FRPS 已卸载')
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
.header-actions > * {
  margin-left: 0 !important;
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
.config-editor {
  margin-top: 12px;
  font-family: var(--font-mono);
}
.dialog-toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 10px;
}
.log-view {
  min-height: 360px;
  max-height: 560px;
  margin: 0;
  overflow: auto;
  padding: 12px;
  background: #111827;
  color: #d1fae5;
  border-radius: 6px;
  font: 12px/1.6 var(--font-mono);
  white-space: pre-wrap;
  word-break: break-all;
}

@media (max-width: 1100px) {
  .detail-header {
    flex-wrap: wrap;
  }
  .header-actions {
    width: 100%;
    margin-left: 0;
    justify-content: flex-end;
    flex-wrap: wrap;
  }
}
</style>
