<template>
  <div class="profile-page">
    <!-- Header card -->
    <el-card class="profile-header animate-in">
      <div class="header-content">
        <div class="avatar-lg">{{ profile.email?.charAt(0).toUpperCase() || 'U' }}</div>
        <div class="header-info">
          <h2 class="header-name">{{ profile.email || '加载中...' }}</h2>
          <div class="header-meta">
            <el-tag size="small" :type="profile.role === 'admin' ? 'warning' : ''">
              {{ roleMap[profile.role] || profile.role }}
            </el-tag>
            <span class="header-joined">注册于 {{ formatDate(profile.created_at) }}</span>
          </div>
        </div>
      </div>
    </el-card>

    <div class="profile-grid">
      <!-- Balance card -->
      <el-card class="balance-card animate-in animate-in-delay-1">
        <template #header>
          <div class="card-header">
            <div class="card-header-left">
              <div class="stat-icon stat-icon--amber">
                <el-icon :size="20"><Coin /></el-icon>
              </div>
              <span>账户余额</span>
            </div>
            <el-button type="primary" size="small" @click="showRecharge = true">
              <el-icon><Plus /></el-icon>充值
            </el-button>
          </div>
        </template>
        <div class="balance-amount">
          <span class="balance-symbol">&yen;</span>
          <span class="balance-value">{{ profile.balance?.toFixed(2) || '0.00' }}</span>
        </div>
        <div class="balance-quick-row">
          <span v-for="amt in [50, 100, 200, 500]" :key="amt" class="quick-chip"
            @click="quickRecharge(amt)">
            &yen;{{ amt }}
          </span>
        </div>
      </el-card>

      <!-- Credentials card -->
      <el-card class="animate-in animate-in-delay-2">
        <template #header>
          <div class="card-header">
            <span>凭证信息</span>
          </div>
        </template>
        <div class="credential-item">
          <div class="credential-label">邀请码</div>
          <div class="credential-value">
            <code class="mono-text">{{ profile.invite_code || '-' }}</code>
            <button class="copy-btn" @click="copyCode" title="复制">
              <el-icon :size="14"><CopyDocument /></el-icon>
            </button>
          </div>
        </div>
        <div class="credential-divider"></div>
        <div class="credential-item">
          <div class="credential-label">API Key</div>
          <div class="credential-value">
            <code class="mono-text api-key">{{ maskKey(profile.api_key) }}</code>
            <button class="copy-btn" @click="copyKey" title="复制">
              <el-icon :size="14"><CopyDocument /></el-icon>
            </button>
            <button class="copy-btn" @click="handleRegenerateKey" title="重新生成" style="margin-left: 4px">
              <el-icon :size="14"><Refresh /></el-icon>
            </button>
          </div>
        </div>
      </el-card>

      <!-- Edit profile card -->
      <el-card class="animate-in animate-in-delay-3">
        <template #header>
          <div class="card-header">
            <span>编辑资料</span>
          </div>
        </template>
        <el-form :model="profile" label-position="top">
          <el-form-item label="邮箱">
            <el-input v-model="profile.email" placeholder="your@email.com" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="handleUpdate" :loading="saving">保存修改</el-button>
          </el-form-item>
        </el-form>
      </el-card>

      <!-- Change password card -->
      <el-card class="animate-in animate-in-delay-4">
        <template #header>
          <div class="card-header">
            <span>修改密码</span>
          </div>
        </template>
        <el-form :model="pwdForm" label-position="top">
          <el-form-item label="当前密码">
            <el-input v-model="pwdForm.old_password" type="password" show-password placeholder="输入当前密码" />
          </el-form-item>
          <el-form-item label="新密码">
            <el-input v-model="pwdForm.new_password" type="password" show-password placeholder="输入新密码" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="handleChangePwd" :loading="changingPwd">更新密码</el-button>
          </el-form-item>
        </el-form>
      </el-card>
    </div>

    <!-- Recharge dialog -->
    <el-dialog v-model="showRecharge" title="余额充值" width="420" append-to-body :close-on-click-modal="false">
      <div class="recharge-section">
        <div class="recharge-label">选择金额</div>
        <div class="recharge-presets">
          <div v-for="amt in presetAmounts" :key="amt" class="preset-card"
            :class="{ active: rechargeAmount === amt }" @click="rechargeAmount = amt">
            <span class="preset-amount">&yen;{{ amt }}</span>
          </div>
        </div>
        <el-input-number v-model="rechargeAmount" :min="1" :max="99999" :precision="2" size="large" style="width: 100%" placeholder="自定义金额" />
      </div>

      <div class="recharge-section">
        <div class="recharge-label">支付方式</div>
        <div class="pay-method-grid">
          <div v-for="m in rechargeMethods" :key="m.type" class="pay-method-card"
            :class="{ active: rechargePayMethod === m.type }" @click="rechargePayMethod = m.type">
            <span class="pay-method-icon" :class="'pay-icon-' + m.type">{{ methodIcon[m.type] }}</span>
            <span class="pay-method-name">{{ m.name }}</span>
          </div>
        </div>
      </div>

      <template #footer>
        <el-button @click="showRecharge = false">取消</el-button>
        <el-button type="primary" @click="handleRecharge" :loading="recharging" :disabled="!rechargeAmount || !rechargePayMethod">
          确认充值 &yen;{{ rechargeAmount?.toFixed(2) || '0.00' }}
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
          <span class="pay-amount-label">充值金额</span>
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
import { reactive, onMounted, ref, onUnmounted } from 'vue'
import { getProfile, updateProfile, changePassword, getPaymentMethods, createRechargeOrder, getOrder, regenerateApiKey } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const roleMap: Record<string, string> = { super_admin: '超级管理员', admin: '管理员', user: '普通用户' }

const profile = reactive<any>({ email: '', role: '', balance: 0, invite_code: '', api_key: '', created_at: '' })
const pwdForm = reactive({ old_password: '', new_password: '' })
const saving = ref(false)
const changingPwd = ref(false)

// Recharge
const showRecharge = ref(false)
const rechargeAmount = ref(100)
const rechargePayMethod = ref('')
const rechargeMethods = ref<any[]>([])
const recharging = ref(false)
const presetAmounts = [50, 100, 200, 500, 1000]
const methodIcon: Record<string, string> = { alipay: '支', wechat: '微', usdt: '$', epay: '易' }

// QR code payment
const showPayDialog = ref(false)
const payQRCode = ref('')
const payURL = ref('')
const payAmount = ref(0)
const payOrderId = ref<number | null>(null)
const payCountdown = ref(0)
const checkingStatus = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  const [res, methodsRes] = await Promise.all([getProfile(), getPaymentMethods()])
  Object.assign(profile, res.data)
  rechargeMethods.value = methodsRes.data || []
  if (rechargeMethods.value.length > 0 && !rechargePayMethod.value) {
    rechargePayMethod.value = rechargeMethods.value[0].type
  }
})

onUnmounted(() => {
  stopPolling()
})

function formatDate(date: string) {
  if (!date) return '-'
  return new Date(date).toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' })
}

function maskKey(key: string) {
  if (!key) return '-'
  if (key.length <= 8) return key
  return key.slice(0, 6) + '••••••••' + key.slice(-4)
}

async function handleUpdate() {
  saving.value = true
  try {
    await updateProfile({ email: profile.email })
    ElMessage.success('资料已更新')
    localStorage.setItem('user_info', JSON.stringify({ email: profile.email }))
  } finally {
    saving.value = false
  }
}

async function handleChangePwd() {
  if (!pwdForm.old_password || !pwdForm.new_password) {
    ElMessage.warning('请填写完整')
    return
  }
  changingPwd.value = true
  try {
    await changePassword(pwdForm)
    ElMessage.success('密码已修改')
    pwdForm.old_password = ''
    pwdForm.new_password = ''
  } finally {
    changingPwd.value = false
  }
}

async function handleRegenerateKey() {
  try {
    await ElMessageBox.confirm('重新生成 API Key 后，旧的 Key 将立即失效，确认继续？', '重新生成 API Key', { type: 'warning' })
  } catch {
    return
  }
  try {
    const res = await regenerateApiKey()
    profile.api_key = res.data.api_key
    ElMessage.success('API Key 已重新生成')
  } catch (e: any) {
    ElMessage.error(e?.message || '重新生成失败')
  }
}

function copyToClipboard(text: string) {
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  textarea.select()
  document.execCommand('copy')
  document.body.removeChild(textarea)
}

function copyCode() {
  copyToClipboard(profile.invite_code)
  ElMessage.success('邀请码已复制')
}

function copyKey() {
  copyToClipboard(profile.api_key)
  ElMessage.success('API Key 已复制')
}

function quickRecharge(amt: number) {
  rechargeAmount.value = amt
  showRecharge.value = true
}

async function handleRecharge() {
  if (!rechargeAmount.value || !rechargePayMethod.value) return
  recharging.value = true
  try {
    const res = await createRechargeOrder({
      amount: rechargeAmount.value,
      pay_method: rechargePayMethod.value,
    })
    showRecharge.value = false
    const data = res.data
    payAmount.value = rechargeAmount.value
    payOrderId.value = data.order_id
    payQRCode.value = data.qr_code || ''
    payURL.value = data.pay_url || ''
    payCountdown.value = 30 * 60
    showPayDialog.value = true
    startPolling()
    startCountdown()
  } finally {
    recharging.value = false
  }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(async () => {
    await checkPayStatus()
  }, 3000)
}

function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null }
}

function startCountdown() {
  countdownTimer = setInterval(() => {
    payCountdown.value--
    if (payCountdown.value <= 0) stopPolling()
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
      ElMessage.success('充值成功！')
      const profileRes = await getProfile()
      Object.assign(profile, profileRes.data)
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
  if (payURL.value) window.open(payURL.value, '_blank')
}

function formatCountdown(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}
</script>

<style scoped>
.profile-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

/* ---- Header card ---- */
.profile-header {
  background: linear-gradient(135deg, var(--color-accent-light) 0%, var(--color-surface) 100%);
}

.header-content {
  display: flex;
  align-items: center;
  gap: var(--space-5);
}

.avatar-lg {
  width: 64px;
  height: 64px;
  background: var(--color-accent);
  color: #fff;
  border-radius: var(--radius-xl);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  font-weight: var(--font-bold);
  flex-shrink: 0;
}

.header-info {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.header-name {
  font-size: var(--text-xl);
  font-weight: var(--font-bold);
  color: var(--color-text);
  line-height: 1.2;
}

.header-meta {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.header-joined {
  font-size: var(--text-sm);
  color: var(--color-text-muted);
}

/* ---- Grid ---- */
.profile-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-5);
}

/* ---- Card header ---- */
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-header-left {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

/* ---- Balance card ---- */
.balance-amount {
  display: flex;
  align-items: baseline;
  gap: 4px;
  margin-bottom: var(--space-4);
}

.balance-symbol {
  font-size: 22px;
  font-weight: var(--font-semibold);
  color: var(--color-text-secondary);
}

.balance-value {
  font-size: 40px;
  font-weight: 800;
  color: var(--color-text);
  letter-spacing: -0.03em;
  font-family: var(--font-mono);
  line-height: 1;
}

.balance-quick-row {
  display: flex;
  gap: 8px;
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border-light);
}

.quick-chip {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text-secondary);
  background: var(--color-bg);
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-full);
  padding: 4px 16px;
  cursor: pointer;
  transition: all var(--transition-fast);
  font-family: var(--font-mono);
}

.quick-chip:hover {
  color: var(--color-accent);
  border-color: var(--color-accent);
  background: var(--color-accent-light);
}

/* ---- Credentials ---- */
.credential-item {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2) 0;
}

.credential-label {
  font-size: var(--text-xs);
  font-weight: var(--font-semibold);
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.credential-value {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.mono-text {
  font-family: var(--font-mono);
  font-size: var(--text-sm);
  color: var(--color-text);
  background: var(--color-bg);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border-light);
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.api-key {
  font-size: 12px;
  color: var(--color-text-secondary);
}

.copy-btn {
  width: 30px;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all var(--transition-fast);
  flex-shrink: 0;
}

.copy-btn:hover {
  background: var(--color-bg);
  color: var(--color-accent);
  border-color: var(--color-accent);
}

.credential-divider {
  height: 1px;
  background: var(--color-border-light);
  margin: var(--space-1) 0;
}

/* ---- Recharge ---- */
.recharge-section {
  margin-bottom: var(--space-5);
}

.recharge-section:last-of-type {
  margin-bottom: 0;
}

.recharge-label {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text);
  margin-bottom: var(--space-3);
}

.recharge-presets {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 8px;
  margin-bottom: var(--space-3);
}

.preset-card {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 10px 0;
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
  background: var(--color-surface);
}

.preset-card:hover {
  border-color: var(--color-accent);
}

.preset-card.active {
  border-color: var(--color-accent);
  background: var(--color-accent-light);
}

.preset-amount {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text);
  font-family: var(--font-mono);
}

.preset-card.active .preset-amount {
  color: var(--color-accent);
}

/* ---- Payment method ---- */
.pay-method-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px;
}

.pay-method-card {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: 12px 16px;
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
  background: var(--color-surface);
}

.pay-method-card:hover {
  border-color: var(--color-accent);
}

.pay-method-card.active {
  border-color: var(--color-accent);
  background: var(--color-accent-light);
}

.pay-method-name {
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  color: var(--color-text);
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

.pay-icon-alipay { background: linear-gradient(135deg, #1677ff, #0958d9); }
.pay-icon-wechat { background: linear-gradient(135deg, #07c160, #06ae56); }
.pay-icon-usdt { background: linear-gradient(135deg, #26a17b, #1a8a6a); }
.pay-icon-epay { background: linear-gradient(135deg, #ff6a00, #e05500); }

/* ---- Pay dialog ---- */
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
</style>
