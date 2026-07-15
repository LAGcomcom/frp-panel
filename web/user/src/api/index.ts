import http from './http'

// Auth
export const register = (data: { email: string; password: string; invite_code?: string; code?: string }) =>
  http.post('/user/register', data) as any
export const sendCode = (data: { email: string }) =>
  http.post('/user/send-code', data) as any
export const getPublicSettings = () => http.get('/settings/public') as any
export const login = (data: { email: string; password: string }) =>
  http.post('/user/login', data) as any
export const getProfile = () => http.get('/user/profile') as any
export const updateProfile = (data: any) => http.put('/user/profile', data) as any
export const changePassword = (data: { old_password: string; new_password: string }) =>
  http.post('/user/change-password', data) as any

// Proxies
export const getProxies = (params?: any) => http.get('/proxies', { params }) as any
export const getProxy = (id: number) => http.get(`/proxies/${id}`) as any
export const createProxy = (data: any) => http.post('/proxies', data) as any
export const updateProxy = (id: number, data: any) => http.put(`/proxies/${id}`, data) as any
export const deleteProxy = (id: number) => http.delete(`/proxies/${id}`) as any
export const enableProxy = (id: number) => http.post(`/proxies/${id}/enable`) as any
export const disableProxy = (id: number) => http.post(`/proxies/${id}/disable`) as any

// Plans
export const getPlans = () => http.get('/plans') as any
export const getPlan = (id: number) => http.get(`/plans/${id}`) as any

// Coupons
export const verifyCoupon = (params: { code: string; plan_id: number; duration_type: string }) =>
  http.post('/coupons/verify', null, { params }) as any

// Orders
export const createOrder = (data: any) => http.post('/orders', data) as any
export const getOrders = (params?: any) => http.get('/orders', { params }) as any
export const getOrder = (id: number) => http.get(`/orders/${id}`) as any
export const createRechargeOrder = (data: { amount: number; pay_method: string }) =>
  http.post('/orders/recharge', data) as any

// Payment methods
export const getPaymentMethods = () => http.get('/payment-methods') as any

// Traffic
export const getTrafficStats = () => http.get('/traffic/stats') as any
export const getTrafficLogs = (params?: any) => http.get('/traffic/logs', { params }) as any

// Alerts
export const getAlerts = (params?: any) => http.get('/alerts', { params }) as any
export const markAlertRead = (id: number) => http.post(`/alerts/${id}/read`) as any

// Invite
export const getInviteStats = () => http.get('/user/invite-stats') as any

// User Coupons
export const createUserCoupon = (data: { assigned_to: number; amount: number; end_time: string }) =>
  http.post('/coupons/create', data) as any
export const getMyCoupons = () => http.get('/coupons/mine') as any
export const getMyAvailableCoupons = () => http.get('/coupons/available') as any

// API Key
export const getApiKey = () => http.get('/user/api-key') as any
export const regenerateApiKey = () => http.post('/user/api-key/regenerate') as any

// Frpc Config
export const getFrpcConfig = (serverId: number) => http.get(`/proxies/config/${serverId}`) as any

// Servers
export const getAvailableServers = (params?: any) => http.get('/servers/available', { params }) as any

// Ports
export const getServerPorts = (serverId: number) => http.get(`/proxies/ports/${serverId}`) as any

// Announcements
export const getActiveAnnouncements = () => http.get('/announcements') as any
