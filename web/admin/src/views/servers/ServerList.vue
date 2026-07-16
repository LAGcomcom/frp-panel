<template>
  <div>
    <el-card class="animate-in">
      <template #header>
        <div class="page-header">
          <span class="page-title">服务器列表</span>
          <el-button type="primary" @click="showAdd = true">
            <el-icon><Plus /></el-icon>添加服务器
          </el-button>
        </div>
      </template>

      <el-table :data="servers" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="ip" label="IP 地址" />
        <el-table-column prop="region" label="地区" width="80" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 'running' ? 'success' : row.status === 'error' ? 'danger' : 'warning'" size="small">
              {{ statusMap[row.status] || row.status }}
            </el-tag>
          </template>
        </el-table-column>
		<el-table-column label="鉴权" width="100">
		  <template #default="{ row }">
			<el-tag :type="row.plugin_auth_enabled ? 'success' : 'warning'" size="small">
			  {{ row.plugin_auth_enabled ? '安全模式' : '需重新部署' }}
			</el-tag>
		  </template>
		</el-table-column>
        <el-table-column label="延迟" width="80">
          <template #default="{ row }">
            <span v-if="row.latency > 0" class="text-mono" :style="{ color: row.latency < 100 ? '#1a7f37' : row.latency < 300 ? '#9a6700' : '#cf222e' }">{{ row.latency }}ms</span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="client_count" label="客户端" width="80" />
        <el-table-column prop="proxy_count" label="代理数" width="80" />
        <el-table-column label="操作" width="200" align="center" fixed="right">
          <template #default="{ row }">
            <div class="action-btns">
              <el-button size="small" @click="$router.push(`/servers/${row.id}`)">详情</el-button>
              <el-button size="small" @click="handleRestart(row)" :disabled="row.status !== 'running'">重启</el-button>
              <el-button size="small" type="danger" @click="handleStop(row)" :disabled="row.status !== 'running'">停止</el-button>
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

    <!-- Add Server Dialog -->
    <el-dialog v-model="showAdd" title="添加服务器" width="500">
      <el-form :model="addForm" label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="addForm.name" placeholder="服务器名称" />
        </el-form-item>
        <el-form-item label="IP 地址" required>
          <el-input v-model="addForm.ip" placeholder="服务器 IP" />
        </el-form-item>
        <el-form-item label="SSH 端口">
          <el-input-number v-model="addForm.ssh_port" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="SSH 用户">
          <el-input v-model="addForm.ssh_user" />
        </el-form-item>
        <el-form-item label="认证方式" required>
          <el-radio-group v-model="addForm.ssh_auth_type">
            <el-radio value="password">密码</el-radio>
            <el-radio value="key">密钥</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="密码" v-if="addForm.ssh_auth_type === 'password'">
          <el-input v-model="addForm.ssh_password" type="password" show-password />
        </el-form-item>
        <el-form-item label="私钥" v-if="addForm.ssh_auth_type === 'key'">
          <el-input v-model="addForm.ssh_private_key" type="textarea" :rows="4" />
        </el-form-item>
        <el-form-item label="地区">
          <el-input v-model="addForm.region" placeholder="如：US、CN、HK" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAdd = false">取消</el-button>
        <el-button type="primary" @click="handleAdd" :loading="addLoading">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { getServers, createServer, restartServer, stopServer } from '../../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const servers = ref<any[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const showAdd = ref(false)
const addLoading = ref(false)

const statusMap: Record<string, string> = {
  pending: '待部署', installing: '安装中', running: '运行中', stopped: '已停止', error: '异常',
}

const addForm = reactive({
  name: '', ip: '', ssh_port: 22, ssh_user: 'root',
  ssh_auth_type: 'password', ssh_password: '', ssh_private_key: '', region: '',
})

onMounted(() => fetchData())

async function fetchData() {
  loading.value = true
  try {
    const res = await getServers({ page: page.value, size: pageSize.value })
    servers.value = res.data.list
    total.value = res.data.total
  } finally {
    loading.value = false
  }
}

async function handleAdd() {
  addLoading.value = true
  try {
    await createServer(addForm)
    ElMessage.success('服务器创建成功')
    showAdd.value = false
    fetchData()
  } finally {
    addLoading.value = false
  }
}

async function handleRestart(row: any) {
  await ElMessageBox.confirm(`确认重启 ${row.name} 上的 frps？`, '确认重启')
  await restartServer(row.id)
  ElMessage.success('重启指令已发送')
}

async function handleStop(row: any) {
  await ElMessageBox.confirm(`确认停止 ${row.name} 上的 frps？`, '确认停止')
  await stopServer(row.id)
  ElMessage.success('停止指令已发送')
  row.status = 'stopped'
}
</script>

<style scoped>
/* page-header and page-title are defined in design-system.css */
</style>
