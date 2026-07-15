<template>
  <div class="home">
    <!-- Announcement dialog -->
    <el-dialog v-model="showAnnouncement" :title="currentAnnouncement.title || '站点公告'" width="460" append-to-body :close-on-click-modal="false">
      <div class="announcement-body">
        <div class="announcement-text">{{ currentAnnouncement.content }}</div>
      </div>
      <template #footer>
        <el-button type="primary" @click="dismissAnnouncement">我知道了</el-button>
      </template>
    </el-dialog>

    <!-- Stat cards -->
    <div class="stat-grid">
      <div class="stat-card animate-in animate-in-delay-1">
        <div class="stat-icon stat-icon--blue">
          <el-icon :size="22"><Connection /></el-icon>
        </div>
        <div class="stat-body">
          <div class="stat-label">活跃代理</div>
          <div class="stat-value">{{ proxyStats.running }} / {{ proxyStats.total }}</div>
        </div>
      </div>

      <div class="stat-card animate-in animate-in-delay-2">
        <div class="stat-icon stat-icon--green">
          <el-icon :size="22"><Goods /></el-icon>
        </div>
        <div class="stat-body">
          <div class="stat-label">当前套餐</div>
          <div class="stat-value">{{ planName }}</div>
          <div v-if="planExpiry" class="stat-detail">{{ planExpiry }}</div>
        </div>
      </div>

      <div class="stat-card animate-in animate-in-delay-3">
        <div class="stat-icon stat-icon--purple">
          <el-icon :size="22"><DataLine /></el-icon>
        </div>
        <div class="stat-body">
          <div class="stat-label">本月流量</div>
          <div class="stat-value">{{ formatTraffic(monthlyTraffic) }}</div>
        </div>
      </div>

      <div class="stat-card animate-in animate-in-delay-4">
        <div class="stat-icon stat-icon--amber">
          <el-icon :size="22"><Coin /></el-icon>
        </div>
        <div class="stat-body">
          <div class="stat-label">账户余额</div>
          <div class="stat-value">&yen;{{ balance.toFixed(2) }}</div>
        </div>
      </div>
    </div>

    <!-- Quick actions -->
    <el-card class="animate-in animate-in-delay-4">
      <template #header>快捷操作</template>
      <div class="actions">
        <el-button type="primary" @click="$router.push('/proxies/create')">
          <el-icon><Plus /></el-icon>创建代理
        </el-button>
        <el-button @click="$router.push('/plans')">
          <el-icon><Goods /></el-icon>{{ hasActivePlan ? '续费套餐' : '升级套餐' }}
        </el-button>
        <el-button @click="$router.push('/proxies')">
          <el-icon><Connection /></el-icon>管理代理
        </el-button>
        <el-button @click="$router.push('/invite')">
          <el-icon><Share /></el-icon>邀请好友
        </el-button>
      </div>
    </el-card>

    <!-- Two-column: Quota + Orders -->
    <div class="two-col">
      <!-- Plan quota -->
      <el-card class="animate-in animate-in-delay-4">
        <template #header>
          <div class="page-header">
            <span class="page-title">套餐配额</span>
            <el-button size="small" @click="$router.push('/plans')">升级套餐 <el-icon><ArrowRight /></el-icon></el-button>
          </div>
        </template>
        <div class="quota-list">
          <div class="quota-item">
            <div class="quota-info">
              <span class="quota-label">代理数量</span>
              <span class="quota-value">{{ proxyStats.total }} / {{ planLimits.maxProxies }}</span>
            </div>
            <el-progress :percentage="proxyPercent" :color="getQuotaColor(proxyPercent)" :stroke-width="8" />
          </div>
          <div class="quota-item">
            <div class="quota-info">
              <span class="quota-label">本月流量</span>
              <span class="quota-value">{{ formatTraffic(monthlyTraffic) }} / {{ formatTraffic(planLimits.maxTraffic) }}</span>
            </div>
            <el-progress :percentage="trafficPercent" :color="getQuotaColor(trafficPercent)" :stroke-width="8" />
          </div>
          <div class="quota-item">
            <div class="quota-info">
              <span class="quota-label">带宽上限</span>
              <span class="quota-value">{{ formatBandwidth(planLimits.maxBandwidth) }}</span>
            </div>
          </div>
        </div>
      </el-card>

      <!-- Recent orders -->
      <el-card class="animate-in animate-in-delay-4">
        <template #header>
          <div class="page-header">
            <span class="page-title">最近订单</span>
            <el-button size="small" @click="$router.push('/orders')">查看全部 <el-icon><ArrowRight /></el-icon></el-button>
          </div>
        </template>
        <el-table v-if="recentOrders.length" :data="recentOrders" size="small" stripe :show-header="true">
          <el-table-column label="订单号" min-width="140">
            <template #default="{ row }"><span class="text-mono text-sm">{{ row.order_no }}</span></template>
          </el-table-column>
          <el-table-column label="类型" width="70">
            <template #default="{ row }">
              <el-tag size="small" :type="row.order_type === 'recharge' ? 'info' : ''">{{ row.order_type === 'recharge' ? '充值' : '套餐' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="金额" width="90">
            <template #default="{ row }"><span class="text-mono">&yen;{{ row.amount?.toFixed(2) }}</span></template>
          </el-table-column>
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-tag size="small" :type="row.pay_status === 'paid' ? 'success' : row.pay_status === 'refunded' ? 'danger' : 'warning'">
                {{ orderStatusMap[row.pay_status] || row.pay_status }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else description="暂无订单" :image-size="80" />
      </el-card>
    </div>

    <!-- Available coupons -->
    <el-card v-if="availableCoupons.length" class="animate-in animate-in-delay-4">
      <template #header>
        <div class="page-header">
          <span class="page-title">可用优惠券</span>
        </div>
      </template>
      <div class="coupon-list">
        <div v-for="coupon in availableCoupons.slice(0, 3)" :key="coupon.id" class="coupon-item">
          <div class="coupon-left">
            <div class="coupon-amount">&yen;{{ coupon.discount_value.toFixed(0) }}</div>
            <div class="coupon-label">优惠券</div>
          </div>
          <div class="coupon-right">
            <code class="coupon-code">{{ coupon.code }}</code>
            <div class="coupon-meta" v-if="coupon.end_time">到期 {{ formatDate(coupon.end_time) }}</div>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { getProfile, getProxies, getTrafficStats, getOrders, getMyAvailableCoupons, getActiveAnnouncements } from '../api'

const proxyStats = ref({ total: 0, running: 0 })
const planName = ref('免费版')
const planExpiry = ref('')
const balance = ref(0)
const hasActivePlan = ref(false)
const monthlyTraffic = ref(0)
const planLimits = ref({ maxProxies: 5, maxTraffic: 0, maxBandwidth: 0 })
const recentOrders = ref<any[]>([])
const availableCoupons = ref<any[]>([])
const currentAnnouncement = ref<any>({})
const showAnnouncement = ref(false)

const orderStatusMap: Record<string, string> = { paid: '已支付', refunded: '已退款', pending: '待支付', expired: '已过期' }

const proxyPercent = computed(() => {
  if (!planLimits.value.maxProxies) return 0
  return Math.min(Math.round((proxyStats.value.total / planLimits.value.maxProxies) * 100), 100)
})

const trafficPercent = computed(() => {
  if (!planLimits.value.maxTraffic) return 0
  return Math.min(Math.round((monthlyTraffic.value / planLimits.value.maxTraffic) * 100), 100)
})

function formatTraffic(bytes: number): string {
  if (!bytes) return '0 B'
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB'
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB'
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return bytes + ' B'
}

function formatBandwidth(bytesPerSec: number): string {
  if (!bytesPerSec) return '无限制'
  if (bytesPerSec >= 1048576) return (bytesPerSec / 1048576).toFixed(0) + ' MB/s'
  if (bytesPerSec >= 1024) return (bytesPerSec / 1024).toFixed(0) + ' KB/s'
  return bytesPerSec + ' B/s'
}

function formatDate(date: string): string {
  if (!date) return '-'
  return new Date(date).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

function getQuotaColor(percent: number): string {
  if (percent >= 100) return '#ef4444'
  if (percent >= 80) return '#f59e0b'
  return '#2563eb'
}

function dismissAnnouncement() {
  showAnnouncement.value = false
  const dismissed = JSON.parse(localStorage.getItem('dismissed_announcements') || '[]')
  dismissed.push(currentAnnouncement.value.id)
  localStorage.setItem('dismissed_announcements', JSON.stringify(dismissed))
}

onMounted(async () => {
  const [profileRes, proxiesRes, trafficRes, ordersRes, couponsRes, announcementsRes] = await Promise.allSettled([
    getProfile(),
    getProxies({ size: 1000 }),
    getTrafficStats(),
    getOrders({ size: 5 }),
    getMyAvailableCoupons(),
    getActiveAnnouncements(),
  ])

  if (profileRes.status === 'fulfilled') {
    const user = profileRes.value.data
    balance.value = user.balance || 0
    planName.value = user.plan?.name || '免费版'
    if (user.plan_id && user.plan_expires_at) {
      const expires = new Date(user.plan_expires_at)
      const now = new Date()
      if (expires > now) {
        hasActivePlan.value = true
        const days = Math.ceil((expires.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))
        planExpiry.value = `${days} 天后到期`
      } else {
        planExpiry.value = '已过期'
      }
    }
    if (user.plan) {
      planLimits.value = {
        maxProxies: user.plan.max_proxies || 5,
        maxTraffic: user.plan.max_traffic || 0,
        maxBandwidth: user.plan.max_bandwidth || 0,
      }
    }
  }

  if (proxiesRes.status === 'fulfilled') {
    const proxies = proxiesRes.value.data.list || []
    proxyStats.value.total = proxies.length
    proxyStats.value.running = proxies.filter((p: any) => p.status === 'running').length
  }

  if (trafficRes.status === 'fulfilled') {
    const data = trafficRes.value.data
    monthlyTraffic.value = (data.monthly?.traffic_in || 0) + (data.monthly?.traffic_out || 0)
    if (data.plan) {
      planLimits.value.maxTraffic = data.plan.max_traffic || planLimits.value.maxTraffic
      planLimits.value.maxBandwidth = data.plan.max_bandwidth || planLimits.value.maxBandwidth
      planLimits.value.maxProxies = data.plan.max_proxies || planLimits.value.maxProxies
    }
  }

  if (ordersRes.status === 'fulfilled') {
    recentOrders.value = ordersRes.value.data.list || []
  }

  if (couponsRes.status === 'fulfilled') {
    availableCoupons.value = couponsRes.value.data || []
  }

  if (announcementsRes.status === 'fulfilled') {
    const announcements = announcementsRes.value.data || []
    if (announcements.length > 0) {
      const dismissed = JSON.parse(localStorage.getItem('dismissed_announcements') || '[]')
      const first = announcements[0]
      if (!dismissed.includes(first.id)) {
        currentAnnouncement.value = first
        showAnnouncement.value = true
      }
    }
  }
})
</script>

<style scoped>
.home {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

/* ---- Announcement ---- */
.announcement-body {
  padding: var(--space-2) 0;
}

.announcement-text {
  font-size: var(--text-md);
  color: var(--color-text);
  line-height: 1.7;
  white-space: pre-wrap;
}

/* ---- Stat grid ---- */
.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-4);
}

@media (max-width: 900px) {
  .stat-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

/* ---- Quick actions ---- */
.actions {
  display: flex;
  gap: var(--space-3);
  flex-wrap: wrap;
}

/* ---- Plan quota ---- */
.quota-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.quota-item {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.quota-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.quota-label {
  font-size: var(--text-sm);
  color: var(--color-text-secondary);
  font-weight: var(--font-medium);
}

.quota-value {
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  color: var(--color-text);
  font-family: var(--font-mono);
}

/* ---- Two column layout ---- */
.two-col {
  display: grid;
  grid-template-columns: 1fr 360px;
  gap: var(--space-5);
}

@media (max-width: 900px) {
  .two-col {
    grid-template-columns: 1fr;
  }
}

/* ---- Coupons ---- */
.coupon-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.coupon-item {
  display: flex;
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-lg);
  overflow: hidden;
  transition: border-color var(--transition-fast);
}

.coupon-item:hover {
  border-color: var(--color-border);
}

.coupon-left {
  width: 90px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  background: var(--color-bg);
  border-right: 1px dashed var(--color-border-light);
  padding: var(--space-3);
  flex-shrink: 0;
}

.coupon-amount {
  font-size: var(--text-xl);
  font-weight: var(--font-bold);
  color: var(--color-accent);
}

.coupon-label {
  font-size: var(--text-xs);
  color: var(--color-text-muted);
}

.coupon-right {
  flex: 1;
  padding: var(--space-3) var(--space-4);
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: var(--space-1);
  min-width: 0;
}

.coupon-code {
  font-family: var(--font-mono);
  font-size: var(--text-sm);
  color: var(--color-text);
}

.coupon-meta {
  font-size: var(--text-xs);
  color: var(--color-text-muted);
}
</style>
