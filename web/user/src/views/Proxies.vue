<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">我的代理</span>
		<div class="header-actions">
		  <el-button :disabled="configServers.length === 0" @click="openConfigDialog">
			<el-icon><Document /></el-icon>FRPC 配置
		  </el-button>
		  <el-button type="primary" @click="$router.push('/proxies/create')">
			<el-icon><Plus /></el-icon>创建代理
		  </el-button>
		</div>
      </div>
    </template>

    <el-table :data="proxies" v-loading="loading" stripe>
      <el-table-column prop="name" label="名称" />
      <el-table-column prop="type" label="类型" width="80">
        <template #default="{ row }"><el-tag size="small">{{ row.type }}</el-tag></template>
      </el-table-column>
      <el-table-column label="服务器" width="130">
        <template #default="{ row }">{{ row.server?.name }}</template>
      </el-table-column>
      <el-table-column label="状态" width="160">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'" size="small" style="margin-right: 4px">
            {{ row.enabled ? '已启用' : '已禁用' }}
          </el-tag>
          <el-tag :type="row.status === 'running' ? 'success' : row.status === 'error' ? 'danger' : 'warning'" size="small">
            {{ row.status === 'running' ? '已连接' : row.status === 'error' ? '异常' : '未连接' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="远程地址">
        <template #default="{ row }">{{ displayAddr(row) }}</template>
      </el-table-column>
      <el-table-column label="流量" width="160">
        <template #default="{ row }">
          <span class="text-mono" style="font-size: 12px">↓{{ formatBytes(row.traffic_in) }} ↑{{ formatBytes(row.traffic_out) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="180">
        <template #default="{ row }">
          <el-button v-if="!row.enabled" size="small" type="success" @click="handleEnable(row)">启用</el-button>
          <el-button v-else size="small" type="warning" @click="handleDisable(row)">禁用</el-button>
          <el-button size="small" @click="$router.push(`/proxies/create?edit=${row.id}`)">编辑</el-button>
          <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

	<el-dialog v-model="showConfigDialog" title="FRPC 配置" width="680" append-to-body>
	  <el-form label-width="70px">
		<el-form-item label="节点">
		  <el-select v-model="configServerId" style="width: 100%" @change="generateConfig">
			<el-option v-for="server in configServers" :key="server.id" :label="server.name" :value="server.id" />
		  </el-select>
		</el-form-item>
	  </el-form>
	  <el-input
		v-model="configContent"
		type="textarea"
		:rows="18"
		readonly
		resize="none"
		v-loading="generatingConfig"
	  />
	  <template #footer>
		<el-button :disabled="!configContent" @click="copyConfig">
		  <el-icon><CopyDocument /></el-icon>复制
		</el-button>
		<el-button :disabled="!configContent" @click="downloadConfig">
		  <el-icon><Download /></el-icon>下载
		</el-button>
		<el-button type="primary" :loading="generatingConfig" @click="generateConfig">
		  <el-icon><RefreshRight /></el-icon>重新生成
		</el-button>
	  </template>
	</el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { getProxies, getFrpcConfig, enableProxy, disableProxy, deleteProxy } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, Document, Download, RefreshRight } from '@element-plus/icons-vue'

const proxies = ref<any[]>([])
const loading = ref(false)
const showConfigDialog = ref(false)
const configServerId = ref<number | null>(null)
const configContent = ref('')
const generatingConfig = ref(false)

const configServers = computed(() => {
  const servers = new Map<number, any>()
  for (const proxy of proxies.value) {
	if (proxy.enabled && proxy.server?.id) servers.set(proxy.server.id, proxy.server)
  }
  return Array.from(servers.values())
})

onMounted(() => fetchData())

async function fetchData() {
  loading.value = true
  try {
    const res = await getProxies({ size: 1000 })
    proxies.value = res.data.list
  } finally {
    loading.value = false
  }
}

async function handleEnable(row: any) {
  await enableProxy(row.id)
  ElMessage.success('代理已启用')
  fetchData()
}

async function handleDisable(row: any) {
  await disableProxy(row.id)
  ElMessage.success('代理已禁用')
  fetchData()
}

async function handleDelete(row: any) {
  await ElMessageBox.confirm(`确认删除代理"${row.name}"？`, '确认删除', { type: 'warning' })
  await deleteProxy(row.id)
  ElMessage.success('代理已删除')
  fetchData()
}

async function openConfigDialog() {
  const firstServer = configServers.value[0]
  if (!firstServer) return
  configServerId.value = firstServer.id
  configContent.value = ''
  showConfigDialog.value = true
  await generateConfig()
}

async function generateConfig() {
  if (!configServerId.value) return
  generatingConfig.value = true
  try {
	const res = await getFrpcConfig(configServerId.value)
	configContent.value = res.data.config || ''
  } finally {
	generatingConfig.value = false
  }
}

async function copyConfig() {
  if (!configContent.value) return
  try {
	await navigator.clipboard.writeText(configContent.value)
  } catch {
	const textarea = document.createElement('textarea')
	textarea.value = configContent.value
	textarea.style.position = 'fixed'
	textarea.style.opacity = '0'
	document.body.appendChild(textarea)
	textarea.select()
	document.execCommand('copy')
	document.body.removeChild(textarea)
  }
  ElMessage.success('配置已复制')
}

function downloadConfig() {
  if (!configContent.value) return
  const server = configServers.value.find((item: any) => item.id === configServerId.value)
  const safeName = String(server?.name || configServerId.value || 'node').replace(/[^a-zA-Z0-9_-]+/g, '-')
  const url = URL.createObjectURL(new Blob([configContent.value], { type: 'text/plain;charset=utf-8' }))
  const link = document.createElement('a')
  link.href = url
  link.download = `frpc-${safeName}.toml`
  link.click()
  URL.revokeObjectURL(url)
}

function displayAddr(row: any): string {
  if (row.remote_addr) return row.remote_addr
  if (row.type === 'tcp' || row.type === 'udp') {
    const ip = row.server?.ip || '?'
    return row.remote_port ? `${ip}:${row.remote_port}` : '-'
  }
  if (row.type === 'http' || row.type === 'https') {
    if (row.subdomain) return row.subdomain
    try { return JSON.parse(row.custom_domains || '[]').join(', ') || '-' } catch { return '-' }
  }
  return row.type?.toUpperCase() || '-'
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
.header-actions {
  display: flex;
  gap: 8px;
}
</style>
