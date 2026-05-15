## 记忆隔离规则

### 多租户隔离原则
用户间的记忆信息严格隔离，不可交叉访问。

### 隔离机制
1. 所有记忆表查询必须带 `WHERE user_id = ?`
2. Milvus 向量检索按 user_id 分区（partition）
3. Redis 中的对话历史按 `session:{user_id}:{session_id}` 命名空间隔离
4. 记忆写入时必须绑定 user_id，不允许无归属的记忆

### 六层记忆存储规则

| 层级 | 存储位置 | 向量化 | 隔离方式 |
|------|---------|--------|---------|
| L1 用户画像 | MySQL | 不需要 | user_id 字段 |
| L2 事实记忆 | MySQL + Milvus | 文本描述向量化 | user_id 字段 + Milvus partition |
| L3 情景记忆 | MySQL + Milvus | 场景描述向量化 | user_id 字段 + Milvus partition |
| L4 对话历史 | Redis（近期）+ MySQL（归档） | 不需要 | Redis key 命名空间 |
| L5 消费模式 | MySQL（consumption_patterns 表） | 待接入 | user_id 字段 |
| L6 序列模式 | MySQL（sequence_patterns 表） | 待接入 | user_id 字段 |

### 记忆来源标记
每条记忆必须标记来源：
- `user_stated`：用户直接陈述
- `system_inferred`：系统推断（引用时需标注"根据历史记录推断"）
- `tool_result`：工具查询结果

### 记忆验证
- 事实记忆（L2）定期与实际数据校验
- 过期或矛盾的记忆标记为 `is_verified = false`
- LLM 引用未验证记忆时需标注不确定性
