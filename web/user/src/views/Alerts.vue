<template>
  <div class="alerts-page">
    <el-card class="animate-in">
      <template #header>
        <div class="card-header">
          <span>告警通知</span>
          <el-switch v-model="unreadOnly" active-text="仅未读" @change="loadAlerts" />
        </div>
      </template>

      <el-table :data="alerts" stripe>
        <el-table-column prop="level" label="级别" width="100">
          <template #default="{ row }">
            <el-tag :type="row.level === 'error' ? 'danger' : row.level === 'warning' ? 'warning' : 'info'">
              {{ row.level === 'error' ? '错误' : row.level === 'warning' ? '警告' : '信息' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="130">
          <template #default="{ row }">
            <el-tag v-if="row.type === 'admin_message'" type="primary" size="small">系统通知</el-tag>
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
        <el-table-column prop="created_at" label="时间" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button v-if="!row.is_read" type="primary" link @click="markRead(row.id)">
              标为已读
            </el-button>
            <el-text v-else type="info">已读</el-text>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="alerts.length === 0" description="暂无告警" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getAlerts, markAlertRead } from '../api'
import { ElMessage } from 'element-plus'

const alerts = ref<any[]>([])
const unreadOnly = ref(false)

const typeMap: Record<string, string> = {
  server_down: '服务器离线',
  traffic_exceeded: '流量超额',
  traffic_warning: '流量警告',
  plan_expired: '套餐过期',
  plan_expiring: '套餐即将过期',
}

const formatTime = (time: string) => {
  return new Date(time).toLocaleString('zh-CN')
}

const loadAlerts = async () => {
  const res = await getAlerts({ unread_only: unreadOnly.value })
  alerts.value = res.data
}

const markRead = async (id: number) => {
  await markAlertRead(id)
  ElMessage.success('已标记为已读')
  await loadAlerts()
}

onMounted(loadAlerts)
</script>

<style scoped>
.card-header { display: flex; justify-content: space-between; align-items: center; }
</style>
