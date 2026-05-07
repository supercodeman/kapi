# 咔皮记账 AI 助手 — 故障预案手册

> 版本：v1.0 | 更新日期：2026-04-30 | 适用环境：Go 1.22 / Gin / GORM / MySQL 8.0 / Redis 7 / Milvus / MiniMax LLM API

---

## 目录

1. [故障分类总览](#1-故障分类总览)
2. [LLM API 故障](#2-llm-api-故障)
3. [MySQL 故障](#3-mysql-故障)
4. [Redis 故障](#4-redis-故障)
5. [Milvus 故障](#5-milvus-故障)
6. [Go Server 故障](#6-go-server-故障)
7. [监控指标与告警阈值](#7-监控指标与告警阈值)
8. [应急联系与升级流程](#8-应急联系与升级流程)
9. [定期演练清单](#9-定期演练清单)

---

## 1. 故障分类总览

| 组件 | 故障等级 | 自动降级能力 | 对用户的影响 |
|------|---------|-------------|-------------|
| LLM API | P1 | 有（超时兜底 + Tool 结果兜底） | AI 对话不可用，CRUD 操作不受影响 |
| MySQL | P0 | 无 | 全站不可用 |
| Redis | P2 | 有（跳过历史注入 + 缓存穿透到 MySQL） | 对话丢失上下文，分析查询变慢 |
| Milvus | P3 | 有（降级到 MySQL keyword search） | 记忆检索精度下降，功能不中断 |
| Go Server | P0 | 依赖部署方式（多实例可自动摘除） | 服务完全不可用 |

---

## 2. LLM API 故障

### 2.1 场景一：LLM 调用超时（>30s）

**故障现象**
- 用户发送消息后长时间无响应，最终收到"AI 响应超时了，请稍后再试，或者简化一下你的问题。"
- 服务端日志出现 `context deadline exceeded` 或 `timeout`

**影响范围**
- AI 对话功能不可用
- 所有依赖 LLM 的意图识别、参数提取、自然语言回复均受影响
- REST API（账单/预算/资产 CRUD）不受影响

**自动降级行为**
- `internal/engine/fallback.go` 中 `withLLMTimeout` 设置 30s 硬超时
- 超时后 `llmFallbackResponse` 返回友好提示，不会阻塞用户
- 如果是 ReAct 多步调用中间步骤超时，且已有 Tool 执行结果，则用已有结果拼接回复（`engine.go:246-248`）

**人工介入步骤**
1. 检查 LLM API 提供商状态页（MiniMax 控制台）
2. 检查网络连通性：`curl -v <LLM_BASE_URL>/v1/models`
3. 检查 API Key 是否过期或额度耗尽
4. 如果是提供商侧故障，考虑切换备用 LLM 端点（修改 `LLM_BASE_URL` 环境变量后重启）

**恢复确认方法**
- 发送测试对话消息，确认 AI 正常回复
- 检查日志中 `DurationMs` 恢复到正常范围（通常 2-10s）

### 2.2 场景二：LLM API 完全不可用

**故障现象**
- 用户收到"AI 服务暂时不可用，请稍后再试。"
- 日志中出现非超时类 LLM 错误（如 HTTP 5xx、连接拒绝）

**影响范围**
- 同 2.1

**自动降级行为**
- `llmFallbackResponse` 对非超时错误返回通用不可用提示
- 如果 LLM Provider 未配置（`e.llm == nil`），直接返回"AI 服务尚未配置"

**人工介入步骤**
1. 同 2.1 的排查步骤
2. 如果是 API Key 问题，更新 `LLM_API_KEY` 环境变量
3. 如果是模型下线，更新 `LLM_MODEL` 环境变量

### 2.3 场景三：ReAct 中间步骤 LLM 失败

**故障现象**
- 用户查询涉及多步操作（如"帮我查本月消费并和上月对比"），部分结果返回但不完整

**自动降级行为**
- `executeWorkflow` 中 ReAct 循环的中间步骤 LLM 失败时，如果已有 Tool 执行结果，用 `buildFromToolMessages` 拼接已有结果返回
- 达到最大步数（5 步）时同样用已有结果兜底

---

## 3. MySQL 故障

### 3.1 场景一：MySQL 连接失败

**故障现象**
- 服务启动时直接 Fatal：`failed to connect mysql`
- 运行中出现大量 GORM 错误日志，API 返回 500

**影响范围**
- **全站不可用**。MySQL 是核心存储，所有业务数据（账单、预算、资产、用户、记忆、操作日志、技能、消费模式）均依赖 MySQL
- AI 对话中的 Tool 调用全部失败
- 无自动降级，属于 P0 故障

**自动降级行为**
- 无。MySQL 是不可替代的核心依赖
- 连接池配置：MaxIdleConns=10, MaxOpenConns=100, ConnMaxLifetime=1h

**人工介入步骤**
1. 检查 MySQL 进程状态：`systemctl status mysql` 或 `docker ps | grep mysql`
2. 检查连接数是否耗尽：`SHOW PROCESSLIST;` / `SHOW STATUS LIKE 'Threads_connected';`
3. 检查磁盘空间：`df -h`（MySQL 磁盘满会导致写入失败）
4. 检查慢查询日志：`SHOW VARIABLES LIKE 'slow_query_log_file';`
5. 如果是连接数耗尽，考虑 kill 空闲连接或调整 `max_connections`
6. 如果是磁盘满，清理 binlog：`PURGE BINARY LOGS BEFORE DATE_SUB(NOW(), INTERVAL 7 DAY);`

**恢复确认方法**
- 访问 `/health` 端点确认返回 `{"status": "ok"}`
- 执行一次账单查询确认数据读写正常

### 3.2 场景二：MySQL 慢查询导致超时

**故障现象**
- API 响应变慢，部分请求超时
- 日志中出现 GORM 超时错误

**人工介入步骤**
1. 检查慢查询日志，定位问题 SQL
2. 检查是否缺少索引（特别是 `user_id` 相关的查询）
3. 检查表锁情况：`SHOW ENGINE INNODB STATUS;`

---

## 4. Redis 故障

### 4.1 场景一：Redis 连接失败

**故障现象**
- 服务启动时 Fatal：`failed to connect redis`
- 运行中 Redis 操作报错，但服务不会崩溃

**影响范围**
- 对话历史（L4 层）丢失：用户新对话无法获取之前的上下文
- 分析缓存失效：所有分析查询（分类汇总、月度对比、预算执行）直接穿透到 MySQL
- 缓存失效机制（`InvalidateUser`）不工作，但不影响数据正确性

**自动降级行为**
- 对话历史获取失败时，`stream.go:73-76` 中 `GetRecentConversation` 返回 error 后跳过历史注入，AI 继续对话但无上下文
- 分析缓存层（`analytics/cache.go`）Redis Get 失败时自动穿透到 MySQL 查询
- 对话消息写入 Redis 失败时仅打印日志，不影响当前回复

**人工介入步骤**
1. 检查 Redis 进程：`redis-cli ping` 或 `docker ps | grep redis`
2. 检查内存使用：`redis-cli info memory`
3. 如果 OOM，检查 `maxmemory` 配置和淘汰策略
4. 检查连接数：`redis-cli info clients`
5. 重启 Redis 后，对话历史会丢失（Redis 中的 L4 数据是临时的，TTL 7 天）

**恢复确认方法**
- `redis-cli ping` 返回 PONG
- 发送一条对话消息，再发第二条，确认第二条能引用第一条的上下文

### 4.2 场景二：Redis 内存耗尽

**故障现象**
- Redis 写入操作报 OOM 错误
- 对话历史无法保存，分析缓存无法写入

**自动降级行为**
- 同 4.1，读操作可能仍然正常，写入失败不影响核心功能

**人工介入步骤**
1. `redis-cli info memory` 查看内存使用
2. 检查是否有异常大的 key：`redis-cli --bigkeys`
3. 考虑设置 `maxmemory-policy allkeys-lru` 自动淘汰
4. 对话历史 key 格式为 `session:{user_id}:{session_id}`，TTL 7 天，正常情况下不会无限增长
5. 分析缓存 key 格式为 `cache:*:{user_id}:*`，TTL 25 小时

---

## 5. Milvus 故障

### 5.1 场景一：Milvus 连接失败

**故障现象**
- 启动日志出现 `warning: failed to connect milvus`
- 记忆检索走 MySQL keyword search，精度下降但功能正常

**影响范围**
- L2 事实记忆和 L3 情景记忆的语义检索降级为 MySQL 关键词搜索
- 新记忆的 Embedding 无法写入 Milvus（`embedding_id` 保持为空）
- 不影响 L1 用户画像（纯 MySQL）和 L4 对话历史（纯 Redis）

**自动降级行为**
- `main.go:88-98`：Milvus 连接失败时 `milvusClient` 为 nil，服务正常启动
- `memory/manager.go:75-77`：`SearchFacts` 检测到 `m.milvus == nil` 时直接走 `SearchByContent`（MySQL LIKE 查询）
- `memory/manager.go:106-107`：`SearchEpisodes` 同理
- `memory/manager.go:162-164`：`embedSync` 检测到 `m.milvus == nil` 时直接 return，不报错
- Embedding 生成失败时（`embedding.Embed` 返回 error），同样降级到 MySQL keyword search（`manager.go:80-82`）

### 5.2 场景二：Milvus 搜索失败

**故障现象**
- Milvus 已连接但搜索报错（如 collection 未加载、索引损坏）
- 日志出现 `milvus search failed, falling back to keyword search`

**自动降级行为**
- `SearchFacts` 和 `SearchEpisodes` 中 Milvus Search 返回 error 时，自动降级到 MySQL keyword search

**人工介入步骤**
1. 检查 Milvus 进程：`docker ps | grep milvus`
2. 检查 collection 状态：通过 Milvus CLI 或 Attu 管理界面
3. 确认 collection `memories_vectors` 是否已加载：`show collections`
4. 如果 collection 损坏，重启 Milvus 后服务会自动调用 `EnsureCollection` 重建
5. 重建后需要运行 Embedding 补偿：重启 Go Server 会自动触发 `BackfillEmbeddings`

**恢复确认方法**
- 重启 Go Server，观察日志中是否出现 `Milvus connected` 和 `backfill: completed`
- 发送一条涉及记忆检索的对话（如"我上次买咖啡花了多少"），确认能检索到相关记忆

### 5.3 场景三：Embedding 生成失败

**故障现象**
- 日志出现 `embedding failed, falling back to keyword search` 或 `sync embedding failed for memory`
- Embedding Provider 配置缺失或 API 不可用

**自动降级行为**
- 检索侧：降级到 MySQL keyword search
- 写入侧：记忆正常保存到 MySQL，但 `embedding_id` 为空，后续可通过 `BackfillEmbeddings` 补偿

**人工介入步骤**
1. 检查 Embedding 配置：`EMBEDDING_BASE_URL`、`EMBEDDING_API_KEY`、`EMBEDDING_MODEL` 是否完整
2. 测试 Embedding API 连通性
3. 恢复后重启服务，`BackfillEmbeddings` 会自动补偿历史记忆

---

## 6. Go Server 故障

### 6.1 场景一：进程崩溃（Panic）

**故障现象**
- 服务进程退出，端口不可达
- 如果有 Gin Recovery 中间件拦截，单次请求返回 500 但服务不退出

**影响范围**
- 全站不可用（如果进程退出）
- 单请求失败（如果被 Recovery 拦截）

**自动降级行为**
- Gin Recovery 中间件（`gin.Recovery()`）拦截 handler 层 panic，返回 500 但不崩溃
- `embedAsync` 中有 `recover()` 保护，异步 Embedding 的 panic 不会影响主流程

**人工介入步骤**
1. 检查进程状态：`ps aux | grep kapi` 或 `docker ps`
2. 检查崩溃日志，定位 panic 堆栈
3. 如果是 OOM，检查内存使用并调整容器限制
4. 重启服务：确保 MySQL 和 Redis 可用后再启动

**恢复确认方法**
- `/health` 返回 `{"status": "ok"}`
- 发送测试对话确认 AI 功能正常

### 6.2 场景二：Goroutine 泄漏

**故障现象**
- 内存持续增长，响应变慢
- `runtime.NumGoroutine()` 持续上升

**人工介入步骤**
1. 通过 pprof 分析 goroutine：`go tool pprof http://localhost:8080/debug/pprof/goroutine`
2. 重点关注：WebSocket 连接未正确关闭、`embedAsync` 的 goroutine 是否正常退出
3. 检查 `skillWatcher`（10s 轮询）是否正常

### 6.3 场景三：优雅关闭失败

**故障现象**
- 收到 SIGINT/SIGTERM 后服务未在 10s 内关闭
- 日志出现 `server forced to shutdown`

**自动降级行为**
- `main.go:455-460`：10s 优雅关闭超时后强制退出
- 关闭顺序：停止 SkillWatcher → HTTP Server Shutdown → 关闭 MySQL 连接池 → 关闭 Redis 连接

---

## 7. 监控指标与告警阈值

### 7.1 LLM API

| 指标 | 采集方式 | 告警阈值 | 说明 |
|------|---------|---------|------|
| LLM 调用延迟（P99） | 从 `ChatResponse.DurationMs` 采集 | > 15s 告警，> 25s 严重 | 接近 30s 超时阈值时需关注 |
| LLM 调用失败率 | 统计 `llmFallbackResponse` 触发次数 | > 5% 告警，> 20% 严重 | 区分超时和非超时错误 |
| LLM 超时率 | 统计 `isTimeoutError` 为 true 的比例 | > 10% 告警 | 可能需要调整超时时间或简化 prompt |
| ReAct 步数分布 | 统计每次对话的 Tool Call 轮次 | 平均 > 3 步告警 | 过多步数说明意图识别不准 |

### 7.2 MySQL

| 指标 | 采集方式 | 告警阈值 | 说明 |
|------|---------|---------|------|
| 连接池活跃连接数 | `sql.DB.Stats().InUse` | > 80（总量 100）告警 | 接近 MaxOpenConns 时需扩容 |
| 连接池等待数 | `sql.DB.Stats().WaitCount` | > 0 持续 1 分钟告警 | 说明连接池不够用 |
| 慢查询数量 | MySQL slow_query_log | > 10/min 告警 | 需要优化 SQL 或加索引 |
| 磁盘使用率 | `df -h` | > 80% 告警，> 90% 严重 | MySQL 磁盘满会导致写入失败 |
| 主从延迟 | `SHOW SLAVE STATUS` | > 5s 告警 | 如有主从架构 |

### 7.3 Redis

| 指标 | 采集方式 | 告警阈值 | 说明 |
|------|---------|---------|------|
| 内存使用率 | `redis-cli info memory` | > 80% maxmemory 告警 | 防止 OOM |
| 连接数 | `redis-cli info clients` | > 500 告警 | 默认 maxclients=10000 |
| 缓存命中率 | `keyspace_hits / (keyspace_hits + keyspace_misses)` | < 60% 告警 | 命中率过低说明缓存策略需调整 |
| Key 数量 | `redis-cli dbsize` | 异常增长告警 | 防止 key 泄漏 |
| 对话历史 key 数量 | `redis-cli keys "session:*" \| wc -l` | 监控趋势 | TTL 7 天，正常会自动过期 |

### 7.4 Milvus

| 指标 | 采集方式 | 告警阈值 | 说明 |
|------|---------|---------|------|
| 连接状态 | 启动日志 / 健康检查 | 连接失败告警 | 降级不影响功能但影响检索质量 |
| 搜索延迟 | 日志中 Milvus Search 耗时 | > 500ms 告警 | 正常应在 100ms 以内 |
| Collection 向量数 | Milvus 管理界面 | 监控趋势 | 向量数过多可能影响搜索性能 |
| Embedding 补偿队列 | `embedding_id` 为空的记忆数量 | > 100 告警 | 说明 Embedding 服务长时间不可用 |

### 7.5 Go Server

| 指标 | 采集方式 | 告警阈值 | 说明 |
|------|---------|---------|------|
| HTTP 5xx 率 | Gin 日志 / 反向代理日志 | > 1% 告警，> 5% 严重 | |
| 请求延迟（P99） | Gin 中间件 | > 5s 告警（非 AI 接口） | AI 对话接口延迟由 LLM 决定 |
| Goroutine 数量 | `runtime.NumGoroutine()` | > 1000 告警 | 防止 goroutine 泄漏 |
| 内存使用 | `runtime.MemStats` | > 1GB 告警 | 根据实际部署调整 |
| Health Check | `/health` 端点 | 连续 3 次失败告警 | |

---

## 8. 应急联系与升级流程

### 8.1 升级矩阵

| 故障等级 | 响应时间 | 处理人 | 升级条件 |
|---------|---------|--------|---------|
| P0（全站不可用） | 5 分钟内响应 | 值班运维 + 后端开发 | 15 分钟未恢复 → 升级至技术负责人 |
| P1（核心功能不可用） | 15 分钟内响应 | 值班运维 | 30 分钟未恢复 → 升级至后端开发 |
| P2（功能降级） | 30 分钟内响应 | 值班运维 | 2 小时未恢复 → 升级至后端开发 |
| P3（体验下降） | 工作时间处理 | 后端开发 | 24 小时未恢复 → 排入迭代计划 |

### 8.2 联系人模板

| 角色 | 姓名 | 联系方式 | 备注 |
|------|------|---------|------|
| 值班运维 | （填写） | （填写） | 7x24 |
| 后端开发 | （填写） | （填写） | 工作日 |
| 技术负责人 | （填写） | （填写） | P0 升级 |
| LLM 供应商支持 | MiniMax | （填写） | API 故障时联系 |
| 数据库 DBA | （填写） | （填写） | MySQL 故障时联系 |

### 8.3 故障处理流程

```
发现故障
  │
  ├─ 自动告警触发 / 用户反馈 / 巡检发现
  │
  ▼
判断故障等级（参考第 1 节分类表）
  │
  ▼
通知对应处理人
  │
  ▼
执行对应组件的排查步骤（参考第 2-6 节）
  │
  ├─ 恢复 → 执行恢复确认方法 → 记录故障报告
  │
  └─ 未恢复 → 按升级矩阵升级 → 继续排查
```

---

## 9. 定期演练清单

### 9.1 月度演练

| 序号 | 演练项目 | 操作方法 | 预期结果 | 负责人 |
|------|---------|---------|---------|--------|
| 1 | LLM API 超时降级 | 临时将 LLM_BASE_URL 指向一个慢响应端点 | 用户收到超时友好提示，服务不崩溃 | |
| 2 | Milvus 不可用降级 | 停止 Milvus 容器 | 记忆检索自动降级到 MySQL keyword search，日志有降级记录 | |
| 3 | Redis 不可用降级 | 停止 Redis 容器（注意：启动时会 Fatal，需在运行中模拟） | 对话继续但无历史上下文，分析查询穿透到 MySQL | |
| 4 | Embedding 不可用降级 | 临时清空 EMBEDDING_API_KEY | 新记忆正常保存但无向量，检索降级到关键词搜索 | |
| 5 | 优雅关闭 | 发送 SIGTERM 信号 | 服务在 10s 内完成关闭，无请求丢失 | |

### 9.2 季度演练

| 序号 | 演练项目 | 操作方法 | 预期结果 | 负责人 |
|------|---------|---------|---------|--------|
| 1 | MySQL 主从切换 | 模拟主库故障，切换到从库 | 服务自动连接新主库，数据无丢失 | |
| 2 | 全链路故障恢复 | 依次停止 Milvus → Redis → 恢复 Redis → 恢复 Milvus | 各阶段降级行为符合预期，全部恢复后功能完整 | |
| 3 | Embedding 补偿验证 | 停止 Embedding 服务 → 写入若干记忆 → 恢复并重启 Server | `BackfillEmbeddings` 自动补偿，日志显示补偿完成 | |
| 4 | 高并发压测 | 模拟 100 并发用户同时对话 | MySQL 连接池不耗尽，Redis 不 OOM，LLM 超时率可控 | |

### 9.3 演练记录模板

```
演练日期：____
演练项目：____
参与人员：____
演练步骤：
  1. ____
  2. ____
实际结果：____
是否符合预期：是 / 否
发现的问题：____
改进措施：____
```
