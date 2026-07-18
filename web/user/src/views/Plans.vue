<template>
  <div>
    <div class="plans-grid">
      <div v-for="(plan, idx) in plans" :key="plan.id" class="plan-card animate-in"
        :class="{ 'plan-featured': plan.price_monthly > 0, 'plan-current': currentPlanId === plan.id }"
        :style="{ animationDelay: idx * 0.08 + 's' }">
        <div v-if="currentPlanId === plan.id" class="plan-badge plan-badge--current">当前套餐</div>
        <div v-else-if="plan.price_monthly > 0" class="plan-badge">推荐</div>
        <div class="plan-header">
          <h3 class="plan-name">{{ plan.name }}</h3>
          <p class="plan-desc">{{ plan.description }}</p>
        </div>
        <div class="plan-price">
          <span class="price-currency">&yen;</span>
          <span class="price-amount">{{ plan.price_monthly }}</span>
          <span class="price-period">/月</span>
        </div>
        <div class="plan-prices-row">
          <div class="price-option">
            <span class="price-option-label">季付</span>
            <span class="price-option-value">&yen;{{ plan.price_quarterly }}</span>
          </div>
          <div class="price-option">
            <span class="price-option-label">年付</span>
            <span class="price-option-value">&yen;{{ plan.price_yearly }}</span>
          </div>
        </div>
        <ul class="plan-features">
          <li>{{ plan.max_proxies }} 个代理</li>
          <li>{{ formatBytes(plan.max_bandwidth) }}/s 带宽</li>
          <li>{{ formatBytes(plan.max_traffic) }} 流量</li>
          <li>{{ plan.max_ports }} 个端口</li>
          <li>{{ plan.duration_days }} 天有效期</li>
        </ul>
        <el-button type="primary" style="width: 100%" @click="handleBuy(plan)">
          {{ currentPlanId === plan.id ? '续费' : '立即购买' }}
        </el-button>
      </div>
    </div>

    <!-- Order dialog -->
    <el-dialog v-model="showBuy" title="确认订单" width="440" append-to-body :close-on-click-modal="false">
      <div class="order-summary">
        <div class="order-plan-name">{{ selectedPlan?.name }}</div>
        <el-radio-group v-model="durationType" size="small" style="margin: 12px 0">
          <el-radio-button value="monthly">月付</el-radio-button>
          <el-radio-button value="quarterly">季付</el-radio-button>
          <el-radio-button value="yearly">年付</el-radio-button>
        </el-radio-group>
      </div>

      <el-alert v-if="purchaseNotice" :title="purchaseNotice" type="info" :closable="false" show-icon
        style="margin-bottom: 16px" />

      <div class="order-section">
        <div class="order-section-label">优惠码</div>
        <div class="coupon-row">
          <el-input v-model="couponCode" placeholder="输入优惠码（可选）" clearable size="small" @clear="resetCoupon" />
          <el-button size="small" type="primary" plain @click="handleVerifyCoupon" :loading="verifying" :disabled="!couponCode">
            验证
          </el-button>
        </div>
        <div v-if="couponVerified" class="coupon-result coupon-result--success">
          <el-icon><CircleCheck /></el-icon>
          <span>{{ couponDesc }}</span>
        </div>
        <div v-if="couponError" class="coupon-result coupon-result--error">
          <el-icon><CircleClose /></el-icon>
          <span>{{ couponError }}</span>
        </div>
      </div>

      <div class="order-detail">
		<div class="order-row">
		  <span class="order-label">有效期</span>
		  <span class="order-value">{{ selectedDurationDays }} 天</span>
		</div>
        <div class="order-row">
          <span class="order-label">原价</span>
          <span class="order-value">&yen;{{ originalPrice.toFixed(2) }}</span>
        </div>
        <div class="order-row" v-if="couponDiscount > 0">
          <span class="order-label">优惠码折扣</span>
          <span class="order-value text-success">-&yen;{{ couponDiscount.toFixed(2) }}</span>
        </div>
        <el-divider style="margin: 8px 0" />
        <div class="order-row order-row--total">
          <span class="order-label">应付</span>
          <span class="order-value order-total">&yen;{{ finalPrice.toFixed(2) }}</span>
        </div>
      </div>

      <div class="order-section">
        <div class="order-section-label">支付方式</div>
        <el-radio-group v-model="payMethod" class="pay-method-group">
          <el-radio value="balance" class="pay-method-option">
            <div class="pay-method-info">
              <span class="pay-method-icon pay-icon-balance">余</span>
              <span>余额支付</span>
              <span class="pay-balance-amount">&yen;{{ userBalance.toFixed(2) }}</span>
            </div>
          </el-radio>
          <el-radio v-for="m in paymentMethods" :key="m.type" :value="m.type" class="pay-method-option">
            <div class="pay-method-info">
              <span class="pay-method-icon" :class="'pay-icon-' + m.type">{{ methodIcon[m.type] }}</span>
              <span>{{ m.name }}</span>
            </div>
          </el-radio>
        </el-radio-group>
        <div v-if="payMethod === 'balance' && finalPrice > userBalance" class="pay-insufficient">
          余额不足，请选择其他支付方式或先充值
        </div>
      </div>

      <template #footer>
        <el-button @click="showBuy = false" size="large">取消</el-button>
        <el-button type="primary" @click="handleOrder" :loading="ordering"
          :disabled="payMethod === 'balance' && finalPrice > userBalance" size="large">
          {{ finalPrice <= 0 ? '免费开通' : (payMethod === 'balance' ? '确认支付' : '去支付') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- QR Code payment dialog -->
    <el-dialog v-model="showPayDialog" title="扫码支付" width="380" append-to-body :close-on-click-modal="false" :close-on-press-escape="false">
      <div class="pay-dialog-content">
        <div v-if="payQRCode" class="pay-qr">
          <img :src="payQRCode" alt="支付二维码" class="pay-qr-img" />
        </div>
        <div v-if="payURL && !payQRCode" class="pay-redirect">
          <p>点击下方按钮前往支付</p>
          <el-button type="primary" @click="openPayURL">前往支付</el-button>
        </div>
        <div class="pay-amount-display">
          <span class="pay-amount-label">支付金额</span>
          <span class="pay-amount-value">&yen;{{ payAmount.toFixed(2) }}</span>
        </div>
        <div class="pay-countdown" v-if="payCountdown > 0">
          支付剩余时间：{{ formatCountdown(payCountdown) }}
        </div>
        <div class="pay-status-checking" v-if="checkingStatus">
          <el-icon class="is-loading"><Loading /></el-icon>
          <span>正在确认支付状态...</span>
        </div>
      </div>
      <template #footer>
        <el-button @click="cancelPayDialog">取消支付</el-button>
        <el-button type="primary" @click="checkPayStatus" :loading="checkingStatus">
          我已完成支付
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, watch, onUnmounted } from 'vue'
import { getPlans, createOrder, getProfile, verifyCoupon, getPaymentMethods, getOrder } from '../api'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'

const router = useRouter()
const plans = ref<any[]>([])
const showBuy = ref(false)
const selectedPlan = ref<any>(null)
const durationType = ref('monthly')
const couponCode = ref('')
const ordering = ref(false)
const userBalance = ref(0)
const currentPlanId = ref<number | null>(null)
const currentPlanName = ref('')
const verifying = ref(false)
const couponVerified = ref(false)
const couponError = ref('')
const couponDiscount = ref(0)
const couponDesc = ref('')
const payMethod = ref('balance')
const paymentMethods = ref<any[]>([])

// QR code payment dialog
const showPayDialog = ref(false)
const payQRCode = ref('')
const payURL = ref('')
const payAmount = ref(0)
const payOrderId = ref<number | null>(null)
const payCountdown = ref(0)
const checkingStatus = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null

const methodIcon: Record<string, string> = {
  alipay: '支', wechat: '微', usdt: '$', epay: '易',
}

const originalPrice = computed(() => {
  if (!selectedPlan.value) return 0
  const p = selectedPlan.value
  switch (durationType.value) {
    case 'monthly': return p.price_monthly
    case 'quarterly': return p.price_quarterly
    case 'yearly': return p.price_yearly
    default: return p.price_monthly
  }
})

const finalPrice = computed(() => Math.max(0, originalPrice.value - couponDiscount.value))
const selectedDurationDays = computed(() => {
  const baseDays = Math.max(1, Number(selectedPlan.value?.duration_days || 30))
  if (durationType.value === 'quarterly') return baseDays * 3
  if (durationType.value === 'yearly') return baseDays * 12
  return baseDays
})
const purchaseNotice = computed(() => {
  if (!currentPlanId.value || !selectedPlan.value) return ''
  if (currentPlanId.value === selectedPlan.value.id) {
    return `续费时长会叠加到当前 ${currentPlanName.value || '套餐'} 的到期时间之后。`
  }
  return `当前 ${currentPlanName.value || '套餐'} 和已经排队的套餐都不会被覆盖；新套餐将按购买顺序自动生效。`
})

// Re-verify coupon when duration type changes
watch(durationType, () => {
  if (couponVerified.value && couponCode.value && selectedPlan.value) {
    handleVerifyCoupon()
  }
})

onMounted(async () => {
  const [plansRes, profileRes, methodsRes] = await Promise.all([getPlans(), getProfile(), getPaymentMethods()])
  plans.value = plansRes.data
  userBalance.value = profileRes.data.balance || 0
  currentPlanId.value = profileRes.data.plan_id || null
  currentPlanName.value = profileRes.data.plan?.name || ''
  paymentMethods.value = methodsRes.data || []
})

onUnmounted(() => {
  stopPolling()
})

function handleBuy(plan: any) {
  selectedPlan.value = plan
  couponCode.value = ''
  couponDiscount.value = 0
  couponVerified.value = false
  couponError.value = ''
  couponDesc.value = ''
  durationType.value = 'monthly'
  payMethod.value = 'balance'
  showBuy.value = true
}

function resetCoupon() {
  couponDiscount.value = 0
  couponVerified.value = false
  couponError.value = ''
  couponDesc.value = ''
}

async function handleVerifyCoupon() {
  if (!couponCode.value || !selectedPlan.value) return
  verifying.value = true
  couponError.value = ''
  couponVerified.value = false
  couponDiscount.value = 0
  try {
    const res = await verifyCoupon({
      code: couponCode.value,
      plan_id: selectedPlan.value.id,
      duration_type: durationType.value,
    })
    couponVerified.value = true
    couponDiscount.value = res.data.discount
    if (res.data.discount_type === 'percent') {
      couponDesc.value = `${res.data.discount_value}% 折扣，减 ¥${res.data.discount.toFixed(2)}`
    } else {
      couponDesc.value = `减 ¥${res.data.discount_value}`
    }
  } catch (e: any) {
    couponError.value = e.response?.data?.message || '验证失败'
    couponDiscount.value = 0
  } finally {
    verifying.value = false
  }
}

async function handleOrder() {
  ordering.value = true
  try {
    const res = await createOrder({
      plan_id: selectedPlan.value.id,
      duration_type: durationType.value,
      pay_method: payMethod.value,
      coupon_code: couponCode.value || undefined,
    })

    if (payMethod.value === 'balance') {
      ElMessage.success(deliveryMessage(res.data?.entitlement?.status))
      showBuy.value = false
      const profileRes = await getProfile()
      userBalance.value = profileRes.data.balance || 0
      currentPlanId.value = profileRes.data.plan_id || null
      currentPlanName.value = profileRes.data.plan?.name || ''
    } else {
      // External payment
      showBuy.value = false
      const data = res.data
      payAmount.value = finalPrice.value
      payOrderId.value = data.order_id
      payQRCode.value = data.qr_code || ''
      payURL.value = data.pay_url || ''
      payCountdown.value = 30 * 60 // 30 minutes
      showPayDialog.value = true
      startPolling()
      startCountdown()
    }
  } finally {
    ordering.value = false
  }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(async () => {
    await checkPayStatus()
  }, 3000)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

function startCountdown() {
  countdownTimer = setInterval(() => {
    payCountdown.value--
    if (payCountdown.value <= 0) {
      stopPolling()
    }
  }, 1000)
}

async function checkPayStatus() {
  if (!payOrderId.value) return
  checkingStatus.value = true
  try {
    const res = await getOrder(payOrderId.value)
    if (res.data.pay_status === 'paid') {
      stopPolling()
      showPayDialog.value = false
      ElMessage.success(deliveryMessage(res.data.entitlement?.status))
      const profileRes = await getProfile()
      userBalance.value = profileRes.data.balance || 0
      currentPlanId.value = profileRes.data.plan_id || null
      currentPlanName.value = profileRes.data.plan?.name || ''
    } else if (res.data.pay_status === 'expired') {
      stopPolling()
      showPayDialog.value = false
      ElMessage.warning('订单已过期')
    }
  } finally {
    checkingStatus.value = false
  }
}

function cancelPayDialog() {
  stopPolling()
  showPayDialog.value = false
}

function openPayURL() {
  if (payURL.value) {
    window.open(payURL.value, '_blank')
  }
}

function formatCountdown(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

function deliveryMessage(status?: string): string {
  if (status === 'queued') return '购买成功，新套餐已排队，将在当前套餐到期后自动生效。'
  if (status === 'extended') return '续费成功，有效期已叠加。'
  return '购买成功，套餐已生效。'
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(0) + ' ' + sizes[i]
}
</script>

<style scoped>
.plans-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--space-5);
}

.plan-card {
  background: var(--color-surface);
  border: 1.5px solid var(--color-border-light);
  border-radius: var(--radius-xl);
  padding: var(--space-6);
  display: flex;
  flex-direction: column;
  transition: all var(--transition-normal);
  position: relative;
  overflow: hidden;
}

.plan-card:hover {
  box-shadow: var(--shadow-lg);
  border-color: var(--color-border);
  transform: translateY(-4px);
}

.plan-featured {
  border-color: var(--color-primary);
  box-shadow: var(--shadow-primary);
}

.plan-featured:hover {
  box-shadow: var(--shadow-primary-lg);
  border-color: var(--color-primary);
}

.plan-current {
  border-color: var(--color-success);
  box-shadow: 0 0 0 1px var(--color-success), var(--shadow-lg);
}

.plan-badge {
  position: absolute;
  top: var(--space-4);
  right: var(--space-4);
  background: var(--color-primary);
  color: #fff;
  font-size: var(--text-xs);
  font-weight: var(--font-semibold);
  padding: 2px var(--space-2);
  border-radius: var(--radius-full);
  line-height: 1.6;
}

.plan-badge--current {
  background: var(--color-success);
}

.plan-header {
  margin-bottom: var(--space-4);
}

.plan-name {
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
  color: var(--color-text);
  margin: 0 0 var(--space-1);
}

.plan-desc {
  font-size: var(--text-sm);
  color: var(--color-text-muted);
  margin: 0;
}

.plan-price {
  margin-bottom: var(--space-2);
  display: flex;
  align-items: baseline;
  gap: 2px;
}

.price-currency {
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}

.price-amount {
  font-size: var(--text-3xl);
  font-weight: var(--font-bold);
  color: var(--color-text);
  line-height: 1;
  letter-spacing: -0.02em;
}

.price-period {
  font-size: var(--text-sm);
  color: var(--color-text-muted);
  margin-left: var(--space-1);
}

.plan-prices-row {
  display: flex;
  gap: var(--space-4);
  margin-bottom: var(--space-4);
  padding-bottom: var(--space-3);
  border-bottom: 1px solid var(--color-border-light);
}

.price-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.price-option-label {
  font-size: 11px;
  color: var(--color-text-muted);
}

.price-option-value {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text-secondary);
}

.plan-features {
  list-style: none;
  padding: 0;
  margin: 0 0 var(--space-5);
  flex: 1;
}

.plan-features li {
  padding: var(--space-2) 0;
  font-size: var(--text-sm);
  color: var(--color-text-secondary);
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.plan-features li::before {
  content: "\2713 ";
  color: var(--color-success);
  font-weight: var(--font-semibold);
  flex-shrink: 0;
}

/* Order dialog */
.order-summary {
  text-align: center;
  margin-bottom: var(--space-4);
}

.order-plan-name {
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}

.order-detail {
  background: var(--el-fill-color-lighter);
  border-radius: var(--radius-lg);
  padding: var(--space-4);
  margin-bottom: var(--space-4);
}

.order-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
}

.order-row--total {
  padding-top: 4px;
}

.order-label {
  font-size: var(--text-sm);
  color: var(--color-text-secondary);
}

.order-value {
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  color: var(--color-text);
}

.order-total {
  font-size: var(--text-xl);
  font-weight: var(--font-bold);
  color: var(--color-primary);
}

.text-success {
  color: var(--color-success);
}

.order-section {
  margin-bottom: var(--space-3);
}

.order-section-label {
  font-size: 12px;
  font-weight: var(--font-medium);
  color: var(--color-text-muted);
  margin-bottom: 6px;
}

/* Payment method selection */
.pay-method-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.pay-method-option {
  margin-right: 0;
  padding: 8px 12px;
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-md);
  transition: all var(--transition-fast);
  width: 100%;
  height: auto;
}

.pay-method-option:hover {
  border-color: var(--color-primary);
}

.pay-method-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.pay-method-icon {
  width: 24px;
  height: 24px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  flex-shrink: 0;
}

.pay-icon-balance { background: linear-gradient(135deg, var(--color-primary), #6366f1); }
.pay-icon-alipay { background: linear-gradient(135deg, #1677ff, #0958d9); }
.pay-icon-wechat { background: linear-gradient(135deg, #07c160, #06ae56); }
.pay-icon-usdt { background: linear-gradient(135deg, #26a17b, #1a8a6a); }
.pay-icon-epay { background: linear-gradient(135deg, #ff6a00, #e05500); }

.pay-balance-amount {
  margin-left: auto;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-primary);
}

.pay-insufficient {
  font-size: 12px;
  color: var(--color-danger);
  margin-top: 4px;
}

/* QR code payment dialog */
.pay-dialog-content {
  text-align: center;
}

.pay-qr {
  margin-bottom: 16px;
}

.pay-qr-img {
  width: 240px;
  height: 240px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-light);
}

.pay-redirect {
  margin-bottom: 16px;
}

.pay-redirect p {
  font-size: 14px;
  color: var(--color-text-secondary);
  margin-bottom: 12px;
}

.pay-amount-display {
  display: flex;
  justify-content: center;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 8px;
}

.pay-amount-label {
  font-size: 13px;
  color: var(--color-text-muted);
}

.pay-amount-value {
  font-size: 24px;
  font-weight: var(--font-bold);
  color: var(--color-primary);
}

.pay-countdown {
  font-size: 12px;
  color: var(--color-text-muted);
  margin-bottom: 8px;
}

.pay-status-checking {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font-size: 12px;
  color: var(--color-text-muted);
  margin-top: 8px;
}

.coupon-row {
  display: flex;
  gap: 8px;
}

.coupon-row .el-input {
  flex: 1;
}

.coupon-result {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  margin-top: 6px;
  padding: 6px 10px;
  border-radius: var(--radius-md);
}

.coupon-result--success {
  color: var(--color-success);
  background: rgba(var(--color-success-rgb, 34, 197, 94), 0.08);
}

.coupon-result--error {
  color: var(--color-danger);
  background: rgba(var(--color-danger-rgb, 239, 68, 68), 0.08);
}
</style>
