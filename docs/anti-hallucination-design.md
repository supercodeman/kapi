# 反幻觉防线 — 技术设计文档

## 1. 设计目标

本系统的核心原则：**LLM 是"推理引擎 + 表达引擎"，不是"数据源"**。

具体约束：
- 所有金额、统计数据必须来自工具（Tool）查询结果，LLM 不得自行生成
- 禁止 LLM 自行做加减乘除运算（如"调整后还剩多少"、"购买后会超支多少"）
- 禁止 LLM 从对话历史中引用过期金额数据
- 禁止 LLM 在未调用工具的情况下声称操作已完成

设计哲学：宁可追问用户、宁可返回工具原始消息，也不允许 LLM 编造或推算数据。

---

## 2. 五层防线架构

```
┌─────────────────────────────────────────────────────────┐
│  第一层：System Prompt 硬约束（prompt.go）               │
│  ↓ LLM 生成意图                                         │
├─────────────────────────────────────────────────────────┤
│  第二层：金额来源校验（validate.go — amountFoundInContext）│
│  ↓ Tool 执行前参数校验                                   │
├─────────────────────────────────────────────────────────┤
│  第三层：对话历史脱敏（history.go）                       │
│  ↓ 切断 LLM 从历史中获取过期数据的路径                    │
├─────────────────────────────────────────────────────────┤
│  第四层：LLM 回复金额校验（validate.go — containsUnverifiedAmount）│
│  ↓ 对比 Tool 返回数据，拦截编造金额                       │
├─────────────────────────────────────────────────────────┤
│  第五层：操作声明校验（engine.go — validateResponse）      │
│  ↓ 拦截无 Tool 执行的虚假操作声明                         │
└─────────────────────────────────────────────────────────┘
```

---

### 2.1 第一层：System Prompt 硬约束（prompt.go）

**文件位置**：`internal/engine/prompt.go`

System Prompt 中定义了 6 条最高优先级规则（"最重要的规则"），其中与反幻觉直接相关的核心规则：

| 规则编号 | 内容 | 防御目标 |
|---------|------|---------|
| 规则 1 | 必须调用工具，不能跳过工具直接回复"已记录" | 防止虚假操作声明 |
| 规则 2 | 没有调用工具就不能声称操作已完成 | 防止虚假操作声明 |
| 规则 3 | 所有金额/统计必须来自工具返回，不自己计算或编造 | 防止数据编造 |
| 规则 4 | 不从对话历史中引用金额，必须调用查询工具 | 防止引用过期数据 |
| 规则 6 | 绝不自己做加减乘除，不用工具数值做推算 | 防止二次计算 |

**行为规范补充**：
- 查询消费统计必须使用 `get_category_summary` 或 `get_budget_execution` 工具
- 预算可承受性必须调用 `check_budget_affordability` 工具
- 环比/同比必须调用 `get_month_comparison` 工具
- 绝不对账单金额求和

**设计意图**：System Prompt 是第一道防线，依赖 LLM 的指令遵循能力。但 LLM 不是 100% 可靠的，因此需要后续层级做硬校验。

---

### 2.2 第二层：金额来源校验（validate.go — amountFoundInContext）

**文件位置**：`internal/engine/validate.go`

**函数签名**：
```go
func amountFoundInContext(amount float64, userMsg string, recentUserMsgs []string) bool
```

**校验逻辑（四层递进）**：

1. **直接匹配**：金额数值直接出现在用户当前消息中
   - 使用 `ParseChineseNumber` 提取所有数字（支持阿拉伯数字和中文数字）
   - 精度容差：`|found - expected| < 0.01`

2. **乘积匹配**：两个数字相乘等于金额（单价 × 数量场景）
   - 遍历消息中所有数字对 `(i, j)`，检查 `numbers[i] * numbers[j] == amount`
   - 约束：`numbers[j] > 1`（数量必须大于 1）

3. **求和匹配**：多个数字相加等于金额（多商品场景）
   - 使用组合算法 `checkSumCombination`，检查 2~4 个数字的组合和
   - 限制：数字总数不超过 20 个，组合数量限制在 4 个以内（防止组合爆炸）

4. **上一条消息匹配**：金额出现在上一条用户消息中（多轮补参场景）
   - 从 `recentUserMsgs[0]` 中提取数字进行直接匹配

**失败时的处理链路**：
- 金额校验失败 → RAG 预填机制介入（从 L5 消费模式中查找历史金额）
- RAG 无匹配 → 返回 `need_confirm` 状态，追问用户金额

---

### 2.3 第三层：对话历史脱敏（history.go）

**文件位置**：`internal/engine/history.go`

**核心参数**：
```go
const recentRounds = 3  // 近期保留完整内容的轮数（1轮 = 1 user + 1 assistant）
```

**脱敏规则**：

| 条件 | 处理方式 |
|------|---------|
| 最近 3 轮（6 条消息） | 完整保留，不做任何脱敏 |
| 远期 assistant 消息 | 执行脱敏替换 |
| 所有 user 消息 | 永不脱敏（保留用户原始表达） |
| 包含问号的 assistant 消息 | 永不脱敏（追问是上下文连贯的关键） |

**三类脱敏正则**：

1. **预算执行数据**（优先匹配，避免被金额模式部分匹配）：
   ```go
   budgetExecPattern = regexp.MustCompile(`(?:已花|已用|剩余|超出预算?|还剩)\s*¥?[\d,.]+`)
   // 替换为：[金额已省略]
   ```

2. **统计数据**：
   ```go
   statsPattern = regexp.MustCompile(`共\s*\d+\s*笔|总[计共]?\s*¥?[\d,.]+|[环同]比[\d.]+%|占[\d.]+%|增长[\d.]+%|下降[\d.]+%`)
   // 替换为：[数据已省略，请通过工具查询]
   ```

3. **金额**：
   ```go
   amountPattern = regexp.MustCompile(`¥[\d,]+\.?\d*|[\d,]+(\.\d+)?\s*[元块]`)
   // 替换为：¥***
   ```

**后处理**：清理连续的省略标记（如 `¥***，¥***` → `¥***`）

**设计意图**：切断 LLM 从远期对话历史中"记住"具体金额的路径，迫使其必须调用工具获取最新数据。

---

### 2.4 第四层：LLM 回复金额校验（validate.go — containsUnverifiedAmount）

**文件位置**：`internal/engine/validate.go`

**函数签名**：
```go
func containsUnverifiedAmount(display string, toolResults []ToolResult) bool
```

**校验逻辑**：

1. 从所有 `ToolResult.Message` 中提取 `¥` 格式的金额，构建"合法金额集合"
2. 从 LLM 回复（`display`）中提取所有 `¥` 格式的金额
3. 逐一对比：如果 LLM 回复中出现任何不在合法集合中的金额 → 返回 `true`
4. 精度容差：`|llm_amount - tool_amount| < 0.01`

**触发条件**（在 `executeWorkflow` 中）：

```go
// 写操作场景：LLM 回复中有 Tool 数据中不存在的金额 → 用 Tool message 替换
if hasWriteToolResult(toolResults) && containsUnverifiedAmount(result.Display, toolResults) {
    result.Display = buildToolMessageReply(toolResults)
}

// 数值敏感查询场景：同样替换
if hasNumericQueryResult(toolResults) && containsUnverifiedAmount(result.Display, toolResults) {
    result.Display = buildToolMessageReply(toolResults)
}
```

**替换策略**：`buildToolMessageReply` 优先取最后一个写操作 Tool 的 message，兜底取最后一个成功 Tool 的 message。

**适用工具类型**：
- 写操作工具：`create_bill`、`update_bill`、`delete_bill`、`create_budget`、`update_budget`、`delete_budget`、`create_asset`、`update_asset`、`delete_asset`、`transfer`
- 数值敏感查询工具：见 2.5 节 `numericQueryTools`

---

### 2.5 第五层：操作声明校验（engine.go — validateResponse）

**文件位置**：`internal/engine/engine.go`

**函数签名**：
```go
func (e *Engine) validateResponse(resp *ChatResponse, toolResults []ToolResult) *ChatResponse
```

**校验逻辑**：

1. 检查 `toolResults` 中是否有任何 `status == "success"` 的结果
2. 如果没有成功的 Tool 执行，但 LLM 回复中包含操作完成声明 → 拦截

**操作声明关键词列表**（`containsOperationClaim`）：
```go
claims := []string{
    "已记录", "记好了", "已创建", "已修改", "已调整", "已删除", "已设置",
    "记录了", "创建了", "修改了", "删除了", "设置了", "调整了",
    "为你记录", "帮你记录", "帮你创建", "帮你修改", "帮你删除",
    "记上了", "记好啦", "记录成功", "创建成功", "修改成功", "删除成功",
}
```

**拦截后的响应**：
```
"抱歉，操作未能完成，请重新描述你的需求。"
```

---

## 3. 工具调用强制机制（handleNoToolCall）

**文件位置**：`internal/engine/engine.go`

当 LLM 第一轮调用没有输出 Tool Call 时，Engine 会检查用户意图是否需要工具调用。

**处理优先级**：

1. **操作声明检测**（最高优先级）：LLM 声称操作已完成但没调 Tool → 立即重试
2. **追问检测**：LLM 回复包含问号且无操作声明 → 允许直接回复（追问是合理行为）
3. **关键词检测**：用户消息包含操作/查询关键词但 LLM 没调 Tool → 重试

**needsToolKeywords 关键词列表**：

```go
needsToolKeywords := []string{
    // 写操作
    "记", "花了", "消费", "买了", "支出", "收入",
    "设置预算", "创建预算", "调整预算", "修改预算", "删除",
    "添加资产", "创建资产", "修改资产", "转账",
    // 查询操作
    "偏好", "分析", "统计", "汇总", "趋势", "环比", "同比",
    "花了多少", "消费了多少", "还剩多少", "预算执行", "预算情况",
    "账单", "明细", "记录", "查询", "查看",
    "资产", "净资产", "负债", "习惯", "模式", "规律",
    // 预算可承受性查询
    "能不能买", "够不够", "买个", "买得起", "超支", "预算够",
}
```

**重试机制**：
- 追加强制指令：`"请务必调用对应的工具来查询或执行这个操作，不要自己编造数据，不要直接回复。"`
- 重试成功（LLM 输出了 Tool Call）→ 进入 `executeWorkflow`
- 重试失败 → 返回拦截消息：`"抱歉，操作未能完成，请重新描述你的需求，例如：'帮我记一笔午饭30元'"`

**流式路径一致性**：`messageNeedsTool` 函数使用相同的关键词列表，在流式路径中决定是否使用同步调用（确保 Tool Call 不丢失）。

---

### 3.1 数值敏感查询工具保护（numericQueryTools）

**文件位置**：`internal/engine/validate.go`

```go
var numericQueryTools = map[string]bool{
    "check_budget_affordability": true,  // 预算可承受性查询
    "get_budget_execution":       true,  // 预算执行情况
    "get_category_summary":       true,  // 分类汇总统计
    "get_month_comparison":       true,  // 月度对比
}
```

**保护机制**：
- 这些工具返回的数值结果是最终结论，LLM 不得对其做二次计算
- 如果 LLM 回复中出现这些工具数据中不存在的金额，直接用工具原始 message 替换 LLM 回复
- 通过 `hasNumericQueryResult` + `containsUnverifiedAmount` 组合实现

**典型防御场景**：
- 用户问"预算够不够买 XX"，`check_budget_affordability` 返回"超支 ¥20"，LLM 不得自行解读为"刚好够"
- 用户问"本月花了多少"，`get_category_summary` 返回具体数据，LLM 不得自行求和或做百分比计算

---

## 4. RAG 预填机制（rag.go）

**文件位置**：`internal/engine/rag.go`

### 4.1 消费模式匹配

RAG 预填从 L5 消费模式（`consumption_patterns` 表）中检索用户历史消费数据，用于补全缺失的记账参数。

**匹配优先级**：
1. **商户精确匹配**：用户消息中包含已知商户名 → 使用该商户的历史金额和分类
2. **分类关键词匹配**：用户消息中包含已知分类名 → 使用该分类的历史金额

### 4.2 置信度评分

```go
func calcConfidence(count int, effectScore float64) float64 {
    if count >= 5 && effectScore >= 2.0 { return 0.9 }  // 高置信度
    if count >= 3 && effectScore >= 1.5 { return 0.7 }  // 中置信度
    if count >= 2 && effectScore >= 1.0 { return 0.5 }  // 中低置信度
    return 0.3                                           // 低置信度
}
```

**置信度决策矩阵**：

| 置信度范围 | 行为 | 提示文案 |
|-----------|------|---------|
| >= 0.8 | 直接使用，调用 create_bill | "根据你的消费习惯预填" |
| 0.5 ~ 0.8 | 返回 need_confirm，等用户确认 | "根据你的消费记录推测" |
| < 0.5 | 追问用户金额 | — |

### 4.3 RAG Hint 注入 System Prompt

`BuildRAGHint` 函数生成的提示文本会注入到 System Prompt 的 `## 消费习惯提示` 段落中：

```go
if pl.ragHint != "" {
    messages[0].Content += "\n\n## 消费习惯提示\n" + pl.ragHint
}
```

**Hint 格式示例**：
```
用户在「星巴克」有 12 次消费记录，上次 ¥38.00，分类食饮。用户没说金额时，直接调用 create_bill 工具，amount=38.00，category=食饮，不需要追问金额。
```

**设计意图**：RAG Hint 是对 LLM 的"指令级"引导，告诉它可以直接使用哪个金额，避免 LLM 自行猜测或编造金额。

---

## 5. 数量计算引擎（quantity.go）

**文件位置**：`internal/engine/quantity.go`

### 5.1 单价 × 数量模式识别

**函数**：`calcTotalFromQuantity(userMsg string) float64`

**识别模式**：
- 数量提取：`(?:买了|共|一共)\s*([中文数字|\d]+)\s*[量词]`
- 单价提取：`[每一]\s*[量词]\s*(\d+(?:\.\d+)?)` 或 `(\d+(?:\.\d+)?)\s*[元块]?\s*[一每/]\s*[量词]`
- 量词集合：`杯个份瓶包袋盒箱碗碟盘张套件双只条块斤两升辆艘匹`

**计算流程**：
1. 正则提取数量（必须 > 1）
2. 正则提取单价
3. 兜底：去掉日期后从剩余文本中找金额
4. Engine 层直接计算 `price * qty`，不依赖 LLM

### 5.2 多商品求和模式

**函数**：`calcTotalFromSum(userMsg string) float64`

**识别模式**：
- `一[量词]是?\s*(\d+)，一[量词]是?\s*(\d+)`
- `(\d+)\s*[和加+]\s*(\d+)`

### 5.3 多商品解析

**函数**：`parseMultipleItems(userMsg string) []ParsedItem`

按逗号/顿号/分号拆分用户消息，逐段提取商品名和金额，用于 `create_bills_batch` 批量记账。

### 5.4 中文数字解析

**文件位置**：`internal/engine/chinese_number.go`

**函数**：`ParseChineseNumber(s string) []float64`

支持：
- 阿拉伯数字：`30`、`9.9`
- 中文数字：`三千五百二十八`、`两万三`、`一百五`
- 货币表达：`九块九`、`三毛`
- 省略单位推断：`两万三` → 23000，`一百五` → 150

**设计意图**：所有数值计算在 Engine 层完成，100% 准确，不依赖 LLM 的数学能力。

---

## 6. 边界情况处理

### 6.1 LLM 对预算查询结果做错误解读

**场景**：`check_budget_affordability` 返回"超支 ¥20"，LLM 回复"刚好 ¥20 可以买"

**防御**：第四层 `containsUnverifiedAmount` 检测到 LLM 回复中的金额与 Tool 返回不一致，直接替换为 Tool 原始 message。

### 6.2 LLM 从对话历史中引用过期金额

**场景**：用户上周问过"本月花了多少"，LLM 从历史中找到上周的统计数据直接回复

**防御**：
- 第三层：远期 assistant 消息中的金额已被替换为 `¥***`，LLM 无法获取具体数值
- 第一层：System Prompt 明确禁止从历史中引用金额
- 第三层补充：`handleNoToolCall` 检测到查询类关键词但无 Tool Call → 强制重试

### 6.3 LLM 对多个工具结果做交叉计算

**场景**：用户问"餐饮和交通加起来多少"，LLM 调用两次 `get_category_summary` 后自行求和

**防御**：
- 第一层：System Prompt 规则 6 明确禁止自行做加减乘除
- 第四层：如果 LLM 回复中出现的金额不在任何 Tool 返回中 → 替换为 Tool 原始 message
- `numericQueryTools` 保护：`get_category_summary` 属于数值敏感工具，其结果受严格校验

### 6.4 写操作快速返回（跳过 LLM 二次生成）

**场景**：`create_bill` 成功执行后，不再让 LLM 生成回复，直接使用 Tool 的 message

**实现**：
```go
if hasWriteToolResult(toolResults) && allToolsSucceeded(toolResults) {
    display := buildToolMessageReply(toolResults)
    // 直接返回，不经过 LLM 第二轮
}
```

**设计意图**：写操作的确认消息由 Tool 层生成（结构化、准确），完全绕过 LLM 的表达层，从根本上消除幻觉风险。

---

## 7. 配置与扩展

### 7.1 numericQueryTools 可扩展

```go
var numericQueryTools = map[string]bool{
    "check_budget_affordability": true,
    "get_budget_execution":       true,
    "get_category_summary":       true,
    "get_month_comparison":       true,
}
```

新增数值敏感的查询工具时，只需在此 map 中添加条目，即可自动获得第四层金额校验保护。

### 7.2 needsToolKeywords 可扩展

`handleNoToolCall` 和 `messageNeedsTool` 中的关键词列表可按需扩展。新增业务场景时，添加对应的触发关键词即可启用工具调用强制机制。

**注意**：两处关键词列表必须保持同步（`engine.go` 中的 `handleNoToolCall` 和 `messageNeedsTool`），否则同步/流式路径行为不一致。

### 7.3 脱敏正则可配置

`history.go` 中的三个正则模式：
- `amountPattern`：金额模式
- `statsPattern`：统计模式
- `budgetExecPattern`：预算执行模式

可根据业务扩展调整匹配范围。替换顺序有依赖关系：`budgetExecPattern` 必须先于 `amountPattern` 执行，避免部分匹配。

### 7.4 操作声明关键词可扩展

`containsOperationClaim` 中的 `claims` 列表可按需添加新的操作完成表达方式。

### 7.5 双入口一致性约束

所有反幻觉逻辑在同步端点（`engine.go`）和流式端点（`stream.go`）中必须保持一致：
- `executeWorkflow` ↔ `executeWorkflowStream`：金额校验、写操作快速返回逻辑相同
- `handleNoToolCall`：流式路径通过 `mayNeedTool` 判断后走同步 `handleNoToolCall`
- `validateResponse`：两个路径都调用

---

## 附录：数据流全景图

```
用户消息
  │
  ├─→ [Engine 层] calcTotalFromQuantity / parseMultipleItems
  │     → 数量计算（不依赖 LLM）
  │
  ├─→ [Engine 层] preloadContext
  │     ├─→ buildHistoryMessages（第三层脱敏）
  │     └─→ BuildRAGHint（RAG 预填）
  │
  ├─→ [LLM 层] 第一轮调用
  │     ├─→ 有 Tool Call → executeWorkflow
  │     │     ├─→ Tool 执行
  │     │     ├─→ Guard 校验（金额异常/日期异常）
  │     │     ├─→ amountFoundInContext（第二层）
  │     │     ├─→ 写操作快速返回（跳过 LLM 二次生成）
  │     │     └─→ LLM 第二轮 → containsUnverifiedAmount（第四层）
  │     │                      → validateResponse（第五层）
  │     │
  │     └─→ 无 Tool Call → handleNoToolCall
  │           ├─→ containsOperationClaim → 重试
  │           ├─→ needsToolKeywords 匹配 → 重试
  │           └─→ 闲聊 → validateResponse（第五层）
  │
  └─→ [输出] ChatResponse.Display
```
