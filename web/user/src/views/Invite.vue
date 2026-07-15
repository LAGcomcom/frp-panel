<template>
  <div class="invite-page" v-loading="loading">
    <!-- Invite Code + Stats -->
    <el-card class="animate-in">
      <template #header>
        <div class="page-header">
          <span class="page-title">邀请好友</span>
        </div>
      </template>

      <div class="invite-top">
        <div class="invite-code-section">
          <div class="code-display">
            <code class="mono-text code-lg">{{ stats.invite_code }}</code>
            <el-button size="small" @click="copyCode">复制邀请码</el-button>
          </div>
          <div class="invite-hint">分享邀请码给好友，注册时填写即可建立邀请关系</div>
        </div>

        <div class="stats-grid">
          <div class="stat-item">
            <span class="stat-num">{{ stats.level1_count || 0 }}</span>
            <span class="stat-label">一级下级</span>
          </div>
          <div class="stat-divider"></div>
          <div class="stat-item">
            <span class="stat-num">{{ stats.level2_count || 0 }}</span>
            <span class="stat-label">二级下级</span>
          </div>
          <div class="stat-divider"></div>
          <div class="stat-item">
            <span class="stat-num">&yen;{{ (stats.total_rebate_earned || 0).toFixed(2) }}</span>
            <span class="stat-label">累计返利</span>
          </div>
          <div class="stat-divider"></div>
          <div class="stat-item">
            <span class="stat-num">{{ stats.level1_rebate_pct || 10 }}% / {{ stats.level2_rebate_pct || 5 }}%</span>
            <span class="stat-label">一级 / 二级比例</span>
          </div>
        </div>

        <div class="invite-rules">
          <div class="rule-title">返利规则</div>
          <div class="rule-list">
            <div class="rule-item">邀请好友注册并完成付费后，你可获得订单金额的返利</div>
            <div class="rule-item">一级下级（直接邀请）返利 <b>{{ stats.level1_rebate_pct || 10 }}%</b></div>
            <div class="rule-item">二级下级（下级邀请）返利 <b>{{ stats.level2_rebate_pct || 5 }}%</b></div>
            <div class="rule-item">返利自动发放到账户余额，可用于购买套餐</div>
          </div>
        </div>
      </div>
    </el-card>

    <!-- Tabs -->
    <el-card class="animate-in animate-in-delay-1">
      <el-tabs v-model="activeTab">
        <el-tab-pane label="一级下级" name="level1">
          <el-table :data="stats.level1_users || []" stripe>
            <el-table-column prop="email" label="邮箱" />
            <el-table-column label="注册时间" width="180">
              <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!stats.level1_users?.length" description="暂无一级下级" />
        </el-tab-pane>

        <el-tab-pane label="二级下级" name="level2">
          <el-table :data="stats.level2_users || []" stripe>
            <el-table-column prop="email" label="邮箱" />
            <el-table-column prop="referred_by_email" label="邀请人" width="180" />
            <el-table-column label="注册时间" width="180">
              <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!stats.level2_users?.length" description="暂无二级下级" />
        </el-tab-pane>

        <el-tab-pane label="我的优惠券" name="coupons">
          <div class="coupon-header">
            <el-button type="primary" size="small" @click="showCreateDialog = true" :disabled="!stats.level1_users?.length">
              <el-icon><Plus /></el-icon> 发优惠券给下级
            </el-button>
          </div>

          <div v-if="myCoupons.length" class="coupon-list">
            <div v-for="coupon in myCoupons" :key="coupon.id" class="coupon-item">
              <div class="coupon-left">
                <div class="coupon-amount">&yen;{{ coupon.discount_value.toFixed(2) }}</div>
                <div class="coupon-label">优惠券</div>
              </div>
              <div class="coupon-right">
                <div class="coupon-top-row">
                  <code class="coupon-code">{{ coupon.code }}</code>
                  <el-tag v-if="coupon.refund_status === 'used'" type="success" size="small">已使用</el-tag>
                  <el-tag v-else-if="coupon.refund_status === 'refunded'" type="info" size="small">已退回</el-tag>
                  <el-tag v-else-if="coupon.status === 'active'" type="primary" size="small">有效</el-tag>
                  <el-tag v-else type="danger" size="small">已失效</el-tag>
                </div>
                <div class="coupon-meta">
                  <span>{{ coupon.end_time ? '到期 ' + formatTime(coupon.end_time) : '永久有效' }}</span>
                </div>
              </div>
            </div>
          </div>
          <el-empty v-else description="暂未创建优惠券" />
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- Create Coupon Dialog -->
    <el-dialog v-model="showCreateDialog" title="发优惠券给下级" width="420">
      <el-form label-width="80px">
        <el-form-item label="下级用户">
          <el-select v-model="createForm.assigned_to" placeholder="选择直推下级" style="width: 100%">
            <el-option v-for="u in stats.level1_users || []" :key="u.id" :label="u.email" :value="u.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="券面金额">
          <el-input-number v-model="createForm.amount" :min="0.01" :precision="2" :step="10" />
          <span class="form-hint">元（从余额扣除）</span>
        </el-form-item>
        <el-form-item label="有效期">
          <el-date-picker v-model="createForm.end_time" type="date" placeholder="选择到期日" value-format="YYYY-MM-DD" style="width: 100%" :disabled-date="disablePastDate" />
        </el-form-item>
      </el-form>
      <div class="balance-preview">
        <span>当前余额：<b>&yen;{{ userBalance.toFixed(2) }}</b></span>
        <span>创建后：<b>&yen;{{ Math.max(0, userBalance - createForm.amount).toFixed(2) }}</b></span>
      </div>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreateCoupon" :loading="creating" :disabled="createForm.amount > userBalance">
          {{ createForm.amount > userBalance ? '余额不足' : '确认创建' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { getInviteStats, createUserCoupon, getMyCoupons, getProfile } from '../api'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const creating = ref(false)
const activeTab = ref('level1')
const stats = ref<any>({})
const myCoupons = ref<any[]>([])
const userBalance = ref(0)
const showCreateDialog = ref(false)
const createForm = reactive({ assigned_to: 0, amount: 10, end_time: '' })

function formatTime(t: string) {
  if (!t) return ''
  return new Date(t).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })
}

function disablePastDate(date: Date) {
  return date.getTime() < Date.now() - 86400000
}

function copyCode() {
  const text = stats.value.invite_code || ''
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).then(() => ElMessage.success('邀请码已复制'))
  } else {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.left = '-9999px'
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
    ElMessage.success('邀请码已复制')
  }
}

async function fetchData() {
  loading.value = true
  try {
    const [statsRes, couponsRes, profileRes] = await Promise.all([
      getInviteStats(),
      getMyCoupons(),
      getProfile(),
    ])
    stats.value = statsRes.data || {}
    myCoupons.value = couponsRes.data || []
    userBalance.value = profileRes.data?.balance || 0
  } finally {
    loading.value = false
  }
}

async function handleCreateCoupon() {
  if (!createForm.assigned_to) { ElMessage.warning('请选择下级用户'); return }
  if (!createForm.end_time) { ElMessage.warning('请选择到期日'); return }
  if (createForm.amount > userBalance.value) { ElMessage.error('余额不足'); return }
  creating.value = true
  try {
    await createUserCoupon(createForm)
    ElMessage.success('优惠券已创建')
    showCreateDialog.value = false
    fetchData()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '创建失败')
  } finally {
    creating.value = false
  }
}

onMounted(fetchData)
</script>

<style scoped>
.invite-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

/* ---- Invite top section ---- */
.invite-top {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.invite-code-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.code-display {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.mono-text {
  font-family: var(--font-mono);
  font-size: var(--text-sm);
  color: var(--color-text);
  background: var(--color-bg);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border-light);
}

.code-lg {
  font-size: var(--text-lg);
  padding: var(--space-2) var(--space-4);
  letter-spacing: 0.1em;
  font-weight: 600;
}

.invite-hint {
  font-size: var(--text-xs);
  color: var(--color-text-muted);
}

/* ---- Stats grid ---- */
.stats-grid {
  display: flex;
  align-items: center;
  gap: var(--space-6);
  padding: var(--space-4) var(--space-5);
  background: var(--color-bg);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-light);
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  flex: 1;
}

.stat-num {
  font-size: var(--text-lg);
  font-weight: var(--font-bold);
  color: var(--color-text);
}

.stat-label {
  font-size: var(--text-xs);
  color: var(--color-text-muted);
}

.stat-divider {
  width: 1px;
  height: 28px;
  background: var(--color-border-light);
  flex-shrink: 0;
}

/* ---- Invite rules ---- */
.invite-rules {
  padding: var(--space-4);
  background: var(--color-bg);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-light);
}

.rule-title {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text);
  margin-bottom: var(--space-3);
}

.rule-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.rule-item {
  font-size: var(--text-sm);
  color: var(--color-text-secondary);
  padding-left: var(--space-3);
  position: relative;
}

.rule-item::before {
  content: '';
  position: absolute;
  left: 0;
  top: 8px;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--color-text-muted);
}

.rule-item b {
  color: var(--color-accent);
  font-weight: 600;
}

/* ---- Coupon list ---- */
.coupon-header {
  margin-bottom: var(--space-4);
}

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
  width: 100px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  background: var(--color-bg);
  border-right: 1px dashed var(--color-border-light);
  padding: var(--space-4) var(--space-3);
  flex-shrink: 0;
}

.coupon-amount {
  font-size: var(--text-xl);
  font-weight: var(--font-bold);
  color: var(--color-primary);
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
  gap: var(--space-2);
  min-width: 0;
}

.coupon-top-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
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

/* ---- Form hint ---- */
.form-hint {
  font-size: var(--text-xs);
  color: var(--color-text-muted);
  margin-left: var(--space-2);
}

/* ---- Balance preview ---- */
.balance-preview {
  display: flex;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  background: var(--color-bg);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  color: var(--color-text-secondary);
}

.balance-preview b {
  color: var(--color-text);
}
</style>
