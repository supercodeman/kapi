# PendingAction 待确认操作机制 — 技术设计文档

## 1. 概述

### 1.1 什么是 PendingAction

PendingAction 是 kapi AI 记账助手中的**多轮确认流程控制机制**。当 AI Engine 在执行写操作（记账、删除、修改）时检测到参数不完整、数据异常或需要用户二次确认的情况，会暂停执行并将操作状态存入内存，等待用户在下一轮对话中提供补充信息或确认指令后再执行。

### 1.2 解决的核心问题

- **防止误操作**：金额异常、日期异常等场景下，避免静默执行错误数据
- **参数补全**：金额缺失、分类模糊时，通过追问获取准确参数
- **批量操作安全**：多笔操作需用户确认清单后再执行
- **零幻觉保障**：LLM 无法从上下文中找到金额来源时，追问而非编造

### 1.3 适用场景

| 场景 | 触发条件 | 追问示例 |
|------|----------|----------|
| 收入类型模糊 | `isVagueIncomeCategory` 返回 true | "这笔是什么类型的收入？工资、兼职、理财还是其他？" |
| 金额缺失 | `amountFoundInContext` 失败 + RAG 无法补全 | "这笔消费金额是多少呢？" |
| 金额异常 | 超过该分类历史最大值 3 倍 | "这笔 ¥3000 超过了餐饮分类历史最高金额的 3 倍，确认要记录吗？" |
| 日期异常 | 超过 30 天前或在未来 | "午饭 餐饮 ¥30.00，日期 2025-01-01（120 天前），确认要记录吗？" |
| RAG 预填确认 | 置信度 >= 0.5 时预填金额需确认 | "星巴克 餐饮，金额 ¥38.00？（根据你的消费习惯预填）" |
| 批量操作确认 | `create_bills_batch` 返回确认清单 | 列出多笔账单明细，等待确认 |
| 删除操作确认 | 删除账单/预算/资产 | "确认要删除这笔记录吗？" |

## 2. 数据结构

### 2.1 PendingAction

```go
// PendingAction 表示一个等待用户确认的操作（支持单笔或多笔）
type PendingAction struct {
    UserID      uint64
    SessionID   uint64
    ToolName    string             // 待执行的 Tool 名称，如 "create_bill"
    Params      map[string]any     // 单笔操作参数
    BatchParams []map[string]any   // 多笔操作时使用，每个元素是一笔的完整参数
    ExpiresAt   time.Time          // 过期时间，默认 5 分钟
}
```

### 2.2 PendingStore

```go
// PendingStore 管理待确认操作，按 userID:sessionID 存储
type PendingStore struct {
    mu    sync.RWMutex
    store map[string]*PendingAction  // key = "userID:sessionID"
}
```

**存储特性：**
- **内存存储**：不持久化，服务重启后丢失（可接受，5 分钟过期窗口内重启概率极低）
- **Key 格式**：`fmt.Sprintf("%d:%d", userID, sessionID)`，确保同一用户同一会话只有一个待确认操作
- **一次性消费**：`Get` 方法取出后立即从 store 中删除
- **5 分钟过期**：超时后 `Get` 返回 nil，操作自动失效
- **并发安全**：`sync.RWMutex` 保护读写

### 2.3 关键方法

| 方法 | 语义 | 说明 |
|------|------|------|
| `Set(userID, sessionID, toolName, params)` | 存储 | 创建/覆盖待确认操作，设置 5 分钟过期 |
| `Get(userID, sessionID)` | 取出并删除 | 一次性消费，过期返回 nil |
| `Clear(userID, sessionID)` | 清除 | 用户取消时调用 |

## 3. 触发条件

所有触发点位于 `internal/engine/tools.go` 的 Tool Handler 中，通过返回 `status: "need_confirm"` 的 `ToolResult` 触发。

### 3.1 金额缺失（优先级最高）

```
位置：create_bill handler, L133
条件：!isConfirmation(ctx) && quantityTotal <= 0 && !amountFoundInContext(amount, userMsg, recentMsgs)
```

校验链路：
1. 先尝试 `calcTotalFromQuantity`（单价x数量计算）
2. 再尝试 `amountFoundInContext`（直接匹配 / 乘积匹配 / 求和匹配 / 上一条消息）
3. 若都失败，尝试 RAG 预填（`ragFiller.FillParams`）
4. RAG 置信度 >= 0.5 → 返回预填确认；否则 → 追问金额

### 3.2 收入类型模糊

```
位置：create_bill handler, L185
条件：!isConfirmation(ctx) && billType == "income" && catResult.SubCategory == "" && isVagueIncomeCategory(catResult.Category)
```

`isVagueIncomeCategory` 匹配的模糊分类：`"其他"`, `"其他收入"`, `"收入"`, `""`

### 3.3 日期异常

```
位置：create_bill handler, L209-213
条件：!isConfirmation(ctx) && guardDateAnomaly 返回非 nil
```

触发规则：
- 日期在未来（`t.After(today)`）
- 日期超过 30 天前（`daysBefore > 30`）

### 3.4 金额异常

```
位置：create_bill handler, L216-227
条件：!isConfirmation(ctx) && billType == "expense" && amount > historyMaxAmount * 3
```

仅对支出生效，查询该分类历史最大金额作为基准。

### 3.5 删除操作确认

```
位置：delete_bill / delete_budget / delete_asset handler
条件：所有删除操作均需确认
```

### 3.6 批量操作确认

```
位置：create_bills_batch handler
条件：批量创建多笔账单时，返回清单让用户确认
```

批量参数通过 `tr.Data["_batch_params"]` 传递。

## 4. 核心流程

### 4.1 存储流程

**触发路径**：`executeWorkflow` / `executeWorkflowStream` → Tool 返回 `need_confirm`

```
1. LLM 输出 ToolCall（含参数 JSON）
2. Engine 调用 Tool Handler
3. Tool Handler 检测到异常条件 → 返回 ToolResult{Status: "need_confirm", Message: "追问文本", Data: 补充参数}
4. executeWorkflow 遍历 toolResults，发现 need_confirm：
   a. 从对应 ToolCall 的 Arguments 中解析 LLM 提取的参数
   b. 合并 ToolResult.Data 中的补充参数（如 RAG 预填的 amount）
   c. 提取 _batch_params（如有）
   d. 构造 PendingAction，存入 PendingStore
5. 返回 ChatResponse{Display: 追问消息}
```

**参数合并规则**（存储时）：
- LLM ToolCall 参数为基础
- ToolResult.Data 中的字段仅在 LLM 参数中不存在时补充（`if _, exists := params[k]; !exists`）
- `_batch_params` 单独提取，不混入 Params

### 4.2 消费流程（下一轮用户消息）

**入口**：`Chat()` / `chatStreamInternal()` 的 PendingAction 快速路径

```
1. 检查取消意图：isCancelMessage → 清除 PendingAction，返回"已取消"
2. 取出 PendingAction：pending.Get（一次性消费）
3. 判断用户消息是否为参数回答：isPendingParamAnswer
   - 是 → executePendingAction（直接执行，不经过 LLM）
   - 否 → 放回 PendingStore，走正常 LLM 流程
```

### 4.3 isPendingParamAnswer 判断逻辑

```go
func isPendingParamAnswer(msg string) bool {
    // 前置条件：消息长度 <= 10 个字符（超过视为新意图）

    // 1. 疑问词前置过滤（排除追问）
    //    "吗", "呢", "？", "?", "哪些", "什么", "怎么", "如何", "为什么"
    //    → 命中任一则返回 false

    // 2. 确认词匹配
    //    "确认", "记录吧", "记吧", "好的", "可以", "是的", "对", "没错",
    //    "确定", "就这样", "行", "嗯", "ok", "OK", "yes"
    //    → 命中任一则返回 true

    // 3. 纯金额匹配（长度 <= 6 字符）
    //    ParseChineseNumber 提取到数字 → 返回 true

    // 4. 精确分类名匹配（knownCategories 白名单）
    //    "工资", "奖金", "兼职", "理财", "红包", "退款", "报销", "其他",
    //    "餐饮", "交通", "购物", "娱乐", "居住", "医疗", "教育", "通讯",
    //    "食饮", "日用", "服饰", "运动", "旅行", "社交", "宠物",
    //    "职业收入", "投资收入", "其他收入"
    //    → 精确匹配则返回 true

    // 5. 以上都不满足 → 返回 false
}
```

### 4.4 参数合并（mergePendingParams）

在 `executePendingAction` 执行前，将用户回答合并进 PendingAction 参数：

```go
func mergePendingParams(action *PendingAction, userMsg string) {
    // 1. 提取金额：ParseChineseNumber → 覆盖 action.Params["amount"]
    // 2. 分类补全（仅 create_bill 且分类为空/模糊时）：
    //    a. NormalizeCategoryV2(msg, "", msg) 标准化用户回答
    //    b. 收入场景下 "其他" → "其他收入"
    //    c. 写入 action.Params["category"] 和 action.Params["sub_category"]
}
```

**关键设计**：金额覆盖优先——用户说了金额就用用户的，即使 PendingAction 中已有预填值。

### 4.5 执行流程（executePendingAction）

```go
func (e *Engine) executePendingAction(ctx, req, action, startTime) {
    // 1. 注入 context
    ctx = ContextWithRequest(ctx, req.SessionID, req.Message, nil)
    ctx = ContextWithConfirmation(ctx)  // 标记为确认轮次 → 跳过所有 Guard

    // 2. 批量 vs 单笔分流
    if len(action.BatchParams) > 0 {
        return executeBatchPendingAction(...)
    }

    // 3. 单笔执行
    mergePendingParams(action, req.Message)  // 合并用户回答
    argsJSON := json.Marshal(action.Params)
    tc := ToolCall{ID: "pending_confirm", Name: action.ToolName, Arguments: argsJSON}
    result := toolExecutor.Execute(ctx, userID, tc)  // 直接调用 Tool，不经过 LLM

    // 4. 返回结果
    return ChatResponse{Display: result.Message, AffectedPages: ...}
}
```

**ContextWithConfirmation 的作用**：Tool Handler 中所有 `!isConfirmation(ctx)` 的 Guard 检查都会被跳过，包括：
- 金额来源校验
- 收入类型模糊检查
- 日期异常检测
- 金额异常检测

这确保了用户确认后操作能直接执行，不会再次触发追问循环。

## 5. 多轮对话中的 PendingAction 保持

### 5.1 "放回"机制

当用户发送的消息不被 `isPendingParamAnswer` 识别为参数回答时（如追问、闲聊、无关消息），PendingAction 不会丢失：

```go
if isPendingParamAnswer(req.Message) {
    // 执行
} else {
    // 放回 PendingStore，走正常 LLM 流程
    e.pending.Set(action.UserID, action.SessionID, action.ToolName, action.Params)
}
```

**注意**：放回时使用 `Set` 方法，会重置 5 分钟过期时间。这意味着只要用户持续对话（即使是无关消息），PendingAction 的过期时间会不断延长。

### 5.2 不被覆盖的条件

PendingAction 在以下情况下不会被正常 LLM 流程覆盖：
- 正常 LLM 流程中 Tool 返回 `need_confirm` 时会**覆盖**现有 PendingAction（同一 key）
- 正常 LLM 流程中 Tool 全部成功时**不会**触碰 PendingStore
- 放回操作发生在 LLM 调用之前，如果 LLM 流程中产生新的 `need_confirm`，会覆盖放回的 PendingAction

### 5.3 过期保护

- 默认 5 分钟过期
- `Get` 时检查 `time.Now().After(action.ExpiresAt)`，过期返回 nil
- 过期后用户的确认消息会走正常 LLM 流程（可能重新触发记账意图识别）

### 5.4 取消机制

```go
var cancelKeywords = []string{
    "取消", "不要了", "不需要", "算了", "不记了", "不用了", "不了",
}
```

取消检查在 PendingAction 取出之前执行，优先级最高。

## 6. 双入口一致性

### 6.1 共享实例

`Engine` 结构体持有唯一的 `pending *PendingStore`，同步端点和流式端点共享同一个实例：

```go
type Engine struct {
    // ...
    pending *PendingStore  // 唯一实例，Chat 和 ChatStream 共享
}
```

### 6.2 代码路径对比

| 逻辑 | engine.go (Chat) | stream.go (ChatStream) |
|------|-------------------|------------------------|
| 取消检查 | `isCancelMessage` → `pending.Clear` | 相同 |
| 取出 PendingAction | `pending.Get` | 相同 |
| 判断参数回答 | `isPendingParamAnswer` | 相同 |
| 执行 | `executePendingAction` | 相同（共享方法） |
| 放回 | `pending.Set` | 相同 |
| Workflow 中存储 | `executeWorkflow` 内直接操作 `pending.store` | `executeWorkflowStream` 内直接操作 `pending.store` |

### 6.3 修改规则

**强制要求**：修改 PendingAction 相关逻辑时，必须同时检查并同步以下位置：
1. `engine.go` → `Chat()` 中的 PendingAction 快速路径
2. `stream.go` → `chatStreamInternal()` 中的 PendingAction 快速路径
3. `engine.go` → `executeWorkflow()` 中的 `need_confirm` 存储逻辑
4. `stream.go` → `executeWorkflowStream()` 中的 `need_confirm` 存储逻辑
5. `pending.go` → `mergePendingParams` 参数合并逻辑

## 7. 已知边界情况与修复记录

### 7.1 "其他" 作为收入类型回答

**问题**：用户回答"其他"时，`isPendingParamAnswer` 无法识别，因为"其他"不在早期的确认词列表中，也不是纯金额。

**修复**：在 `isPendingParamAnswer` 中增加 `knownCategories` 白名单精确匹配，将"其他"纳入已知分类名。同时在 `mergePendingParams` 中，收入场景下将"其他"映射为"其他收入"。

### 7.2 疑问句误判为参数回答

**问题**：用户发送"这是什么？"等疑问句时，如果恰好包含确认词子串，会被误判为参数回答并触发执行。

**修复**：在 `isPendingParamAnswer` 中增加疑问词前置过滤，包含"吗"、"呢"、"？"、"哪些"、"什么"、"怎么"、"如何"、"为什么"中任一词时，直接返回 false。

### 7.3 NormalizeCategoryV2 短关键词误匹配

**问题**：早期 `NormalizeCategoryV2` 使用 `strings.Contains` 做模糊匹配，导致"包"匹配到"包含"等无关词。

**修复**：改用精确匹配替代模糊匹配，分类映射表中的关键词必须完整匹配用户输入或作为独立词出现。

### 7.4 消息长度阈值

**设计决策**：`isPendingParamAnswer` 限制消息长度 <= 10 个字符。超过 10 个字的消息被视为新意图而非简短参数回答，会触发"放回"机制走正常 LLM 流程。

## 8. 时序图

### 8.1 正常确认流程

```
用户                    Engine                  PendingStore            Tool
 |                        |                        |                     |
 |-- "记一笔午饭" ------->|                        |                     |
 |                        |-- LLM → ToolCall ----->|                     |
 |                        |                        |-- Execute --------->|
 |                        |                        |                     |
 |                        |<-- need_confirm -------|<-- {status:"need_confirm",
 |                        |    (金额缺失)           |    message:"金额是多少？"}
 |                        |                        |                     |
 |                        |-- Store PendingAction ->|                     |
 |                        |   key="uid:sid"        |                     |
 |                        |   params={category:餐饮,|                     |
 |                        |     merchant:午饭}      |                     |
 |                        |                        |                     |
 |<-- "金额是多少呢？" ---|                        |                     |
 |                        |                        |                     |
 |-- "30" --------------->|                        |                     |
 |                        |-- Get(uid, sid) ------>|                     |
 |                        |<-- PendingAction ------|                     |
 |                        |                        |                     |
 |                        |-- isPendingParamAnswer("30") = true          |
 |                        |-- mergePendingParams(action, "30")           |
 |                        |   → params.amount = 30                       |
 |                        |-- ContextWithConfirmation                    |
 |                        |                        |                     |
 |                        |-- Execute(ctx, tc) ----|-- create_bill ----->|
 |                        |                        |   (跳过所有Guard)    |
 |                        |<-- success ------------|<-- "已记录" --------|
 |                        |                        |                     |
 |<-- "已记录午饭 ¥30" ---|                        |                     |
```

### 8.2 用户插入无关消息后恢复流程

```
用户                    Engine                  PendingStore            LLM
 |                        |                        |                     |
 |-- "记一笔收入500" ---->|                        |                     |
 |                        |-- [Tool 返回 need_confirm: 收入类型模糊] --->|
 |                        |-- Store PendingAction ->|                     |
 |<-- "什么类型的收入？" -|                        |                     |
 |                        |                        |                     |
 |-- "今天天气怎么样" --->|                        |                     |
 |                        |-- Get(uid, sid) ------>|                     |
 |                        |<-- PendingAction ------|                     |
 |                        |                        |                     |
 |                        |-- isPendingParamAnswer("今天天气怎么样")      |
 |                        |   → len > 10 → false                         |
 |                        |                        |                     |
 |                        |-- Set(放回) ---------->|  (重置5分钟过期)     |
 |                        |                        |                     |
 |                        |-- 走正常 LLM 流程 ---->|-- ChatSync -------->|
 |                        |<-- "今天天气不错..." ---|<-- 闲聊回复 --------|
 |<-- "今天天气不错..." --|                        |                     |
 |                        |                        |                     |
 |-- "工资" ------------->|                        |                     |
 |                        |-- Get(uid, sid) ------>|                     |
 |                        |<-- PendingAction ------|                     |
 |                        |                        |                     |
 |                        |-- isPendingParamAnswer("工资")                |
 |                        |   → knownCategories["工资"] = true           |
 |                        |                        |                     |
 |                        |-- mergePendingParams(action, "工资")          |
 |                        |   → params.category = "工资"                 |
 |                        |-- ContextWithConfirmation                    |
 |                        |-- Execute → success                          |
 |                        |                        |                     |
 |<-- "已记录工资收入 ¥500" |                      |                     |
```

### 8.3 批量操作确认流程

```
用户                    Engine                  PendingStore            Tool
 |                        |                        |                     |
 |-- "帮我记三笔：       |                        |                     |
 |    午饭30、咖啡25、    |                        |                     |
 |    打车15" ----------->|                        |                     |
 |                        |-- LLM → create_bills_batch ToolCall -------->|
 |                        |                        |                     |
 |                        |<-- need_confirm -------|<-- {status:"need_confirm",
 |                        |    Data: {              |    message: "确认以下3笔：
 |                        |      _batch_params: [   |      1. 午饭 ¥30
 |                        |        {amount:30,...}, |      2. 咖啡 ¥25
 |                        |        {amount:25,...}, |      3. 打车 ¥15
 |                        |        {amount:15,...}  |    确认记录吗？"}
 |                        |      ]                  |
 |                        |    }                    |
 |                        |                        |                     |
 |                        |-- Store PendingAction ->|                     |
 |                        |   BatchParams = [...]   |                     |
 |                        |                        |                     |
 |<-- "确认以下3笔..." --|                        |                     |
 |                        |                        |                     |
 |-- "确认" ------------->|                        |                     |
 |                        |-- Get(uid, sid) ------>|                     |
 |                        |<-- PendingAction ------|                     |
 |                        |   (BatchParams有值)     |                     |
 |                        |                        |                     |
 |                        |-- isPendingParamAnswer("确认") = true         |
 |                        |-- executeBatchPendingAction                   |
 |                        |   for each BatchParams[i]:                   |
 |                        |     Execute(ctx, create_bill, params[i]) --->|
 |                        |                        |                     |
 |<-- "第1笔已记录...    |                        |                     |
 |     第2笔已记录...    |                        |                     |
 |     第3笔已记录..." --|                        |                     |
```

### 8.4 取消流程

```
用户                    Engine                  PendingStore
 |                        |                        |
 |-- "记一笔午饭" ------->|                        |
 |                        |-- [触发 need_confirm]  |
 |                        |-- Store PendingAction ->|
 |<-- "金额是多少？" ----|                        |
 |                        |                        |
 |-- "算了" ------------->|                        |
 |                        |-- isCancelMessage("算了") = true
 |                        |-- pending.Clear ------->| (删除)
 |                        |                        |
 |<-- "好的，已取消。" --|                        |
```

## 9. 设计决策总结

| 决策 | 选择 | 理由 |
|------|------|------|
| 存储方式 | 内存 map | 5 分钟过期窗口短，无需持久化；避免 Redis 网络开销 |
| 消费模式 | 一次性取出 | 防止重复执行；放回时显式调用 Set |
| 确认跳过 Guard | ContextWithConfirmation | 避免确认后再次触发追问的死循环 |
| 参数合并策略 | 用户回答覆盖预填值 | 用户意图优先，预填仅为建议 |
| 消息长度阈值 | 10 字符 | 平衡"简短参数"与"新意图"的区分 |
| 双入口共享 | 同一 PendingStore 实例 | 用户可能在同步/流式端点间切换 |
