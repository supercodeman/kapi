# 咔皮记账 AI 助手 — 容量估算与性能方案

## 一、用户规模假设

| 阶段 | DAU | 峰值并发对话 | 日均对话轮次/人 |
|------|-----|------------|---------------|
| Prototype | 1~5 | 2 | 20 |
| 内测 | 50~200 | 20 | 15 |
| 公测 | 1000~5000 | 200 | 10 |

## 二、各组件容量估算

### MySQL

| 表 | 单用户年增量 | 5000 用户年总量 | 索引策略 |
|----|-----------|---------------|---------|
| bills | ~3000 行 | 1500 万 | user_id + date 联合索引 |
| budgets | ~20 行 | 10 万 | user_id 索引 |
| assets | ~10 行 | 5 万 | user_id 索引 |
| memories | ~500 行 | 250 万 | user_id + layer 联合索引 |
| operation_logs | ~3000 行 | 1500 万 | user_id + created_at |
| consumption_patterns | ~50 行 | 25 万 | user_id + merchant + category |

结论：单 MySQL 实例（8C16G）可支撑 5000 用户，无需分库分表。

### Redis

| 用途 | 单用户内存 | 5000 用户总量 | TTL |
|------|----------|-------------|-----|
| 对话历史（L4） | ~50KB（50轮） | 250MB | 7 天 |
| 分析缓存 | ~5KB | 25MB | 25 小时 |
| Session | ~1KB | 5MB | 7 天 |

结论：总内存 < 300MB，单 Redis 实例（1G）足够。

### Milvus

| Collection | 向量维度 | 单用户向量数 | 5000 用户总量 |
|-----------|---------|-----------|-------------|
| memories_vectors | 1024 | ~500 | 250 万 |

结论：单 Milvus 实例可支撑，IVF_FLAT 索引，nlist=128。

### LLM API

| 场景 | LLM 调用次数 | 预期延迟 | Token 消耗/次 |
|------|-----------|---------|-------------|
| 单步记账 | 2 次 | 3~5s | ~1500 |
| 查询统计 | 2 次 | 3~5s | ~2000 |
| ReAct 2步 | 3 次 | 7~10s | ~3000 |
| PendingAction 确认 | 0 次 | <1s | 0 |

日均 Token 消耗（5000 DAU × 10 轮 × 2000 token）= 1 亿 token/天

## 三、性能目标

| 指标 | 目标 | 实测（2026-05-06） | 说明 |
|------|------|---------|------|
| 单步记账 P95 | ≤ 12s | ~6.9s | 写操作快速返回，跳过第二次 LLM |
| 查询统计 P95 | ≤ 12s | ~6.1s | 查询快速返回，直接用 Tool Message |
| PendingAction 确认 P95 | ≤ 500ms | ~26ms | 跳过 LLM，直接执行 Tool |
| 首字延迟（闲聊） | ≤ 5s | ~1.3s | 真流式 + thinking 实时展示 |
| 首字延迟（操作） | ≤ 5s | ~0ms（thinking 事件） | thinking 事件即时推送 |
| 30 路并发 P95 | ≤ 12s | 6.7s | 信号量 max_concurrent=20 |
| 30 路并发成功率 | 100% | 100% | 无 OOM/panic/超时 |
| 端到端 P95（混合场景） | ≤ 12s | 10.8s | 8 种场景混合测试 |

### 优化措施（已实施）

1. **并行预加载**（preloadContext）
   - L1/L2/L3 记忆 + L4 对话历史 + L5 RAG 提示并行加载
   - 共享 Embedding：同一 query 只调一次 Embed API
   - pre-LLM 阶段从 85-480ms 降到 30-210ms

2. **写操作快速返回**
   - Tool 执行成功后直接用 Tool Message 回复，跳过第二次 LLM
   - 记账场景从 12s 降到 5-7s

3. **查询快速返回**
   - 已知查询 Tool（list_bills/get_category_summary 等）直接用 Message 回复
   - 查询场景从 15-19s 降到 3-7s

4. **真流式输出**（闲聊场景）
   - LLM 首 token 即时推送，thinking 内容实时展示
   - 首字从 5-7s 降到 ~1s

5. **LLM 并发控制**（信号量）
   - max_concurrent=20，防止 API rate limit 和 OOM
   - 超出排队等待，ctx 超时后降级响应

6. **历史污染防护**
   - 失败回复不存入对话历史
   - 加载历史时过滤失败记录
   - 减少 LLM 被历史"带偏"的概率

7. **页面感知 Tool 过滤**
   - 按页面裁剪 Tool 定义，减少 prompt token 数
   - 降低 LLM 调错 Tool 的概率

## 四、部署资源估算

### Prototype 阶段（当前）

| 组件 | 规格 | 说明 |
|------|------|------|
| Go Server | 1C2G | 单实例 |
| MySQL | 1C2G | 单实例 |
| Redis | 512MB | 单实例 |
| Milvus | 2C4G | 单实例（standalone） |

### 公测阶段

| 组件 | 规格 | 说明 |
|------|------|------|
| Go Server | 2C4G × 2 | Nginx 负载均衡 |
| MySQL | 4C8G | 主从复制 |
| Redis | 2G | 单实例（Sentinel 备份） |
| Milvus | 4C8G | 单实例 |
