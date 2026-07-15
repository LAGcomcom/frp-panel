import http from './http'

// Auth
export const login = (data: { email: string; password: string }) =>
  http.post('/admin/login', data) as any
export const changePassword = (data: { old_password: string; new_password: string }) =>
  http.post('/user/change-password', data) as any

// Dashboard
export const getDashboard = () => http.get('/admin/dashboard') as any

// Users
export const getUsers = (params?: any) => http.get('/admin/users', { params }) as any
export const getUser = (id: number) => http.get(`/admin/users/${id}`) as any
export const updateUser = (id: number, data: any) => http.put(`/admin/users/${id}`, data) as any
export const banUser = (id: number) => http.post(`/admin/users/${id}/ban`) as any
export const unbanUser = (id: number) => http.post(`/admin/users/${id}/unban`) as any

// Servers
export const getServers = (params?: any) => http.get('/admin/servers', { params }) as any
export const getServer = (id: number) => http.get(`/admin/servers/${id}`) as any
export const createServer = (data: any) => http.post('/admin/servers', data) as any
export const updateServer = (id: number, data: any) => http.put(`/admin/servers/${id}`, data) as any
export const deleteServer = (id: number) => http.delete(`/admin/servers/${id}`) as any
export const deployServer = (id: number) => http.post(`/admin/servers/${id}/deploy`) as any
export const restartServer = (id: number) => http.post(`/admin/servers/${id}/restart`) as any
export const stopServer = (id: number) => http.post(`/admin/servers/${id}/stop`) as any
export const getServerClients = (id: number) => http.get(`/admin/servers/${id}/clients`) as any
export const getServerProxies = (id: number) => http.get(`/admin/servers/${id}/proxies`) as any
export const getServerMetrics = (id: number, hours?: number) =>
  http.get(`/admin/servers/${id}/metrics`, { params: { hours: hours || 1 } }) as any
export const installAgent = (id: number) => http.post(`/admin/servers/${id}/install-agent`) as any

// Plans
export const getPlans = (params?: any) => http.get('/admin/plans', { params }) as any
const planNumberFields = [
  'max_proxies', 'max_bandwidth', 'max_traffic', 'max_ports', 'duration_days',
  'price_monthly', 'price_quarterly', 'price_yearly', 'sort_order',
]
const normalizePlanPayload = (data: any) => {
  const payload = { ...data }
  for (const field of planNumberFields) {
    if (payload[field] === undefined) continue
    const value = payload[field] === '' || payload[field] === null ? 0 : Number(payload[field])
    if (!Number.isFinite(value)) throw new Error(`${field} must be a number`)
    payload[field] = value
  }
  return payload
}
export const createPlan = (data: any) => http.post('/admin/plans', normalizePlanPayload(data)) as any
export const updatePlan = (id: number, data: any) => http.put(`/admin/plans/${id}`, normalizePlanPayload(data)) as any
export const togglePlanStatus = (id: number) => http.put(`/admin/plans/${id}/toggle`) as any
export const deletePlan = (id: number) => http.delete(`/admin/plans/${id}`) as any

// Orders
export const getOrders = (params?: any) => http.get('/admin/orders', { params }) as any
export const refundOrder = (id: number) => http.post(`/admin/orders/${id}/refund`) as any

// Proxies
export const getProxies = (params?: any) => http.get('/admin/proxies', { params }) as any

// Traffic
export const getTrafficStats = () => http.get('/admin/traffic/stats') as any

// Alerts
export const getAlerts = (params?: any) => http.get('/admin/alerts', { params }) as any
export const sendNotification = (data: { user_id?: number; title: string; message: string }) =>
  http.post('/admin/alerts/send', data) as any

// Recharge
export const rechargeBalance = (data: { user_id: number; amount: number; remark?: string }) =>
  http.post('/admin/orders/recharge', data) as any

// Websites
export const getWebsites = (params?: any) => http.get('/admin/websites', { params }) as any
export const createWebsite = (data: any) => http.post('/admin/websites', data) as any
export const getWebsite = (id: number) => http.get(`/admin/websites/${id}`) as any
export const updateWebsite = (id: number, data: any) => http.put(`/admin/websites/${id}`, data) as any
export const deleteWebsite = (id: number) => http.delete(`/admin/websites/${id}`) as any
export const checkWebsite = (id: number) => http.post(`/admin/websites/${id}/check`) as any

// Payment Configs
export const getPaymentConfigs = () => http.get('/admin/payment-configs') as any
export const createPaymentConfig = (data: any) => http.post('/admin/payment-configs', data) as any
export const getPaymentConfig = (id: number) => http.get(`/admin/payment-configs/${id}`) as any
export const updatePaymentConfig = (id: number, data: any) => http.put(`/admin/payment-configs/${id}`, data) as any
export const deletePaymentConfig = (id: number) => http.delete(`/admin/payment-configs/${id}`) as any
export const togglePaymentConfig = (id: number) => http.post(`/admin/payment-configs/${id}/toggle`) as any

// Settings
export const getSettings = () => http.get('/admin/settings') as any
export const updateSettings = (data: Record<string, string>) => http.put('/admin/settings', data) as any
export const testSMTP = (data: { to: string }) => http.post('/admin/settings/test-smtp', data) as any
export const getPublicSettings = () => http.get('/settings/public') as any
export const checkPanelUpdate = () => http.get('/admin/update/check') as any

// Coupons
export const getCoupons = () => http.get('/admin/coupons') as any
export const createCoupon = (data: any) => http.post('/admin/coupons', data) as any
export const updateCoupon = (id: number, data: any) => http.put(`/admin/coupons/${id}`, data) as any
export const deleteCoupon = (id: number) => http.delete(`/admin/coupons/${id}`) as any
export const toggleCoupon = (id: number) => http.post(`/admin/coupons/${id}/toggle`) as any

// Announcements
export const getAnnouncements = () => http.get('/admin/announcements') as any
export const createAnnouncement = (data: any) => http.post('/admin/announcements', data) as any
export const updateAnnouncement = (id: number, data: any) => http.put(`/admin/announcements/${id}`, data) as any
export const deleteAnnouncement = (id: number) => http.delete(`/admin/announcements/${id}`) as any
export const toggleAnnouncement = (id: number) => http.post(`/admin/announcements/${id}/toggle`) as any
