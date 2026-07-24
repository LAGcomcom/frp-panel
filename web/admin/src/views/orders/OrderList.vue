<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">订单管理</span>
        <div class="order-filters">
          <el-input
            v-model="keyword"
            class="order-search"
            placeholder="搜索订单号、用户或账务说明"
            clearable
            prefix-icon="Search"
            @keyup.enter="applyFilters"
            @clear="applyFilters"
          />
          <el-select v-model="statusFilter" class="order-filter" placeholder="全部状态" clearable @change="applyFilters">
            <el-option label="待支付" value="pending" />
            <el-option label="已支付" value="paid" />
            <el-option label="已退款" value="refunded" />
            <el-option label="已过期" value="expired" />
          </el-select>
          <el-select v-model="typeFilter" class="order-filter" placeholder="全部类型" clearable @change="applyFilters">
            <el-option label="套餐订单" value="plan" />
            <el-option label="余额充值" value="recharge" />
          </el-select>
        </div>
      </div>
    </template>

    <el-table :data="orders" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="order_no" label="订单号" width="200">
        <template #default="{ row }"><span class="text-mono">{{ row.order_no }}</span></template>
      </el-table-column>
      <el-table-column label="用户" width="200">
        <template #default="{ row }">{{ row.user?.email }}</template>
      </el-table-column>
      <el-table-column label="套餐" width="120">
        <template #default="{ row }">{{ row.order_type === 'recharge' ? '余额充值' : row.plan?.name || '-' }}</template>
      </el-table-column>
      <el-table-column prop="amount" label="金额" width="100">
        <template #default="{ row }"><span class="text-mono">&yen;{{ row.amount?.toFixed(2) }}</span></template>
      </el-table-column>
      <el-table-column prop="duration_type" label="周期" width="80">
        <template #default="{ row }">{{ durationMap[row.duration_type] || row.duration_type }}</template>
      </el-table-column>
      <el-table-column prop="pay_method" label="支付方式" width="90">
        <template #default="{ row }">{{ methodMap[row.pay_method] || row.pay_method }}</template>
      </el-table-column>
      <el-table-column prop="pay_status" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.pay_status === 'paid' ? 'success' : row.pay_status === 'refunded' ? 'danger' : 'warning'" size="small">
            {{ statusMap[row.pay_status] || row.pay_status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="账务说明" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <div>{{ row.remark || '-' }}</div>
          <div v-if="row.operator_email" class="operator-email">操作人：{{ row.operator_email }}</div>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="170">
        <template #default="{ row }">{{ new Date(row.created_at).toLocaleString('zh-CN') }}</template>
      </el-table-column>
      <el-table-column label="操作" width="100" fixed="right">
        <template #default="{ row }">
          <el-button
            v-if="canRefund(row)"
            type="danger"
            plain
            size="small"
            :loading="refundingOrderId === row.id"
            :disabled="refundingOrderId !== null && refundingOrderId !== row.id"
            @click="handleRefund(row)"
          >退款</el-button>
          <el-tooltip v-else-if="row.pay_status === 'paid'" :content="refundUnavailableReason(row)" placement="top">
            <span><el-button type="info" plain size="small" disabled>退款</el-button></span>
          </el-tooltip>
          <span v-else class="empty-action">-</span>
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
import { ElMessage, ElMessageBox } from 'element-plus'
import { getOrders, refundOrder } from '../../api'

const orders = ref<any[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const refundingOrderId = ref<number | null>(null)
const keyword = ref('')
const statusFilter = ref('')
const typeFilter = ref('')
let fetchSequence = 0

const durationMap: Record<string, string> = { monthly: '月付', quarterly: '季付', yearly: '年付', recharge: '充值' }
const methodMap: Record<string, string> = { balance: '余额', alipay: '支付宝', wechat: '微信', admin: '管理员' }
const statusMap: Record<string, string> = { paid: '已支付', refunded: '已退款', pending: '待支付', expired: '已过期' }

onMounted(() => fetchData())

async function fetchData() {
  const requestID = ++fetchSequence
  loading.value = true
  try {
    const res = await getOrders({
      page: page.value,
      size: pageSize.value,
      keyword: keyword.value.trim() || undefined,
      status: statusFilter.value || undefined,
      order_type: typeFilter.value || undefined,
    })
    if (requestID !== fetchSequence) return
    orders.value = res.data.list
    total.value = res.data.total
  } finally {
    if (requestID === fetchSequence) loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  fetchData()
}

function canRefund(order: any): boolean {
  return order.refundable === true
}

function refundUnavailableReason(order: any): string {
  return order.refund_unavailable_reason || '该订单需要人工核对后退款'
}

async function handleRefund(order: any) {
  try {
    await ElMessageBox.confirm(
      `确认退回订单 ${order.order_no} 的 ¥${Number(order.amount).toFixed(2)}？未生效的排队套餐将同时取消。`,
      '确认退款',
      { type: 'warning', confirmButtonText: '确认退款', cancelButtonText: '取消' },
    )
  } catch {
    return
  }

  refundingOrderId.value = order.id
  try {
    await refundOrder(order.id)
    ElMessage.success('退款成功，金额已退回用户余额')
    await fetchData()
  } finally {
    refundingOrderId.value = null
  }
}

</script>

<style scoped>
/* page-header and page-title are defined in design-system.css */
.empty-action {
  color: var(--el-text-color-placeholder);
}
.operator-email {
  margin-top: 2px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.page-header {
  gap: 12px;
  flex-wrap: wrap;
}
.order-filters {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}
.order-search {
  width: 280px;
}
.order-filter {
  width: 120px;
}
@media (max-width: 760px) {
  .order-filters {
    width: 100%;
    justify-content: flex-start;
  }
  .order-search {
    width: min(100%, 280px);
  }
}
</style>
