# 咔皮记账 AI 助手 — 开发沟通记录（续）

> 日期：2026-04-29（下半场）
> 接续：session-2026-04-29.md（P1~P6 完成后）

---

## 一、实测问题发现

### 1.1 首次实测反馈
用户实测发现以下问题：
1. 响应时间远超 5s
2. 意图识别能力差——用户说"将我本月的预算设置为30元"，AI 无法完成（缺少 create_budget Tool）
3. 多轮对话无上下文——"创建吧"AI 不知道指什么
4. Think 标签暴露在回复中
5. 页面切换右栏不联动
6. 缺少耗时统计

### 1.2 根因分析
- SaveMemory 从未被调用（死代码）
- ConversationStore.AddMessage 从未被调用（对话历史不存储）
- 对话历史未注入 LLM 消息列表（每次对话独立）
- System Prompt 只有一句话，缺乏行为指引
- Tool 不完整（缺 create_budget、delete_bill 等）

## 二、对话体验优化方案（8 个模块）

### 设计决策
1. Think 标签过滤 + Thinking 字段
2. System Prompt 精细化（prompt.go，含预设分类、行为规范）
3. 对话历史持久化到 Redis + 消息列表注入 LLM
4. 写操作后自动保存 L3 情景记忆
5. Tool 补全（6 个新 Tool + Budget/Asset 软删除）
6. Skill 精细化（9 个 YAML + 2 个新 Skill）
7. 前端优化（Think 开关、右栏联动、耗时展示、冷启动欢迎）
8. 冷启动 System Prompt 新用户引导

### 实施过程
- 模块 1~3 在主会话实施
- 模块 4~8 通过两个并行 agent 实施（前端 + 后端）
- 遇到 MySQL JSON 空值问题（OpLog 和 Memory 写入失败）→ 修复为 "null" / "{}" / "[]"
- 遇到 go build 缓存问题 → 使用 -a 强制重编译

## 三、核心架构反思（v2.0）

### 3.1 问题升级
实测发现更严重的问题：
- LLM 编造操作结果（说"已记录"但没调 Tool，数据库无记录）
- LLM 自己计算金额（把全部历史账单求和当作当月消费）
- 日期推理错误（输出 2025 年日期而非 2026 年）

### 3.2 深度讨论：LLM 的定位
用户追问："LLM 在这个系统中的定位到底是什么？"

经过多轮讨论，达成共识：
- **LLM 应该是"意图翻译器"，不是"执行者"**
- 记账应用 90% 是 Workflow 场景，只有 10% 需要 Agent
- 但 v1.0 用 100% Agent 模式处理所有请求
- Prompt 约束是软约束，LLM 在 10% 情况下会绕过
- **任何关键约束都不能只靠 Prompt，必须在代码层强制执行**

### 3.3 架构决策
**Workflow 优先，Agent 兜底：**
- 90% 场景走 Workflow：Engine 控制流程，LLM 只做意图识别和回复润色
- 10% 场景走 Agent：LLM 自主决策，但受 Engine 监管

### 3.4 角色重新定义

**LLM：意图翻译器 + 表达润色器**
- 做：理解自然语言、提取参数、润色回复
- 不做：数值计算、日期推理、操作声明、记忆决策

**Engine：流程控制器 + 数据守门人 + 记忆管理者**
- 做：流程路由、参数标准化、强制 Tool 调用、结果校验、记忆写入
- 核心能力：validateResponse 拦截虚假操作声明、handleNoToolCall 强制重试

### 3.5 ToolResult 重新设计
加 Status + Message 字段，Message 由 Tool 代码生成（不是 LLM 生成）

### 3.6 Skill 重新定义
加 mode（workflow/agent）、mandatory_tool、forbidden_without_tool、forbidden_tools

## 四、v2.0 Engine 重构实施

### 4.1 核心改动
1. Engine.Chat 拆分为 executeWorkflow + handleNoToolCall 两条路径
2. handleNoToolCall 检测操作意图后强制重试（追加"请务必调用工具"指令）
3. validateResponse 拦截虚假操作声明（扩展匹配模式）
4. ToolResult 加 Status + Message
5. 所有 Tool 返回 Status + Message（16 个 Tool 全部更新）
6. create_bill Tool 内部自动查预算附带超支提醒
7. Skill YAML 全部更新为 v2.0 格式
8. System Prompt 强化 Tool 调用约束

### 4.2 参数标准化层（normalize.go）
- NormalizeDate：处理"今天"/"昨天"/"上周三"等相对日期 + 年份修正
- NormalizeCategory：分类别名映射（"吃饭"→"餐饮"）
- NormalizePeriod：周期标准化
- NormalizeAssetType：资产类型标准化

### 4.3 自测结果
| 场景 | 结果 |
|------|------|
| 基础记账 | 通过，日期正确（2026-04-29） |
| 上下文连贯 | 通过 |
| 预算创建 | 通过 |
| 超预算记账 | 通过（记账成功 + 超支提醒） |
| 虚假声明拦截 | 通过（LLM 没调 Tool → Engine 重试 → 成功） |
| OpLog trigger_text | 通过 |
| Memory 写入 | 通过 |

## 五、持续实测问题修复

### 5.1 日期问题
用户说"今天肯德基消费56元"，记录日期为 2025-01-24 而非 2026-04-29。
- 根因：LLM 日期推理错误
- 修复：NormalizeDate 在 Tool 层强制处理，不依赖 LLM
- System Prompt 注入当前日期

### 5.2 预算计算错误
AI 说"餐饮已花 ¥128"但实际当月只有一笔 ¥38。
- 根因：LLM 调 list_bills 拿到全部历史账单自己求和
- 修复：统计场景 forbidden_tools 禁用 list_bills，强制走 get_category_summary
- System Prompt 加"绝不自己对账单金额求和"

### 5.3 收入被记为支出
用户说"工资收入5万"被记为支出。
- 根因：LLM 没传 bill_type=income 参数
- 修复：Bill 模型加 bill_type 字段 + Tool 层收入分类自动修正 + System Prompt 明确收入/支出区分
- 统计查询加 bill_type='expense' 过滤

### 5.4 右栏不刷新
记账后右侧账单列表不更新。
- 根因：Vue emit 事件名大小写不匹配（@refreshPage vs @refresh-page）
- 修复：改为 @refresh-page + 无条件触发 + 500ms 延迟

### 5.5 拉面记录未写入数据库
LLM 声称"已记录"但数据库无记录。
- 根因：LLM 没有调用 Tool，直接编造了回复
- 修复：v2.0 的 validateResponse + handleNoToolCall 重试机制

## 六、新增功能

### 6.1 首页概览
右栏展示：今日消费、本月消费、预算执行进度条、最近 5 笔消费

### 6.2 对话历史加载
登录后自动加载最近 20 轮对话 + "加载更多"按钮

### 6.3 退出登录显示用户名
侧栏底部显示当前用户名

### 6.4 农历日期转换 Tool
convert_lunar_date Tool，使用 lunar-go 库

### 6.5 账单收入/支出区分
前端账单列表展示类型标签（收入绿色/支出红色）+ 金额前缀 +/-

### 6.6 查看详情按钮修复
点击后数据写入 store，右栏展示详情面板

### 6.7 中间对话区宽度调整
从 flex:1 改为 width:40%，右栏 flex:1 自适应

## 七、v2.1 设计方案

### 核心主题
记账-资产联动 + 转账能力 + 信用卡场景

### 关键设计
- Bill 模型加 asset_id / to_asset_id
- 支出 → 资产余额减少，收入 → 资产余额增加
- 新增 transfer Tool 和 repay_credit_card Tool
- 信用卡消费/还款场景
- suggestions API（主动提醒基础版）
- L2 事实提取规则扩展（周期性事件、纪念日、购买计划）
- LLM 辅助事实提取（extracted_facts 字段）

### 状态
设计方案已完成（docs/design-decisions-v2.1.md），待用户确认后实施。

## 八、关键经验总结

### 架构层面
1. **记账场景是 Workflow 为主，不是 Agent 为主** — 这个认知偏差导致了大量问题
2. **LLM 的约束不能只靠 Prompt** — 必须在代码层强制执行
3. **Tool 返回值必须有明确状态** — 否则 Engine 无法区分"执行成功"和"LLM 编造"
4. **参数标准化必须在 Tool 层做** — 不能依赖 LLM 做日期计算、分类推断

### 开发层面
1. **编译通过 ≠ 运行正确** — MySQL JSON 空值、go build 缓存都是编译通过但运行失败的例子
2. **死代码是最危险的 bug** — SaveMemory/AddMessage 实现了但从未被调用，直到实测才发现
3. **每次改动必须做运行时自测** — 不能只验证编译

### 产品层面
1. **准确性 > 智能感** — 用户最在意"记的数对不对"，不是"对话多灵活"
2. **收入/支出区分是基础需求** — 不能遗漏
3. **统计数字必须由 Tool 计算** — LLM 自己算会出错
