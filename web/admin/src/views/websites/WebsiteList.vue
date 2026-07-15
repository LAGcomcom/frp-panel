<template>
  <div>
    <el-card class="animate-in">
      <template #header>
        <div class="page-header">
          <span class="page-title">网站管理</span>
          <el-button type="primary" @click="openAdd">
            <el-icon><Plus /></el-icon>添加网站
          </el-button>
        </div>
      </template>

      <!-- Search & Filter -->
      <div class="filter-bar">
        <el-input v-model="search" placeholder="搜索域名/名称" clearable style="width: 240px" @clear="fetchData" @keyup.enter="fetchData">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-select v-model="filterStatus" placeholder="状态" clearable style="width: 120px" @change="fetchData">
          <el-option label="运行中" value="running" />
          <el-option label="待部署" value="pending" />
          <el-option label="已停止" value="stopped" />
          <el-option label="异常" value="error" />
        </el-select>
        <el-button @click="fetchData">查询</el-button>
      </div>

      <el-table :data="websites" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="名称" width="120" />
        <el-table-column label="域名" min-width="200">
          <template #default="{ row }">
            <span class="text-mono">{{ row.domain }}{{ row.subdomain ? '.' + row.subdomain : '' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="type" label="类型" width="70">
          <template #default="{ row }"><el-tag size="small" :type="row.type === 'https' ? 'success' : 'info'">{{ row.type }}</el-tag></template>
        </el-table-column>
        <el-table-column label="后端地址" width="150">
          <template #default="{ row }"><span class="text-mono">{{ row.backend_addr }}</span></template>
        </el-table-column>
        <el-table-column label="用户" width="160">
          <template #default="{ row }">{{ row.user?.email || '-' }}</template>
        </el-table-column>
        <el-table-column label="服务器" width="100">
          <template #default="{ row }">{{ row.server?.name || '-' }}</template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.status === 'running' ? 'success' : row.status === 'error' ? 'danger' : 'info'" size="small">
              {{ statusMap[row.status] || row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="SSL" width="80">
          <template #default="{ row }">
            <el-tag :type="row.ssl_status === 'active' ? 'success' : row.ssl_status === 'error' ? 'danger' : 'info'" size="small">
              {{ sslMap[row.ssl_status] || row.ssl_status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="响应时间" width="90">
          <template #default="{ row }">{{ row.response_time ? row.response_time + 'ms' : '-' }}</template>
        </el-table-column>
        <el-table-column label="流量" width="150">
          <template #default="{ row }">
            <span class="text-mono" style="font-size: 12px">↓{{ formatBytes(row.traffic_in) }} ↑{{ formatBytes(row.traffic_out) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <div class="action-btns">
              <el-button size="small" @click="openEdit(row)">编辑</el-button>
              <el-button size="small" type="success" @click="handleCheck(row)" :loading="row._checking">检测</el-button>
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

    <!-- Add/Edit Dialog -->
    <el-dialog v-model="showDialog" :title="editingId ? '编辑网站' : '添加网站'" width="520">
      <el-form :model="form" label-width="100px">
        <el-form-item label="网站名称" required>
          <el-input v-model="form.name" placeholder="如：我的博客" />
        </el-form-item>
        <el-form-item label="域名" required>
          <el-input v-model="form.domain" placeholder="如：example.com" />
        </el-form-item>
        <el-form-item label="子域名">
          <el-input v-model="form.subdomain" placeholder="如：www（可选）" />
        </el-form-item>
        <el-form-item label="类型" required>
          <el-radio-group v-model="form.type">
            <el-radio value="http">HTTP</el-radio>
            <el-radio value="https">HTTPS</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="服务器" required>
          <el-select v-model="form.server_id" placeholder="选择服务器" style="width: 100%">
            <el-option v-for="s in servers" :key="s.id" :label="s.name + ' (' + s.ip + ')'" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="所属用户" required>
          <el-select v-model="form.user_id" placeholder="选择用户" filterable style="width: 100%">
            <el-option v-for="u in users" :key="u.id" :label="u.email" :value="u.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="后端地址" required>
          <el-input v-model="form.backend_addr" placeholder="如：127.0.0.1:8080" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitLoading">{{ editingId ? '更新' : '创建' }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Delete } from '@element-plus/icons-vue'
import { getWebsites, createWebsite, updateWebsite, deleteWebsite, checkWebsite, getServers, getUsers } from '../../api'

const websites = ref<any[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const search = ref('')
const filterStatus = ref('')
const showDialog = ref(false)
const submitLoading = ref(false)
const editingId = ref<number | null>(null)
const servers = ref<any[]>([])
const users = ref<any[]>([])

const statusMap: Record<string, string> = {
  pending: '待部署', running: '运行中', stopped: '已停止', error: '异常',
}
const sslMap: Record<string, string> = {
  none: '无', active: '有效', expired: '过期', error: '异常',
}

const form = reactive({
  name: '',
  domain: '',
  subdomain: '',
  type: 'http',
  server_id: null as number | null,
  user_id: null as number | null,
  backend_addr: '',
})

onMounted(() => {
  fetchData()
})

async function fetchData() {
  loading.value = true
  try {
    const res = await getWebsites({
      page: page.value,
      size: pageSize.value,
      search: search.value,
      status: filterStatus.value,
    })
    websites.value = res.data.list || []
    total.value = res.data.total
  } finally {
    loading.value = false
  }
}

async function loadOptions() {
  const [sRes, uRes] = await Promise.all([
    getServers({ page: 1, size: 100 }),
    getUsers({ page: 1, size: 100 }),
  ])
  servers.value = sRes.data?.list || []
  users.value = uRes.data?.list || []
}

function openAdd() {
  editingId.value = null
  Object.assign(form, { name: '', domain: '', subdomain: '', type: 'http', server_id: null, user_id: null, backend_addr: '' })
  loadOptions()
  showDialog.value = true
}

function openEdit(row: any) {
  editingId.value = row.id
  Object.assign(form, {
    name: row.name,
    domain: row.domain,
    subdomain: row.subdomain || '',
    type: row.type,
    server_id: row.server_id,
    user_id: row.user_id,
    backend_addr: row.backend_addr,
  })
  loadOptions()
  showDialog.value = true
}

async function handleSubmit() {
  if (!form.name || !form.domain || !form.server_id || !form.user_id || !form.backend_addr) {
    ElMessage.error('请填写必填项')
    return
  }
  submitLoading.value = true
  try {
    if (editingId.value) {
      await updateWebsite(editingId.value, form)
      ElMessage.success('更新成功')
    } else {
      await createWebsite(form)
      ElMessage.success('创建成功')
    }
    showDialog.value = false
    fetchData()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '操作失败')
  } finally {
    submitLoading.value = false
  }
}

async function handleCheck(row: any) {
  row._checking = true
  try {
    const res = await checkWebsite(row.id)
    ElMessage.success(res.message || '检测完成')
    fetchData()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '检测失败')
  } finally {
    row._checking = false
  }
}

async function handleDelete(row: any) {
  await ElMessageBox.confirm(`确认删除网站 ${row.name}（${row.domain}）？`, '确认删除', { type: 'warning' })
  await deleteWebsite(row.id)
  ElMessage.success('删除成功')
  fetchData()
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
.filter-bar {
  display: flex;
  gap: var(--space-2);
  margin-bottom: var(--space-4);
}
</style>
