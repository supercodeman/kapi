import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useChatStore = defineStore('chat', () => {
  const messages = ref([])
  const currentPage = ref('首页')
  const sessionId = ref(1)
  const isLoading = ref(false)
  const executionPlan = ref(null)
  const pageData = ref({})
  const detailData = ref(null)
  const historyOffset = ref(0)
  const hasMoreHistory = ref(true)
  // 由外部组件设置，ChatWindow 监听后填入输入框
  const pendingInput = ref('')

  /**
   * 添加消息到列表
   * @param {string} role - 消息角色 (user/assistant)
   * @param {string} content - 消息内容
   * @param {string} pageContext - 页面上下文
   * @param {object} data - 关联数据
   * @param {object} extra - 额外字段 { thinking, durationMs }
   */
  function addMessage(role, content, pageContext, data, extra = {}) {
    messages.value.push({
      id: Date.now(),
      role,
      content,
      pageContext: pageContext || currentPage.value,
      data,
      thinking: extra.thinking || null,
      durationMs: extra.durationMs || null,
      createdAt: new Date().toISOString(),
    })
  }

  /**
   * 将历史消息插入到列表头部
   * @param {Array} msgs - 消息数组
   */
  function prependMessages(msgs) {
    const formatted = msgs.map((m, i) => ({
      id: m.created_at ? m.created_at * 1000 + i : Date.now() - 100000 + i,
      role: m.role,
      content: m.content,
      pageContext: m.page_context || '',
      data: null,
      thinking: null,
      durationMs: null,
      createdAt: m.created_at ? new Date(m.created_at * 1000).toISOString() : '',
    }))
    messages.value = [...formatted, ...messages.value]
  }

  function switchPage(page) {
    currentPage.value = page
  }

  function setLoading(val) {
    isLoading.value = val
  }

  function setExecutionPlan(plan) {
    executionPlan.value = plan
  }

  function clearPlan() {
    executionPlan.value = null
  }

  // 7b: 设置指定页面的数据
  function setPageData(page, data) {
    pageData.value[page] = data
  }

  function setDetailData(data) {
    detailData.value = data
  }

  function updateLastMessage(updates) {
    const msgs = messages.value
    if (msgs.length === 0) return
    const last = msgs[msgs.length - 1]
    if (last.role !== 'assistant') return
    if (updates.appendContent) last.content += updates.appendContent
    if (updates.content !== undefined) last.content = updates.content
    if (updates.data !== undefined) last.data = updates.data
    if (updates.thinking !== undefined) last.thinking = updates.thinking
    if (updates.durationMs !== undefined) last.durationMs = updates.durationMs
    if (updates.appendThinking) {
      last.thinking = (last.thinking || '') + updates.appendThinking
    }
  }

  function setPendingInput(text) {
    pendingInput.value = text
  }

  function reset() {
    messages.value = []
    currentPage.value = '首页'
    sessionId.value = 1
    isLoading.value = false
    executionPlan.value = null
    pageData.value = {}
    detailData.value = null
    historyOffset.value = 0
    hasMoreHistory.value = true
    pendingInput.value = ''
  }

  return {
    messages, currentPage, sessionId, isLoading, executionPlan, pageData, detailData,
    historyOffset, hasMoreHistory, pendingInput,
    addMessage, prependMessages, switchPage, setLoading, setExecutionPlan, clearPlan, setPageData, setDetailData, updateLastMessage, setPendingInput, reset,
  }
})
