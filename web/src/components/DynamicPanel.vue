<template>
  <div class="dynamic-panel">
    <div class="panel-header">
      <span class="panel-title">{{ panelTitle }}</span>
    </div>

    <Transition name="panel-fade" mode="out-in">
    <!-- 记一笔页面：分类选择器 -->
    <div v-if="chatStore.currentPage === '记一笔'" key="category" class="panel-content category-picker-section">
      <CategoryPicker @select="onCategorySelect" />
    </div>

    <!-- 7b: 页面数据展示 -->
    <div v-else-if="currentPageData && currentPageData.items && currentPageData.items.length > 0" key="pagedata" class="panel-content">
      <!-- 首页概览 -->
      <div v-if="currentPageData.type === 'overview'" class="page-data-section">
        <div class="overview-cards">
          <div class="overview-card">
            <div class="overview-label">今日消费</div>
            <div class="overview-amount">¥{{ formatAmount(currentPageData.items[0].todayTotal) }}</div>
          </div>
          <div class="overview-card">
            <div class="overview-label">本月消费</div>
            <div class="overview-amount">¥{{ formatAmount(currentPageData.items[0].monthTotal) }}</div>
          </div>
        </div>

        <div v-if="currentPageData.items[0].budgets && currentPageData.items[0].budgets.length > 0" class="overview-section">
          <div class="overview-section-title">预算执行</div>
          <div v-for="bg in currentPageData.items[0].budgets" :key="bg.id || bg.category" class="budget-progress-item">
            <div class="budget-progress-header">
              <span class="budget-category">{{ budgetName(bg) }}</span>
              <span class="budget-numbers">¥{{ formatAmount(bg.spent_amount) }} / ¥{{ formatAmount(bg.amount || bg.budget_amount) }}</span>
            </div>
            <div class="budget-progress-bar">
              <div
                class="budget-progress-fill"
                :style="{ width: budgetPercent(bg) + '%' }"
                :class="{ over: budgetPercent(bg) > 100 }"
              ></div>
            </div>
          </div>
        </div>

        <div v-if="currentPageData.items[0].recentBills && currentPageData.items[0].recentBills.length > 0" class="overview-section">
          <div class="overview-section-title">最近消费</div>
          <div v-for="bill in currentPageData.items[0].recentBills" :key="bill.id" class="recent-bill-item">
            <div class="recent-bill-left">
              <span class="recent-bill-category">{{ bill.category || '-' }}</span>
              <span class="recent-bill-date">{{ formatDate(bill.date) }}</span>
            </div>
            <span class="recent-bill-amount">¥{{ formatAmount(bill.amount) }}</span>
          </div>
        </div>
      </div>

      <!-- 账单列表：简单表格 -->
      <div v-if="currentPageData.type === 'bills'" class="page-data-section">
        <table class="bill-table">
          <thead>
            <tr>
              <th>日期</th>
              <th>类型</th>
              <th>分类</th>
              <th>商户</th>
              <th>金额</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in currentPageData.items" :key="item.id">
              <td>{{ formatDate(item.date || item.created_at) }}</td>
              <td><span :class="billTypeClass(item.bill_type)">{{ billTypeLabel(item.bill_type) }}</span></td>
              <td>{{ item.category || '-' }}</td>
              <td>{{ item.merchant || '-' }}</td>
              <td class="amount" :class="item.bill_type === 'income' ? 'income' : ''">{{ item.bill_type === 'income' ? '+' : '-' }}¥{{ formatAmount(item.amount) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 预算列表：卡片 -->
      <div v-if="currentPageData.type === 'budgets'" class="page-data-section">
        <div
          v-for="item in currentPageData.items"
          :key="item.id"
          class="budget-card"
          :class="{ 'budget-over': budgetPercent(item) > 100 }"
        >
          <div class="budget-card-header">
            <div class="budget-card-title">{{ budgetName(item) }}</div>
            <div class="budget-card-percent" :class="{ over: budgetPercent(item) > 100 }">
              {{ budgetPercent(item) }}%
            </div>
          </div>
          <div v-if="parsedCategories(item)" class="category-tags">
            <span v-for="cat in parsedCategories(item)" :key="cat" class="category-tag">{{ cat }}</span>
          </div>
          <div class="budget-card-ratio">
            ¥{{ formatAmount(item.spent_amount) }} / ¥{{ formatAmount(item.amount || item.budget_amount) }}
          </div>
          <div class="budget-progress-bar">
            <div
              class="budget-progress-fill"
              :style="{ width: budgetPercent(item) + '%' }"
              :class="{ over: budgetPercent(item) > 100 }"
            ></div>
          </div>
        </div>
      </div>

      <!-- 资产列表：卡片 -->
      <div v-if="currentPageData.type === 'assets'" class="page-data-section">
        <div v-for="item in currentPageData.items" :key="item.id" class="info-card">
          <div class="info-card-title">{{ item.name || item.type || '资产' }}</div>
          <div class="info-card-row">
            <span class="info-label">余额</span>
            <span class="info-value">¥{{ formatAmount(item.balance || item.amount) }}</span>
          </div>
          <div v-if="item.type" class="info-card-row">
            <span class="info-label">类型</span>
            <span class="info-value">{{ item.type }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 查看详情数据展示 -->
    <div v-else-if="chatStore.detailData" key="detail" class="panel-content">
      <div class="detail-header">
        <span>数据详情</span>
        <button class="close-detail" @click="chatStore.setDetailData(null)">关闭</button>
      </div>
      <div class="data-cards">
        <div v-for="(value, key) in flatDetail" :key="key" class="data-card">
          <div class="card-label">{{ formatLabel(key) }}</div>
          <div class="card-value">{{ formatValue(value) }}</div>
        </div>
      </div>
    </div>

    <!-- 对话触发的数据展示（保留原有逻辑） -->
    <div v-else-if="hasData" key="chatdata" class="panel-content">
      <div v-if="lastData" class="data-cards">
        <div v-for="(value, key) in displayData" :key="key" class="data-card">
          <div class="card-label">{{ formatLabel(key) }}</div>
          <div class="card-value">{{ formatValue(value) }}</div>
        </div>
      </div>
      <div ref="chartRef" class="chart-container"></div>
    </div>

    <!-- 空状态 -->
    <div v-else key="empty" class="panel-empty">
      <div class="empty-icon">
        <div class="empty-icon-circle">
          <span class="empty-icon-text">{{ emptyIconText }}</span>
        </div>
      </div>
      <p>{{ emptyHint }}</p>
    </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import * as echarts from 'echarts'
import { useChatStore } from '../stores/chat'
import CategoryPicker from './CategoryPicker.vue'

const chatStore = useChatStore()
const chartRef = ref(null)
let chartInstance = null

// 分类选择器回调：将选中的分类填入聊天输入框
function onCategorySelect({ category, subCategory }) {
  const text = subCategory ? `${category}-${subCategory}` : category
  chatStore.setPendingInput(text)
}

// 7b: 当前页面的预加载数据
const currentPageData = computed(() => chatStore.pageData[chatStore.currentPage] || null)

// 7b: 面板标题根据页面动态变化
const panelTitleMap = {
  '首页': '概览',
  '记一笔': '快速记账',
  '账单详情': '账单列表',
  '预算': '预算概览',
  '报表': '数据报表',
  '资产管理': '资产概览',
}
const panelTitle = computed(() => panelTitleMap[chatStore.currentPage] || '动态内容')

// 7b: 空状态提示
const emptyHintMap = {
  '首页': '欢迎使用咔皮记账，与助手对话开始吧',
  '记一笔': '告诉助手你的消费，例如"午饭 25 元"',
  '报表': '与助手对话后，报表数据将在这里展示',
}
const emptyHint = computed(() =>
  emptyHintMap[chatStore.currentPage] || '与助手对话后，相关数据和图表将在这里展示'
)

// 空状态图标文字（纯 CSS 圆形 + 文字，不用 emoji）
const emptyIconMap = {
  '首页': '咔',
  '记一笔': '记',
  '账单详情': '账',
  '预算': '预',
  '报表': '表',
  '资产管理': '资',
}
const emptyIconText = computed(() =>
  emptyIconMap[chatStore.currentPage] || '咔'
)

// 原有：对话触发的数据展示
const lastAssistantMsg = computed(() => {
  const msgs = chatStore.messages
  for (let i = msgs.length - 1; i >= 0; i--) {
    if (msgs[i].role === 'assistant' && msgs[i].data) return msgs[i]
  }
  return null
})

const flatDetail = computed(() => {
  if (!chatStore.detailData) return {}
  const d = {}
  for (const [k, v] of Object.entries(chatStore.detailData)) {
    if (typeof v !== 'object' || v === null) {
      d[k] = v
    }
  }
  return d
})

const lastData = computed(() => lastAssistantMsg.value?.data)
const hasData = computed(() => !!lastData.value)

const displayData = computed(() => {
  if (!lastData.value) return {}
  const d = {}
  for (const [k, v] of Object.entries(lastData.value)) {
    if (typeof v !== 'object' || v === null) {
      d[k] = v
    }
  }
  return d
})

function formatLabel(key) {
  const labels = {
    current_total: '本期总额',
    previous_total: '上期总额',
    diff: '差额',
    growth_rate: '增长率',
    count: '笔数',
    net_balance: '净资产',
    total: '总计',
  }
  return labels[key] || key
}

function formatValue(value) {
  if (typeof value === 'number') {
    if (Math.abs(value) < 1 && value !== 0) {
      return (value * 100).toFixed(1) + '%'
    }
    return '¥' + value.toLocaleString('zh-CN', { minimumFractionDigits: 2 })
  }
  return String(value)
}

// 7b: 格式化日期
function formatDate(dateStr) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

// 7b: 格式化金额
function formatAmount(val) {
  if (val === undefined || val === null) return '0.00'
  return Number(val).toLocaleString('zh-CN', { minimumFractionDigits: 2 })
}

// 计算预算使用百分比
function budgetName(item) {
  if (item.name && item.name.trim() && item.name !== '[]') return item.name
  if (item.category && item.category.trim() && item.category !== '[]') return item.category
  const cats = parsedCategories(item)
  if (cats && cats.length > 0) return cats.join('、')
  return '预算'
}

function budgetPercent(bg) {
  const total = Number(bg.amount || bg.budget_amount) || 1
  const spent = Number(bg.spent_amount) || 0
  return Math.min(Math.round((spent / total) * 100), 150)
}

// 解析预算包含的分类列表（categories 可能是 JSON 字符串或数组）
function parsedCategories(item) {
  if (!item.categories) return null
  try {
    const cats = typeof item.categories === 'string' ? JSON.parse(item.categories) : item.categories
    return Array.isArray(cats) ? cats : null
  } catch { return null }
}

// 账单类型展示标签
function billTypeLabel(type) {
  if (type === 'income') return '收入'
  if (type === 'transfer') return '转账'
  return '支出'
}

// 账单类型样式 class
function billTypeClass(type) {
  if (type === 'income') return 'type-income'
  if (type === 'transfer') return 'type-transfer'
  return 'type-expense'
}

function renderChart(data) {
  if (!chartRef.value) return
  if (!chartInstance) {
    chartInstance = echarts.init(chartRef.value)
  }

  if (data.summary && Array.isArray(data.summary)) {
    chartInstance.setOption({
      tooltip: { trigger: 'item' },
      series: [{
        type: 'pie',
        radius: ['40%', '70%'],
        data: data.summary.map(s => ({ name: s.category, value: s.total })),
        label: { formatter: '{b}: {d}%' },
      }],
    })
  } else if (data.current_total !== undefined && data.previous_total !== undefined) {
    chartInstance.setOption({
      tooltip: {},
      xAxis: { type: 'category', data: ['上期', '本期'] },
      yAxis: { type: 'value' },
      series: [{
        type: 'bar',
        data: [data.previous_total, data.current_total],
        itemStyle: { color: '#4fc3f7' },
        barWidth: '40%',
      }],
    })
  } else if (data.executions && Array.isArray(data.executions)) {
    const categories = data.executions.map(e => e.category)
    chartInstance.setOption({
      tooltip: {},
      xAxis: { type: 'category', data: categories },
      yAxis: { type: 'value' },
      series: [
        { name: '预算', type: 'bar', data: data.executions.map(e => e.budget_amount), itemStyle: { color: '#e0e0e0' } },
        { name: '已花', type: 'bar', data: data.executions.map(e => e.spent_amount), itemStyle: { color: '#4fc3f7' } },
      ],
    })
  }
}

watch(lastData, (data) => {
  if (data) {
    nextTick(() => renderChart(data))
  }
})
</script>

<style scoped>
.dynamic-panel { padding: 16px; height: 100%; display: flex; flex-direction: column; }
.panel-header { margin-bottom: 16px; }
.panel-title { font-size: 14px; font-weight: 600; color: #333; }

.panel-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  color: #aaa;
  font-size: 13px;
  padding: 20px;
  gap: 12px;
}

/* 空状态 CSS 图标 */
.empty-icon { margin-bottom: 4px; }
.empty-icon-circle {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: linear-gradient(135deg, #e0f4fd, #b3e5fc);
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2px solid #4fc3f7;
}
.empty-icon-text {
  font-size: 22px;
  font-weight: 700;
  color: #4fc3f7;
  line-height: 1;
}

/* 页面切换 fade 过渡 */
.panel-fade-enter-active,
.panel-fade-leave-active {
  transition: opacity 200ms ease;
}
.panel-fade-enter-from,
.panel-fade-leave-to {
  opacity: 0;
}

.panel-content { flex: 1; overflow-y: auto; }

/* 分类选择器区域 */
.category-picker-section { padding-top: 4px; }

/* 7b: 页面数据区块 */
.page-data-section { margin-bottom: 16px; }

/* 账单表格 */
.bill-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.bill-table th {
  text-align: left;
  padding: 8px 6px;
  color: #888;
  font-weight: 500;
  border-bottom: 1px solid #e8e8e8;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.bill-table td {
  padding: 8px 6px;
  border-bottom: 1px solid #f0f0f0;
  color: #333;
}
.bill-table .amount {
  font-weight: 600;
  color: #ef5350;
}
.bill-table .amount.income {
  color: #66bb6a;
}
.type-expense {
  font-size: 11px;
  color: #ef5350;
}
.type-income {
  font-size: 11px;
  color: #66bb6a;
}
.type-transfer {
  font-size: 11px;
  color: #4fc3f7;
}

/* 预算分类标签 */
.category-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px; margin-bottom: 8px; }
.category-tag {
  font-size: 11px;
  padding: 1px 6px;
  background: #f0f0f0;
  border-radius: 4px;
  color: #666;
}

.bill-table th:last-child { text-align: right; }

/* 信息卡片（预算/资产） */
.info-card {
  background: #f8f9fa;
  border-radius: 8px;
  padding: 12px;
  margin-bottom: 8px;
}
.info-card-title {
  font-size: 14px;
  font-weight: 600;
  color: #333;
  margin-bottom: 8px;
}
.info-card-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}
.info-label { font-size: 12px; color: #888; }
.info-value { font-size: 13px; font-weight: 600; color: #333; }
.info-value.spent { color: #ef5350; }

/* 预算卡片 */
.budget-card {
  background: #f8f9fa;
  border-radius: 8px;
  padding: 12px;
  margin-bottom: 8px;
  border-left: 3px solid #4fc3f7;
}
.budget-card.budget-over {
  border-left-color: #ef5350;
  background: #fff5f5;
}
.budget-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}
.budget-card-title {
  font-size: 14px;
  font-weight: 600;
  color: #333;
}
.budget-card-percent {
  font-size: 13px;
  font-weight: 600;
  color: #4fc3f7;
}
.budget-card-percent.over {
  color: #ef5350;
}
.budget-card-ratio {
  font-size: 12px;
  color: #888;
  margin-bottom: 6px;
}

/* 原有样式 */
.data-cards {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  margin-bottom: 16px;
}
.data-card {
  background: #f8f9fa;
  border-radius: 8px;
  padding: 12px;
}
.card-label { font-size: 11px; color: #888; margin-bottom: 4px; }
.card-value { font-size: 16px; font-weight: 600; color: #333; }

.chart-container { width: 100%; height: 260px; }

/* 首页概览样式 */
.overview-cards {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-bottom: 16px;
}
.overview-card {
  background: linear-gradient(135deg, #4fc3f7, #039be5);
  border-radius: 10px;
  padding: 16px 12px;
  color: #fff;
}
.overview-label { font-size: 12px; opacity: 0.85; margin-bottom: 6px; }
.overview-amount { font-size: 20px; font-weight: 700; }

.overview-section { margin-bottom: 16px; }
.overview-section-title {
  font-size: 13px;
  font-weight: 600;
  color: #555;
  margin-bottom: 10px;
  padding-bottom: 4px;
  border-bottom: 1px solid #eee;
}

/* 预算进度条 */
.budget-progress-item { margin-bottom: 10px; }
.budget-progress-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}
.budget-category { font-size: 13px; color: #333; }
.budget-numbers { font-size: 12px; color: #888; }
.budget-progress-bar {
  height: 6px;
  background: #eee;
  border-radius: 3px;
  overflow: hidden;
}
.budget-progress-fill {
  height: 100%;
  background: #4fc3f7;
  border-radius: 3px;
  transition: width 0.3s ease;
}
.budget-progress-fill.over { background: #ef5350; }

/* 最近消费列表 */
.recent-bill-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid #f5f5f5;
}
.recent-bill-item:last-child { border-bottom: none; }
.recent-bill-left { display: flex; flex-direction: column; gap: 2px; }
.recent-bill-category { font-size: 13px; color: #333; }
.recent-bill-date { font-size: 11px; color: #aaa; }
.recent-bill-amount { font-size: 14px; font-weight: 600; color: #333; }
</style>
