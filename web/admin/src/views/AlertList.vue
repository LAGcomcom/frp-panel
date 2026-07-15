<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">告警列表</span>
        <el-button type="primary" @click="openSend">
          <el-icon><Promotion /></el-icon>发送通知
        </el-button>
      </div>
    </template>

    <el-table :data="alerts" stripe>
      <el-table-column prop="level" label="级别" width="80">
        <template #default="{ row }">
          <el-tag :type="row.level === 'error' ? 'danger' : row.level === 'warning' ? 'warning' : 'info'" size="small">
            {{ levelMap[row.level] || row.level }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="130">
        <template #default="{ row }">
          <el-tag v-if="row.type === 'admin_message'" type="primary" size="small">管理员通知</el-tag>
          <span v-else>{{ typeMap[row.type] || row.type }}</span>
        </template>
      </el-table-column>
      <el-table-column label="标题" width="180">
        <template #default="{ row }">
          <span v-if="row.title" style="font-weight: 500">{{ row.title }}</span>
          <span v-else class="text-muted">-</span>
        </template>
      </el-table-column>
      <el-table-column prop="message" label="内容" />
      <el-table-column prop="user_id" label="用户" width="80">
        <template #default="{ row }">
          <span v-if="row.user_id">{{ row.user_id }}</span>
          <el-tag v-else size="small" type="warning">全部</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="is_read" label="状态" width="70">
        <template #default="{ row }">
          <el-tag :type="row.is_read ? 'info' : 'danger'" size="small">
            {{ row.is_read ? '已读' : '未读' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="page"
      :page-size="20"
      :total="total"
      layout="total, prev, pager, next"
      @current-change="loadAlerts"
    />

    <!-- Send Notification Dialog -->
    <el-dialog v-model="showSend" title="发送通知" width="500" append-to-body>
      <el-form :model="sendForm" label-width="80px">
        <el-form-item label="接收人">
          <el-select v-model="sendForm.user_id" placeholder="全部用户" clearable style="width: 100%">
            <el-option :value="0" label="全部用户" />
            <el-option v-for="u in users" :key="u.id" :value="u.id" :label="u.email" />
          </el-select>
        </el-form-item>
        <el-form-item label="标题" required>
          <el-input v-model="sendForm.title" placeholder="通知标题" />
        </el-form-item>
        <el-form-item label="内容" required>
          <el-input v-model="sendForm.message" type="textarea" :rows="4" placeholder="通知内容" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showSend = false">取消</el-button>
        <el-button type="primary" @click="handleSend" :loading="sending">发送</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { getAlerts, sendNotification, getUsers } from '../api'
import { ElMessage } from 'element-plus'

const alerts = ref<any[]>([])
const page = ref(1)
const total = ref(0)
const showSend = ref(false)
const sending = ref(false)
const users = ref<any[]>([])

const sendForm = reactive({
  user_id: 0 as number,
  title: '',
  message: '',
})

const levelMap: Record<string, string> = { error: '错误', warning: '警告', info: '信息' }

const typeMap: Record<string, string> = {
  server_down: '服务器离线',
  traffic_exceeded: '流量超额',
  traffic_warning: '流量警告',
  plan_expired: '套餐过期',
  plan_expiring: '套餐即将过期',
  admin_message: '管理员通知',
}

const formatTime = (time: string) => new Date(time).toLocaleString('zh-CN')

const loadAlerts = async () => {
  const res = await getAlerts({ page: page.value, size: 20 })
  alerts.value = res.data.list
  total.value = res.data.total
}

const openSend = async () => {
  sendForm.user_id = 0
  sendForm.title = ''
  sendForm.message = ''
  showSend.value = true
  // Load users for target selection
  if (users.value.length === 0) {
    try {
      const res = await getUsers({ size: 100 })
      users.value = res.data.list || res.data
    } catch {}
  }
}

const handleSend = async () => {
  if (!sendForm.title || !sendForm.message) {
    ElMessage.warning('请填写标题和内容')
    return
  }
  sending.value = true
  try {
    await sendNotification({
      user_id: sendForm.user_id || undefined,
      title: sendForm.title,
      message: sendForm.message,
    })
    ElMessage.success('通知已发送')
    showSend.value = false
    await loadAlerts()
  } finally {
    sending.value = false
  }
}

onMounted(loadAlerts)
</script>

<style scoped>
.text-muted {
  color: var(--color-text-muted);
}
</style>
