# 咔皮记账 AI 助手 — v3.0 RAG 记忆增强方案（完整版）

> 版本：3.0 完整版 | 日期：2026-04-30
> 核心主题：RAG 记忆驱动的智能预填 + 消费模式识别 + 上下文联想
> 前置依赖：v2.1（分类体系 + 资产联动 + Milvus/Embedding 接入）

---

## 一、当前记忆方案问题

| 层级 | 设计 | 实际状态 | 问题 |
|------|------|---------|------|
| L1 画像 | 用户偏好标签 | 无写入路径 | 完全空置 |
| L2 事实 | 结构化财务事实 | 粗糙规则提取 | 覆盖面极窄 |
| L3 情景 | 操作场景记录 | 格式是原始 map 字符串 | 无法做语义检索 |
| L4 对话 | Redis 对话历史 | 正常工作 | 只用于多轮上下文 |

核心缺陷：记忆系统只有"存"没有"用"。无消费习惯建模，无上下文联想。

---

## 二、记忆体系 v3.0

```
L1 用户画像     — 偏好标签（长期，低频更新）
L2 事实记忆     — 结构化事实（月租、工资等）
L3 情景记忆     — 操作场景（v2.1 已升级为结构化自然语言）
L4 对话历史     — 近期对话（Redis 滑动窗口）
L5 消费模式     — 从历史账单提取的重复消费模式（新增）
L6 序列模式     — 消费之间的关联关系（新增）
```

---

## 三、Embedding 模型

### 选型：Qwen text-embedding-v3

| 维度 | 详情 |
|------|------|
| 模型 | text-embedding-v3 |
| API | https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings（OpenAI 兼容） |
| 维度 | 1024 |
| 认证 | DashScope API Key |
| 中文支持 | 原生支持 |

选择理由：Qwen Embedding 中文语义理解能力强，1024 维在精度和性能之间取得良好平衡，DashScope 平台稳定可靠，OpenAI 兼容格式接入成本低。

### config.yaml 配置

```yaml
llm:
  base_url: "https://api.minimax.chat/v1/"
  api_key: "your-key"
  model: "MiniMax-M2.7"

embedding:
  base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1/"
  api_key: "your-dashscope-key"
  model: "text-embedding-v3"
```

---

## 四、L5 消费模式（Consumption Pattern）

### 数据结构

```go
type ConsumptionPattern struct {
    ID           uint64
    UserID       uint64
    Merchant     string
    Category     string
    SubCategory  string
    AvgAmount    float64   // 加权平均金额（近期权重高）
    Count        int       // 总次数
    RecentCount  int       // 最近 30 天次数
    EffectScore  float64   // 有效分（时间加权）
    LastDate     string
    Frequency    string    // daily/weekly/monthly/occasional
    Disabled     bool      // 用户主动禁用
    Embedding    []float32
}
```

### 提取方式

定期从 bills 表聚合分析（Go 代码，非实时 SQL）：
1. 按用户按商户分组
2. 计算加权平均金额、有效分、频率
3. 向量化内容存入 Milvus

### 提取时机

| 时机 | 触发条件 |
|------|---------|
| 定时任务 | 每天凌晨 |
| 事件触发 | 每记 10 笔账单后增量分析 |
| 首次使用 | 有历史数据时立即分析 |

### 向量化内容

```
"每周六在 XX 面包店消费 25 元，分类食饮-零食水果"
```

---

## 五、L6 序列模式（Sequence Pattern）

### 数据结构

```go
type SequencePattern struct {
    ID            uint64
    UserID        uint64
    PrevCategory  string
    PrevMerchant  string
    NextCategory  string
    NextMerchant  string
    NextAvgAmount float64
    Count         int
    Embedding     []float32
}
```

### 提取规则

| 设计点 | 决策 |
|--------|------|
| 时间窗口 | 同一天（date 字段），按 created_at 排序 |
| 最小次数 | 3 次 |
| 方向性 | 有方向（A→B ≠ B→A） |
| 数据范围 | 最近 90 天，硬截断 |

### 提取方式

Go 代码离线提取：
1. 按用户按天分组账单
2. 每天内按 created_at 排序
3. 相邻两笔组成一对 (prev, next)
4. 统计每种 pair 出现次数
5. 次数 >= 3 的存入 L6

### 触发方式

| 方式 | 场景 | 优先级 |
|------|------|--------|
| 被动检索（主要） | 用户说"洗车"，Engine 检查上一笔是否有序列匹配 | 默认 |
| 主动提示（辅助） | 记完加油后，suggestions API 推送"要记洗车吗？" | 可选 |

---

## 六、冷启动策略

| 阶段 | 账单数 | 可用能力 | 不可用 |
|------|--------|---------|--------|
| 零数据 | 0 | 引导式记账、L2 事实记忆 | 预填、模式、序列 |
| 少量 | 1~20 | L3 单次消费记忆（"上次 ¥38"） | 模式提取、序列模式 |
| 中等 | 20~50 | 初步模式（确认式预填，置信度阈值高） | 高置信度直接执行、周期识别 |
| 充足 | 50+ | 完整预填 + 序列模式 + 周期识别 | 无 |

### 冷启动期策略

- 不做通用模式预设（错误预填比没有预填更糟）
- 引导式记账快速积累数据
- 少量数据阶段用 L3 情景记忆做"上次消费"提示

---

## 七、预填交互设计

### 流程

```
用户输入 → LLM 意图识别（必须先过意图识别）
    ↓
确认是记账意图后：
    ↓
Engine RAG 检索 → 找到消费模式 → 预填参数
    ↓
根据置信度决定交互方式
```

### 置信度策略

| 置信度 | 条件 | 交互方式 |
|--------|------|---------|
| 高（≥ 0.8） | L5 精确匹配，商户一致，历史 ≥5 次，有效分 ≥ 2.0 | 直接执行 + 标注"根据消费习惯预填" |
| 中（0.5~0.8） | L5 语义匹配或 L6 序列匹配，历史 ≥3 次 | 返回确认请求，用户确认后执行 |
| 低（< 0.5） | 无匹配或匹配弱 | 不预填，LLM 追问缺失参数 |

### 参数优先级

```
用户显式输入 > RAG 预填 > NormalizeCategoryV2 推断 > 默认值
```

RAG 只补全缺失参数，绝不覆盖用户已提供的参数。

### 交互示例

**高置信度：**
```
用户：面包
AI：已记录：今天 XX面包店 ¥25.00 食饮-零食水果 ✨
    （根据你每周六的消费习惯预填，如有不对请告诉我修改）
```

**中置信度：**
```
用户：面包
AI：根据你的消费习惯：XX面包店 ¥25.00 食饮-零食水果
    回复"确认"记录，或告诉我需要修改的地方
```

**低置信度：**
```
用户：面包
AI：好的，面包消费多少钱？
```

### 确认操作的快速路径

用户说"确认"时，Engine 直接调 Tool，不经过 LLM。通过 PendingAction 机制实现：

```go
type PendingAction struct {
    UserID    uint64
    SessionID uint64
    ToolName  string
    Params    map[string]any
    ExpiresAt time.Time  // 5 分钟过期
}
```

---

## 八、模式衰减策略

### L5 消费模式：时间加权衰减

```
权重函数：
  最近 30 天：1.0
  30~90 天：0.6
  90~180 天：0.3
  180 天以上：0.1

有效分 = Σ(每次消费的权重)

有效分 >= 2.0 → 模式有效（可高置信度预填）
有效分 1.0~2.0 → 模式弱有效（只用于确认式预填）
有效分 < 1.0 → 模式失效
```

### L5 金额：加权平均

```
预填金额 = Σ(金额 × 时间权重) / Σ(时间权重)
```

近期消费权重更高，预填金额更接近用户当前消费水平。

### L6 序列模式：硬截断

```
只看最近 90 天
90 天内出现 >= 3 次 → 有效
90 天内 < 3 次 → 失效，直接删除
```

### 不做的（留给后续版本）

- 季节性模式识别（v3.1，需要一年数据）
- 用户主动管理预填偏好（v3.1，等用户反馈后再做）

---

## 九、与 Workflow 架构的整合

### RAG 介入位置

```
LLM 意图识别 + 参数提取
    ↓
Engine Workflow：
  1. NormalizeDate（日期标准化）
  2. NormalizeCategoryV2（分类推断）
  3. ★ RAG 参数补全 ★
     - merchant 有值但 amount 缺失 → L5 按商户检索
     - 只有关键词 → L5 语义检索
     - 刚执行过写操作 → L6 序列检索
  4. 收入分类自动修正
  5. 置信度判断：
     - >= 0.8 → 直接调 Tool
     - 0.5~0.8 → 返回确认请求
     - < 0.5 → LLM 追问
  6. 调用 Tool
```

### RAG 的身份

参数补全器，不是独立环节。不改变 Workflow 的控制流，只增强参数填充能力。

---

## 十、隐私保护

### 多租户隔离

| 数据 | 隔离方式 |
|------|---------|
| L5 消费模式 | Milvus filter（user_id == X），后续改 partition |
| L6 序列模式 | 同上 |
| L2/L3 记忆 | MySQL WHERE user_id = ? + Milvus filter |
| L4 对话 | Redis key 含 userID |

### LLM/Embedding API 数据传输

- 最小化传输：System Prompt 只注入必要记忆
- 合规声明：用户协议说明数据传输到 AI 服务提供商
- 后续支持本地模型（v4.0）

### 用户数据控制（v3.0 实现）

- `list_my_patterns` Tool：查看系统记住的消费模式
- `delete_pattern` Tool：删除指定模式
- 用户说"你记住了我的哪些习惯" → 展示模式列表
- 用户说"不要记住我在星巴克的消费" → 标记 Disabled

### 不做的（留给生产化阶段）

- 数据导出（GDPR）
- 账号注销清除全部数据
- Redis 认证（生产环境配置）
- Milvus partition 物理隔离

---

## 十一、实施步骤

### 第一步：接入 Milvus + Embedding（v2.1 已包含）
- MiniMaxEmbeddingProvider 实现
- MilvusClient 初始化
- L2/L3 记忆写入时生成 Embedding

### 第二步：L5 消费模式提取 + 预填
- ConsumptionPattern 模型 + Milvus Collection
- 离线提取任务（定时 + 事件触发）
- ragFillParams 逻辑
- 置信度判断 + 确认式预填交互
- PendingAction 确认机制

### 第三步：L6 序列模式 + 上下文联想
- SequencePattern 模型 + Milvus Collection
- 离线提取任务
- 序列检索逻辑
- suggestions API 主动提示

### 第四步：用户数据控制
- list_my_patterns Tool
- delete_pattern Tool

---

## 十二、验证场景

### 消费模式预填
1. 用户记了 5 次星巴克 38 元 → 说"星巴克" → 直接预填 ¥38（高置信度）
2. 用户记了 3 次面包 25 元 → 说"面包" → 确认式预填（中置信度）
3. 用户第一次说"寿司" → 无匹配 → 追问金额（低置信度）

### 序列模式
4. 用户 3 次"加油后洗车" → 记完加油说"洗车" → 预填 ¥30
5. 用户 2 次"加油后洗车" → 不触发（次数不够）

### 冷启动
6. 新用户说"午饭" → 无模式 → 正常追问
7. 新用户记了 1 次星巴克 38 元 → 再说"星巴克" → "上次 ¥38，这次也是吗？"（L3 单次记忆）

### 衰减
8. 半年前常买面包（有效分 < 1.0）→ 说"面包" → 不预填
9. 最近常买咖啡（有效分 > 2.0）→ 说"咖啡" → 预填

### 隐私
10. "你记住了我的哪些习惯" → 展示消费模式列表
11. "不要记住我在星巴克的消费" → 模式标记 Disabled

### 参数优先级
12. 用户说"星巴克 45 元" → RAG 匹配到 38 元 → 用用户的 45 元（不覆盖）
13. 用户说"星巴克" → RAG 补全 38 元 → 用 RAG 的 38 元（补全缺失）
