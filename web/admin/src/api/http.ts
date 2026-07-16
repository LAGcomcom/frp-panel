import axios from 'axios'
import { ElMessage } from 'element-plus'

const http = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

const technicalMessagePattern = /json:|cannot unmarshal|struct field|type float|type int|validation for|failed on the|strconv\.|sql:|database error|gorm|\.go:\d+|stack trace|panic:/i

function friendlyErrorMessage(message?: string, status?: number): string {
  if (status === 401) return '登录已过期，请重新登录'
  if (status === 403) return '当前账号没有权限执行此操作'
  if (status === 404) return '请求的内容不存在或已被删除'
  if (status && status >= 500) return '服务暂时不可用，请稍后重试'
  if (message && !technicalMessagePattern.test(message)) return message
  if (status === 400) return '提交内容有误，请检查后重试'
  return '操作失败，请稍后重试'
}

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('admin_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

http.interceptors.response.use(
  (response) => {
    const data = response.data
    if (data.code !== 0) {
      const message = friendlyErrorMessage(data.message, data.code)
      ElMessage.error(message)
      return Promise.reject(new Error(message))
    }
    return data
  },
  (error) => {
    const status = error.response?.status
    if (status === 401) {
      localStorage.removeItem('admin_token')
      window.location.href = '/admin/login'
    }
    console.error('[API request failed]', error)
    const message = error.code === 'ECONNABORTED'
      ? '请求超时，请稍后重试'
      : !error.response
        ? '网络连接失败，请检查网络后重试'
        : friendlyErrorMessage(error.response?.data?.message, status)
    ElMessage.error(message)
    return Promise.reject(new Error(message))
  }
)

export default http
