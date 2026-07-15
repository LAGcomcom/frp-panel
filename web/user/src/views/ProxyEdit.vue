<template>
  <el-card class="animate-in">
    <template #header>
      <div class="page-header">
        <span class="page-title">{{ isEdit ? '编辑代理' : '创建代理' }}</span>
      </div>
    </template>
    <el-form :model="form" label-width="100px" style="max-width: 600px">
      <el-form-item label="选择服务器" required>
        <el-select v-model="form.server_id" placeholder="选择服务器" style="width: 100%">
          <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.ip})`" :value="s.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="代理名称" required>
        <el-input v-model="form.name" placeholder="my-proxy" />
      </el-form-item>
      <el-form-item label="代理类型" required>
        <el-select v-model="form.type" style="width: 100%">
          <el-option label="TCP" value="tcp" />
          <el-option label="UDP" value="udp" />
          <el-option label="HTTP" value="http" />
          <el-option label="HTTPS" value="https" />
          <el-option label="STCP" value="stcp" />
          <el-option label="XTCP" value="xtcp" />
        </el-select>
      </el-form-item>
      <el-form-item label="本地 IP">
        <el-input v-model="form.local_ip" placeholder="127.0.0.1" />
      </el-form-item>
      <el-form-item label="本地端口" required>
        <el-input-number v-model="form.local_port" :min="1" :max="65535" />
      </el-form-item>
      <el-form-item label="远程端口" v-if="['tcp', 'udp'].includes(form.type)" required
        :error="form.remote_port > 0 && form.remote_port < 1024 ? '1-1023 为系统保留端口，不可使用' : ''"
        :validate-status="form.remote_port > 0 && form.remote_port < 1024 ? 'error' : ''">
        <el-input-number v-model="form.remote_port" :min="1" :max="65535" />
      </el-form-item>
      <el-form-item label="子域名" v-if="['http', 'https'].includes(form.type)">
        <el-input v-model="form.subdomain" placeholder="myapp" />
      </el-form-item>
      <el-form-item label="自定义域名" v-if="['http', 'https'].includes(form.type)">
        <el-input v-model="form.custom_domains_str" placeholder="每行一个域名" type="textarea" :rows="2" />
      </el-form-item>
      <el-form-item label="密钥" v-if="['stcp', 'xtcp'].includes(form.type)">
        <el-input v-model="form.secret_key" placeholder="访问密钥" />
      </el-form-item>
      <el-form-item label="加密传输">
        <el-switch v-model="form.use_encryption" />
      </el-form-item>
      <el-form-item label="压缩传输">
        <el-switch v-model="form.use_compression" />
      </el-form-item>
      <el-form-item label="带宽限制">
        <el-input-number v-model="form.bandwidth_limit_kb" :min="0" :step="100" />
        <span style="margin-left: 8px; color: var(--color-text-muted); font-size: 13px;">KB/s（0 表示不限制）</span>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="handleSubmit" :loading="loading">{{ isEdit ? '更新' : '创建' }}</el-button>
        <el-button @click="$router.back()">取消</el-button>
      </el-form-item>
    </el-form>
  </el-card>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { createProxy, getProxy, updateProxy, getAvailableServers } from '../api'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const servers = ref<any[]>([])
const isEdit = computed(() => !!route.query.edit)

const form = reactive({
  server_id: null as number | null,
  name: '', type: 'tcp', local_ip: '127.0.0.1', local_port: 8080,
  remote_port: 0, subdomain: '', custom_domains_str: '', secret_key: '',
  use_encryption: false, use_compression: false,
  bandwidth_limit_kb: 0,
})

onMounted(async () => {
  const res = await getAvailableServers()
  servers.value = Array.isArray(res.data) ? res.data : []

  if (isEdit.value) {
    const proxyRes = await getProxy(Number(route.query.edit))
    const p = proxyRes.data
    Object.assign(form, {
      server_id: p.server_id, name: p.name.replace(/^\d+_/, ''), type: p.type,
      local_ip: p.local_ip, local_port: p.local_port, remote_port: p.remote_port,
      subdomain: p.subdomain, custom_domains_str: (p.custom_domains || []).join('\n'),
      secret_key: p.secret_key,
      use_encryption: p.use_encryption, use_compression: p.use_compression,
      bandwidth_limit_kb: Math.floor((p.bandwidth_limit || 0) / 1024),
    })
  }
})

async function handleSubmit() {
  if (['tcp', 'udp'].includes(form.type) && form.remote_port > 0 && form.remote_port < 1024) {
    ElMessage.warning('1-1023 为系统保留端口，不可使用')
    return
  }

  loading.value = true
  try {
    const data: any = { ...form }
    if (form.custom_domains_str) {
      data.custom_domains = form.custom_domains_str.split('\n').map(s => s.trim()).filter(Boolean)
    }
    delete data.custom_domains_str
    data.bandwidth_limit = (data.bandwidth_limit_kb || 0) * 1024
    delete data.bandwidth_limit_kb

    if (isEdit.value) {
      await updateProxy(Number(route.query.edit), data)
      ElMessage.success('代理已更新')
    } else {
      await createProxy(data)
      ElMessage.success('代理已创建')
    }
    router.push('/proxies')
  } catch (e: any) {
    const msg = e.response?.data?.message || e.message || '操作失败'
    ElMessage.error(msg)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
</style>
