<template>
  <div class="category-picker">
    <!-- 支出/收入切换 -->
    <div class="type-switch">
      <button
        class="type-btn"
        :class="{ active: billType === 'expense' }"
        @click="billType = 'expense'"
      >支出</button>
      <button
        class="type-btn"
        :class="{ active: billType === 'income' }"
        @click="billType = 'income'"
      >收入</button>
    </div>

    <!-- 一级分类横向滚动标签栏 -->
    <div class="primary-categories">
      <div class="primary-scroll">
        <button
          v-for="cat in currentPrimaryList"
          :key="cat.name"
          class="primary-tag"
          :class="{ active: selectedPrimary === cat.name }"
          @click="selectPrimary(cat.name)"
        >
          <span class="primary-icon">{{ cat.icon }}</span>
          <span class="primary-name">{{ cat.name }}</span>
        </button>
      </div>
    </div>

    <!-- 二级分类网格 -->
    <div v-if="currentSubList.length > 0" class="sub-categories">
      <div class="sub-grid">
        <button
          v-for="sub in currentSubList"
          :key="sub"
          class="sub-card"
          :class="{ active: selectedSub === sub }"
          @click="selectSub(sub)"
        >{{ sub }}</button>
      </div>
    </div>
    <div v-else-if="selectedPrimary" class="sub-empty">
      <span>该分类无子分类，点击上方分类直接选用</span>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'

const emit = defineEmits(['select'])

// 支出/收入切换
const billType = ref('expense')
const selectedPrimary = ref('')
const selectedSub = ref('')

// 分类数据（与后端 seed.go 保持一致）
const expenseCategories = [
  { name: '食饮', icon: '🍽', subs: ['三餐', '饮品', '零食水果', '食材'] },
  { name: '居住', icon: '🏠', subs: ['房租/房贷', '水电燃气', '物业', '家居用品'] },
  { name: '交通', icon: '🚗', subs: ['公共交通', '打车', '加油/充电', '停车', '车辆保养'] },
  { name: '通讯', icon: '📱', subs: ['话费', '网费', '会员订阅'] },
  { name: '医疗', icon: '🏥', subs: ['门诊', '药品', '体检', '保险'] },
  { name: '购物', icon: '🛒', subs: ['服饰', '数码', '日用品', '其他购物'] },
  { name: '娱乐', icon: '🎮', subs: ['电影/演出', '游戏', '运动健身', '旅行'] },
  { name: '社交', icon: '🤝', subs: ['聚餐请客', '礼物红包', '人情往来'] },
  { name: '教育', icon: '📚', subs: ['课程培训', '书籍', '考试'] },
  { name: '宠物', icon: '🐾', subs: ['宠物食品', '宠物医疗', '宠物用品'] },
  { name: '金融', icon: '💳', subs: ['信用卡还款', '贷款还款', '理财亏损', '手续费'] },
  { name: '其他', icon: '📌', subs: [] },
]

const incomeCategories = [
  { name: '职业收入', icon: '💰', subs: ['工资', '奖金', '兼职'] },
  { name: '投资收入', icon: '💰', subs: ['理财收益', '股票分红', '房租收入'] },
  { name: '其他收入', icon: '💰', subs: ['红包', '退款', '报销'] },
]

// 切换支出/收入时重置选中状态
watch(billType, () => {
  selectedPrimary.value = ''
  selectedSub.value = ''
})

// 当前类型对应的一级分类列表
const currentPrimaryList = computed(() =>
  billType.value === 'expense' ? expenseCategories : incomeCategories
)

// 当前选中一级分类下的二级分类
const currentSubList = computed(() => {
  if (!selectedPrimary.value) return []
  const found = currentPrimaryList.value.find(c => c.name === selectedPrimary.value)
  return found ? found.subs : []
})

// 选择一级分类
function selectPrimary(name) {
  selectedPrimary.value = name
  selectedSub.value = ''
  // 如果该分类没有子分类，直接 emit
  const found = currentPrimaryList.value.find(c => c.name === name)
  if (found && found.subs.length === 0) {
    emit('select', { category: name, subCategory: '' })
  }
}

// 选择二级分类
function selectSub(sub) {
  selectedSub.value = sub
  emit('select', { category: selectedPrimary.value, subCategory: sub })
}
</script>

<style scoped>
.category-picker {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* 支出/收入切换 */
.type-switch {
  display: flex;
  gap: 8px;
}
.type-btn {
  flex: 1;
  padding: 6px 0;
  font-size: 13px;
  font-weight: 600;
  border: 1px solid #e0e0e0;
  border-radius: 20px;
  background: #fff;
  color: #888;
  cursor: pointer;
  transition: all 0.2s;
}
.type-btn:hover {
  border-color: #4fc3f7;
  color: #4fc3f7;
}
.type-btn.active {
  background: #4fc3f7;
  color: #fff;
  border-color: #4fc3f7;
}

/* 一级分类横向滚动 */
.primary-categories {
  overflow: hidden;
}
.primary-scroll {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  padding-bottom: 4px;
  scrollbar-width: thin;
  scrollbar-color: #ddd transparent;
}
.primary-scroll::-webkit-scrollbar {
  height: 3px;
}
.primary-scroll::-webkit-scrollbar-thumb {
  background: #ddd;
  border-radius: 2px;
}
.primary-tag {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 5px 12px;
  border: 1px solid #e8e8e8;
  border-radius: 20px;
  background: #fafafa;
  color: #555;
  font-size: 12px;
  white-space: nowrap;
  cursor: pointer;
  transition: all 0.2s;
  flex-shrink: 0;
}
.primary-tag:hover {
  border-color: #4fc3f7;
  color: #4fc3f7;
}
.primary-tag.active {
  background: #4fc3f7;
  color: #fff;
  border-color: #4fc3f7;
}
.primary-icon {
  font-size: 14px;
}
.primary-name {
  font-size: 12px;
}

/* 二级分类网格 */
.sub-categories {
  margin-top: 2px;
}
.sub-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}
.sub-card {
  padding: 8px 4px;
  font-size: 12px;
  text-align: center;
  background: #f5f5f5;
  color: #555;
  border: 1.5px solid transparent;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sub-card:hover {
  background: #eef7fd;
  border-color: #4fc3f7;
  color: #4fc3f7;
}
.sub-card.active {
  background: #e3f2fd;
  border-color: #4fc3f7;
  color: #039be5;
  font-weight: 600;
}

/* 无子分类提示 */
.sub-empty {
  text-align: center;
  font-size: 12px;
  color: #bbb;
  padding: 12px 0;
}
</style>
