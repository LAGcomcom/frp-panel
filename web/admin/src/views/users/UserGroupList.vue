<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <div>
          <div class="page-title">用户组管理</div>
          <div class="page-subtitle">控制不同用户组可以使用的节点</div>
        </div>
        <el-button type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon>添加用户组
        </el-button>
      </div>
    </template>

    <el-table :data="groups" v-loading="loading" stripe>
      <el-table-column prop="name" label="名称" min-width="140" />
      <el-table-column prop="description" label="说明" min-width="180">
        <template #default="{ row }">{{ row.description || '-' }}</template>
      </el-table-column>
      <el-table-column label="可用节点" min-width="260">
        <template #default="{ row }">
          <div class="node-tags" v-if="row.servers?.length">
            <el-tag v-for="server in row.servers" :key="server.id" size="small">{{ server.name }}</el-tag>
          </div>
          <span v-else class="muted">未分配节点</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" align="right">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="removeGroup(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑用户组' : '添加用户组'" width="520" append-to-body>
      <el-form label-width="90px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" maxlength="100" />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="form.description" type="textarea" :rows="2" maxlength="500" />
        </el-form-item>
        <el-form-item label="可用节点">
          <el-select v-model="form.server_ids" multiple clearable style="width: 100%" placeholder="选择该组可用的节点">
            <el-option
              v-for="server in servers"
              :key="server.id"
              :label="serverReady(server) ? server.name : `${server.name}（需重新部署）`"
              :value="server.id"
              :disabled="!serverReady(server)"
            />
          </el-select>
          <div class="form-help">旧节点重新部署后才能加入用户组，避免节点权限被绕过。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveGroup">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createUserGroup, deleteUserGroup, getServers, getUserGroups, updateUserGroup } from '../../api'

const groups = ref<any[]>([])
const servers = ref<any[]>([])
const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const editing = ref<any>(null)
const form = reactive({ name: '', description: '', server_ids: [] as number[] })

onMounted(loadData)

function serverReady(server: any) {
  if (server.plugin_auth_status) return server.plugin_auth_status === 'ready'
  return !!server.plugin_auth_enabled
}

async function loadData() {
  loading.value = true
  try {
    const [groupRes, serverRes] = await Promise.all([getUserGroups(), getServers({ size: 1000 })])
    groups.value = groupRes.data || []
    servers.value = serverRes.data?.list || []
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, { name: '', description: '', server_ids: [] })
  dialogVisible.value = true
}

function openEdit(group: any) {
  editing.value = group
  Object.assign(form, {
    name: group.name,
    description: group.description || '',
    server_ids: (group.servers || []).map((server: any) => server.id),
  })
  dialogVisible.value = true
}

async function saveGroup() {
  if (!form.name.trim()) {
    ElMessage.warning('请填写用户组名称')
    return
  }
  saving.value = true
  try {
    const payload = { name: form.name.trim(), description: form.description.trim(), server_ids: form.server_ids }
    if (editing.value) await updateUserGroup(editing.value.id, payload)
    else await createUserGroup(payload)
    ElMessage.success(editing.value ? '用户组已更新' : '用户组已创建')
    dialogVisible.value = false
    await loadData()
  } finally {
    saving.value = false
  }
}

async function removeGroup(group: any) {
  await ElMessageBox.confirm(`确认删除用户组“${group.name}”？`, '确认删除', { type: 'warning' })
  await deleteUserGroup(group.id)
  ElMessage.success('用户组已删除')
  await loadData()
}
</script>

<style scoped>
.page-subtitle { margin-top: 3px; font-size: 12px; color: var(--color-text-muted); }
.node-tags { display: flex; flex-wrap: wrap; gap: 6px; }
.muted, .form-help { color: var(--color-text-muted); font-size: 12px; }
.form-help { margin-top: 6px; line-height: 1.5; }
</style>
