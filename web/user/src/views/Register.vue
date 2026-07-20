<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-header">
        <div class="login-logo">
          <el-icon :size="28" color="#fff"><Connection /></el-icon>
        </div>
        <h1 class="login-title">注册账号</h1>
        <p class="login-subtitle">创建您的 FRP Panel 账户</p>
      </div>

      <el-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleRegister" label-position="top" size="large">
        <el-form-item label="邮箱地址" prop="email">
          <el-input v-model="form.email" placeholder="your@email.com" prefix-icon="Message" />
        </el-form-item>
        <el-form-item v-if="needCode" label="验证码" prop="code">
          <div class="code-row">
            <el-input v-model="form.code" placeholder="输入 6 位验证码" prefix-icon="Key" maxlength="6" />
            <el-button type="primary" plain :disabled="codeCooldown > 0" @click="handleSendCode" :loading="sendingCode" style="flex-shrink: 0">
              {{ codeCooldown > 0 ? codeCooldown + 's' : '获取验证码' }}
            </el-button>
          </div>
        </el-form-item>
        <el-form-item label="登录密码" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="至少 6 个字符" prefix-icon="Lock" />
        </el-form-item>
        <el-form-item v-if="inviteRebateEnabled" label="邀请码（选填）">
          <el-input v-model="form.invite_code" placeholder="输入邀请码" prefix-icon="Ticket" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="handleRegister" class="login-btn">
            {{ loading ? '注册中...' : '注 册' }}
          </el-button>
        </el-form-item>
      </el-form>

      <div class="login-footer">
        <span class="text-muted">已有账号？</span>
        <router-link to="/login" class="register-link">立即登录</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { register, sendCode, getPublicSettings } from '../api'
import { ElMessage, type FormInstance } from 'element-plus'

const router = useRouter()
const loading = ref(false)
const sendingCode = ref(false)
const formRef = ref<FormInstance>()
const needCode = ref(false)
const inviteRebateEnabled = ref(true)
const codeCooldown = ref(0)
const form = reactive({ email: '', password: '', invite_code: '', code: '' })

const rules = {
  email: [{ required: true, message: '请输入邮箱', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }, { min: 6, message: '密码至少 6 个字符', trigger: 'blur' }],
}

onMounted(async () => {
  try {
    const res = await getPublicSettings()
    needCode.value = res.data?.email_verification_enabled === 'true'
    inviteRebateEnabled.value = res.data?.invite_rebate_enabled !== 'false'
  } catch {}
})

async function handleSendCode() {
  if (!form.email) {
    ElMessage.warning('请先输入邮箱地址')
    return
  }
  sendingCode.value = true
  try {
    await sendCode({ email: form.email })
    ElMessage.success('验证码已发送，请检查邮箱')
    codeCooldown.value = 60
    const timer = setInterval(() => {
      codeCooldown.value--
      if (codeCooldown.value <= 0) clearInterval(timer)
    }, 1000)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '发送失败')
  } finally {
    sendingCode.value = false
  }
}

async function handleRegister() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    const payload = inviteRebateEnabled.value ? form : { ...form, invite_code: '' }
    const res = await register(payload)
    localStorage.setItem('user_token', res.data.token)
    ElMessage.success('注册成功')
    router.push('/home')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: #f6f8fa;
  position: relative;
}

.login-page::before {
  content: '';
  position: absolute;
  top: -50%;
  left: -50%;
  width: 200%;
  height: 200%;
  background: none;
  pointer-events: none;
}

.login-card {
  width: 400px;
  background: var(--color-surface);
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-2xl);
  padding: var(--space-10);
  box-shadow: var(--shadow-xl);
  position: relative;
  z-index: 1;
  animation: fadeIn 0.4s var(--ease-out) both;
}

.login-header {
  text-align: center;
  margin-bottom: var(--space-8);
}

.login-logo {
  width: 56px;
  height: 56px;
  background: var(--color-primary-gradient);
  border-radius: var(--radius-xl);
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto var(--space-4);
  box-shadow: var(--shadow-primary);
}

.login-title {
  font-size: var(--text-xl);
  font-weight: var(--font-bold);
  color: var(--color-text);
  margin: 0 0 var(--space-1);
  letter-spacing: -0.01em;
}

.login-subtitle {
  font-size: var(--text-sm);
  color: var(--color-text-muted);
  margin: 0;
}

.login-btn {
  width: 100%;
  height: 44px !important;
  font-size: var(--text-base) !important;
  margin-top: var(--space-2);
}

.login-footer {
  text-align: center;
  margin-top: var(--space-6);
  font-size: var(--text-sm);
}

.register-link {
  color: var(--color-primary);
  text-decoration: none;
  font-weight: var(--font-medium);
}

.register-link:hover {
  text-decoration: underline;
}

.code-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.code-row .el-input {
  flex: 1;
}
</style>
