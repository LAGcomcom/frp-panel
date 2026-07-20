<template>
  <div class="layout">
    <!-- Sidebar -->
    <aside class="sidebar" :class="{ collapsed: isCollapsed }">
      <div class="sidebar-header">
        <div class="logo-mark"></div>
        <el-icon :size="20" color="var(--color-accent)"><Connection /></el-icon>
        <span v-if="!isCollapsed" class="sidebar-title">{{ siteTitle }}</span>
      </div>

      <nav class="sidebar-nav">
        <span v-if="!isCollapsed" class="nav-group-label">概览</span>
        <router-link
          :to="menuItems[0].path"
          class="nav-item"
          :class="{ active: isActive(menuItems[0].path) }"
        >
          <el-icon :size="16"><component :is="menuItems[0].icon" /></el-icon>
          <span v-if="!isCollapsed" class="nav-label">{{ menuItems[0].title }}</span>
        </router-link>

        <span v-if="!isCollapsed" class="nav-group-label">服务</span>
        <router-link
          v-for="item in menuItems.slice(1, 5)"
          :key="item.path"
          :to="item.path"
          class="nav-item"
          :class="{ active: isActive(item.path) }"
        >
          <el-icon :size="16"><component :is="item.icon" /></el-icon>
          <span v-if="!isCollapsed" class="nav-label">{{ item.title }}</span>
          <span v-if="item.path === '/alerts' && unreadCount > 0" class="nav-badge">{{ unreadCount > 99 ? '99+' : unreadCount }}</span>
        </router-link>

        <span v-if="!isCollapsed" class="nav-group-label">账户</span>
        <router-link
          v-for="item in accountMenuItems"
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
              <span class="user-role">用户</span>
            </div>
          </div>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="profile">
                <el-icon><Setting /></el-icon>个人设置
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

    <!-- Main -->
    <div class="main-wrapper">
      <header class="topbar">
        <div class="topbar-left">
          <span class="page-title">{{ route.meta.title }}</span>
        </div>
      </header>

      <main class="content">
        <div class="content-inner">
          <router-view />
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElNotification } from 'element-plus'
import { getPublicSettings } from '../api'

const route = useRoute()
const router = useRouter()
const isCollapsed = ref(false)
const unreadCount = ref(0)
const siteTitle = ref('FRP Panel')
const inviteRebateEnabled = ref(true)
let ws: WebSocket | null = null

const userEmail = computed(() => {
  try { return JSON.parse(localStorage.getItem('user_info') || '{}').email || '用户' } catch { return '用户' }
})

function connectWebSocket() {
  const token = localStorage.getItem('user_token')
  if (!token) return

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${protocol}//${window.location.host}/ws/user?token=${token}`
  ws = new WebSocket(wsUrl)

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data)
      if (msg.type === 'unread_count') {
        unreadCount.value = msg.data.count
      } else if (msg.type === 'notification') {
        unreadCount.value++
        const alert = msg.data
        ElNotification({
          title: alert.title || (alert.type === 'admin_message' ? '系统通知' : '告警'),
          message: alert.message,
          type: alert.level === 'error' ? 'error' : alert.level === 'warning' ? 'warning' : 'info',
          duration: 5000,
          position: 'top-right',
        })
      }
    } catch {}
  }

  ws.onclose = () => {
    // Reconnect after 5 seconds
    setTimeout(connectWebSocket, 5000)
  }

  ws.onerror = () => {
    ws?.close()
  }
}

onMounted(async () => {
  connectWebSocket()
  try {
    const res = await getPublicSettings()
    if (res.data?.site_title) siteTitle.value = res.data.site_title
    inviteRebateEnabled.value = res.data?.invite_rebate_enabled !== 'false'
    if (!inviteRebateEnabled.value && route.path === '/invite') {
      router.replace('/home')
    }
  } catch {}
})

onUnmounted(() => {
  ws?.close()
  ws = null
})

const menuItems = [
  { path: '/home', title: '首页', icon: 'House' },
  { path: '/proxies', title: '我的代理', icon: 'Connection' },
  { path: '/servers', title: '服务器列表', icon: 'Monitor' },
  { path: '/traffic', title: '流量统计', icon: 'DataLine' },
  { path: '/alerts', title: '告警通知', icon: 'Bell' },
  { path: '/plans', title: '套餐购买', icon: 'Goods' },
  { path: '/orders', title: '我的订单', icon: 'Document' },
  { path: '/invite', title: '邀请好友', icon: 'Share' },
  { path: '/profile', title: '个人设置', icon: 'Setting' },
]

const accountMenuItems = computed(() =>
  menuItems.slice(5).filter(item => item.path !== '/invite' || inviteRebateEnabled.value)
)

function isActive(path: string) {
  return route.path === path || route.path.startsWith(path + '/')
}

function handleCmd(cmd: string) {
  if (cmd === 'logout') {
    localStorage.removeItem('user_token')
    localStorage.removeItem('user_info')
    router.push('/login')
  } else if (cmd === 'profile') {
    router.push('/profile')
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

.nav-badge {
  margin-left: auto;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  background: var(--color-danger);
  color: #fff;
  font-size: 11px;
  font-weight: var(--font-bold);
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
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

.page-title {
  font-size: var(--text-base);
  font-weight: var(--font-semibold);
  color: var(--color-text);
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
</style>
