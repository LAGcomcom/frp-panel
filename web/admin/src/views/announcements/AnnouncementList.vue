<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">公告管理</span>
        <el-button type="primary" @click="openAdd">
          <el-icon><Plus /></el-icon>发布公告
        </el-button>
      </div>
    </template>

    <el-table :data="items" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="title" label="标题" min-width="180" />
      <el-table-column label="类型" width="90" align="center">
        <template #default="{ row }">
          <el-tag :type="typeTagMap[row.type] || 'info'" size="small">{{ typeNameMap[row.type] || row.type }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="排序" width="80" align="center">
        <template #default="{ row }">
          <span class="text-mono">{{ row.sort_order }}</span>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="80" align="center">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" @change="handleToggle(row)" size="small" />
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="170">
        <template #default="{ row }">{{ new Date(row.created_at).toLocaleString('zh-CN') }}</template>
      </el-table-column>
      <el-table-column label="操作" width="100" align="center" fixed="right">
        <template #default="{ row }">
          <div class="action-btns">
            <el-button size="small" @click="handleEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>
  </el-card>

  <el-dialog v-model="showDialog" :title="editingItem ? '编辑公告' : '发布公告'" width="580" append-to-body>
    <el-form :model="form" label-width="80px" class="announcement-form">
      <div class="form-section">
        <div class="form-section-title">基本信息</div>
        <el-form-item label="标题" required>
          <el-input v-model="form.title" placeholder="请输入公告标题" />
        </el-form-item>
      </div>
      <div class="form-section">
        <div class="form-section-title">公告内容</div>
        <el-form-item label="正文">
          <el-input v-model="form.content" type="textarea" :rows="6" placeholder="请输入公告内容" />
        </el-form-item>
      </div>
      <div class="form-section">
        <div class="form-section-title">显示设置</div>
        <el-row :gutter="16">
          <el-col :span="10">
            <el-form-item label="类型">
              <el-select v-model="form.type" style="width: 100%">
                <el-option label="信息" value="info" />
                <el-option label="警告" value="warning" />
                <el-option label="成功" value="success" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="排序">
              <el-input-number v-model="form.sort_order" :min="0" :max="9999" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="启用">
              <el-switch v-model="form.enabled" />
            </el-form-item>
          </el-col>
        </el-row>
      </div>
    </el-form>
    <template #footer>
      <el-button @click="showDialog = false">取消</el-button>
      <el-button type="primary" @click="handleSubmit" :loading="submitLoading">{{ editingItem ? '保存' : '发布' }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { getAnnouncements, createAnnouncement, updateAnnouncement, deleteAnnouncement, toggleAnnouncement } from '../../api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Delete } from '@element-plus/icons-vue'

const items = ref<any[]>([])
const loading = ref(false)
const showDialog = ref(false)
const editingItem = ref<any>(null)
const submitLoading = ref(false)

const typeNameMap: Record<string, string> = { info: '信息', warning: '警告', success: '成功' }
const typeTagMap: Record<string, string> = { info: 'info', warning: 'warning', success: 'success' }

const form = reactive({
  title: '',
  content: '',
  type: 'info',
  sort_order: 0,
  enabled: true,
})

onMounted(() => fetchData())

async function fetchData() {
  loading.value = true
  try {
    const res = await getAnnouncements()
    items.value = res.data
  } finally {
    loading.value = false
  }
}

function openAdd() {
  editingItem.value = null
  Object.assign(form, { title: '', content: '', type: 'info', sort_order: 0, enabled: true })
  showDialog.value = true
}

function handleEdit(row: any) {
  editingItem.value = row
  Object.assign(form, {
    title: row.title,
    content: row.content,
    type: row.type,
    sort_order: row.sort_order,
    enabled: row.enabled,
  })
  showDialog.value = true
}

async function handleSubmit() {
  if (!form.title.trim()) {
    ElMessage.warning('请输入公告标题')
    return
  }
  submitLoading.value = true
  try {
    if (editingItem.value) {
      await updateAnnouncement(editingItem.value.id, form)
      ElMessage.success('公告已更新')
    } else {
      await createAnnouncement(form)
      ElMessage.success('公告已发布')
    }
    showDialog.value = false
    editingItem.value = null
    fetchData()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '操作失败')
  } finally {
    submitLoading.value = false
  }
}

async function handleToggle(row: any) {
  await toggleAnnouncement(row.id)
  ElMessage.success('状态已更新')
  fetchData()
}

async function handleDelete(row: any) {
  await ElMessageBox.confirm(`确认删除公告"${row.title}"？`, '确认删除', { type: 'warning' })
  await deleteAnnouncement(row.id)
  ElMessage.success('公告已删除')
  fetchData()
}
</script>

<style scoped>
.text-mono {
  font-family: var(--font-mono);
  font-size: 13px;
}

.action-btns {
  display: flex;
  gap: 6px;
  justify-content: center;
}

.announcement-form {
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
</style>
