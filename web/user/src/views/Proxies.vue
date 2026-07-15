<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">我的代理</span>
        <el-button type="primary" @click="$router.push('/proxies/create')">
          <el-icon><Plus /></el-icon>创建代理
        </el-button>
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
  </el-card>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getProxies, enableProxy, disableProxy, deleteProxy } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Delete } from '@element-plus/icons-vue'

const proxies = ref<any[]>([])
const loading = ref(false)

onMounted(() => fetchData())

async function fetchData() {
  loading.value = true
  try {
    const res = await getProxies({ size: 100 })
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
</style>
