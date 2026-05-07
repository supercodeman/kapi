<template>
  <div class="main-layout">
    <aside class="sidebar">
      <div class="logo">咔皮 AI</div>
      <nav class="page-nav">
        <div
          v-for="page in pages"
          :key="page.name"
          class="nav-item"
          :class="{ active: chatStore.currentPage === page.name }"
          @click="chatStore.switchPage(page.name)"
        >
          <span class="nav-icon">{{ page.icon }}</span>
          <span class="nav-label">{{ page.name }}</span>
        </div>
      </nav>
      <div class="skills-section">
        <div class="skills-title">当前 Skills</div>
        <div v-for="s in currentSkills" :key="s" class="skill-tag">{{ s }}</div>
      </div>
      <div class="sidebar-footer">
        <span class="username-display" v-if="authStore.username">{{ authStore.username }}</span>
        <button class="logout-btn" @click="handleLogout">退出登录</button>
      </div>
    </aside>

    <main class="chat-panel">
      <ChatWindow @refresh-page="refreshCurrentPage" />
    </main>

    <aside class="content-panel">
      <DynamicPanel />
    </aside>
  </div>
</template>

<script setup>
import { computed, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useChatStore } from '../stores/chat'
import { billAPI, budgetAPI, assetAPI, chatAPI } from '../api'
import ChatWindow from '../components/ChatWindow.vue'
import DynamicPanel from '../components/DynamicPanel.vue'

const router = useRouter()
const authStore = useAuthStore()
const chatStore = useChatStore()

const pages = [
  { name: '首页', icon: '🏠' },
  { name: '记一笔', icon: '✏️' },
  { name: '账单详情', icon: '📋' },
  { name: '预算', icon: '💰' },
  { name: '报表', icon: '📊' },
  { name: '资产管理', icon: '🏦' },
]

const skillMap = {
  '首页': ['消费概览', '快速记账'],
  '记一笔': ['自然语言记账', '批量录入'],
  '账单详情': ['账单查询', '消费统计', '异常检测'],
  '预算': ['预算管理', '预算分析'],
  '报表': ['趋势分析', '同比环比'],
  '资产管理': ['资产概览', '财务建议'],
}

const currentSkills = computed(() => skillMap[chatStore.currentPage] || [])

// 7b: 页面切换时加载对应数据
const pageLoaders = {
  '首页': async () => {
    try {
      const [billRes, budgetRes] = await Promise.all([
        billAPI.list(),
        budgetAPI.list(),
      ])
      const bills = billRes?.data || []
      const budgets = budgetRes?.data || []

      const now = new Date()
      const todayStr = now.toISOString().slice(0, 10)
      const yearMonth = todayStr.slice(0, 7)

      // 今日消费总额（只算支出）
      const todayTotal = bills
        .filter(b => (b.date || '').startsWith(todayStr) && b.bill_type === 'expense')
        .reduce((sum, b) => sum + (Number(b.amount) || 0), 0)

      // 本月消费总额（只算支出）
      const monthTotal = bills
        .filter(b => (b.date || '').startsWith(yearMonth) && b.bill_type === 'expense')
        .reduce((sum, b) => sum + (Number(b.amount) || 0), 0)

      // 预算执行概况：只统计支出
      const monthBills = bills.filter(b => (b.date || '').startsWith(yearMonth) && b.bill_type === 'expense')
      const spentByCategory = {}
      monthBills.forEach(b => {
        const cat = b.category || '未分类'
        spentByCategory[cat] = (spentByCategory[cat] || 0) + (Number(b.amount) || 0)
      })
      const budgetItems = budgets.map(bg => {
        let cats = []
        try {
          if (bg.categories) {
            const parsed = typeof bg.categories === 'string' ? JSON.parse(bg.categories) : bg.categories
            if (Array.isArray(parsed) && parsed.length > 0) cats = parsed
          }
        } catch {}
        if (cats.length === 0 && bg.category && bg.category !== '总预算' && bg.category !== '[]') {
          cats = [bg.category]
        }

        // 总预算：已花 = 所有支出之和；分类预算：按 categories 匹配
        const isTotalBudget = cats.length === 0 || bg.name === '总预算' || bg.category === '总预算'
        const spent = isTotalBudget
          ? monthTotal
          : cats.reduce((sum, cat) => sum + (spentByCategory[cat] || 0), 0)
        return { ...bg, spent_amount: spent }
      })

      // 最近 5 笔消费（只显示支出）
      const recentBills = [...bills]
        .filter(b => b.bill_type === 'expense')
        .sort((a, b) => (b.date || '').localeCompare(a.date || ''))
        .slice(0, 5)

      chatStore.setPageData('首页', {
        type: 'overview',
        items: [{
          todayTotal,
          monthTotal,
          budgets: budgetItems,
          recentBills,
        }],
      })
    } catch {
      chatStore.setPageData('首页', { type: 'overview', items: [], error: true })
    }
  },
  '账单详情': async () => {
    try {
      const res = await billAPI.list()
      chatStore.setPageData('账单详情', { type: 'bills', items: res?.data || [] })
    } catch {
      chatStore.setPageData('账单详情', { type: 'bills', items: [], error: true })
    }
  },
  '预算': async () => {
    try {
      const [budgetRes, billRes] = await Promise.all([
        budgetAPI.list(),
        billAPI.list(),
      ])
      const budgets = budgetRes?.data || []
      const bills = billRes?.data || []

      const now = new Date()
      const yearMonth = now.toISOString().slice(0, 7)
      const monthBills = bills.filter(b => (b.date || '').startsWith(yearMonth) && b.bill_type === 'expense')

      // 按分类汇总已花金额
      const spentByCategory = {}
      let totalSpent = 0
      monthBills.forEach(b => {
        const cat = b.category || '未分类'
        spentByCategory[cat] = (spentByCategory[cat] || 0) + (Number(b.amount) || 0)
        totalSpent += (Number(b.amount) || 0)
      })

      const items = budgets.map(bg => {
        let cats = []
        try {
          if (bg.categories) {
            const parsed = typeof bg.categories === 'string' ? JSON.parse(bg.categories) : bg.categories
            if (Array.isArray(parsed) && parsed.length > 0) cats = parsed
          }
        } catch {}
        if (cats.length === 0 && bg.category && bg.category !== '总预算' && bg.category !== '[]') {
          cats = [bg.category]
        }

        const isTotalBudget = cats.length === 0 || bg.name === '总预算' || bg.category === '总预算'
        const spent = isTotalBudget
          ? totalSpent
          : cats.reduce((sum, cat) => sum + (spentByCategory[cat] || 0), 0)
        return { ...bg, spent_amount: spent, _isTotal: isTotalBudget }
      })

      // 总预算排最前面
      items.sort((a, b) => (b._isTotal ? 1 : 0) - (a._isTotal ? 1 : 0))

      chatStore.setPageData('预算', { type: 'budgets', items })
    } catch {
      chatStore.setPageData('预算', { type: 'budgets', items: [], error: true })
    }
  },
  '资产管理': async () => {
    try {
      const res = await assetAPI.list()
      chatStore.setPageData('资产管理', { type: 'assets', items: res?.data || [] })
    } catch {
      chatStore.setPageData('资产管理', { type: 'assets', items: [], error: true })
    }
  },
}

watch(() => chatStore.currentPage, (page) => {
  const loader = pageLoaders[page]
  if (loader) loader()
}, { immediate: true })

function refreshCurrentPage() {
  const loader = pageLoaders[chatStore.currentPage]
  if (loader) loader()
}

function handleLogout() {
  chatStore.reset()
  authStore.logout()
  router.push('/login')
}

// 登录后初始化：检测用户切换 + 加载对话历史
onMounted(async () => {
  if (!authStore.isLoggedIn) return

  // 检测是否切换了用户（用 localStorage 记录上次登录的用户名）
  const lastUser = localStorage.getItem('last_chat_user')
  if (lastUser && lastUser !== authStore.username) {
    chatStore.reset()
  }
  localStorage.setItem('last_chat_user', authStore.username)
  chatStore.sessionId = 1

  // 只在没有消息时加载历史（避免刷新页面重复加载）
  if (chatStore.messages.length > 0) return

  try {
    const res = await chatAPI.getHistory(chatStore.sessionId, 20)
    const history = res?.data || []
    if (history.length > 0) {
      chatStore.prependMessages(history)
      chatStore.historyOffset = history.length
      chatStore.hasMoreHistory = history.length >= 20
    } else {
      chatStore.hasMoreHistory = false
    }
  } catch {
    // 加载历史失败不影响正常使用
  }
})
</script>

<style scoped>
.main-layout {
  display: flex;
  height: 100vh;
  background: #f5f5f5;
}

.sidebar {
  width: 200px;
  background: #1a1a2e;
  color: #e0e0e0;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.logo {
  padding: 20px 16px;
  font-size: 18px;
  font-weight: 700;
  color: #4fc3f7;
  border-bottom: 1px solid #333;
}

.page-nav { flex: 1; padding: 12px 8px; }

.nav-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
  transition: background 0.2s;
  margin-bottom: 2px;
}
.nav-item:hover { background: #16213e; }
.nav-item.active { background: #16213e; color: #4fc3f7; }
.nav-icon { font-size: 16px; }

.skills-section {
  padding: 12px 16px;
  border-top: 1px solid #333;
}
.skills-title {
  font-size: 11px;
  color: #888;
  text-transform: uppercase;
  letter-spacing: 1px;
  margin-bottom: 8px;
}
.skill-tag {
  font-size: 12px;
  color: #4fc3f7;
  margin-bottom: 4px;
}

.sidebar-footer { padding: 12px 16px; border-top: 1px solid #333; }
.username-display {
  display: block;
  font-size: 13px;
  color: #ccc;
  margin-bottom: 8px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.logout-btn {
  width: 100%;
  padding: 8px;
  background: transparent;
  border: 1px solid #555;
  color: #aaa;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}
.logout-btn:hover { border-color: #ef5350; color: #ef5350; }

.chat-panel {
  width: 40%;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.content-panel {
  flex: 1;
  background: #fff;
  border-left: 1px solid #e0e0e0;
  flex-shrink: 0;
  overflow-y: auto;
}
</style>
