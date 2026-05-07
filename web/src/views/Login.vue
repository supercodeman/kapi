<template>
  <div class="login-container">
    <div class="login-card">
      <h1>咔皮记账 AI 助手</h1>
      <p class="subtitle">智能财务管理，从对话开始</p>
      <form @submit.prevent="handleSubmit">
        <input v-model="username" type="text" placeholder="用户名" required minlength="3" />
        <input v-model="password" type="password" placeholder="密码" required minlength="6" />
        <button type="submit" :disabled="loading">{{ isLogin ? '登录' : '注册' }}</button>
        <p class="toggle" @click="isLogin = !isLogin">
          {{ isLogin ? '没有账号？点击注册' : '已有账号？点击登录' }}
        </p>
        <p v-if="error" class="error">{{ error }}</p>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const username = ref('')
const password = ref('')
const isLogin = ref(true)
const loading = ref(false)
const error = ref('')

async function handleSubmit() {
  loading.value = true
  error.value = ''
  try {
    if (isLogin.value) {
      await authStore.login(username.value, password.value)
      router.push('/')
    } else {
      await authStore.register(username.value, password.value)
      error.value = ''
      isLogin.value = true
      alert('注册成功，请登录')
    }
  } catch (e) {
    error.value = e?.message || '操作失败'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f5f5f5;
}
.login-card {
  background: #fff;
  border-radius: 12px;
  padding: 40px;
  width: 360px;
  box-shadow: 0 2px 12px rgba(0,0,0,0.08);
  text-align: center;
}
h1 { font-size: 22px; color: #1a1a2e; margin-bottom: 4px; }
.subtitle { color: #888; font-size: 14px; margin-bottom: 28px; }
input {
  width: 100%;
  padding: 12px 16px;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  font-size: 14px;
  margin-bottom: 12px;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.2s;
}
input:focus { border-color: #4fc3f7; }
button {
  width: 100%;
  padding: 12px;
  background: #4fc3f7;
  color: #fff;
  border: none;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.2s;
}
button:hover { background: #039be5; }
button:disabled { background: #ccc; cursor: not-allowed; }
.toggle { color: #4fc3f7; font-size: 13px; margin-top: 16px; cursor: pointer; }
.error { color: #ef5350; font-size: 13px; margin-top: 8px; }
</style>
