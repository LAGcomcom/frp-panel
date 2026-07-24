<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">代理管理</span>
        <div class="proxy-filters">
          <el-input
            v-model="keyword"
            class="proxy-search"
            placeholder="搜索代理、用户、节点或域名"
            clearable
            prefix-icon="Search"
            @keyup.enter="applyFilters"
            @clear="applyFilters"
          />
          <el-select v-model="typeFilter" class="proxy-filter" placeholder="全部类型" clearable @change="applyFilters">
            <el-option label="TCP" value="tcp" />
            <el-option label="UDP" value="udp" />
            <el-option label="HTTP" value="http" />
            <el-option label="HTTPS" value="https" />
            <el-option label="STCP" value="stcp" />
            <el-option label="XTCP" value="xtcp" />
          </el-select>
          <el-select v-model="statusFilter" class="proxy-filter" placeholder="连接状态" clearable @change="applyFilters">
            <el-option label="已连接" value="running" />
            <el-option label="未连接" value="pending" />
            <el-option label="已停止" value="stopped" />
            <el-option label="异常" value="error" />
          </el-select>
          <el-select v-model="enabledFilter" class="proxy-filter" placeholder="启用状态" clearable @change="applyFilters">
            <el-option label="已启用" value="true" />
            <el-option label="已禁用" value="false" />
          </el-select>
        </div>
      </div>
    </template>

    <el-table :data="proxies" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="name" label="名称" />
      <el-table-column prop="type" label="类型" width="80">
        <template #default="{ row }"><el-tag size="small">{{ row.type }}</el-tag></template>
      </el-table-column>
      <el-table-column label="用户" width="180">
        <template #default="{ row }">{{ row.user?.email }}</template>
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
	  <el-table-column label="操作" width="170" fixed="right">
		<template #default="{ row }">
		  <div class="action-btns">
			<el-button v-if="row.enabled" size="small" @click="handleDisable(row)">禁用</el-button>
			<el-button v-else size="small" type="success" @click="handleEnable(row)">启用</el-button>
			<el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
		  </div>
		</template>
	  </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="page"
      v-model:page-size="pageSize"
      :total="total"
      layout="total, prev, pager, next"
      @current-change="fetchData"
    />
  </el-card>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { deleteAdminProxy, disableAdminProxy, enableAdminProxy, getProxies } from '../../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const proxies = ref<any[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const keyword = ref('')
const typeFilter = ref('')
const statusFilter = ref('')
const enabledFilter = ref('')
let fetchSequence = 0

onMounted(() => fetchData())

async function fetchData() {
  const requestID = ++fetchSequence
  loading.value = true
  try {
    const res = await getProxies({
      page: page.value,
      size: pageSize.value,
      keyword: keyword.value.trim() || undefined,
      type: typeFilter.value || undefined,
      status: statusFilter.value || undefined,
      enabled: enabledFilter.value || undefined,
    })
    if (requestID !== fetchSequence) return
    proxies.value = res.data.list
    total.value = res.data.total
  } finally {
    if (requestID === fetchSequence) loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  fetchData()
}

async function handleEnable(row: any) {
	await enableAdminProxy(row.id)
	row.enabled = true
	ElMessage.success('代理已启用')
}

async function handleDisable(row: any) {
	await ElMessageBox.confirm(`确认禁用代理“${row.name}”？`, '确认禁用')
	await disableAdminProxy(row.id)
	row.enabled = false
	ElMessage.success('代理已禁用')
}

async function handleDelete(row: any) {
	await ElMessageBox.confirm(`确认删除代理“${row.name}”？此操作不可恢复。`, '确认删除', { type: 'warning' })
	await deleteAdminProxy(row.id)
	ElMessage.success('代理已删除')
	await fetchData()
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
/* page-header and page-title are defined in design-system.css */
.action-btns :deep(.el-button + .el-button) {
  margin-left: 0;
}

.page-header {
  gap: 12px;
  flex-wrap: wrap;
}

.proxy-filters {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex: 1;
  flex-wrap: wrap;
}

.proxy-search {
  width: 260px;
}

.proxy-filter {
  width: 120px;
}

@media (max-width: 760px) {
  .proxy-filters {
    width: 100%;
    justify-content: flex-start;
  }

  .proxy-search {
    width: min(100%, 260px);
  }
}
</style>
