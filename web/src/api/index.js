import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(error.response?.data || error)
  }
)

export const authAPI = {
  register: (username, password) => api.post('/auth/register', { username, password }),
  login: (username, password) => api.post('/auth/login', { username, password }),
}

export const billAPI = {
  list: () => api.get('/bills'),
  create: (data) => api.post('/bills', data),
  update: (id, data) => api.put(`/bills/${id}`, data),
  delete: (id) => api.delete(`/bills/${id}`),
}

export const budgetAPI = {
  list: () => api.get('/budgets'),
  create: (data) => api.post('/budgets', data),
  update: (id, data) => api.put(`/budgets/${id}`, data),
}

export const assetAPI = {
  list: () => api.get('/assets'),
  create: (data) => api.post('/assets', data),
  update: (id, data) => api.put(`/assets/${id}`, data),
}

export const chatAPI = {
  send: (message, pageContext, sessionId) =>
    api.post('/chat', { message, page_context: pageContext, session_id: sessionId }),
  sendStream: (message, pageContext, sessionId, onChunk, onDone, onError, onThinking) => {
    const token = localStorage.getItem('token')
    const ctrl = new AbortController()
    fetch('/api/chat/stream', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
      },
      body: JSON.stringify({ message, page_context: pageContext, session_id: sessionId }),
      signal: ctrl.signal,
    }).then(async (resp) => {
      if (!resp.ok) {
        onError('请求失败')
        return
      }
      const reader = resp.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop()
        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          try {
            const event = JSON.parse(line.slice(6))
            if (event.type === 'chunk') onChunk(event.data)
            else if (event.type === 'thinking' && onThinking) onThinking(event.data)
            else if (event.type === 'thinking') onChunk('')
            else if (event.type === 'done') onDone(JSON.parse(event.data))
            else if (event.type === 'error') onError(event.data)
          } catch {}
        }
      }
    }).catch((err) => {
      if (err.name !== 'AbortError') onError('网络错误')
    })
    return ctrl
  },
  getHistory: (sessionId, limit = 20) =>
    api.get('/chat/history', { params: { session_id: sessionId, limit } }),
}

export const suggestionAPI = {
  get: () => api.get('/suggestions'),
}

export default api
