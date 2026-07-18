<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">我的代理</span>
		<div class="header-actions">
		  <el-button @click="showApiDocs = true">
			<el-icon><Guide /></el-icon>API 文档
		  </el-button>
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
      <el-table-column label="远程地址" min-width="190">
        <template #default="{ row }">
          <div class="remote-address">
            <span class="text-mono">{{ displayAddr(row) }}</span>
            <el-tooltip content="复制远程地址" placement="top">
              <el-button
                link
                class="copy-address-button"
                :disabled="displayAddr(row) === '-'"
                :aria-label="`复制代理 ${row.name} 的远程地址`"
                @click="copyRemoteAddress(row)"
              >
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </el-tooltip>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="流量" width="160">
        <template #default="{ row }">
          <span class="text-mono" style="font-size: 12px">↓{{ formatBytes(row.traffic_in) }} ↑{{ formatBytes(row.traffic_out) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="260">
        <template #default="{ row }">
          <div class="action-btns">
            <el-button
              size="small"
              :loading="loadingConfigServerIds.includes(Number(row.server?.id))"
              :disabled="!row.enabled"
              @click="copyProxyConfig(row)"
            >
              <el-icon><CopyDocument /></el-icon>{{ getCachedConfig(row) ? '复制配置' : '生成配置' }}
            </el-button>
            <el-button v-if="!row.enabled" size="small" type="success" @click="handleEnable(row)">启用</el-button>
            <el-button v-else size="small" type="warning" @click="handleDisable(row)">禁用</el-button>
            <el-button size="small" @click="$router.push(`/proxies/create?edit=${row.id}`)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
          </div>
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

	<el-dialog v-model="showApiDocs" title="客户端配置 API" width="min(720px, 92vw)" append-to-body>
	  <div class="api-docs">
		<section class="api-doc-section">
		  <div class="api-doc-heading">请求地址</div>
		  <div class="api-code-row">
			<code>{{ apiEndpoint }}</code>
			<el-tooltip content="复制请求地址" placement="top">
			  <el-button link aria-label="复制 API 请求地址" @click="copyApiText(apiEndpoint, '请求地址')">
				<el-icon><CopyDocument /></el-icon>
			  </el-button>
			</el-tooltip>
		  </div>
		</section>

		<section class="api-doc-section">
		  <div class="api-doc-heading">身份验证</div>
		  <p>使用“个人设置”中的用户 API Key，请勿使用节点 Token，也不要把 Key 放在 URL 查询参数中。</p>
		  <div class="api-auth-lines">
			<code>X-API-Key: YOUR_API_KEY</code>
			<span>或</span>
			<code>Authorization: Bearer YOUR_API_KEY</code>
		  </div>
		</section>

		<section class="api-doc-section">
		  <div class="api-doc-heading-row">
			<div class="api-doc-heading">请求示例</div>
			<el-button size="small" @click="copyApiText(apiCurlExample, '请求示例')">
			  <el-icon><CopyDocument /></el-icon>复制
			</el-button>
		  </div>
		  <pre class="api-code-block">{{ apiCurlExample }}</pre>
		</section>

		<section class="api-doc-section">
		  <div class="api-doc-heading">响应格式</div>
		  <pre class="api-code-block api-response-example">{{ apiResponseExample }}</pre>
		</section>

		<section class="api-doc-section api-doc-notes">
		  <div class="api-doc-heading">返回规则</div>
		  <p>仅返回当前有效账号可用节点上的已启用代理，并按节点分组。已禁用代理、无权访问的用户组节点以及 FRPS 节点密钥、SSH 凭据和插件密钥不会返回。</p>
		</section>
	  </div>
	</el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { getProxies, getFrpcConfig, enableProxy, disableProxy, deleteProxy } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, Document, Download, Guide, RefreshRight } from '@element-plus/icons-vue'

const proxies = ref<any[]>([])
const loading = ref(false)
const showConfigDialog = ref(false)
const showApiDocs = ref(false)
const configServerId = ref<number | null>(null)
const configContent = ref('')
const generatingConfig = ref(false)
const configCache = ref<Record<number, string>>({})
const loadingConfigServerIds = ref<number[]>([])

const apiEndpoint = computed(() => `${window.location.origin}/api/client/configs`)
const apiCurlExample = computed(() => `curl -H "X-API-Key: YOUR_API_KEY" "${apiEndpoint.value}"`)
const apiResponseExample = `{
  "code": 0,
  "message": "success",
  "data": {
    "generatedAt": "2026-07-18T08:00:00Z",
    "configs": [
      {
        "serverId": 1,
        "serverName": "节点名称",
        "frpVersion": "0.68.0",
        "serverAddr": "203.0.113.10",
        "serverPort": 7000,
        "auth": { "method": "token", "token": "YOUR_API_KEY" },
        "transport": { "tcpMux": true },
        "metadatas": { "apikey": "YOUR_API_KEY", "server_id": "1" },
        "proxies": [
          {
            "name": "remote-desktop",
            "type": "tcp",
            "localIP": "127.0.0.1",
            "localPort": 3389,
            "remotePort": 6000
          }
        ]
      }
    ]
  }
}`

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
    await preloadConfigCache()
  } finally {
    loading.value = false
  }
}

async function preloadConfigCache() {
  const serverIds = configServers.value.map((server: any) => Number(server.id)).filter(Boolean)
  await Promise.all(serverIds.map(serverId => loadServerConfig(serverId, true)))
}

async function loadServerConfig(serverId: number, force = false): Promise<string> {
  if (!serverId) return ''
  if (!force && configCache.value[serverId]) return configCache.value[serverId]
  if (force) invalidateServerConfig(serverId)
  if (!loadingConfigServerIds.value.includes(serverId)) {
	loadingConfigServerIds.value = [...loadingConfigServerIds.value, serverId]
  }
  try {
	const res = await getFrpcConfig(serverId)
	const config = res.data.config || ''
	configCache.value = { ...configCache.value, [serverId]: config }
	return config
  } catch {
	return ''
  } finally {
	loadingConfigServerIds.value = loadingConfigServerIds.value.filter(id => id !== serverId)
  }
}

function invalidateServerConfig(serverId: number) {
  if (!serverId || !configCache.value[serverId]) return
  const nextCache = { ...configCache.value }
  delete nextCache[serverId]
  configCache.value = nextCache
}

async function handleEnable(row: any) {
  await enableProxy(row.id)
  invalidateServerConfig(Number(row.server?.id))
  ElMessage.success('代理已启用')
  fetchData()
}

async function handleDisable(row: any) {
  await disableProxy(row.id)
  invalidateServerConfig(Number(row.server?.id))
  ElMessage.success('代理已禁用')
  fetchData()
}

async function handleDelete(row: any) {
  await ElMessageBox.confirm(`确认删除代理“${row.name}”？`, '确认删除', { type: 'warning' })
  await deleteProxy(row.id)
  invalidateServerConfig(Number(row.server?.id))
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
	configCache.value = { ...configCache.value, [configServerId.value]: configContent.value }
  } finally {
	generatingConfig.value = false
  }
}

async function copyConfig() {
  if (!configContent.value) return
  if (!await copyText(configContent.value)) {
	ElMessage.error('复制失败，请在配置框中手动复制')
	return
  }
  ElMessage.success('配置已复制')
}

async function copyText(text: string): Promise<boolean> {
  try {
	if (!navigator.clipboard || !window.isSecureContext) throw new Error('clipboard unavailable')
	await navigator.clipboard.writeText(text)
	return true
  } catch {
	let textarea: HTMLTextAreaElement | null = null
	try {
	  textarea = document.createElement('textarea')
	  textarea.value = text
	  textarea.style.position = 'fixed'
	  textarea.style.opacity = '0'
	  document.body.appendChild(textarea)
	  textarea.select()
	  return document.execCommand('copy')
	} catch {
	  return false
	} finally {
	  textarea?.remove()
	}
  }
}

async function copyRemoteAddress(row: any) {
  const address = displayAddr(row)
  if (address === '-') return
  if (!await copyText(address)) {
	ElMessage.error('复制失败，请手动选择远程地址')
	return
  }
  ElMessage.success('远程地址已复制')
}

async function copyApiText(text: string, label: string) {
  if (!await copyText(text)) {
	ElMessage.error(`${label}复制失败，请手动选择复制`)
	return
  }
  ElMessage.success(`${label}已复制`)
}

async function copyProxyConfig(row: any) {
  const serverId = Number(row.server?.id)
  let config = getCachedConfig(row)
  if (!config) {
	config = await loadServerConfig(serverId)
	if (config) {
	  ElMessage.info('配置已生成，请再次点击复制')
	} else {
	  ElMessage.error('配置生成失败，请稍后重试')
	}
	return
  }
  if (!await copyText(config)) {
	ElMessage.error('复制失败，请点击顶部 FRPC 配置后手动复制')
	return
  }
  ElMessage.success('FRPC 配置已复制，包含该节点的全部已启用代理')
}

function getCachedConfig(row: any): string {
  return configCache.value[Number(row.server?.id)] || ''
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

.remote-address {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.remote-address .text-mono {
  overflow-wrap: anywhere;
}

.copy-address-button {
  flex: 0 0 28px;
  width: 28px !important;
  min-width: 28px !important;
  padding: 0 !important;
}

.action-btns {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
}

.action-btns :deep(.el-button + .el-button) {
  margin-left: 0;
}

.api-docs {
  color: var(--el-text-color-regular);
}

.api-doc-section + .api-doc-section {
  margin-top: 20px;
}

.api-doc-heading,
.api-doc-heading-row {
  color: var(--el-text-color-primary);
  font-size: 14px;
  font-weight: 600;
}

.api-doc-heading-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.api-doc-section p {
  margin: 8px 0 0;
  line-height: 1.7;
}

.api-code-row,
.api-auth-lines {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 8px;
}

.api-code-row {
  justify-content: space-between;
  min-height: 38px;
  padding: 0 10px 0 12px;
  border: 1px solid var(--el-border-color);
  border-radius: 4px;
  background: var(--el-fill-color-light);
}

.api-code-row code,
.api-auth-lines code {
  overflow-wrap: anywhere;
}

.api-auth-lines {
  flex-wrap: wrap;
}

.api-auth-lines code {
  padding: 6px 8px;
  border-radius: 4px;
  background: var(--el-fill-color-light);
}

.api-code-block {
  max-height: 380px;
  margin: 8px 0 0;
  padding: 12px;
  overflow: auto;
  border: 1px solid var(--el-border-color);
  border-radius: 4px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre;
}

.api-response-example {
  max-height: 320px;
}

.api-doc-notes {
  padding-top: 16px;
  border-top: 1px solid var(--el-border-color-lighter);
}

@media (max-width: 768px) {
  .page-header {
    align-items: flex-start;
    gap: 12px;
  }

  .header-actions {
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}
</style>
