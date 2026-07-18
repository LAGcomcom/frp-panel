<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">我的订单</span>
      </div>
    </template>

    <el-table :data="orders" v-loading="loading" stripe>
      <el-table-column prop="order_no" label="订单号" width="200">
        <template #default="{ row }"><span class="text-mono">{{ row.order_no }}</span></template>
      </el-table-column>
      <el-table-column label="套餐" width="120">
        <template #default="{ row }">{{ row.plan?.name }}</template>
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
      <el-table-column label="权益状态" min-width="150">
        <template #default="{ row }">
          <span v-if="row.order_type === 'recharge'">-</span>
          <el-tag v-else-if="row.entitlement" :type="entitlementType[row.entitlement.status] || 'info'" size="small">
            {{ entitlementMap[row.entitlement.status] || row.entitlement.status }}
          </el-tag>
          <span v-else-if="row.pay_status === 'paid'">历史订单</span>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="170">
        <template #default="{ row }">{{ new Date(row.created_at).toLocaleString('zh-CN') }}</template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getOrders } from '../api'

const orders = ref<any[]>([])
const loading = ref(false)

const durationMap: Record<string, string> = { monthly: '月付', quarterly: '季付', yearly: '年付', recharge: '充值' }
const methodMap: Record<string, string> = { balance: '余额', alipay: '支付宝', wechat: '微信', usdt: 'USDT', epay: '易支付', admin: '管理员' }
const statusMap: Record<string, string> = { paid: '已支付', refunded: '已退款', pending: '待支付', expired: '已过期' }
const entitlementMap: Record<string, string> = {
  active: '已生效', extended: '已续期', queued: '排队中（按购买顺序）', expired: '已结束',
}
const entitlementType: Record<string, string> = {
  active: 'success', extended: 'success', queued: 'warning', expired: 'info',
}

onMounted(async () => {
  loading.value = true
  try {
    const res = await getOrders({ size: 100 })
    orders.value = res.data.list || []
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
/* page-header and page-title are defined in design-system.css */
</style>
