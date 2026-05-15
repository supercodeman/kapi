## 写操作规则

### 确认流程
所有通过 AI 助手触发的写操作（创建、修改、删除），必须经过用户确认后才能执行。

### 流程
1. LLM 输出 Tool Call（标记为 write 类型）
2. Tool Dispatcher 识别为写操作，不立即执行
3. 生成操作预览（展示 before/after 对比）
4. 推送确认请求到客户端
5. 用户确认 → 执行 → 记录操作日志 → 更新记忆
6. 用户拒绝 → 回到对话流程

### 操作日志
每次写操作记录：
- who: user_id
- what: operation_type + target_table + target_id
- when: created_at
- before_snapshot: 操作前数据快照（JSON）
- after_snapshot: 操作后数据快照（JSON）
- session_id: 关联会话
- trigger_text: 触发该操作的用户原话

### 软删除
- 所有删除操作为软删除（is_deleted = 1）
- 30 天后可物理清理
- 用户可通过自然语言恢复已删除数据

### 业务规则兜底
- 金额超过该分类历史最大值 3 倍 → 追问确认
- 批量操作超过 5 条 → 强制确认 + 列出清单
- 预算修改幅度超过 50% → 提示变化幅度
- 删除全部某分类账单 → 二次确认
- 记账日期超过 30 天前或在未来 → 追问确认
