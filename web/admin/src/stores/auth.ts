import { defineStore } from 'pinia'
import { ref } from 'vue'
import { login as loginApi } from '../api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('admin_token') || '')
  const user = ref<any>(null)

  async function login(email: string, password: string) {
    const res = await loginApi({ email, password })
    token.value = res.data.token
    user.value = res.data.user
    localStorage.setItem('admin_token', res.data.token)
  }

  function logout() {
    token.value = ''
    user.value = null
    localStorage.removeItem('admin_token')
  }

  return { token, user, login, logout }
})
