# 咔皮记账 AI 助手 — 关键时序图

## 一、记账流程（单步直达）

用户说"午饭30元"，Engine 识别意图后调用 create_bill Tool。

```mermaid
sequenceDiagram
    participant U as 用户
    participant FE as 前端
    participant H as Handler(SSE)
    participant E as Engine
    participant LLM as LLM Provider
    participant T as ToolExecutor
    participant DB as MySQL

    U->>FE: "午饭30元"
    FE->>H: POST /api/chat/stream
    H->>E: ChatStream(req)
    E->>E: retrieveMemories + buildSystemPrompt
    E->>E: buildHistoryMessages（远期脱敏）
    E-->>FE: SSE: {type:"thinking"}

    E->>LLM: ChatSync(messages, tools)
    LLM-->>E: ToolCall: create_bill{amount:30, category:"食饮", date:"today"}

    E->>T: Execute(create_bill)
    T->>T: amountFoundInContext(30, "午饭30元") ✓
    T->>T: NormalizeCategoryV2("食饮","","午饭30元") → 食饮/三餐
    T->>T: guardDateAnomaly ✓
    T->>T: guardAmountAnomaly ✓
    T->>DB: INSERT bills
    T->>DB: UPDATE assets (余额扣减)
    T->>T: logOperation
    T-->>E: ToolResult{status:"success", message:"已记录..."}

    E->>E: saveToolMemory (L3)
    E->>LLM: ChatSync(messages + tool_result)
    LLM-->>E: "已记录：今天午饭 ¥30.00，分类食饮（三餐）"

    E->>E: validateResponse（检查未验证金额）
    E-->>FE: SSE: {type:"chunk"} × N
    E-->>FE: SSE: {type:"done", data:ChatResponse}
    E->>E: persistConversation (Redis)
    E->>E: extractFacts (L2)
    E->>E: go updateUserProfile (L1)
    FE->>U: 流式显示回复
```

## 二、预算可负担性查询 + 调整（多步 ReAct）

用户问"买6000的会员预算够吗"→ 不够 → "调整到10000" → 确认。

```mermaid
sequenceDiagram
    participant U as 用户
    participant E as Engine
    participant LLM as LLM
    participant T as Tool
    participant DB as MySQL
    participant PS as PendingStore

    Note over U,DB: 第一轮：查询预算可负担性
    U->>E: "买minimax会员6000元，预算够吗"
    E->>LLM: ChatSync
    LLM-->>E: ToolCall: check_budget_affordability{category:"教育", planned_amount:6000}
    E->>T: Execute
    T->>DB: 查预算执行情况
    T-->>E: "教育预算¥7000，已花¥3877.50，剩余¥3122.50，购买¥6000会超支¥2877.50，建议调整到¥10000"
    E->>LLM: ChatSync(tool_result)
    LLM-->>E: 转述 Tool message
    E-->>U: 展示结果 + 确认按钮

    Note over U,DB: 第二轮：用户调整预算
    U->>E: "调整到10000"
    E->>LLM: ChatSync
    LLM-->>E: ToolCall: update_budget{budget_id:12, amount:10000}
    E->>T: Execute
    T->>DB: UPDATE budgets
    T->>DB: 查本月已花金额
    T-->>E: "教育预算已调整为¥10000/月。本月已花¥3877.50，剩余¥6122.50"
    E->>E: validateResponse（containsUnverifiedAmount 校验）
    E-->>U: 展示 Tool message（不让 LLM 自己算）
```

## 三、need_confirm + PendingAction 确认流程

用户记大额消费触发金额异常检测，确认后通过 PendingAction 快速执行。

```mermaid
sequenceDiagram
    participant U as 用户
    participant E as Engine
    participant LLM as LLM
    participant T as Tool
    participant PS as PendingStore

    Note over U,PS: 第一轮：触发金额异常
    U->>E: "读书app会员600元"
    E->>LLM: ChatSync
    LLM-->>E: ToolCall: create_bill{amount:600, category:"教育"}
    E->>T: Execute
    T->>T: guardAmountAnomaly(600, 历史最高49.50) → 超过3倍
    T-->>E: ToolResult{status:"need_confirm", message:"¥600超过历史最高3倍，确认？"}

    E->>PS: Set(userID, sessionID, "create_bill", {amount:600, category:"教育", ...})
    E-->>U: 展示确认面板（确认/取消按钮）

    Note over U,PS: 第二轮：用户点击确认
    U->>E: "确认"（点击确认按钮）
    E->>E: isConfirmMessage("确认") → true
    E->>PS: Get(userID, sessionID)
    PS-->>E: PendingAction{tool:"create_bill", params:{...}}

    Note over E: 跳过 LLM，直接执行 Tool
    E->>T: Execute(create_bill, params)
    T->>T: isConfirmation(ctx) → true，跳过 guard
    T-->>E: ToolResult{status:"success"}
    E-->>U: "已记录..." （<1s，无 LLM 延迟）
```

## 四、对话历史脱敏注入

展示 Engine 如何构建注入 LLM 的消息列表。

```mermaid
sequenceDiagram
    participant Redis as Redis
    participant E as Engine
    participant LLM as LLM

    E->>Redis: GetRecentConversation(userID, sessionID, 10)
    Redis-->>E: 10条历史消息（完整版）

    Note over E: buildHistoryMessages 分层处理
    E->>E: 第1-4条（远期）：assistant 消息正则脱敏
    Note right of E: "已记录午饭¥30" → "已记录午饭¥***"
    Note right of E: "本月总支出¥1234" → "本月总支出¥***"
    Note right of E: 追问消息（含?）不脱敏
    Note right of E: user 消息永不脱敏

    E->>E: 第5-10条（近期3轮）：完整保留

    E->>LLM: [system, 脱敏历史..., 完整近期..., 当前user]
    Note over LLM: LLM 看不到远期金额<br/>无法从历史引用数据<br/>必须调 Tool 查询
```

## 五、故障降级流程

```mermaid
sequenceDiagram
    participant U as 用户
    participant E as Engine
    participant LLM as LLM
    participant Milvus as Milvus
    participant Redis as Redis

    Note over E: 场景1：LLM 超时
    E->>LLM: ChatSync (30s timeout)
    LLM--xE: context deadline exceeded
    E-->>U: "AI 响应超时了，请稍后再试"

    Note over E: 场景2：Milvus 不可用
    E->>Milvus: Search(embedding)
    Milvus--xE: connection refused
    E->>E: 降级到 MySQL keyword search
    E-->>U: 正常回复（记忆检索降级但不影响对话）

    Note over E: 场景3：ReAct 中间步骤 LLM 失败
    E->>LLM: ChatSync (step 2)
    LLM--xE: error
    E->>E: 用已有 Tool 结果兜底
    E-->>U: 展示 Tool message（部分结果）

    Note over E: 场景4：Redis 不可用
    E->>Redis: GetRecentConversation
    Redis--xE: connection refused
    E->>E: 跳过历史注入，继续对话
    E-->>U: 正常回复（无上下文但不报错）
```

## 六、总预算约束校验流程

用户创建分类预算时，Engine 校验分类预算之和不超过总预算。

```mermaid
sequenceDiagram
    participant U as 用户
    participant E as Engine
    participant LLM as LLM
    participant T as Tool
    participant DB as MySQL

    U->>E: "设置购物预算3000元"
    E->>LLM: ChatSync
    LLM-->>E: ToolCall: create_budget{categories:["购物"], amount:3000}

    E->>T: Execute(create_budget)
    T->>T: checkBudgetConstraint(userID, 0, 3000)
    T->>DB: 查所有预算
    DB-->>T: 总预算¥15000, 分类预算合计¥11360

    Note over T: 11360 + 3000 = 14360 ≤ 15000 ✓
    T->>DB: INSERT budgets
    T-->>E: ToolResult{status:"success"}
    E-->>U: "已创建购物预算 ¥3000/月"

    Note over U,DB: 场景2：超出总预算
    U->>E: "设置旅行预算5000元"
    E->>LLM: ChatSync
    LLM-->>E: ToolCall: create_budget{categories:["旅行"], amount:5000}

    E->>T: Execute(create_budget)
    T->>T: checkBudgetConstraint(userID, 0, 5000)
    T->>DB: 查所有预算
    DB-->>T: 总预算¥15000, 分类预算合计¥14360

    Note over T: 14360 + 5000 = 19360 > 15000 ✗
    T-->>E: ToolResult{status:"need_confirm", message:"会超出总预算..."}
    E-->>U: "分类预算总额¥19360超过总预算¥15000，请先调整总预算"
```

## 七、RAG 消费模式预填流程

用户说"星巴克"，Engine 从 L5 消费模式中检索历史消费习惯，按置信度决定交互方式。

```mermaid
sequenceDiagram
    participant U as 用户
    participant E as Engine
    participant LLM as LLM
    participant T as Tool
    participant RAG as RAGFiller
    participant DB as MySQL

    U->>E: "星巴克"
    E->>LLM: ChatSync
    LLM-->>E: ToolCall: create_bill{amount:0, category:"食饮", merchant:"星巴克"}

    E->>T: Execute(create_bill)
    T->>T: amountFoundInContext(0, "星巴克") → false

    T->>RAG: FillParams(userID, "星巴克", "食饮", 0)
    RAG->>DB: FindByMerchant(userID, "星巴克")
    DB-->>RAG: Pattern{avg:38, count:5, score:2.5}

    Note over RAG: 置信度 = 0.9（count≥5, score≥2.0）→ 高置信度

    alt 高置信度 ≥ 0.8
        RAG-->>T: RAGResult{amount:38, confidence:0.9}
        T->>T: 用 RAG 金额替换，继续执行
        T->>DB: INSERT bills (amount=38)
        T-->>E: "已记录：星巴克 ¥38.00（根据消费习惯预填）"
    else 中置信度 0.5~0.8
        RAG-->>T: RAGResult{amount:38, confidence:0.6}
        T-->>E: need_confirm "星巴克 ¥38.00？（根据消费记录推测）"
    else 低置信度 < 0.5
        RAG-->>T: nil
        T-->>E: need_confirm "这笔消费金额是多少呢？"
    end
```
