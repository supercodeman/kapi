# 咔皮记账 AI 助手（Kapi）

全场景内嵌式 AI 记账助手，覆盖首页、记一笔、账单详情、预算、报表、资产管理 6 个核心页面。

## 技术栈

- **后端**：Go 1.22 / Gin / GORM
- **前端**：Vue 3 / Pinia / Vite / ECharts
- **存储**：MySQL 8.0 / Redis 7 / Milvus
- **AI**：MiniMax M2.7（LLM）/ Qwen text-embedding-v3（Embedding）

## 快速启动

### 本地开发

```bash
# 1. 启动依赖（MySQL + Redis + Milvus）
docker-compose up -d mysql redis milvus

# 2. 配置
cp config.yaml.example config.yaml
# 编辑 config.yaml，填入以下必要信息（详见下方配置说明）
```

### 配置说明（config.yaml）

项目启动依赖 `config.yaml`，需要填充以下关键配置：

| 配置项 | 说明 | 获取方式 |
|--------|------|----------|
| `llm.base_url` | LLM API 地址（OpenAI 兼容格式） | MiniMax: `https://api.minimax.chat/v1/` |
| `llm.api_key` | LLM API Key | [MiniMax 开放平台](https://platform.minimaxi.com/) 申请 |
| `llm.model` | 模型名称 | 当前使用 `MiniMax-M2.7` |
| `embedding.base_url` | Embedding API 地址 | 通义千问: `https://dashscope.aliyuncs.com/compatible-mode/v1/` |
| `embedding.api_key` | Embedding API Key | [阿里云 DashScope](https://dashscope.console.aliyun.com/) 申请 |
| `embedding.model` | Embedding 模型名称 | 当前使用 `text-embedding-v3` |
| `mysql.dsn` | MySQL 连接串 | 本地开发默认 `root:password@tcp(localhost:3306)/kapi` |
| `jwt.secret` | JWT 签名密钥 | 生产环境务必替换为随机字符串 |

> LLM 和 Embedding 均使用 OpenAI 兼容接口格式，可替换为任何兼容的模型服务。

```bash
# 3. 启动后端
go build -o kapi_server ./cmd/server/
./kapi_server

# 4. 启动前端
cd web && npm install && npm run dev
```

访问 http://localhost:3000

### Docker 部署

```bash
docker-compose up -d
```

## 项目结构

```
kapi/
├── cmd/server/          # 入口
├── internal/
│   ├── analytics/       # 统计分析 + 缓存 + 消费模式提取
│   ├── config/          # 配置加载
│   ├── dao/             # 数据访问层
│   ├── engine/          # AI Engine 核心
│   │   ├── engine.go    # Chat 主流程 + ReAct 循环
│   │   ├── stream.go    # SSE 流式输出
│   │   ├── tools.go     # Tool 注册和执行
│   │   ├── rag.go       # RAG 参数补全
│   │   ├── guard.go     # 写操作业务规则兜底
│   │   ├── validate.go  # 反幻觉校验
│   │   ├── history.go   # 对话历史脱敏
│   │   ├── pending.go   # PendingAction 确认机制
│   │   ├── fallback.go  # 故障降级
│   │   ├── profile.go   # L1 用户画像
│   │   ├── normalize.go # 分类/日期/资产标准化
│   │   └── prompt.go    # System Prompt
│   ├── handler/         # HTTP Handler
│   ├── memory/          # 记忆系统（Redis + Milvus + MySQL）
│   ├── middleware/       # JWT 认证中间件
│   ├── model/           # 数据模型
│   ├── router/          # 路由注册
│   ├── service/         # 业务逻辑层
│   ├── skill/           # Skill 管理（YAML + DB）
│   └── utils/           # 工具函数
├── web/src/             # Vue3 前端
│   ├── components/
│   │   ├── ChatWindow.vue     # 对话窗口（SSE + Markdown + 确认面板）
│   │   ├── DynamicPanel.vue   # 右栏动态内容
│   │   └── CategoryPicker.vue # 两级分类选择器
│   ├── stores/          # Pinia 状态管理
│   └── views/           # 页面
├── skills/              # Skill YAML 定义
├── docs/                # 设计文档
├── test_api.sh          # 集成测试（14 用例）
├── stress_test.sh       # 压力测试
└── docker-compose.yml
```

## 核心架构

**Workflow 优先，Agent 兜底**：90% 场景由 Engine 控制流程（Tool 调用链），LLM 只做意图识别和表达润色。

**反幻觉五层防线**：
1. 对话历史脱敏（远期 assistant 消息金额替换为 ¥***）
2. System Prompt 硬约束（禁止自行计算）
3. amountFoundInContext（金额来源校验）
4. containsUnverifiedAmount（LLM 回复金额校验）
5. validateResponse（操作声明校验）

**记忆六层体系**：
- L1 用户画像 / L2 事实记忆 / L3 情景记忆 / L4 对话历史 / L5 消费模式 / L6 序列模式

## 测试

```bash
# 集成测试（需要后端运行）
./test_api.sh

# 压力测试
./stress_test.sh 10 3    # 10并发 × 3请求

# 单元测试
go test ./internal/engine/ -v
```

## 文档

| 文档 | 说明 |
|------|------|
| [design-decisions.md](docs/design-decisions.md) | 20 项核心设计决策 |
| [sequence-diagrams.md](docs/sequence-diagrams.md) | 5 个关键时序图 |
| [capacity-planning.md](docs/capacity-planning.md) | 容量估算与性能方案 |
| [fault-tolerance.md](docs/fault-tolerance.md) | 故障预案 |
| [architecture.html](docs/architecture.html) | 架构图 |
| [pending-action-design.md](docs/pending-action-design.md) | PendingAction 待确认操作机制 |
| [anti-hallucination-design.md](docs/anti-hallucination-design.md) | 反幻觉五层防线 |
| [income-type-confirmation-design.md](docs/income-type-confirmation-design.md) | 收入类型确认流程 |

## 性能指标

| 场景 | P50 | P95 |
|------|-----|-----|
| CRUD API | 66ms | 105ms |
| AI 对话（单步） | 7.1s | 12.5s |
| PendingAction 确认 | ~200ms | ~500ms |
