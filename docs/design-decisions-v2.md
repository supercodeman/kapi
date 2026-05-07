# 咔皮记账 AI 助手 — 设计决策记录 v2.0

> 版本：2.0 | 日期：2026-04-29
> 重大变更：Engine 架构从 Agent 模式重构为 Workflow 优先 + Agent 兜底模式

---

## 核心架构变更（v2.0）

### 变更原因

v1.0 采用纯 Agent 架构（LLM 自主决策 + ReAct 循环），实测暴露以下问题：
1. LLM 编造操作结果（声称"已记录"但未调用 Tool）
2. LLM 自行计算金额（把全部历史账单求和当作当月消费）
3. LLM 日期推理错误（输出 2025 年日期而非 2026 年）
4. 记忆模块代码已实现但从未被调用（死代码）
5. Prompt 约束是软约束，LLM 在 10% 的情况下会绕过

### 根因分析

记账应用 90% 的场景是 Workflow（固定流程），只有 10% 需要 Agent 能力。但 v1.0 用 100% Agent 模式处理所有请求，给了 LLM 过多自由度。

### 架构决策

**Workflow 优先，Agent 兜底。**

- 90% 场景（记账、查询、预算管理）走 Workflow：Engine 控制流程，LLM 只做意图识别和回复润色
- 10% 场景（综合建议、多步推理）走 Agent：LLM 自主决策，但受 Engine 监管

---

## 角色定义

### LLM 的角色：意图翻译器 + 表达润色器

| 环节 | LLM 做什么 | LLM 不做什么 |
|------|----------|------------|
| 意图识别 | 理解用户自然语言，输出意图类型 + 参数 | 不决定执行流程 |
| 参数提取 | 从用户话语中提取金额、商户、日期表达 | 不做日期计算、不猜测缺失参数 |
| 回复生成 | 基于 Tool 返回的 Message 和 Data 润色回复 | 不编造任何数值、不声称未执行的操作 |
| 对话管理 | 理解指代、省略、上下文 | 不自己记忆数据 |
| 事实标记 | 辅助标记用户陈述中的事实 | 不决定是否保存记忆 |

### Engine 的角色：流程控制器 + 数据守门人 + 记忆管理者

| 环节 | Engine 做什么 |
|------|-------------|
| 流程路由 | 根据 LLM 输出的意图类型，路由到 Workflow 或 Agent 模式 |
| 参数标准化 | 日期转换、分类映射、默认值填充（用记忆补全） |
| Tool 调用 | Workflow 中强制执行 mandatory_tool，不允许跳过 |
| 结果校验 | 校验 LLM 回复中的数值与 Tool 返回是否一致 |
| 操作声明校验 | LLM 说"已记录"但没有 Tool success → 拦截替换 |
| 记忆写入 | L4 每轮自动存；L3 Tool 执行后自动存；L2 规则提取；L1 累积更新 |
| 记忆读取 | 注入 Prompt + 消息列表 + 补全 Tool 参数默认值 |

---

## Workflow vs Agent 路由

```
用户输入 → LLM 第一轮（意图识别 + 参数提取）
    ↓
Engine 路由：
  ├─ 匹配 Workflow（意图明确 + 有 mandatory_tool）
  │   1. 参数标准化（NormalizeDate/Category/Period/AssetType）
  │   2. 记忆补全（从 L2 取默认分类等）
  │   3. 强制调用 mandatory_tool
  │   4. Tool 返回 {status, message, data}
  │   5. status=success → LLM 基于 message+data 润色回复
  │      status=failed → 直接返回 Tool 的 message
  │   6. Engine 校验回复中的数值
  │   7. 记忆更新（L3 + L2 提取）
  │
  └─ 未匹配 Workflow（意图模糊 / 多步推理 / 闲聊）
      Agent 模式（ReAct 循环，受 Engine 监管）
```

---

## ToolResult 设计（v2.0）

```go
type ToolResult struct {
    ID      string         `json:"id"`
    Name    string         `json:"name"`
    Status  string         `json:"status"`   // "success" / "failed" / "need_confirm"
    Message string         `json:"message"`  // Tool 生成的人类可读结果描述
    Data    map[string]any `json:"data"`
    Source  string         `json:"source"`
}
```

Message 由 Tool 代码生成，不是 LLM 生成。示例：
- create_bill success → "已记录：2026-04-29 拉面 ¥15.00 分类餐饮"
- create_bill + 超预算 → "已记录：2026-04-29 拉面 ¥15.00 分类餐饮。注意：餐饮预算 ¥120.00 已超支，当前已花 ¥135.00"
- get_total_spent success → "2026年4月总支出 ¥53.00"
- create_bill failed → "记账失败：金额不能为负数"

---

## Skill 定义（v2.0）

```yaml
name: record_bill
type: write
mode: workflow

trigger: "用户表达记账意图..."

params:
  - name: amount
    required: true
  - name: category
    required: false
    default_from_memory: true
  - name: date
    required: false
    default: today

mandatory_tool: create_bill
pre_tools: [convert_lunar_date]
post_check: [check_budget_status]

response_constraints:
  required_from_tool: [amount, date, category]
  forbidden_without_tool: ["已记录", "记好了", "已创建"]

reply_guide: "基于 Tool 返回的 message 润色回复，金额和日期必须与 Tool 返回一致"
```

---

## 记忆系统（v2.0）

### 写入策略

| 层级 | 写入时机 | 写入方式 | 决策者 |
|------|---------|---------|--------|
| L1 画像 | 累积多轮对话后 | Engine 定期分析 L3/L4 提取偏好标签 | Engine |
| L2 事实 | 用户陈述事实时 | Engine 规则匹配 + LLM 辅助标记 | Engine 最终决定 |
| L3 情景 | Tool 执行成功后 | Engine 自动记录操作场景 | Engine |
| L4 对话 | 每轮对话后 | Engine 自动存储 | Engine |

### 读取策略

| 层级 | 读取时机 | 用途 |
|------|---------|------|
| L1 | 构造 System Prompt | 给 LLM 用户偏好上下文 |
| L2 | 构造 Prompt + 参数补全 | 给 LLM 上下文 + Engine 填充默认参数 |
| L3 | 构造 Prompt | 给 LLM 历史操作上下文 |
| L4 | 注入消息列表 | 多轮对话连贯性 |

### L2 事实提取规则

Engine 在每轮对话后扫描用户输入，匹配模式：
- "我每月 X 是 Y" → fact_type=monthly_expense
- "我的 X 是 Y" → fact_type=personal_info
- "我在 X 工作" → fact_type=work
- "我有 X 张信用卡" → fact_type=asset_info

代码规则为主，LLM 辅助标记为辅，Engine 有最终决定权。

---

## 问题解决验证矩阵

| 问题 | 解决方式 | 验证方法 |
|------|---------|---------|
| LLM 编造操作结果 | Workflow 强制调 Tool + forbidden_without_tool 拦截 | Mock LLM 不输出 Tool Call，验证 Engine 强制调用 |
| LLM 自己算金额 | 统计场景 forbidden_tools 禁用 list_bills | 统计查询验证数字来自 Tool 返回 |
| 日期推理错误 | NormalizeDate 代码层处理 | "今天"→当天日期，"昨天"→昨天日期 |
| 预算超支行为 | Tool 内部自动查预算附带提醒 | 超预算记账验证：记录成功 + 有提醒 |
| 右栏不刷新 | affected_pages + emit 修复 | 记账后右栏列表更新 |
| 对话上下文丢失 | L4 消息列表注入 | "刚才那笔多少钱"正确回答 |
| Think 标签暴露 | 正则过滤 + 前端开关 | display 无 think 标签 |
| 记忆未激活 | Engine 自动写入，不依赖 LLM | 验证 Redis L4 + MySQL L3 有数据 |
| OpLog trigger 为空 | SetRequestContext 注入 | 验证 trigger_text 有用户原话 |

---

## 场景验证用例

### 场景 A：基础记账
1. "今天拉面15元" → 日期=今天，金额=15，分类=餐饮，DB 有记录，右栏刷新

### 场景 B：预算超支记账
1. 预算已满时 "记一笔咖啡30元" → 记账成功 + 超支提醒，不阻止记账

### 场景 C：统计查询
1. "这个月花了多少" → 数字来自 get_total_spent Tool，不是 LLM 计算

### 场景 D：编造拦截
1. 快速连续发送记账请求 → 每笔都有 DB 记录，无"假记录"

### 场景 E：多轮对话
1. "帮我设置餐饮预算500" → 成功
2. "把它改成600" → 理解"它"指餐饮预算，修改成功

### 场景 F：记忆持久化
1. "我每月房租3500" → L2 事实保存
2. 重新登录后 "我的固定支出有哪些" → 回忆起房租

### 场景 G：综合建议（Agent 模式）
1. "我能买5000的相机吗" → 多步查询资产+预算+消费趋势 → 综合建议
