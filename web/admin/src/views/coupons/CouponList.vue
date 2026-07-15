<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">优惠码管理</span>
        <el-button type="primary" @click="openAdd">
          <el-icon><Plus /></el-icon>添加优惠码
        </el-button>
      </div>
    </template>

    <el-table :data="coupons" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="code" label="优惠码" min-width="140">
        <template #default="{ row }">
          <span class="coupon-code">{{ row.code }}</span>
        </template>
      </el-table-column>
      <el-table-column label="折扣" min-width="120">
        <template #default="{ row }">
          <el-tag :type="row.discount_type === 'percent' ? 'warning' : 'success'" size="small">
            {{ row.discount_type === 'percent' ? row.discount_value + '% 折扣' : '减 ¥' + row.discount_value }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="使用情况" width="120">
        <template #default="{ row }">
          <span class="text-mono">{{ row.used_count }} / {{ row.max_uses || '∞' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="有效期" min-width="180">
        <template #default="{ row }">
          <span v-if="row.start_time || row.end_time" style="font-size: 12px">
            {{ row.start_time ? formatDate(row.start_time) : '不限' }} ~ {{ row.end_time ? formatDate(row.end_time) : '不限' }}
          </span>
          <span v-else class="text-muted">永久有效</span>
        </template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="80" align="center">
        <template #default="{ row }">
          <el-switch :model-value="row.status === 'active'" @change="handleToggle(row)" size="small" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="120" align="center" fixed="right">
        <template #default="{ row }">
          <div class="action-btns">
            <el-button size="small" @click="handleEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="showDialog" :title="editingItem ? '编辑优惠码' : '添加优惠码'" width="580" append-to-body>
      <el-form :model="form" label-width="90px" class="coupon-form">
        <div class="form-section">
          <div class="form-section-title">基本信息</div>
          <el-form-item label="优惠码" required>
            <el-input v-model="form.code" placeholder="如 SAVE20" :disabled="!!editingItem">
              <template #prefix><el-icon><Ticket /></el-icon></template>
            </el-input>
          </el-form-item>
        </div>

        <div class="form-section">
          <div class="form-section-title">折扣设置</div>
          <el-form-item label="折扣类型" required>
            <el-segmented v-model="form.discount_type" :options="[
              { label: '百分比折扣', value: 'percent' },
              { label: '固定金额', value: 'fixed' },
            ]" />
          </el-form-item>
          <el-row :gutter="16">
            <el-col :span="14">
              <el-form-item :label="form.discount_type === 'percent' ? '折扣率' : '减免额'" required>
                <el-input v-model="form.discount_value" :placeholder="form.discount_type === 'percent' ? '80' : '10'">
                  <template #append>{{ form.discount_type === 'percent' ? '%' : '元' }}</template>
                </el-input>
              </el-form-item>
            </el-col>
            <el-col :span="10">
              <div class="discount-preview" v-if="form.discount_value">
                <span class="preview-label">预览</span>
                <span class="preview-value" v-if="form.discount_type === 'percent'">
                  打 {{ (Number(form.discount_value) / 10).toFixed(1) }} 折
                </span>
                <span class="preview-value" v-else>
                  减 ¥{{ Number(form.discount_value).toFixed(2) }}
                </span>
              </div>
            </el-col>
          </el-row>
        </div>

        <div class="form-section">
          <div class="form-section-title">使用限制</div>
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="使用上限">
                <el-input v-model="form.max_uses" placeholder="0">
                  <template #append>次</template>
                </el-input>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <div class="form-hint">0 表示不限制使用次数</div>
            </el-col>
          </el-row>
          <el-form-item label="有效期">
            <el-date-picker v-model="form.date_range" type="daterange" range-separator="至" start-placeholder="开始日期" end-placeholder="结束日期" value-format="YYYY-MM-DD" style="width: 100%" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit">{{ editingItem ? '保存修改' : '创建优惠码' }}</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { getCoupons, createCoupon, updateCoupon, deleteCoupon, toggleCoupon } from '../../api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Delete, Ticket } from '@element-plus/icons-vue'

const coupons = ref<any[]>([])
const loading = ref(false)
const showDialog = ref(false)
const editingItem = ref<any>(null)

const form = reactive({
  code: '',
  discount_type: 'percent',
  discount_value: '',
  max_uses: '',
  date_range: null as string[] | null,
})

onMounted(() => fetchData())

async function fetchData() {
  loading.value = true
  try {
    const res = await getCoupons()
    coupons.value = res.data
  } finally {
    loading.value = false
  }
}

function openAdd() {
  editingItem.value = null
  Object.assign(form, {
    code: '', discount_type: 'percent', discount_value: '', max_uses: '', date_range: null,
  })
  showDialog.value = true
}

function handleEdit(row: any) {
  editingItem.value = row
  Object.assign(form, {
    code: row.code,
    discount_type: row.discount_type,
    discount_value: String(row.discount_value),
    max_uses: String(row.max_uses ?? 0),
    date_range: row.start_time && row.end_time ? [row.start_time.slice(0, 10), row.end_time.slice(0, 10)] : null,
  })
  showDialog.value = true
}

async function handleSubmit() {
  const data: any = {
    code: form.code,
    discount_type: form.discount_type,
    discount_value: Number(form.discount_value),
    max_uses: Number(form.max_uses),
  }
  if (form.date_range && form.date_range.length === 2) {
    data.start_time = form.date_range[0]
    data.end_time = form.date_range[1]
  }
  if (editingItem.value) {
    await updateCoupon(editingItem.value.id, data)
    ElMessage.success('优惠码已更新')
  } else {
    await createCoupon(data)
    ElMessage.success('优惠码已创建')
  }
  showDialog.value = false
  editingItem.value = null
  fetchData()
}

async function handleToggle(row: any) {
  await toggleCoupon(row.id)
  ElMessage.success('状态已更新')
  fetchData()
}

async function handleDelete(row: any) {
  await ElMessageBox.confirm(`确认删除优惠码"${row.code}"？`, '确认删除', { type: 'warning' })
  await deleteCoupon(row.id)
  ElMessage.success('优惠码已删除')
  fetchData()
}

function formatDate(s: string): string {
  return s ? s.slice(0, 10) : ''
}
</script>

<style scoped>
.coupon-code {
  font-family: var(--font-mono);
  font-weight: 600;
  color: var(--color-primary);
  letter-spacing: 0.5px;
}

.text-muted {
  color: var(--color-text-muted);
  font-size: 12px;
}

.text-mono {
  font-family: var(--font-mono);
  font-size: 13px;
}

.action-btns {
  display: flex;
  gap: 6px;
  justify-content: center;
}

.coupon-form {
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

.discount-preview {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 32px;
  background: var(--el-fill-color-lighter);
  border-radius: 6px;
  padding: 0 12px;
  margin-top: 2px;
}

.preview-label {
  font-size: 10px;
  color: var(--color-text-muted);
  line-height: 1;
}

.preview-value {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-primary);
  line-height: 1.2;
}

.form-hint {
  font-size: 12px;
  color: var(--color-text-muted);
  line-height: 32px;
}
</style>
