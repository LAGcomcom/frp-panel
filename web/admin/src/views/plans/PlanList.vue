<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">套餐管理</span>
        <el-button type="primary" @click="openAdd">
          <el-icon><Plus /></el-icon>添加套餐
        </el-button>
      </div>
    </template>

    <el-table :data="plans" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="name" label="名称" min-width="100">
        <template #default="{ row }">
          <div class="plan-name-cell">
            <span class="plan-name">{{ row.name }}</span>
            <span v-if="row.description" class="plan-desc">{{ row.description }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="资源" min-width="160">
        <template #default="{ row }">
          <div class="resource-tags">
            <el-tag size="small" type="info">{{ row.max_proxies }} 代理</el-tag>
            <el-tag size="small" type="info">{{ formatBandwidth(row.max_bandwidth) }}</el-tag>
            <el-tag size="small" type="info">{{ formatTraffic(row.max_traffic) }}</el-tag>
            <el-tag size="small" type="info">{{ row.duration_days }}天</el-tag>
			<el-tag v-if="row.group" size="small" type="success">{{ row.group.name }}</el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="价格" min-width="200">
        <template #default="{ row }">
          <div class="price-cards">
            <div class="price-card">
              <span class="price-period">月付</span>
              <span class="price-value">{{ formatPrice(row.price_monthly) }}</span>
            </div>
            <div class="price-card">
              <span class="price-period">季付</span>
              <span class="price-value">{{ formatPrice(row.price_quarterly) }}</span>
            </div>
            <div class="price-card">
              <span class="price-period">年付</span>
              <span class="price-value">{{ formatPrice(row.price_yearly) }}</span>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="70" align="center">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">
            {{ row.status === 'active' ? '上架' : '下架' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" align="center" fixed="right">
        <template #default="{ row }">
          <div class="action-btns">
            <el-button size="small" @click="handleEdit(row)">编辑</el-button>
            <el-button size="small" @click="handleToggleStatus(row)">{{ row.status === 'active' ? '下架' : '上架' }}</el-button>
            <el-button size="small" type="warning" @click="handleDelete(row)">归档</el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="showDialog" :title="editingPlan ? '编辑套餐' : '添加套餐'" width="680" append-to-body>
      <el-form :model="form" label-width="80px" class="plan-form">
        <!-- 基本信息 -->
        <div class="form-section">
          <div class="form-section-title">基本信息</div>
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="套餐名称" required>
                <el-input v-model="form.name" placeholder="如：基础版" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="有效时长">
				<el-input-number v-model="form.duration_days" :min="1" :step="1" controls-position="right" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-form-item label="套餐描述">
            <el-input v-model="form.description" type="textarea" :rows="2" placeholder="可选，简要描述套餐特点" />
          </el-form-item>
		  <el-form-item label="用户组">
			<el-select v-model="form.group_id" style="width: 100%">
			  <el-option label="不自动分组" :value="0" />
			  <el-option v-for="group in groups" :key="group.id" :label="group.name" :value="group.id" />
			</el-select>
		  </el-form-item>
        </div>

        <!-- 资源限制 -->
        <div class="form-section">
          <div class="form-section-title">资源限制</div>
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="代理数量">
                <el-input v-model="form.max_proxies" placeholder="5">
                  <template #append>个</template>
                </el-input>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="端口数量">
                <el-input v-model="form.max_ports" placeholder="10">
                  <template #append>个</template>
                </el-input>
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="单代理带宽上限">
                <el-input v-model="form.max_bandwidth_mb" placeholder="10">
                  <template #append>MB/s</template>
                </el-input>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="流量限额">
                <el-input v-model="form.max_traffic_gb" placeholder="100">
                  <template #append>GB</template>
                </el-input>
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- 价格设置 -->
        <div class="form-section">
          <div class="form-section-title">价格设置</div>
          <el-row :gutter="16">
            <el-col :span="8">
              <el-form-item label="月付（元）">
                <el-input-number v-model="form.price_monthly" :min="0" :precision="2" :step="0.1" controls-position="right" class="price-number-input" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="季付（元）">
                <el-input-number v-model="form.price_quarterly" :min="0" :precision="2" :step="0.1" controls-position="right" class="price-number-input" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="年付（元）">
                <el-input-number v-model="form.price_yearly" :min="0" :precision="2" :step="0.1" controls-position="right" class="price-number-input" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>
      </el-form>

      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit">{{ editingPlan ? '保存修改' : '创建套餐' }}</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { getPlans, getUserGroups, createPlan, updatePlan, togglePlanStatus, deletePlan } from '../../api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit } from '@element-plus/icons-vue'

const plans = ref<any[]>([])
const loading = ref(false)
const showDialog = ref(false)
const editingPlan = ref<any>(null)
const groups = ref<any[]>([])

const form = reactive({
  name: '', description: '', max_proxies: 5, max_bandwidth_mb: 10,
  max_traffic_gb: 100, max_ports: 10, duration_days: 30,
  price_monthly: 0, price_quarterly: 0, price_yearly: 0,
	group_id: 0,
})

onMounted(async () => {
	const groupRes = await getUserGroups()
	groups.value = groupRes.data || []
	await fetchData()
})

async function fetchData() {
  loading.value = true
  try {
    const res = await getPlans()
    plans.value = res.data
  } finally {
    loading.value = false
  }
}

function openAdd() {
  editingPlan.value = null
  Object.assign(form, {
    name: '', description: '', max_proxies: 5, max_bandwidth_mb: 10,
    max_traffic_gb: 100, max_ports: 10, duration_days: 30,
    price_monthly: 0, price_quarterly: 0, price_yearly: 0,
	group_id: 0,
  })
  showDialog.value = true
}

function handleEdit(row: any) {
  editingPlan.value = row
  Object.assign(form, {
    name: row.name, description: row.description, max_proxies: row.max_proxies,
    max_bandwidth_mb: row.max_bandwidth / 1024 / 1024,
    max_traffic_gb: row.max_traffic / 1024 / 1024 / 1024,
    max_ports: row.max_ports, duration_days: row.duration_days,
    price_monthly: row.price_monthly, price_quarterly: row.price_quarterly, price_yearly: row.price_yearly,
	group_id: row.group_id || 0,
  })
  showDialog.value = true
}

async function handleSubmit() {
  const data = {
    ...form,
    max_proxies: Number(form.max_proxies),
    max_bandwidth: Number(form.max_bandwidth_mb) * 1024 * 1024,
    max_traffic: Number(form.max_traffic_gb) * 1024 * 1024 * 1024,
    max_ports: Number(form.max_ports),
    duration_days: Number(form.duration_days),
    price_monthly: Number(form.price_monthly),
    price_quarterly: Number(form.price_quarterly),
    price_yearly: Number(form.price_yearly),
	group_id: form.group_id || undefined,
	clear_group: form.group_id === 0,
  }
  if (editingPlan.value) {
    await updatePlan(editingPlan.value.id, data)
    ElMessage.success('套餐已更新')
  } else {
    await createPlan(data)
    ElMessage.success('套餐已创建')
  }
  showDialog.value = false
  editingPlan.value = null
  fetchData()
}

async function handleToggleStatus(row: any) {
  const action = row.status === 'active' ? '下架' : '上架'
  await ElMessageBox.confirm(`确认${action}套餐"${row.name}"？`, '确认操作')
  await togglePlanStatus(row.id)
  ElMessage.success(`套餐已${action}`)
  fetchData()
}

async function handleDelete(row: any) {
  await ElMessageBox.confirm(
    `确认归档套餐"${row.name}"？归档后新用户无法购买，已有订单和权益不受影响，之后仍可重新上架。`,
    '确认归档',
    { type: 'warning' },
  )
  await deletePlan(row.id)
  ElMessage.success('套餐已归档')
  fetchData()
}

function formatBandwidth(bytes: number): string {
  if (!bytes) return '0 MB/s'
  const mb = bytes / 1024 / 1024
  if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB/s'
  return mb.toFixed(0) + ' MB/s'
}

function formatTraffic(bytes: number): string {
  if (!bytes) return '0 GB'
  const gb = bytes / 1024 / 1024 / 1024
  if (gb >= 1024) return (gb / 1024).toFixed(1) + ' TB'
  return gb.toFixed(0) + ' GB'
}

function formatPrice(price: number): string {
  if (!price || price === 0) return '免费'
  return '¥' + price.toFixed(2)
}
</script>

<style scoped>
.plan-form {
  padding-top: 4px;
}

.form-section {
  margin-bottom: 20px;
}

.form-section:last-child {
  margin-bottom: 0;
}

.form-section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-secondary);
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--color-border-light);
}

.price-number-input {
  width: 100%;
}

.plan-name-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.plan-name {
  font-weight: 500;
  color: var(--color-text);
}

.plan-desc {
  font-size: 12px;
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 160px;
}

.resource-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.price-cards {
  display: flex;
  gap: 8px;
}

.price-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 4px 10px;
  background: var(--el-fill-color-lighter);
  border-radius: 6px;
  min-width: 56px;
}

.price-period {
  font-size: 11px;
  color: var(--color-text-muted);
}

.price-value {
  font-family: var(--font-mono);
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text);
}

.action-btns {
  display: flex;
  gap: 6px;
  justify-content: center;
}
</style>
