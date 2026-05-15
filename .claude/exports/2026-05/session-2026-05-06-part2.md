# 咔皮记账 AI 助手 — 开发沟通记录

> 日期：2026-05-06（续）
> 接续：session-2026-05-06.md
> 主题：首字延迟优化 + 并发控制 + 稳定性修复

---

## 一、首字 5 秒内响应优化

### 1.1 现状分析

前端已用 SSE 流式（`/api/chat/stream`），`ChatStream()` 已在使用，thinking 事件即时发送。真正的瓶颈：
- 记忆加载串行（L2/L3 各调一次 Embedding API）
- LLM 调用用 `ChatSync` 阻塞（等完整响应后才 `streamText` 模拟分块）

### 1.2 实施方案

**Step 1：并行预加载 + 共享 Embedding**
- `memory/manager.go` 新增 `SearchFactsAndEpisodes()`：单次 Embed + 并行 Milvus 搜索
- `engine.go` 新增 `preloadContext()`：errgroup 并行加载 L1+L2+L3 || L4 || L5
- 双路径一致性：`Chat()` 和 `ChatStream()` 共用 `preloadContext()`

**Step 2：首次 LLM 调用真流式**
- 闲聊场景（`mayNeedTool=false`）：用 `e.llm.Chat()` 流式，边生成边推送
- 操作场景（`mayNeedTool=true`）：保持 `ChatSync` 确保 Tool Call 不丢失
- thinking 标签状态机：过滤 `<think>...</think>`，只推送最终回复

### 1.3 thinking 内容展示

MiniMax M2.7 强制输出 `<think>` 标签（无法关闭），thinking 过程 5-7 秒。
方案：将 thinking 内容作为 `type:"thinking"` 事件实时推送，前端用独立样式展示。
- 前端 `api/index.js` 新增 `onThinking` 回调
- `ChatWindow.vue` 新增 `.thinking-live` 区域
- `chat.js` store 新增 `appendThinking` 方法

### 1.4 超时问题修复

流式调用用了 30s 的 `withLLMTimeout`，但 MiniMax thinking + 回复经常超过 30s。
修复：流式路径超时改为 120s。

---

## 二、写操作跳过第二次 LLM

### 2.1 问题

记账场景需要 2 次 LLM 调用（意图识别 + 最终回复），总耗时 12s+。
但 Tool 已经返回了完整的 Message（如"已记录支出：2026-05-06 午饭 ¥30.00 分类食饮"）。

### 2.2 方案

Tool 执行成功且全是写操作时，直接用 `buildToolMessageReply` 回复，跳过第二次 LLM。
- `engine.go` 和 `stream.go` 中 `executeWorkflow` 加入 `hasWriteToolResult && allToolsSucceeded` 判断
- 记账场景从 12s 降到 5-7s

---

## 三、查询操作跳过第二次 LLM

### 3.1 方案

查询类 Tool（`list_bills`、`get_category_summary` 等）已有完善的 Message 生成逻辑。
直接用 Tool Message 回复，跳过第二次 LLM。
- 新增 `canFastReturnRead()` 判断：只对 6 个已知查询 Tool 生效
- 查询场景从 15-19s 降到 3-7s

### 3.2 注意

不是所有 Tool 都走快速返回。`get_operation_history` 等非标准查询仍走 ReAct 循环。

---

## 四、LLM 不调 Tool 的偶发问题

### 4.1 根因

session_id=1 中对话历史包含大量"已记录"的回复，LLM 被历史"带偏"，有时不调 Tool 而是自己模仿回复。

### 4.2 修复措施

1. **历史污染防护**：失败回复（"操作未能完成"）不存入对话历史
2. **历史加载过滤**：`buildHistoryMessages` 跳过失败回复及对应的 user 消息
3. **历史条数减少**：从 10 条降到 6 条
4. **二级重试**：第一次带历史重试失败后，清除历史做干净重试
5. **追问保护机制**：LLM 自己追问时主动存 PendingAction，后续短回答能正确执行
6. **`isFollowUpAnswer` 检测**：上一轮是追问 + 当前是短回答 → 强制重试调 Tool
7. **Prompt 强化**：新增第 7 条规则 — 不允许自行判断"重复"
8. **`containsOperationClaim` 扩展**：检测"已存在"、"已经有了"等自行去重表述

---

## 五、分类匹配 Bug

### 5.1 问题

"红包"被记录为"购物（服饰）"。Go map 遍历顺序随机，"包"先于"红包"被匹配。

### 5.2 修复

`NormalizeCategoryV2` 的关键词匹配改为优先匹配最长关键词：
```go
var bestMatch CategoryResult
bestMatchLen := 0
for keyword, result := range keywordCategoryMap {
    if strings.Contains(searchText, keyword) && len(keyword) > bestMatchLen {
        bestMatch = result
        bestMatchLen = len(keyword)
    }
}
```

---

## 六、`messageNeedsTool` 关键词优化

### 6.1 问题

单字关键词误伤严重："记"匹配"记得"，"能"匹配"你能做什么"，"资产"匹配闲聊。

### 6.2 修复

- 去掉单字关键词，改为多字精确匹配
- 新增金额检测：消息中包含数字（`ParseChineseNumber`）→ 大概率是记账意图
- `handleNoToolCall` 中的关键词列表统一复用 `messageNeedsTool()`

---

## 七、并发控制

### 7.1 方案

LLM 调用信号量（semaphore），限制同时进行的 LLM 请求数。

### 7.2 实现

- `Engine` 新增 `llmSem chan struct{}`（容量 20）
- 新增 `llmChatSync()` 和 `llmChatStream()` 封装方法
- 所有 LLM 调用点替换为封装方法
- 流式调用在 channel 关闭后才释放信号量
- 排队时 ctx 超时自动放弃

### 7.3 配置

`config.yaml` 新增 `llm.max_concurrent: 20`

### 7.4 验证

30 路并发测试：100% 成功，P50=4.2s，P95=6.7s，总耗时 29.1s。

---

## 八、页面感知 Tool 过滤

### 8.1 问题

所有 25 个 Tool 始终对 LLM 可见，增加 prompt token 数和调错 Tool 概率。

### 8.2 实现

- `skill/registry.go` 新增 `GetToolsForPage(page)`：收集页面关联 Tool 名称
- `tools.go` 新增 `GetToolDefinitionsForPage(pageTools)`：按白名单过滤
- `engine.go` 和 `stream.go` 调用点改为按页面过滤

---

## 九、架构文档补全

新增 `docs/architecture-components.md`：
- 整体组件图（ASCII）
- 关键时序图（记账/查询/闲聊）
- 容量估算表
- 故障预案表
- 多租户隔离说明

更新 `docs/capacity-planning.md`：
- 性能目标表更新为实测数据
- 新增"优化措施（已实施）"章节

---

## 十、最终验证结果

| 目标 | 状态 | 数据 |
|------|------|------|
| 30-100 路并发 | ✅ | 30 路 100% 成功，P95=6.7s |
| 首字 P95 ≤ 5s | ✅ | 闲聊 ~1.3s，操作 thinking 即时 |
| 端到端 P95 ≤ 12s | ✅ | 混合 P95=10.8s，并发 P95=6.7s |
| 用户独立 Memory | ✅ | L1-L5 完整，L6 表存在 |
| Skill 热更新 | ✅ | fsnotify + DB 轮询 |
| 页面感知 | ✅ | Skill + Tool 按页面过滤 |
| 多租户隔离 | ✅ | 全链路 user_id 隔离 |
| 可运行原型 | ✅ | Docker Compose + 6 页面 |
| 架构文档 | ✅ | 组件图+时序图+容量+故障预案 |

---

## 改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/memory/manager.go` | 新增 `SearchFactsAndEpisodes` |
| `internal/engine/engine.go` | `preloadContext`、`llmChatSync`/`llmChatStream`、`messageNeedsTool`、`isFollowUpAnswer`、追问保护、二级重试、历史污染防护 |
| `internal/engine/stream.go` | 真流式 + thinking 过滤 + 写操作/查询快速返回 + 页面 Tool 过滤 |
| `internal/engine/validate.go` | `allToolsSucceeded`、`buildReadToolMessageReply`、`canFastReturnRead`、`containsOperationClaim` 扩展 |
| `internal/engine/normalize.go` | 关键词匹配改为最长优先 |
| `internal/engine/history.go` | 失败回复过滤 |
| `internal/engine/prompt.go` | 新增第 7 条规则（禁止自行去重） |
| `internal/engine/tools.go` | `GetToolDefinitionsForPage` |
| `internal/skill/registry.go` | `GetToolsForPage` |
| `internal/config/config.go` | `MaxConcurrent` 配置 |
| `cmd/server/main.go` | 传入 `MaxConcurrent` |
| `config.yaml` | `max_concurrent: 20` |
| `web/src/api/index.js` | `onThinking` 回调 |
| `web/src/components/ChatWindow.vue` | thinking 实时展示 |
| `web/src/stores/chat.js` | `appendThinking` |
| `docs/architecture-components.md` | 新增 |
| `docs/capacity-planning.md` | 更新性能数据 |
