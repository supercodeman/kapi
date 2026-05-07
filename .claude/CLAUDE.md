# 咔皮记账 AI 助手 — 项目级 Agent 指令

## 项目概述
咔皮记账 App 全场景内嵌式 AI 助手，覆盖 6 个核心页面（首页、记一笔、账单详情、预算、报表、资产管理）。

## 技术栈
- 服务端：Go 1.22 / Gin / GORM
- 前端：Vue 3
- 存储：MySQL 8.0 / Redis 7 / Milvus
- 部署：Docker Compose

## 架构约束
- 单体 + 插件化架构，子系统通过 Go interface 解耦
- 双通道：HTTP REST API（业务 CRUD）+ WebSocket（AI 对话）
- 所有数据查询必须带 `WHERE user_id = ?`，严格多租户隔离
- LLM 不做数值计算，只做推理和表达（零幻觉原则）

## 编码规范
- 遵循 SOLID、DRY、KISS、YAGNI 原则
- 单个文件不超过 500 行，超过则按职责拆分
- 中文注释，英文代码和错误信息
- 分层架构：Router → Handler → Service → DAO → Model
- 统一响应格式：`{"code": 0, "message": "success", "data": {...}}`
- context 传递 user_id，key 为 "user_id"

## 安全规则
- SQL 注入防护：排序字段白名单校验，参数化查询
- 输入验证：所有外部输入在 Handler 层校验
- 密码存储：bcrypt 哈希，不可逆
- JWT：签名校验 + 过期检查

## 关键设计文档
- 业务背景：`docs/application.md`
- 设计决策：`docs/design-decisions.md`
- 完整规格：`docs/superpowers/specs/2026-04-29-kapi-ai-assistant-design.md`
- 架构图：`docs/architecture.html`
- 意图识别：`docs/intent-recognition.html`
- 实施计划：`docs/superpowers/plans/overview.md`

## 问题排查规范（强制）
- 修改任何逻辑时，必须排查该逻辑的所有调用链路（同步端点 engine.go + 流式端点 stream.go + 其他入口），确认一致性
- 不允许只改一处而遗漏另一处相同逻辑的入口
- 修改 PendingAction、Tool 执行、Guard 校验等核心流程时，必须同时检查 engine.go 和 stream.go 两个文件
- 每次修复后，列出所有受影响的代码路径，逐一确认已同步
- 禁止反复打补丁：同一个问题修两次不生效时，必须停下来从根因分析，画出完整调用链再动手

## AI Engine 核心流程（双入口）
- 同步端点：`engine.go` → `Chat()` → `executeWorkflow()`
- 流式端点：`stream.go` → `ChatStream()` → `executeWorkflowStream()`
- 两者共享：`executePendingAction()` / `executeBatchPendingAction()` / `executeSingleTool()`
- **任何对 Tool 执行、PendingAction 存储、Guard 校验的修改，必须同时更新两个 Workflow 函数**

## 写操作确认机制
- 所有写操作（记账、删除、修改）必须经过用户确认，不允许静默执行
- 确认流程：Tool 返回 `status: "need_confirm"` → Engine 存 PendingAction → 用户确认 → `executePendingAction` 执行
- 批量操作：`BatchParams []map[string]any` 存储多笔，确认后 `executeBatchPendingAction` 逐笔调 create_bill
- `isConfirmation(ctx)=true` 时跳过所有 Guard（日期异常、金额异常、RAG 预填）

## 金额计算优先级
1. Engine 正则计算（`calcTotalFromQuantity`）— 同商品×数量，100% 准确
2. LM 计算 + Engine 数学校验（`amountFoundInContext`）— 直接匹配 → 乘积匹配 → 求和匹配 → 上一条消息
3. RAG 预填 — 置信度 ≥0.5 时返回确认，用户确认后执行
4. 追问 — 以上都不满足时追问金额

## 构建与测试
- 构建：`/opt/homebrew/opt/go@1.22/bin/go build -buildvcs=false -o kapi_server ./cmd/server/`
- 单元测试：`/opt/homebrew/opt/go@1.22/bin/go test ./internal/engine/`
- 前端：Vite dev server 自动热更新，改 Vue 文件不需要重启
- 后端：改 Go 文件需要重新编译并重启 kapi_server

## 文件职责
| 文件 | 职责 |
|------|------|
| `cmd/server/main.go` | 初始化、依赖注入、启动（~220行） |
| `cmd/server/tools.go` | 扩展 Tool 注册（Analytics/Budget/Batch/Pattern） |
| `internal/engine/engine.go` | 同步 Chat 主流程、PendingAction 执行 |
| `internal/engine/stream.go` | 流式 Chat 主流程（逻辑必须与 engine.go 一致） |
| `internal/engine/tools.go` | 核心 Tool（create_bill/list_bills/delete_bill 等） |
| `internal/engine/guard.go` | 业务规则校验（日期异常、金额异常、PendingAction 判断） |
| `internal/engine/rag.go` | L5 消费模式 RAG 预填 |
| `internal/engine/quantity.go` | 单价×数量 / 多商品求和计算 |
| `internal/engine/validate.go` | 金额来源校验（amountFoundInContext） |
| `internal/engine/prompt.go` | System Prompt 模板 |
| `web/src/components/ChatWindow.vue` | 前端聊天组件 |

## 子系统 interface 边界
| 子系统 | 关键 interface |
|--------|----------------|
| Auth Middleware | Authenticator |
| Connection Manager | Connector |
| AI Engine | LLMProvider, ToolExecutor |
| Skill Manager | SkillRegistry, SkillLoader |
| Memory Manager | MemoryStore, MemoryRetriever |
| Analytics Engine | Analyzer, Predictor |
| Operation Logger | OpLogger, OpRestorer |
