<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-header">
        <div class="login-logo">
          <el-icon :size="28" color="#fff"><Connection /></el-icon>
        </div>
        <h1 class="login-title">FRP Panel</h1>
        <p class="login-subtitle">登录您的账户</p>
      </div>

      <el-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleLogin" label-position="top" size="large">
        <el-form-item label="邮箱地址" prop="email">
          <el-input v-model="form.email" placeholder="your@email.com" prefix-icon="Message" />
        </el-form-item>
        <el-form-item label="登录密码" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="输入密码" prefix-icon="Lock" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="handleLogin" class="login-btn">
            {{ loading ? '登录中...' : '登 录' }}
          </el-button>
        </el-form-item>
      </el-form>

      <div class="login-footer">
        <span class="text-muted">还没有账号？</span>
        <router-link to="/register" class="register-link">立即注册</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { login as loginApi } from '../api'
import { ElMessage, type FormInstance } from 'element-plus'

const router = useRouter()
const loading = ref(false)
const formRef = ref<FormInstance>()
const form = reactive({ email: '', password: '' })

const rules = {
  email: [{ required: true, message: '请输入邮箱', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function handleLogin() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    const res = await loginApi(form)
    localStorage.setItem('user_token', res.data.token)
    localStorage.setItem('user_info', JSON.stringify(res.data.user))
    ElMessage.success('登录成功')
    await router.push('/home')
  } catch (e: any) {
    ElMessage.error(e?.message || '登录失败')
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
</style>
