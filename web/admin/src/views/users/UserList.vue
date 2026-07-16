<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">用户列表</span>
        <el-input v-model="keyword" placeholder="搜索邮箱..." style="width: 250px" @keyup.enter="fetchData" clearable prefix-icon="Search" />
      </div>
    </template>

    <el-table :data="users" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="email" label="邮箱" />
      <el-table-column prop="role" label="角色" width="100">
        <template #default="{ row }">
          <el-tag :type="row.role === 'super_admin' ? 'danger' : row.role === 'admin' ? 'warning' : ''" size="small">
            {{ roleMap[row.role] || row.role }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="balance" label="余额" width="100">
        <template #default="{ row }">
          <span class="text-mono">&yen;{{ row.balance?.toFixed(2) }}</span>
        </template>
      </el-table-column>
	  <el-table-column label="用户组" width="130">
		<template #default="{ row }">{{ row.group?.name || '未分组' }}</template>
	  </el-table-column>
	  <el-table-column label="单代理带宽" width="120">
		<template #default="{ row }">{{ row.bandwidth_limit > 0 ? formatBandwidth(row.bandwidth_limit) : '继承套餐' }}</template>
	  </el-table-column>
      <el-table-column prop="status" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'danger'" size="small">
            {{ row.status === 'active' ? '正常' : '封禁' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="注册时间" width="170">
        <template #default="{ row }">{{ new Date(row.created_at).toLocaleString('zh-CN') }}</template>
      </el-table-column>
      <el-table-column label="操作" width="160">
        <template #default="{ row }">
          <el-button size="small" @click="$router.push(`/users/${row.id}`)">详情</el-button>
          <el-button v-if="row.status === 'active'" size="small" type="danger" @click="handleBan(row)">封禁</el-button>
          <el-button v-else size="small" type="success" @click="handleUnban(row)">解封</el-button>
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
import { getUsers, banUser, unbanUser } from '../../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const users = ref<any[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const keyword = ref('')

const roleMap: Record<string, string> = {
  super_admin: '超级管理员', admin: '管理员', user: '普通用户',
}

onMounted(() => fetchData())

async function fetchData() {
  loading.value = true
  try {
    const res = await getUsers({ page: page.value, size: pageSize.value, keyword: keyword.value })
    users.value = res.data.list
    total.value = res.data.total
  } finally {
    loading.value = false
  }
}

async function handleBan(row: any) {
  await ElMessageBox.confirm(`确认封禁用户 ${row.email}？`, '确认操作')
  await banUser(row.id)
  ElMessage.success('用户已封禁')
  fetchData()
}

async function handleUnban(row: any) {
  await unbanUser(row.id)
  ElMessage.success('用户已解封')
  fetchData()
}

function formatBandwidth(bytes: number) {
  const mb = bytes / 1024 / 1024
  return `${Number.isInteger(mb) ? mb : mb.toFixed(1)} MB/s`
}
</script>

<style scoped>
/* page-header and page-title are defined in design-system.css */
</style>
