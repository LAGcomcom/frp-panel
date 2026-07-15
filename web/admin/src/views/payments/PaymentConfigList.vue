<template>
  <div>
    <el-card class="animate-in">
      <template #header>
        <div class="page-header">
          <span class="page-title">支付管理</span>
          <el-button type="primary" @click="openAdd">
            <el-icon><Plus /></el-icon>添加支付渠道
          </el-button>
        </div>
      </template>

      <div v-loading="loading" class="payment-grid">
        <div v-for="item in configs" :key="item.id" class="payment-card" :class="{ disabled: !item.enabled }">
          <div class="payment-card-header">
            <div class="payment-card-left">
              <div class="payment-icon" :class="'type-' + item.type">
                {{ typeIcon[item.type] || '$' }}
              </div>
              <div class="payment-info">
                <span class="payment-name">{{ item.name }}</span>
                <el-tag :type="typeTagMap[item.type] || 'info'" size="small">{{ typeMap[item.type] || item.type }}</el-tag>
              </div>
            </div>
            <el-switch v-model="item.enabled" @change="handleToggle(item)" :loading="item._toggling" />
          </div>
          <div class="payment-card-body">
            <div class="payment-meta">
              <span class="meta-label">配置</span>
              <span class="meta-value">{{ summarizeConfig(item.config) }}</span>
            </div>
            <div class="payment-meta">
              <span class="meta-label">排序</span>
              <span class="meta-value">{{ item.sort_order }}</span>
            </div>
          </div>
          <div class="payment-card-actions">
            <el-button size="small" @click="openEdit(item)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(item)">删除</el-button>
          </div>
        </div>

        <div v-if="!loading && configs.length === 0" class="payment-empty">
          <el-empty description="暂无支付渠道" :image-size="48" />
        </div>
      </div>
    </el-card>

    <!-- Add/Edit Dialog -->
    <el-dialog v-model="showDialog" :title="editingId ? '编辑支付渠道' : '添加支付渠道'" width="560" class="payment-dialog">
      <el-form :model="form" label-width="100px" class="payment-form">
        <div class="form-section">
          <div class="form-section-title">基本信息</div>
          <el-form-item label="渠道名称" required>
            <el-input v-model="form.name" placeholder="如：支付宝官方、易支付" />
          </el-form-item>
          <el-form-item label="支付类型" required>
            <el-radio-group v-model="form.type" class="type-radio-group">
              <el-radio-button value="alipay">
                <span class="type-radio-icon type-alipay">支</span>支付宝
              </el-radio-button>
              <el-radio-button value="usdt">
                <span class="type-radio-icon type-usdt">$</span>USDT
              </el-radio-button>
              <el-radio-button value="epay">
                <span class="type-radio-icon type-epay">易</span>易支付
              </el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="排序">
            <el-input-number v-model="form.sort_order" :min="0" :max="999" />
          </el-form-item>
        </div>

        <div class="form-section">
          <div class="form-section-title">商户配置</div>
          <!-- Alipay fields -->
          <template v-if="form.type === 'alipay'">
            <el-form-item label="App ID">
              <el-input v-model="configFields.app_id" placeholder="支付宝应用 AppID" />
            </el-form-item>
            <el-form-item label="应用私钥">
              <el-input v-model="configFields.private_key" type="textarea" :rows="3" placeholder="RSA2 私钥" />
            </el-form-item>
            <el-form-item label="支付宝公钥">
              <el-input v-model="configFields.alipay_public_key" type="textarea" :rows="3" placeholder="支付宝公钥" />
            </el-form-item>
            <el-form-item label="回调地址">
              <el-input v-model="configFields.notify_url" placeholder="https://your-domain.com/api/pay/notify" />
            </el-form-item>
          </template>
          <!-- USDT fields -->
          <template v-if="form.type === 'usdt'">
            <el-form-item label="钱包地址">
              <el-input v-model="configFields.wallet_address" placeholder="TRC20 钱包地址" />
            </el-form-item>
            <el-form-item label="API 地址">
              <el-input v-model="configFields.api_url" placeholder="支付网关 API 地址" />
            </el-form-item>
            <el-form-item label="API Key">
              <el-input v-model="configFields.api_key" placeholder="网关 API Key" />
            </el-form-item>
          </template>
          <!-- Epay fields -->
          <template v-if="form.type === 'epay'">
            <el-form-item label="接口地址">
              <el-input v-model="configFields.api_url" placeholder="https://pay.example.com" />
            </el-form-item>
            <el-form-item label="商户 PID">
              <el-input v-model="configFields.pid" placeholder="商户 ID" />
            </el-form-item>
            <el-form-item label="商户密钥">
              <el-input v-model="configFields.key" placeholder="商户密钥 Key" />
            </el-form-item>
            <el-form-item label="回调地址">
              <el-input v-model="configFields.notify_url" placeholder="https://your-domain.com/api/pay/notify" />
            </el-form-item>
          </template>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitLoading">{{ editingId ? '更新' : '创建' }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Delete } from '@element-plus/icons-vue'
import {
  getPaymentConfigs, createPaymentConfig, updatePaymentConfig,
  deletePaymentConfig, togglePaymentConfig,
} from '../../api'

const configs = ref<any[]>([])
const loading = ref(false)
const showDialog = ref(false)
const submitLoading = ref(false)
const editingId = ref<number | null>(null)

const typeMap: Record<string, string> = {
  alipay: '支付宝', usdt: 'USDT', epay: '易支付',
}
const typeTagMap: Record<string, string> = {
  alipay: 'primary', usdt: 'warning', epay: 'info',
}
const typeIcon: Record<string, string> = {
  alipay: '支', usdt: '$', epay: '易',
}

const defaultConfigFields: Record<string, Record<string, string>> = {
  alipay: { app_id: '', private_key: '', alipay_public_key: '', notify_url: '' },
  usdt: { wallet_address: '', api_url: '', api_key: '' },
  epay: { api_url: '', pid: '', key: '', notify_url: '' },
}

const form = reactive({
  name: '',
  type: 'alipay',
  sort_order: 0,
})

const configFields = reactive<Record<string, string>>({ ...defaultConfigFields.alipay })

// Reset config fields when type changes
watch(() => form.type, (newType) => {
  if (!editingId.value) {
    Object.assign(configFields, defaultConfigFields[newType] || {})
  }
})

onMounted(() => fetchData())

async function fetchData() {
  loading.value = true
  try {
    const res = await getPaymentConfigs()
    configs.value = res.data || []
  } finally {
    loading.value = false
  }
}

function openAdd() {
  editingId.value = null
  Object.assign(form, { name: '', type: 'alipay', sort_order: 0 })
  Object.assign(configFields, defaultConfigFields.alipay)
  showDialog.value = true
}

function openEdit(row: any) {
  editingId.value = row.id
  Object.assign(form, {
    name: row.name,
    type: row.type,
    sort_order: row.sort_order || 0,
  })
  // Parse existing config into fields
  try {
    const parsed = JSON.parse(row.config || '{}')
    Object.assign(configFields, defaultConfigFields[row.type] || {}, parsed)
  } catch {
    Object.assign(configFields, defaultConfigFields[row.type] || {})
  }
  showDialog.value = true
}

async function handleSubmit() {
  if (!form.name || !form.type) {
    ElMessage.error('请填写必填项')
    return
  }
  submitLoading.value = true
  try {
    // Build config JSON from fields, removing empty values
    const config: Record<string, string> = {}
    for (const [k, v] of Object.entries(configFields)) {
      if (v) config[k] = v
    }
    const payload = {
      name: form.name,
      type: form.type,
      sort_order: form.sort_order,
      config: JSON.stringify(config),
    }
    if (editingId.value) {
      await updatePaymentConfig(editingId.value, payload)
      ElMessage.success('更新成功')
    } else {
      await createPaymentConfig(payload)
      ElMessage.success('创建成功')
    }
    showDialog.value = false
    fetchData()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '操作失败')
  } finally {
    submitLoading.value = false
  }
}

async function handleToggle(row: any) {
  row._toggling = true
  try {
    await togglePaymentConfig(row.id)
    ElMessage.success(row.enabled ? '已启用' : '已禁用')
  } catch {
    row.enabled = !row.enabled
    ElMessage.error('操作失败')
  } finally {
    row._toggling = false
  }
}

async function handleDelete(row: any) {
  await ElMessageBox.confirm(`确认删除支付渠道 "${row.name}"？`, '确认删除', { type: 'warning' })
  await deletePaymentConfig(row.id)
  ElMessage.success('删除成功')
  fetchData()
}

function summarizeConfig(configStr: string): string {
  if (!configStr) return '未配置'
  try {
    const obj = JSON.parse(configStr)
    const keys = Object.keys(obj)
    if (keys.length === 0) return '未配置'
    const priority = ['merchant_id', 'mch_id', 'pid', 'app_id', 'wallet_address', 'api_url']
    for (const k of priority) {
      if (obj[k]) return `${k}: ${obj[k]}`
    }
    const firstKey = keys[0]
    return `${firstKey}: ${obj[firstKey]}`
  } catch {
    return configStr.slice(0, 40) + (configStr.length > 40 ? '...' : '')
  }
}
</script>

<style scoped>
/* ---- Card Grid ---- */
.payment-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}

.payment-card {
  border: 1px solid var(--color-border-light);
  border-radius: 10px;
  padding: 16px;
  transition: all var(--transition-fast);
  background: var(--color-bg);
}

.payment-card:hover {
  border-color: var(--color-accent);
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.04);
}

.payment-card.disabled {
  opacity: 0.55;
}

.payment-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.payment-card-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.payment-icon {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  font-weight: 700;
  color: #fff;
  flex-shrink: 0;
}

.payment-icon.type-alipay { background: linear-gradient(135deg, #1677ff, #0958d9); }

.payment-icon.type-usdt { background: linear-gradient(135deg, #26a17b, #1a8a6a); }
.payment-icon.type-epay { background: linear-gradient(135deg, #ff6a00, #e05500); }

.payment-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.payment-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--color-text);
}

.payment-card-body {
  margin-bottom: 12px;
}

.payment-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}

.meta-label {
  font-size: 12px;
  color: var(--color-text-muted);
  flex-shrink: 0;
  width: 32px;
}

.meta-value {
  font-size: 12px;
  color: var(--color-text-secondary);
  font-family: var(--font-mono);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.payment-card-actions {
  display: flex;
  justify-content: flex-end;
  gap: 4px;
  border-top: 1px solid var(--color-border-light);
  padding-top: 10px;
}

.payment-empty {
  grid-column: 1 / -1;
}

/* ---- Dialog Form ---- */
.payment-form {
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

.type-radio-group {
  display: flex;
  gap: 8px;
}

.type-radio-group .el-radio-button {
  flex: 1;
}

.type-radio-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  margin-right: 4px;
  vertical-align: middle;
}

.type-radio-icon.type-alipay { background: #1677ff; }

.type-radio-icon.type-usdt { background: #26a17b; }
.type-radio-icon.type-epay { background: #ff6a00; }
</style>
