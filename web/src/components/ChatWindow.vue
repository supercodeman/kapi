<template>
  <div class="chat-window">
    <div class="chat-header">
      <span class="page-badge">{{ chatStore.currentPage }}</span>
      <span class="header-title">AI 助手</span>
      <!-- 7a: Think 开关 -->
      <button
        class="thinking-toggle"
        :class="{ active: showThinking }"
        @click="showThinking = !showThinking"
      >思维</button>
    </div>

    <div class="messages" ref="messagesRef">
      <!-- 加载更多历史消息 -->
      <div v-if="chatStore.hasMoreHistory" class="load-more-wrapper">
        <button class="load-more-btn" @click="loadMoreHistory" :disabled="loadingHistory">
          {{ loadingHistory ? '加载中...' : '加载更多' }}
        </button>
      </div>

      <!-- 7d: 冷启动欢迎 -->
      <div v-if="chatStore.messages.length === 0" class="empty-state">
        <div class="welcome-icon">🤖</div>
        <p class="welcome-title">你好！我是咔皮记账 AI 助手</p>
        <p class="welcome-hint">我可以帮你记账、查询消费、管理预算和资产</p>
        <div class="quick-actions">
          <button @click="quickSend('今天花了多少钱')">今日消费</button>
          <button @click="quickSend('帮我记一笔')">快速记账</button>
          <button @click="quickSend('本月预算还剩多少')">预算查询</button>
          <button @click="quickSend('我的资产概况')">资产概览</button>
        </div>
      </div>

      <TransitionGroup name="msg-slide">
      <div
        v-for="(msg, index) in chatStore.messages"
        :key="msg.id"
        class="message"
        :class="msg.role"
      >
        <div class="message-meta" v-if="msg.pageContext">
          <span class="page-tag">{{ msg.pageContext }}</span>
        </div>
        <div v-if="msg.role === 'assistant'" class="message-content markdown-body">
          <div class="markdown-inline" v-html="renderMarkdown(msg.content)"></div>
          <span
            v-if="chatStore.isLoading && index === chatStore.messages.length - 1"
            class="streaming-cursor"
          ></span>
        </div>
        <div v-else class="message-content">{{ msg.content }}</div>
        <!-- 实时思考过程展示（流式过程中显示，完成后如果有内容则保留） -->
        <div v-if="msg.thinking && (chatStore.isLoading && index === chatStore.messages.length - 1 && !msg.content)" class="thinking-live">
          <span class="thinking-live-icon">💭</span>
          <span class="thinking-live-text">{{ msg.thinking }}</span>
        </div>
        <!-- 7a: 推理过程展示（完成后，toggle 控制） -->
        <div v-if="showThinking && msg.thinking && msg.content" class="thinking-block">
          <div class="thinking-label">AI 推理过程</div>
          <div class="thinking-content">{{ msg.thinking }}</div>
        </div>
        <div v-if="msg.role === 'assistant' && msg.data" class="message-actions">
          <span class="action-link" @click="showDetail(msg.data)">查看详情</span>
        </div>
        <!-- 7c: 对话耗时展示 -->
        <div v-if="msg.role === 'assistant' && msg.durationMs" class="message-duration">
          耗时 {{ (msg.durationMs / 1000).toFixed(1) }}s
        </div>
        <!-- 写操作确认面板：仅在最后一条 assistant 消息且匹配确认特征词时显示 -->
        <div v-if="shouldShowConfirm(msg, index)" class="confirm-panel">
          <button class="confirm-btn confirm-yes" @click="confirmAction">确认</button>
          <button class="confirm-btn confirm-no" @click="cancelAction">取消</button>
        </div>
      </div>
      </TransitionGroup>

      <div v-if="chatStore.isLoading" class="message assistant loading">
        <div class="typing-indicator">
          <span></span><span></span><span></span>
        </div>
        <div v-if="chatStore.executionPlan" class="plan-info">
          {{ chatStore.executionPlan.description }}
          <div class="progress-bar">
            <div class="progress-fill" :style="{ width: progressWidth }"></div>
          </div>
        </div>
      </div>
    </div>

    <div class="input-area">
      <textarea
        ref="textareaRef"
        v-model="inputText"
        @keydown="handleKeydown"
        @input="autoResize"
        placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
        :disabled="chatStore.isLoading"
        rows="1"
      ></textarea>
      <button @click="sendMessage" :disabled="!inputText.trim() || chatStore.isLoading">
        发送
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, nextTick, watch, onMounted, onUnmounted } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useChatStore } from '../stores/chat'
import { chatAPI, suggestionAPI } from '../api'

marked.setOptions({ breaks: true, gfm: true })

DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A') {
    node.setAttribute('target', '_blank')
    node.setAttribute('rel', 'noopener noreferrer')
  }
})

const purifyConfig = {
  ALLOWED_TAGS: ['table', 'thead', 'tbody', 'tr', 'th', 'td', 'p', 'strong', 'em', 'b', 'i',
    'ul', 'ol', 'li', 'br', 'code', 'pre', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
    'blockquote', 'a', 'del', 'hr', 'span'],
  ALLOWED_ATTR: ['href', 'target', 'rel', 'class'],
}

function renderMarkdown(content) {
  if (!content) return ''
  const html = marked.parse(content)
  return DOMPurify.sanitize(html, purifyConfig)
}

const emit = defineEmits(['showData', 'refreshPage'])
const chatStore = useChatStore()
const inputText = ref('')
const messagesRef = ref(null)
const textareaRef = ref(null)
const progressWidth = ref('0%')
// 7a: Think 开关状态，默认关闭
const showThinking = ref(false)
const loadingHistory = ref(false)

// 确认请求特征词列表
const confirmKeywords = [
  '确认要记录吗', '确认要删除', '确认要修改',
  '确认记录吗', '确认删除吗', '确认修改吗',
  '是否确认', '请确认', '确认执行',
  '确认要创建', '确认创建吗',
  '回复"确认"记录',
  '要继续吗', '是否继续',
]

// 判断消息内容是否为确认请求
function isConfirmRequest(content) {
  if (!content) return false
  return confirmKeywords.some(kw => content.includes(kw))
}

// 计算最后一条 assistant 消息的索引，用于只在最新消息上显示确认按钮
const lastAssistantIndex = computed(() => {
  for (let i = chatStore.messages.length - 1; i >= 0; i--) {
    if (chatStore.messages[i].role === 'assistant') return i
  }
  return -1
})

// 最后一条 assistant 消息是否需要确认（且不在加载中）
function shouldShowConfirm(msg, index) {
  return (
    msg.role === 'assistant' &&
    index === lastAssistantIndex.value &&
    !chatStore.isLoading &&
    isConfirmRequest(msg.content)
  )
}

// 点击确认按钮
function confirmAction() {
  inputText.value = '确认'
  sendMessage()
}

// 点击取消按钮
function cancelAction() {
  inputText.value = '取消，不需要了'
  sendMessage()
}

let progressTimer = null
let suggestionTimer = null

// 10 秒无操作后获取建议
onMounted(() => {
  suggestionTimer = setTimeout(async () => {
    if (chatStore.messages.length === 0) {
      try {
        const res = await suggestionAPI.get()
        const suggestions = res?.data || []
        if (suggestions.length > 0) {
          chatStore.addMessage('assistant', suggestions[0].content, chatStore.currentPage, null, {})
        }
      } catch {
        // 获取建议失败静默处理
      }
    }
  }, 10000)
})

onUnmounted(() => {
  if (suggestionTimer) {
    clearTimeout(suggestionTimer)
    suggestionTimer = null
  }
})

// 加载更多历史消息
async function loadMoreHistory() {
  if (loadingHistory.value || !chatStore.hasMoreHistory) return
  loadingHistory.value = true
  try {
    // 当前已有的历史消息数量作为 limit 的偏移参考
    // Redis LRANGE 取最近 N 条，我们需要取更多条然后截取前面的
    const totalNeeded = chatStore.historyOffset + 20
    const res = await chatAPI.getHistory(chatStore.sessionId, totalNeeded)
    const allHistory = res?.data || []
    if (allHistory.length <= chatStore.historyOffset) {
      // 没有更多历史了
      chatStore.hasMoreHistory = false
    } else {
      // 取出比当前已加载更早的消息
      const newMsgs = allHistory.slice(0, allHistory.length - chatStore.historyOffset)
      chatStore.prependMessages(newMsgs)
      chatStore.historyOffset = allHistory.length
      chatStore.hasMoreHistory = allHistory.length >= totalNeeded
    }
  } catch {
    // 加载失败静默处理
  } finally {
    loadingHistory.value = false
  }
}

// 输入框键盘事件：Enter 发送，Shift+Enter 换行
function handleKeydown(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendMessage()
  }
}

// 自动调整 textarea 高度，最大 120px
function autoResize() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 120) + 'px'
}

async function sendMessage() {
  const text = inputText.value.trim()
  if (!text) return

  chatStore.addMessage('user', text, chatStore.currentPage)
  inputText.value = ''
  // 重置 textarea 高度
  if (textareaRef.value) textareaRef.value.style.height = 'auto'
  chatStore.setLoading(true)

  const startTime = Date.now()

  // 先添加一条空的 assistant 消息，流式追加内容
  chatStore.addMessage('assistant', '', chatStore.currentPage, null, {})

  chatAPI.sendStream(
    text,
    chatStore.currentPage,
    chatStore.sessionId,
    // onChunk: 追加文本片段
    (chunk) => {
      if (chunk) {
        chatStore.updateLastMessage({ appendContent: chunk })
        scrollToBottom()
      }
    },
    // onDone: 收到完整响应，更新最终数据
    (data) => {
      const durationMs = Date.now() - startTime
      chatStore.updateLastMessage({
        content: data.display || '处理完成',
        data: data.data_ref || null,
        thinking: data.thinking || null,
        durationMs,
      })
      if (data.affected_pages && data.affected_pages.length > 0) {
        setTimeout(() => emit('refreshPage'), 500)
      }
      chatStore.setLoading(false)
      chatStore.clearPlan()
      stopProgress()
    },
    // onError
    (errMsg) => {
      const durationMs = Date.now() - startTime
      chatStore.updateLastMessage({
        content: errMsg || '抱歉，处理请求时出了问题，请稍后再试。',
        durationMs,
      })
      chatStore.setLoading(false)
      chatStore.clearPlan()
      stopProgress()
    },
    // onThinking: 实时展示思考过程
    (thinkText) => {
      if (thinkText) {
        chatStore.updateLastMessage({ appendThinking: thinkText })
        scrollToBottom()
      }
    }
  )
}

function showDetail(data) {
  chatStore.setDetailData(data)
}

// 7d: 快捷操作
function quickSend(text) {
  inputText.value = text
  sendMessage()
}

function startProgress(seconds) {
  progressWidth.value = '0%'
  const interval = 100
  const total = seconds * 1000
  let elapsed = 0
  progressTimer = setInterval(() => {
    elapsed += interval
    progressWidth.value = Math.min((elapsed / total) * 100, 95) + '%'
  }, interval)
}

function stopProgress() {
  if (progressTimer) {
    clearInterval(progressTimer)
    progressTimer = null
  }
  progressWidth.value = '100%'
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesRef.value) {
      messagesRef.value.scrollTop = messagesRef.value.scrollHeight
    }
  })
}

watch(() => chatStore.messages.length, scrollToBottom)

// 监听外部组件（如 CategoryPicker）设置的待填入文本
watch(() => chatStore.pendingInput, (text) => {
  if (text) {
    inputText.value = text
    chatStore.setPendingInput('')
  }
})
</script>

<style scoped>
.chat-window {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: #fafafa;
}

.chat-header {
  padding: 12px 20px;
  background: #fff;
  border-bottom: 1px solid #e0e0e0;
  display: flex;
  align-items: center;
  gap: 10px;
}
.page-badge {
  background: #4fc3f7;
  color: #fff;
  padding: 2px 10px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 600;
}
.header-title { font-size: 15px; font-weight: 600; color: #333; }

/* 7a: Think 开关按钮 */
.thinking-toggle {
  margin-left: auto;
  padding: 3px 10px;
  font-size: 12px;
  border: 1px solid #ccc;
  border-radius: 12px;
  background: #f5f5f5;
  color: #888;
  cursor: pointer;
  transition: all 0.2s;
}
.thinking-toggle:hover { border-color: #4fc3f7; color: #4fc3f7; }
.thinking-toggle.active {
  background: #4fc3f7;
  color: #fff;
  border-color: #4fc3f7;
}

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
}

/* 7d: 冷启动欢迎样式 */
.empty-state {
  text-align: center;
  padding: 60px 20px;
  color: #888;
}
.welcome-icon { font-size: 48px; margin-bottom: 12px; }
.welcome-title { font-size: 18px; color: #333; margin-bottom: 8px; font-weight: 600; }
.welcome-hint { font-size: 13px; color: #888; margin-bottom: 24px; }
.quick-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px;
}
.quick-actions button {
  padding: 8px 16px;
  background: #fff;
  border: 1px solid #e0e0e0;
  border-radius: 20px;
  font-size: 13px;
  color: #4fc3f7;
  cursor: pointer;
  transition: all 0.2s;
}
.quick-actions button:hover {
  background: #4fc3f7;
  color: #fff;
  border-color: #4fc3f7;
}

/* 流式输出打字光标 */
.markdown-inline { display: inline; }
.streaming-cursor {
  display: inline-block;
  width: 2px;
  height: 1em;
  background: #4fc3f7;
  margin-left: 2px;
  vertical-align: text-bottom;
  animation: cursor-blink 0.8s step-end infinite;
}
@keyframes cursor-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}

/* 消息出现 slide-up 动画 */
.msg-slide-enter-active {
  transition: all 150ms ease-out;
}
.msg-slide-enter-from {
  opacity: 0;
  transform: translateY(12px);
}

.message {
  margin-bottom: 12px;
  max-width: 80%;
}
.message.user {
  margin-left: auto;
  background: #4fc3f7;
  color: #fff;
  padding: 10px 16px;
  border-radius: 16px 16px 4px 16px;
}
.message.assistant {
  background: #fff;
  color: #333;
  padding: 10px 16px;
  border-radius: 16px 16px 16px 4px;
  border: 1px solid #e8e8e8;
}

.message-meta { margin-bottom: 4px; }
.page-tag {
  font-size: 10px;
  color: #888;
  background: #f0f0f0;
  padding: 1px 6px;
  border-radius: 4px;
}

/* 实时思考过程（流式中显示） */
.thinking-live {
  padding: 8px 12px;
  color: #888;
  font-size: 13px;
  line-height: 1.5;
  animation: fadeIn 0.2s ease;
}
.thinking-live-icon {
  margin-right: 4px;
}
.thinking-live-text {
  white-space: pre-wrap;
  word-break: break-word;
}
@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

/* 7a: 推理过程区块 */
.thinking-block {
  margin-top: 8px;
  padding: 8px 12px;
  background: #f5f5f5;
  border-radius: 6px;
  border-left: 3px solid #ccc;
}
.thinking-label {
  font-size: 11px;
  color: #999;
  font-weight: 600;
  margin-bottom: 4px;
}
.thinking-content {
  font-size: 12px;
  color: #666;
  line-height: 1.5;
  white-space: pre-wrap;
}

.message-actions { margin-top: 6px; }
.action-link {
  font-size: 12px;
  color: #4fc3f7;
  cursor: pointer;
}

/* 7c: 耗时展示 */
.message-duration {
  margin-top: 4px;
  font-size: 11px;
  color: #bbb;
}

/* 写操作确认面板 */
.confirm-panel {
  display: flex;
  gap: 10px;
  margin-top: 10px;
}
.confirm-btn {
  padding: 7px 20px;
  font-size: 13px;
  font-weight: 600;
  border-radius: 16px;
  cursor: pointer;
  transition: all 0.2s;
  border: none;
}
.confirm-yes {
  background: #4fc3f7;
  color: #fff;
}
.confirm-yes:hover {
  background: #39b0e6;
}
.confirm-no {
  background: #fff;
  color: #888;
  border: 1px solid #e0e0e0;
}
.confirm-no:hover {
  background: #f5f5f5;
  color: #666;
  border-color: #ccc;
}

.loading .typing-indicator {
  display: flex;
  gap: 4px;
  padding: 4px 0;
}
.typing-indicator span {
  width: 6px;
  height: 6px;
  background: #aaa;
  border-radius: 50%;
  animation: typing 1.2s infinite;
}
.typing-indicator span:nth-child(2) { animation-delay: 0.2s; }
.typing-indicator span:nth-child(3) { animation-delay: 0.4s; }
@keyframes typing {
  0%, 60%, 100% { opacity: 0.3; transform: scale(0.8); }
  30% { opacity: 1; transform: scale(1); }
}

.plan-info {
  font-size: 12px;
  color: #888;
  margin-top: 8px;
}
.progress-bar {
  height: 3px;
  background: #e0e0e0;
  border-radius: 2px;
  margin-top: 6px;
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  background: #4fc3f7;
  border-radius: 2px;
  transition: width 0.1s linear;
}

.input-area {
  padding: 12px 20px;
  background: #fff;
  border-top: 1px solid #e0e0e0;
  display: flex;
  gap: 8px;
  align-items: flex-end;
}
.input-area textarea {
  flex: 1;
  padding: 10px 16px;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  font-size: 14px;
  font-family: inherit;
  outline: none;
  resize: none;
  max-height: 120px;
  overflow-y: auto;
  line-height: 1.5;
}
.input-area textarea:focus { border-color: #4fc3f7; }
.input-area button {
  padding: 10px 20px;
  background: #4fc3f7;
  color: #fff;
  border: none;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  align-self: flex-end;
}
.input-area button:disabled { background: #ccc; cursor: not-allowed; }

/* 加载更多按钮 */
.load-more-wrapper {
  text-align: center;
  padding: 8px 0 12px;
}
.load-more-btn {
  padding: 6px 16px;
  font-size: 12px;
  color: #4fc3f7;
  background: #fff;
  border: 1px solid #e0e0e0;
  border-radius: 16px;
  cursor: pointer;
  transition: all 0.2s;
}
.load-more-btn:hover { background: #f0f8ff; border-color: #4fc3f7; }
.load-more-btn:disabled { color: #ccc; cursor: not-allowed; }

/* Markdown 渲染样式 */
.markdown-body :deep(p) { margin: 0 0 8px; }
.markdown-body :deep(p:last-child) { margin-bottom: 0; }

.markdown-body :deep(table) {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
  margin: 8px 0;
}
.markdown-body :deep(th) {
  text-align: left;
  padding: 6px 8px;
  background: #f5f5f5;
  border: 1px solid #e0e0e0;
  font-weight: 600;
  font-size: 12px;
  color: #555;
}
.markdown-body :deep(td) {
  padding: 5px 8px;
  border: 1px solid #e8e8e8;
  color: #333;
}
.markdown-body :deep(tr:nth-child(even)) { background: #fafafa; }
.markdown-body :deep(tr:hover) { background: #f0f8ff; }

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  margin: 4px 0;
  padding-left: 20px;
}
.markdown-body :deep(li) { margin-bottom: 2px; }

.markdown-body :deep(code) {
  background: #f0f0f0;
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 12px;
  font-family: 'SF Mono', Monaco, monospace;
}
.markdown-body :deep(pre) {
  background: #f5f5f5;
  padding: 8px 12px;
  border-radius: 6px;
  overflow-x: auto;
  margin: 8px 0;
}
.markdown-body :deep(pre code) {
  background: none;
  padding: 0;
}

.markdown-body :deep(strong) { font-weight: 600; }
.markdown-body :deep(blockquote) {
  margin: 8px 0;
  padding: 4px 12px;
  border-left: 3px solid #e0e0e0;
  color: #666;
}
.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3) {
  margin: 8px 0 4px;
  font-weight: 600;
}
.markdown-body :deep(h1) { font-size: 16px; }
.markdown-body :deep(h2) { font-size: 15px; }
.markdown-body :deep(h3) { font-size: 14px; }
.markdown-body :deep(hr) {
  border: none;
  border-top: 1px solid #e0e0e0;
  margin: 8px 0;
}
.markdown-body :deep(a) { color: #4fc3f7; text-decoration: none; }
.markdown-body :deep(a:hover) { text-decoration: underline; }
</style>
