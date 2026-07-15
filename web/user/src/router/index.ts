import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory('/user/'),
  routes: [
    { path: '/login', name: 'Login', component: () => import('../views/Login.vue') },
    { path: '/register', name: 'Register', component: () => import('../views/Register.vue') },
    {
      path: '/',
      component: () => import('../views/Layout.vue'),
      redirect: '/home',
      children: [
        { path: 'home', name: 'Home', component: () => import('../views/Home.vue'), meta: { title: '首页', icon: 'House' } },
        { path: 'servers', name: 'Servers', component: () => import('../views/Servers.vue'), meta: { title: '服务器列表', icon: 'Monitor' } },
        { path: 'proxies', name: 'Proxies', component: () => import('../views/Proxies.vue'), meta: { title: '我的代理', icon: 'Connection' } },
        { path: 'proxies/create', name: 'CreateProxy', component: () => import('../views/ProxyEdit.vue'), meta: { title: '创建代理', hidden: true } },
        { path: 'traffic', name: 'Traffic', component: () => import('../views/Traffic.vue'), meta: { title: '流量统计', icon: 'DataLine' } },
        { path: 'alerts', name: 'Alerts', component: () => import('../views/Alerts.vue'), meta: { title: '告警通知', icon: 'Bell' } },
        { path: 'plans', name: 'Plans', component: () => import('../views/Plans.vue'), meta: { title: '套餐购买', icon: 'Goods' } },
        { path: 'orders', name: 'Orders', component: () => import('../views/Orders.vue'), meta: { title: '我的订单', icon: 'Document' } },
        { path: 'invite', name: 'Invite', component: () => import('../views/Invite.vue'), meta: { title: '邀请好友', icon: 'Share' } },
        { path: 'profile', name: 'Profile', component: () => import('../views/Profile.vue'), meta: { title: '个人设置', icon: 'Setting' } },
      ],
    },
  ],
})

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('user_token')
  if (!['/login', '/register'].includes(to.path) && !token) {
    next('/login')
  } else {
    next()
  }
})

export default router
