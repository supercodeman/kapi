# 咔皮记账 AI 助手 — v2.1 设计方案（修订版）

> 版本：2.1 修订版 | 日期：2026-04-30
> 核心主题：基础数据模型重构（分类体系 + 账单模型 + 预算模型 + 资产联动）

---

## 一、设计原则

v2.0 解决了 Engine 架构问题（Workflow 优先），v2.1 解决数据模型问题。
分类、账单、预算、资产是记账业务的地基，必须一次性设计到位。

---

## 二、分类体系

### 2.1 两级分类结构

数据模型：
```go
type Category struct {
    ID        uint64 `gorm:"primaryKey"`
    UserID    uint64 // 0=系统预设，>0=用户自定义
    ParentID  uint64 // 0=一级分类，>0=二级分类
    Name      string // 分类名称
    Icon      string // 图标
    BillType  string // expense/income/both
    Necessity string // necessary（必要）/ optional（可选）
    SortOrder int    // 排序
}
```

### 2.2 系统预设分类

**支出分类（一级 → 二级）：**

| 一级 | 二级 | 必要性 |
|------|------|--------|
| 食饮 | 三餐、饮品、零食水果、食材 | 必要 |
| 居住 | 房租/房贷、水电燃气、物业、家居用品 | 必要 |
| 交通 | 公共交通、打车、加油/充电、停车、车辆保养 | 必要 |
| 通讯 | 话费、网费、会员订阅 | 必要 |
| 医疗 | 门诊、药品、体检、保险 | 必要 |
| 购物 | 服饰、数码、日用品、其他购物 | 可选 |
| 娱乐 | 电影/演出、游戏、运动健身、旅行 | 可选 |
| 社交 | 聚餐请客、礼物红包、人情往来 | 可选 |
| 教育 | 课程培训、书籍、考试 | 可选 |
| 宠物 | 宠物食品、宠物医疗、宠物用品 | 可选 |
| 金融 | 信用卡还款、贷款还款、理财亏损、手续费 | 特殊 |
| 其他 | — | — |

**收入分类：**

| 一级 | 二级 |
|------|------|
| 职业收入 | 工资、奖金、兼职 |
| 投资收入 | 理财收益、股票分红、房租收入 |
| 其他收入 | 红包、退款、报销 |

### 2.3 用户交互策略

- 用户说"午饭30元" → 系统自动推断一级分类"食饮"，子分类"三餐"
- 用户说"星巴克38元" → 系统推断"食饮-饮品"
- 用户不需要手动选择子分类，由 NormalizeCategory 自动推断
- 用户可以自定义分类（存入 Category 表，UserID > 0）
- 前端展示时只显示一级分类，子分类作为标签

### 2.4 NormalizeCategory 升级

```go
// 商户 → 子分类 → 一级分类 的映射链
var merchantCategoryMap = map[string]struct{ Sub, Parent string }{
    "星巴克":  {"饮品", "食饮"},
    "瑞幸":   {"饮品", "食饮"},
    "麦当劳":  {"三餐", "食饮"},
    "肯德基":  {"三餐", "食饮"},
    "滴滴":   {"打车", "交通"},
    "美团":   {"三餐", "食饮"},
    "淘宝":   {"其他购物", "购物"},
    "京东":   {"数码", "购物"},
    // ...
}

// 关键词 → 子分类 → 一级分类
var keywordCategoryMap = map[string]struct{ Sub, Parent string }{
    "午饭": {"三餐", "食饮"},
    "早餐": {"三餐", "食饮"},
    "晚饭": {"三餐", "食饮"},
    "咖啡": {"饮品", "食饮"},
    "奶茶": {"饮品", "食饮"},
    "打车": {"打车", "交通"},
    "地铁": {"公共交通", "交通"},
    "加油": {"加油/充电", "交通"},
    "房租": {"房租/房贷", "居住"},
    "电费": {"水电燃气", "居住"},
    // ...
}
```

---

## 三、账单模型重构

### 3.1 Bill 模型

```go
type Bill struct {
    ID           uint64  `gorm:"primaryKey"`
    UserID       uint64
    BillType     string  // expense / income / transfer
    Amount       float64
    Category     string  // 一级分类
    SubCategory  string  // 二级分类（可选）
    Merchant     string  // 商户
    AssetID      uint64  // 关联资产账户（0=未关联）
    ToAssetID    uint64  // 转账目标账户（仅 transfer）
    Date         string  // YYYY-MM-DD
    Note         string
    IsDeleted    bool
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

### 3.2 账单类型说明

| bill_type | 含义 | 资产影响 | 预算影响 |
|-----------|------|---------|---------|
| expense | 支出 | asset.balance -= amount | 计入预算已花 |
| income | 收入 | asset.balance += amount | 不计入预算 |
| transfer | 转账 | from -= amount, to += amount | 不计入预算 |

### 3.3 特殊场景处理

| 场景 | bill_type | category | asset 影响 |
|------|-----------|----------|-----------|
| 信用卡消费 | expense | 对应分类 | 信用卡 balance -= amount |
| 信用卡还款 | transfer | 金融-信用卡还款 | 现金 -= amount, 信用卡 += amount |
| 理财亏损 | expense | 金融-理财亏损 | 投资账户 balance -= amount |
| 理财收益 | income | 投资收入-理财收益 | 投资账户 balance += amount |
| 工资到账 | income | 职业收入-工资 | 储蓄卡 balance += amount |
| 朋友还钱 | income | 其他收入 | 对应账户 += amount |
| 借钱给朋友 | expense | 社交-人情往来 | 对应账户 -= amount |

---

## 四、预算模型重构

### 4.1 Budget 模型

```go
type Budget struct {
    ID         uint64
    UserID     uint64
    Name       string  // 预算名称（如"吃喝预算"）
    Categories string  // JSON 数组：["食饮","社交"]（包含的一级分类）
    Amount     float64 // 预算金额
    Period     string  // monthly / weekly
    BudgetType string  // necessary / optional / total
    IsDeleted  bool
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

### 4.2 预算与分类的关系

**一对多（折中方案）：** 一个预算包含多个一级分类，一个分类只属于一个预算。

示例：
```
吃喝预算 2000/月 → ["食饮", "社交"]
交通预算 500/月  → ["交通"]
购物预算 1000/月 → ["购物"]
总预算 8000/月   → ["食饮","居住","交通","通讯","医疗","购物","娱乐","社交","教育","宠物","金融","其他"]
```

### 4.3 预算执行率计算

```sql
SELECT SUM(b.amount) as spent
FROM bills b
WHERE b.user_id = ?
  AND b.bill_type = 'expense'
  AND b.is_deleted = false
  AND b.category IN (预算包含的分类列表)
  AND b.date BETWEEN 月初 AND 月末
```

### 4.4 智能预算建议

用户说"帮我设置预算"但没指定分类时：
- 查询历史 3 个月各分类的平均支出
- 按必要/可选分组
- 建议预算 = 历史平均 × 1.1（留 10% 余量）

---

## 五、资产联动

### 5.1 联动规则

| 操作 | 资产变化 | 实现位置 |
|------|---------|---------|
| 创建支出账单 | asset.balance -= amount | create_bill Tool 内部 |
| 创建收入账单 | asset.balance += amount | create_bill Tool 内部 |
| 创建转账 | from -= amount, to += amount | transfer Tool 内部 |
| 删除支出账单 | asset.balance += amount | delete_bill Tool 内部 |
| 删除收入账单 | asset.balance -= amount | delete_bill Tool 内部 |
| 修改账单金额 | asset.balance += (old - new) | update_bill Tool 内部 |

### 5.2 资产关联策略

用户记账时不一定会指定资产账户。处理策略：
- 如果用户指定了账户（"用信用卡买的"）→ 关联指定账户
- 如果用户没指定 → 查 L2 记忆中的默认账户偏好
- 如果没有偏好 → asset_id = 0（不关联），不影响任何资产余额
- 后续用户可以补充关联

### 5.3 信用卡场景

信用卡资产的 balance 含义：
- balance > 0：有预存款/溢缴款
- balance = 0：无欠款
- balance < 0：有欠款（负债）

信用卡消费：balance -= amount（变更负）
信用卡还款：balance += amount（负债减少）

---

## 六、新增/修改 Tool

### 6.1 create_bill 修改
- 新增 sub_category 参数（可选）
- 新增 asset_id 参数（可选）
- Tool 内部：创建账单后自动更新关联资产余额

### 6.2 transfer（新增）
- from_asset_id, to_asset_id, amount, note
- 创建 transfer 类型账单 + 更新两个资产余额

### 6.3 create_budget 修改
- category 改为 categories（JSON 数组）
- 新增 name 参数
- 新增 budget_type 参数

### 6.4 get_budget_execution 修改
- 预算执行率计算改为匹配 categories 列表

### 6.5 suggestions（新增）
- 返回当前可提醒事项（预算超支、周期性支出、今日未记账）

---

## 七、前端变更

### 7.1 账单列表
- 展示一级分类 + 子分类标签
- 收入/支出/转账用不同颜色
- 关联资产名称

### 7.2 预算页面
- 预算组展示（名称 + 包含的分类 + 执行进度条）
- 创建预算时可选择包含哪些分类

### 7.3 资产页面
- 资产余额实时反映账单联动
- 信用卡显示欠款金额

---

## 八、数据库迁移

需要迁移的表：
1. bills：新增 sub_category, asset_id, to_asset_id 字段
2. budgets：新增 name, categories(JSON), budget_type 字段；category 字段保留兼容
3. 新增 categories 表（系统预设 + 用户自定义）
4. 历史数据迁移：现有 budget.category → budget.categories = ["原category"]

---

## 九、实施优先级

| 优先级 | 内容 | 预估 |
|--------|------|------|
| P0 | Category 模型 + 系统预设数据 | 1h |
| P0 | Bill 模型重构（sub_category + asset_id） | 2h |
| P0 | Budget 模型重构（name + categories + budget_type） | 2h |
| P0 | NormalizeCategory 升级（两级推断） | 2h |
| P0 | create_bill Tool 资产联动 | 2h |
| P0 | 预算执行率计算改为匹配 categories | 1h |
| P1 | transfer Tool | 1h |
| P1 | delete_bill / update_bill 资产联动 | 2h |
| P1 | suggestions API | 2h |
| P2 | 前端分类展示 + 预算组 UI | 3h |
| P2 | 历史数据迁移脚本 | 1h |

---

## 十、验证场景

### 分类体系
1. "午饭30元" → category=食饮, sub_category=三餐
2. "星巴克38元" → category=食饮, sub_category=饮品
3. "打车25元" → category=交通, sub_category=打车
4. "买了件衣服200" → category=购物, sub_category=服饰

### 账单-资产联动
5. 创建支出（关联储蓄卡）→ 储蓄卡余额减少
6. 创建收入（工资到储蓄卡）→ 储蓄卡余额增加
7. 删除支出 → 储蓄卡余额恢复
8. 信用卡消费 → 信用卡 balance 变更负
9. 信用卡还款 → 信用卡 balance 增加 + 现金减少

### 预算
10. 创建"吃喝预算"包含["食饮","社交"] → 成功
11. 记一笔食饮支出 → 吃喝预算执行率增加
12. 记一笔社交-聚餐支出 → 吃喝预算执行率也增加
13. 记一笔交通支出 → 吃喝预算不受影响

### 转账
14. 储蓄卡转信用卡（还款）→ 储蓄卡减少 + 信用卡负债减少
15. 储蓄卡转投资账户 → 储蓄卡减少 + 投资增加
