<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">代理管理</span>
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
import { getProxies } from '../../api'

const proxies = ref<any[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

onMounted(() => fetchData())

async function fetchData() {
  loading.value = true
  try {
    const res = await getProxies({ page: page.value, size: pageSize.value })
    proxies.value = res.data.list
    total.value = res.data.total
  } finally {
    loading.value = false
  }
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
