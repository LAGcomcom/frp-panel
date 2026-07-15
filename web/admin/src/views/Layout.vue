<template>
  <div class="layout">
    <!-- Sidebar -->
    <aside class="sidebar" :class="{ collapsed: isCollapsed }">
      <div class="sidebar-header">
        <div class="logo-mark"></div>
        <el-icon :size="20" color="var(--color-accent)"><Monitor /></el-icon>
        <span v-if="!isCollapsed" class="sidebar-title">{{ siteTitle }}</span>
      </div>

      <nav class="sidebar-nav">
        <span v-if="!isCollapsed" class="nav-group-label">概览</span>
        <router-link
          v-for="item in menuItems.slice(0, 1)"
          :key="item.path"
          :to="item.path"
          class="nav-item"
          :class="{ active: isActive(item.path) }"
        >
          <el-icon :size="16"><component :is="item.icon" /></el-icon>
          <span v-if="!isCollapsed" class="nav-label">{{ item.title }}</span>
        </router-link>

        <span v-if="!isCollapsed" class="nav-group-label">管理</span>
        <router-link
          v-for="item in menuItems.slice(1, 10)"
          :key="item.path"
          :to="item.path"
          class="nav-item"
          :class="{ active: isActive(item.path) }"
        >
          <el-icon :size="16"><component :is="item.icon" /></el-icon>
          <span v-if="!isCollapsed" class="nav-label">{{ item.title }}</span>
        </router-link>

        <span v-if="!isCollapsed" class="nav-group-label">监控</span>
        <router-link
          v-for="item in menuItems.slice(10)"
          :key="item.path"
          :to="item.path"
          class="nav-item"
          :class="{ active: isActive(item.path) }"
        >
          <el-icon :size="16"><component :is="item.icon" /></el-icon>
          <span v-if="!isCollapsed" class="nav-label">{{ item.title }}</span>
        </router-link>
      </nav>

      <!-- User section -->
      <div class="sidebar-user">
        <el-dropdown @command="handleCmd" trigger="click" placement="top-start">
          <div class="user-btn">
            <div class="user-avatar">{{ userEmail.charAt(0).toUpperCase() }}</div>
            <div v-if="!isCollapsed" class="user-info">
              <span class="user-name">{{ userEmail }}</span>
              <span class="user-role">管理员</span>
            </div>
          </div>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="password">
                <el-icon><Lock /></el-icon>修改密码
              </el-dropdown-item>
              <el-dropdown-item command="logout">
                <el-icon><SwitchButton /></el-icon>退出登录
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>

      <div class="sidebar-footer">
        <button class="collapse-btn" @click="isCollapsed = !isCollapsed">
          <el-icon :size="16">
            <component :is="isCollapsed ? 'Expand' : 'Fold'" />
          </el-icon>
        </button>
      </div>
    </aside>

    <!-- Password Dialog -->
    <el-dialog v-model="showPasswordDialog" title="修改密码" width="420" :close-on-click-modal="false" class="password-dialog">
      <div class="password-icon">
        <el-icon :size="48" color="var(--el-color-primary)"><Lock /></el-icon>
      </div>
      <el-form :model="passwordForm" label-width="0" class="password-form">
        <el-form-item>
          <el-input
            v-model="passwordForm.old_password"
            type="password"
            show-password
            placeholder="请输入旧密码"
            size="large"
            prefix-icon="Lock"
          />
        </el-form-item>
        <el-form-item>
          <el-input
            v-model="passwordForm.new_password"
            type="password"
            show-password
            placeholder="请输入新密码（至少6位）"
            size="large"
            prefix-icon="Lock"
          />
        </el-form-item>
        <el-form-item>
          <el-input
            v-model="passwordForm.confirm_password"
            type="password"
            show-password
            placeholder="请确认新密码"
            size="large"
            prefix-icon="Lock"
            @keyup.enter="handleChangePassword"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="password-footer">
          <el-button @click="showPasswordDialog = false" size="large">取消</el-button>
          <el-button type="primary" @click="handleChangePassword" :loading="passwordLoading" size="large" class="password-submit">
            确认修改
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- Main -->
    <div class="main-wrapper">
      <header class="topbar">
        <div class="topbar-left">
          <el-breadcrumb separator="/">
            <el-breadcrumb-item :to="{ path: '/dashboard' }">首页</el-breadcrumb-item>
            <el-breadcrumb-item v-if="route.meta.title">{{ route.meta.title }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
      </header>

      <main class="content">
        <div class="content-inner">
          <router-view />
        </div>
      </main>
    </div>

    <el-dialog
      v-model="showUpdateDialog"
      title="发现新版本"
      width="520"
      :close-on-click-modal="!updateInfo?.mandatory"
      :show-close="!updateInfo?.mandatory"
    >
      <div class="update-summary">
        <el-icon :size="28" color="var(--color-accent)"><UploadFilled /></el-icon>
        <div>
          <strong>{{ updateInfo?.title || `版本 ${updateInfo?.version}` }}</strong>
          <p>当前版本 {{ currentVersion }}，最新版本 {{ updateInfo?.version }}</p>
        </div>
      </div>
      <div class="update-notes">{{ updateInfo?.notes || '此版本包含稳定性与安全更新。' }}</div>
      <div v-if="updateInfo?.sha256" class="update-checksum">SHA-256：{{ updateInfo.sha256 }}</div>
      <template #footer>
        <el-button v-if="!updateInfo?.mandatory" @click="showUpdateDialog = false">稍后提醒</el-button>
        <el-button type="primary" @click="openUpdateDownload">获取更新</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '../stores/auth'
import { getSettings, changePassword, checkPanelUpdate } from '../api'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const isCollapsed = ref(false)
const siteTitle = ref('FRP 管理面板')

onMounted(async () => {
  try {
    const res = await getSettings()
    if (res.data?.site_title) siteTitle.value = res.data.site_title + ' 管理'
  } catch {}

  try {
    const res = await checkPanelUpdate()
    if (res.data?.enabled && res.data?.update_available && res.data?.release) {
      currentVersion.value = res.data.current_version
      updateInfo.value = res.data.release
      showUpdateDialog.value = true
    }
  } catch {
    // Update service availability must never block the admin panel.
  }
})

const showUpdateDialog = ref(false)
const currentVersion = ref('')
const updateInfo = ref<any>(null)

async function openUpdateDownload() {
  const version = updateInfo.value?.version
  if (!version) return
  try {
    const token = localStorage.getItem('admin_token')
    const response = await fetch(`/api/admin/update/download/${encodeURIComponent(version)}`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const blob = await response.blob()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = response.headers.get('Content-Disposition')?.match(/filename="([^"]+)"/)?.[1] || `frp-panel-${version}`
    link.click()
    URL.revokeObjectURL(url)
  } catch {
    ElMessage.error('更新包下载或校验失败')
  }
}

const showPasswordDialog = ref(false)
const passwordLoading = ref(false)
const passwordForm = ref({
  old_password: '',
  new_password: '',
  confirm_password: '',
})

const userEmail = computed(() => {
  try { return authStore.user?.email || JSON.parse(localStorage.getItem('admin_user') || '{}').email || 'Admin' } catch { return 'Admin' }
})

const menuItems = [
  { path: '/dashboard', title: '仪表盘', icon: 'DataBoard' },
  { path: '/servers', title: '服务器', icon: 'Monitor' },
  { path: '/users', title: '用户', icon: 'User' },
  { path: '/plans', title: '套餐', icon: 'Goods' },
  { path: '/orders', title: '订单', icon: 'Document' },
  { path: '/proxies', title: '代理', icon: 'Connection' },
  { path: '/websites', title: '网站', icon: 'ChromeFilled' },
  { path: '/payment-configs', title: '支付管理', icon: 'Wallet' },
  { path: '/coupons', title: '优惠码', icon: 'Ticket' },
  { path: '/announcements', title: '公告管理', icon: 'Bell' },
  { path: '/settings', title: '系统设置', icon: 'Setting' },
  { path: '/traffic', title: '流量统计', icon: 'DataLine' },
  { path: '/alerts', title: '告警', icon: 'Bell' },
]

function isActive(path: string) {
  return route.path === path || route.path.startsWith(path + '/')
}

function handleCmd(cmd: string) {
  if (cmd === 'logout') {
    authStore.logout()
    router.push('/login')
  } else if (cmd === 'password') {
    passwordForm.value = { old_password: '', new_password: '', confirm_password: '' }
    showPasswordDialog.value = true
  }
}

async function handleChangePassword() {
  if (passwordForm.value.new_password !== passwordForm.value.confirm_password) {
    ElMessage.error('两次输入的密码不一致')
    return
  }
  if (passwordForm.value.new_password.length < 6) {
    ElMessage.error('密码长度至少6位')
    return
  }

  passwordLoading.value = true
  try {
    await changePassword({
      old_password: passwordForm.value.old_password,
      new_password: passwordForm.value.new_password,
    })
    ElMessage.success('密码修改成功')
    showPasswordDialog.value = false
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '修改失败')
  } finally {
    passwordLoading.value = false
  }
}
</script>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  overflow: hidden;
}

/* ---- Sidebar ---- */
.sidebar {
  width: var(--sidebar-width);
  background: var(--color-sidebar);
  display: flex;
  flex-direction: column;
  transition: width var(--transition-normal);
  flex-shrink: 0;
  overflow: hidden;
  border-right: 1px solid var(--color-border-light);
}

.sidebar.collapsed {
  width: var(--sidebar-collapsed);
}

.sidebar-header {
  height: var(--header-height);
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0 var(--space-4);
  border-bottom: 1px solid var(--color-border-light);
  flex-shrink: 0;
  position: relative;
}

.logo-mark {
  width: 3px;
  height: 20px;
  background: var(--color-accent);
  border-radius: 2px;
  flex-shrink: 0;
}

.sidebar-title {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text);
  white-space: nowrap;
  letter-spacing: -0.01em;
}

/* ---- Nav ---- */
.sidebar-nav {
  flex: 1;
  padding: var(--space-2) var(--space-2);
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.nav-group-label {
  font-size: 11px;
  font-weight: var(--font-semibold);
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  padding: var(--space-3) var(--space-3) var(--space-1);
}

.nav-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: 0 var(--space-3);
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  text-decoration: none;
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  transition: all var(--transition-fast);
  white-space: nowrap;
  height: 34px;
  position: relative;
}

.nav-item:hover {
  background: var(--color-sidebar-hover);
  color: var(--color-text);
}

.nav-item.active {
  background: var(--color-sidebar-active);
  color: var(--color-accent);
  font-weight: var(--font-semibold);
}

.nav-item.active::before {
  content: '';
  position: absolute;
  left: -2px;
  top: 6px;
  bottom: 6px;
  width: 3px;
  background: var(--color-accent);
  border-radius: 2px;
}

/* ---- User section ---- */
.sidebar-user {
  padding: var(--space-2);
  border-top: 1px solid var(--color-border-light);
  flex-shrink: 0;
}

.sidebar-user .user-btn {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-2);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--transition-fast);
  width: 100%;
}

.sidebar-user .user-btn:hover {
  background: var(--color-sidebar-hover);
}

.sidebar-user .user-avatar {
  width: 28px;
  height: 28px;
  background: var(--color-accent-light);
  color: var(--color-accent);
  border-radius: var(--radius-full);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: var(--font-semibold);
  flex-shrink: 0;
}

.sidebar-user .user-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.sidebar-user .user-name {
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  color: var(--color-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-user .user-role {
  font-size: 11px;
  color: var(--color-text-muted);
}

.sidebar-footer {
  padding: var(--space-1) var(--space-2) var(--space-2);
  flex-shrink: 0;
}

.collapse-btn {
  width: 100%;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  color: var(--color-text-muted);
  cursor: pointer;
  border-radius: var(--radius-md);
  transition: all var(--transition-fast);
}

.collapse-btn:hover {
  background: var(--color-sidebar-hover);
  color: var(--color-text-secondary);
}

/* ---- Main Wrapper ---- */
.main-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-width: 0;
}

/* ---- Topbar ---- */
.topbar {
  height: var(--header-height);
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--color-border-light);
  display: flex;
  align-items: center;
  padding: 0 var(--space-6);
  flex-shrink: 0;
  position: sticky;
  top: 0;
  z-index: 10;
}

.topbar-left {
  display: flex;
  align-items: center;
}

/* ---- Content ---- */
.content {
  flex: 1;
  overflow-y: auto;
  background: var(--color-bg);
}

.content-inner {
  max-width: 1200px;
  margin: 0 auto;
  padding: var(--space-6);
}

/* ---- Password Dialog ---- */
.password-icon {
  text-align: center;
  margin-bottom: 24px;
}

.password-form {
  padding: 0 8px;
}

.password-form .el-form-item {
  margin-bottom: 20px;
}

.password-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

.password-submit {
  min-width: 120px;
}

.update-summary {
  display: flex;
  align-items: center;
  gap: 14px;
}

.update-summary strong {
  display: block;
  font-size: 16px;
}

.update-summary p {
  margin: 4px 0 0;
  color: var(--color-text-secondary);
}

.update-notes {
  margin-top: 18px;
  padding: 14px;
  background: var(--color-bg);
  border: 1px solid var(--color-border-light);
  border-radius: var(--radius-md);
  white-space: pre-wrap;
  max-height: 220px;
  overflow: auto;
}

.update-checksum {
  margin-top: 12px;
  color: var(--color-text-muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  overflow-wrap: anywhere;
}
</style>
