<template>
  <div v-loading="loading">
    <div class="detail-header">
      <el-button text @click="$router.back()">
        <el-icon><ArrowLeft /></el-icon>返回
      </el-button>
      <span class="detail-title">{{ user.email }}</span>
      <el-tag :type="user.role === 'super_admin' ? 'danger' : user.role === 'admin' ? 'warning' : ''" size="small">
        {{ roleMap[user.role] || user.role }}
      </el-tag>
      <el-tag :type="user.status === 'active' ? 'success' : 'danger'" size="small">
        {{ user.status === 'active' ? '正常' : '封禁' }}
      </el-tag>
      <div class="header-actions">
        <el-button size="small" @click="showRecharge = true">充值</el-button>
        <el-button size="small" type="danger" @click="handleBan" v-if="user.status === 'active'">封禁</el-button>
        <el-button size="small" @click="handleUnban" v-else>解封</el-button>
      </div>
    </div>

    <div class="info-row">
      <el-card class="animate-in info-card">
        <div class="info-label">余额</div>
        <div class="info-value text-mono">&yen;{{ user.balance?.toFixed(2) }}</div>
      </el-card>
      <el-card class="animate-in info-card">
        <div class="info-label">套餐</div>
        <div class="info-value">{{ user.plan?.name || '免费版' }}</div>
      </el-card>
      <el-card class="animate-in info-card">
        <div class="info-label">代理数</div>
        <div class="info-value">{{ proxyCount }}</div>
      </el-card>
      <el-card class="animate-in info-card">
        <div class="info-label">注册时间</div>
        <div class="info-value" style="font-size: 14px">{{ formatDate(user.created_at) }}</div>
      </el-card>
    </div>

    <div class="detail-grid">
      <el-card class="animate-in animate-in-delay-1">
        <template #header>用户信息</template>
        <el-descriptions :column="1" border size="small">
          <el-descriptions-item label="邮箱">{{ user.email }}</el-descriptions-item>
          <el-descriptions-item label="套餐到期">{{ user.plan_expires_at || '-' }}</el-descriptions-item>
          <el-descriptions-item label="邀请码">
            <span class="text-mono">{{ user.invite_code }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="API Key">
            <span class="text-mono" style="font-size: 12px">{{ user.api_key }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <el-card class="animate-in animate-in-delay-1">
        <template #header>代理列表 ({{ proxyCount }})</template>
        <el-table v-if="proxies.length" :data="proxies" size="small" stripe>
          <el-table-column prop="name" label="名称" />
          <el-table-column prop="type" label="类型" width="80">
            <template #default="{ row }"><el-tag size="small">{{ row.type?.toUpperCase() }}</el-tag></template>
          </el-table-column>
          <el-table-column label="远程地址" min-width="160">
            <template #default="{ row }"><span class="text-mono">{{ row.remote_addr || row.local_ip + ':' + row.local_port }}</span></template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="80">
            <template #default="{ row }">
              <el-tag :type="row.status === 'running' ? 'success' : row.status === 'error' ? 'danger' : 'warning'" size="small">
                {{ statusMap[row.status] || row.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="流量" width="140">
            <template #default="{ row }">
              <span class="text-mono" style="font-size: 12px">&darr;{{ formatBytes(row.traffic_in) }} &uarr;{{ formatBytes(row.traffic_out) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="server.name" label="服务器" width="100" />
        </el-table>
        <el-empty v-else description="暂无代理" :image-size="48" />
      </el-card>
    </div>

    <!-- Recharge Dialog -->
    <el-dialog v-model="showRecharge" title="管理员充值" width="420" append-to-body>
      <div class="recharge-preview">
        <div class="recharge-user">{{ user.email }}</div>
        <div class="recharge-balance">当前余额 <b>&yen;{{ user.balance?.toFixed(2) }}</b></div>
      </div>
      <el-form label-width="80px" style="margin-top: 16px">
        <el-form-item label="充值金额">
          <div class="amount-btns">
            <el-button v-for="a in [10, 50, 100, 200]" :key="a" size="small" :type="rechargeAmount === a ? 'primary' : ''" @click="rechargeAmount = a">&yen;{{ a }}</el-button>
          </div>
          <el-input-number v-model="rechargeAmount" :min="0.01" :precision="2" :step="10" style="width: 100%; margin-top: 8px" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="rechargeRemark" placeholder="可选" />
        </el-form-item>
      </el-form>
      <div class="recharge-after">
        充值后余额：<b>&yen;{{ ((user.balance || 0) + rechargeAmount).toFixed(2) }}</b>
      </div>
      <template #footer>
        <el-button @click="showRecharge = false">取消</el-button>
        <el-button type="primary" @click="handleRecharge" :loading="recharging">确认充值</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { getUser, getProxies, rechargeBalance, banUser, unbanUser } from '../../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const user = ref<any>({})
const proxyCount = ref(0)
const proxies = ref<any[]>([])
const loading = ref(false)

const roleMap: Record<string, string> = {
  super_admin: '超级管理员', admin: '管理员', user: '普通用户',
}

const statusMap: Record<string, string> = {
  running: '运行中', stopped: '已停止', pending: '待启用', error: '异常',
}

const showRecharge = ref(false)
const rechargeAmount = ref(100)
const rechargeRemark = ref('')
const recharging = ref(false)

onMounted(async () => {
  loading.value = true
  try {
    const userId = Number(route.params.id)
    const [userRes, proxiesRes] = await Promise.all([
      getUser(userId),
      getProxies({ user_id: userId, size: 100 }),
    ])
    user.value = userRes.data.user
    proxyCount.value = userRes.data.proxy_count
    proxies.value = proxiesRes.data?.list || []
  } finally {
    loading.value = false
  }
})

async function handleBan() {
  await ElMessageBox.confirm(`确认封禁用户 ${user.value.email}？`, '确认封禁', { type: 'warning' })
  await banUser(user.value.id)
  user.value.status = 'banned'
  ElMessage.success('用户已封禁')
}

async function handleUnban() {
  await unbanUser(user.value.id)
  user.value.status = 'active'
  ElMessage.success('用户已解封')
}

function formatDate(d: string) {
  if (!d) return '-'
  return new Date(d).toLocaleString('zh-CN')
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

async function handleRecharge() {
  try {
    await ElMessageBox.confirm(`确认为 ${user.value.email} 充值 &yen;${rechargeAmount.value.toFixed(2)}？`, '确认充值')
  } catch {
    return
  }
  recharging.value = true
  try {
    await rechargeBalance({
      user_id: user.value.id,
      amount: rechargeAmount.value,
      remark: rechargeRemark.value || undefined,
    })
    ElMessage.success('充值成功')
    user.value.balance += rechargeAmount.value
    showRecharge.value = false
    rechargeRemark.value = ''
  } catch (e: any) {
    ElMessage.error(e?.message || '充值失败')
  } finally {
    recharging.value = false
  }
}
</script>

<style scoped>
.detail-header {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-5);
}
.detail-title {
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}
.header-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}
.info-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-4);
  margin-bottom: var(--space-5);
}
.info-card {
  text-align: center;
}
.info-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 4px;
}
.info-value {
  font-size: 18px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.detail-grid {
  display: grid;
  grid-template-columns: 320px 1fr;
  gap: var(--space-4);
}
.recharge-preview {
  text-align: center;
  padding: var(--space-4);
  background: var(--color-bg);
  border-radius: var(--radius-lg);
}
.recharge-user {
  font-size: var(--text-md);
  font-weight: var(--font-semibold);
  color: var(--color-text);
  margin-bottom: var(--space-1);
}
.recharge-balance {
  font-size: var(--text-sm);
  color: var(--color-text-secondary);
}
.recharge-balance b {
  color: var(--color-text);
  font-size: var(--text-lg);
}
.amount-btns {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.recharge-after {
  text-align: center;
  padding: var(--space-3);
  background: var(--color-success-light);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  color: var(--color-success);
  margin-top: var(--space-3);
}
.recharge-after b {
  color: var(--color-success);
  font-size: var(--text-lg);
}
</style>
