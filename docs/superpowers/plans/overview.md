# 咔皮记账 AI 助手 — 实施阶段总览

> 版本：1.0 | 日期：2026-04-29

## 阶段拆分

| 阶段 | 名称 | 内容 | 依赖 | 人工预估 | Agent 预估 |
|------|------|------|------|---------|-----------|
| P1 | 基础设施 + Auth + Biz Mock | Go 服务骨架、Docker Compose、JWT 认证、Mock CRUD API | 无 | 2~3 天 | 2~4 小时 |
| P2 | Memory Manager | 四层记忆系统、Milvus 向量检索、多租户隔离 | P1 | 2~3 天 | 2~3 小时 |
| P3 | Skill Manager | 文件+DB 混合加载、页面感知、热更新 | P1 | 1~2 天 | 1~2 小时 |
| P4 | AI Engine（核心） | LLM 抽象层、意图识别、ReAct、反幻觉、WebSocket | P1+P2+P3 | 3~5 天 | 4~6 小时 |
| P5 | Analytics + Op Logger | 统计分析 Skill、预计算、操作日志、恢复 | P1+P4 | 2~3 天 | 2~3 小时 |
| P6 | 前端 Vue3 Web Demo | 三栏布局、6 页面、对话交互、图表渲染 | P4+P5 | 3~5 天 | 3~5 小时 |

- 人工总预估：13~21 天（单人全职，每天 6~8 小时有效编码）
- Agent 总预估：14~23 小时（主要瓶颈在 P4 AI Engine 和 P6 前端）

## 依赖关系图

```
P1 (基础设施)
├── P2 (Memory)
├── P3 (Skill)
│
├── P4 (AI Engine) ← 依赖 P2 + P3
│   │
│   ├── P5 (Analytics + OpLog)
│   │
│   └── P6 (前端) ← 依赖 P5
```

## 每阶段交付标准

### P1: 基础设施 + Auth + Biz Mock
- docker-compose up 一键启动所有基础服务
- 用户注册/登录获取 JWT Token
- 账单/预算/资产 CRUD API 全部可用
- 多租户隔离验证通过（用户 A 看不到用户 B 的数据）

### P2: Memory Manager
- 四层记忆 CRUD（L1 画像、L2 事实、L3 情景、L4 对话历史）
- Milvus 向量写入和语义检索
- L2 双通道检索（精确 + 语义）
- 多租户记忆隔离验证

### P3: Skill Manager
- 文件层 Skill 加载（YAML 定义）
- DB 层 Skill 加载（覆盖文件层）
- 页面感知：按 page_context 筛选 Skill 集
- 热更新：文件变更自动重载

### P4: AI Engine
- LLM 抽象层（Chat + Embedding interface）
- WebSocket 对话 + HTTP 降级
- 意图识别（全量 Skill 摘要 + LLM 原生识别）
- ReAct 多步推理（5 种路径判定）
- 反幻觉（data_ref 结构化校验）
- 写操作确认流程
- 会话连续性（页面切换不中断）

### P5: Analytics + Operation Logger
- 统计分析 Skill（同比、环比、分类占比）
- 预计算调度（定时 + 事件驱动）
- 操作日志（before/after 快照）
- 自然语言恢复

### P6: 前端 Vue3 Web Demo
- 三栏布局（页面切换器 + 对话窗口 + 动态面板）
- 6 个核心页面
- WebSocket 对话 + 流式输出
- 图表渲染（ECharts）
- 执行反馈（骨架屏 + 进度预估）

## 实施计划文件

| 阶段 | 计划文件 | 状态 |
|------|---------|------|
| P1 | `plans/2026-04-29-p1-foundation.md` | 生成中 |
| P2 | `plans/2026-04-29-p2-memory.md` | 待生成 |
| P3 | `plans/2026-04-29-p3-skill.md` | 待生成 |
| P4 | `plans/2026-04-29-p4-ai-engine.md` | 待生成 |
| P5 | `plans/2026-04-29-p5-analytics.md` | 待生成 |
| P6 | `plans/2026-04-29-p6-frontend.md` | 待生成 |
