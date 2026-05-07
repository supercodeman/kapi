import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authAPI } from '../api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')
  const username = ref(localStorage.getItem('username') || '')
  const user = ref(null)

  const isLoggedIn = computed(() => !!token.value)

  async function login(uname, password) {
    const res = await authAPI.login(uname, password)
    token.value = res.data.token
    username.value = res.data.username || uname
    localStorage.setItem('token', res.data.token)
    localStorage.setItem('username', username.value)
    return res
  }

  async function register(uname, password) {
    const res = await authAPI.register(uname, password)
    return res
  }

  function logout() {
    token.value = ''
    username.value = ''
    user.value = null
    localStorage.removeItem('token')
    localStorage.removeItem('username')
  }

  return { token, username, user, isLoggedIn, login, register, logout }
})
