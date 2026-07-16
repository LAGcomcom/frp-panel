import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory('/admin/'),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('../views/Login.vue'),
    },
    {
      path: '/',
      component: () => import('../views/Layout.vue'),
      redirect: '/dashboard',
      children: [
        {
          path: 'dashboard',
          name: 'Dashboard',
          component: () => import('../views/Dashboard.vue'),
          meta: { title: '仪表盘', icon: 'DataBoard' },
        },
        {
          path: 'servers',
          name: 'Servers',
          component: () => import('../views/servers/ServerList.vue'),
          meta: { title: '服务器管理', icon: 'Monitor' },
        },
        {
          path: 'servers/:id',
          name: 'ServerDetail',
          component: () => import('../views/servers/ServerDetail.vue'),
          meta: { title: '服务器详情', hidden: true },
        },
        {
          path: 'users',
          name: 'Users',
          component: () => import('../views/users/UserList.vue'),
          meta: { title: '用户管理', icon: 'User' },
        },
        {
          path: 'users/:id',
          name: 'UserDetail',
          component: () => import('../views/users/UserDetail.vue'),
          meta: { title: '用户详情', hidden: true },
        },
		{
		  path: 'user-groups',
		  name: 'UserGroups',
		  component: () => import('../views/users/UserGroupList.vue'),
		  meta: { title: '用户组管理', icon: 'UserFilled' },
		},
        {
          path: 'plans',
          name: 'Plans',
          component: () => import('../views/plans/PlanList.vue'),
          meta: { title: '套餐管理', icon: 'Goods' },
        },
        {
          path: 'orders',
          name: 'Orders',
          component: () => import('../views/orders/OrderList.vue'),
          meta: { title: '订单管理', icon: 'Document' },
        },
        {
          path: 'proxies',
          name: 'Proxies',
          component: () => import('../views/proxies/ProxyList.vue'),
          meta: { title: '代理管理', icon: 'Connection' },
        },
        {
          path: 'websites',
          name: 'Websites',
          component: () => import('../views/websites/WebsiteList.vue'),
          meta: { title: '网站管理', icon: 'ChromeFilled' },
        },
        {
          path: 'payment-configs',
          name: 'PaymentConfigs',
          component: () => import('../views/payments/PaymentConfigList.vue'),
          meta: { title: '支付管理', icon: 'Wallet' },
        },
        {
          path: 'coupons',
          name: 'Coupons',
          component: () => import('../views/coupons/CouponList.vue'),
          meta: { title: '优惠码', icon: 'Ticket' },
        },
        {
          path: 'announcements',
          name: 'Announcements',
          component: () => import('../views/announcements/AnnouncementList.vue'),
          meta: { title: '公告管理', icon: 'Bell' },
        },
        {
          path: 'settings',
          name: 'SystemSettings',
          component: () => import('../views/settings/SystemSettings.vue'),
          meta: { title: '系统设置', icon: 'Setting' },
        },
        {
          path: 'traffic',
          name: 'TrafficStats',
          component: () => import('../views/TrafficStats.vue'),
          meta: { title: '流量统计', icon: 'DataLine' },
        },
        {
          path: 'alerts',
          name: 'AlertList',
          component: () => import('../views/AlertList.vue'),
          meta: { title: '告警通知', icon: 'Bell' },
        },
      ],
    },
  ],
})

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('admin_token')
  if (to.path !== '/login' && !token) {
    next('/login')
  } else {
    next()
  }
})

export default router
