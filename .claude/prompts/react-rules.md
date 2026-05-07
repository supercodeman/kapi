## ReAct 路径判定规则

### 判定时机
在 LLM 第一轮输出后，由 Tool Dispatcher 根据输出结构判定执行路径。

### 判定标准

| LLM 输出特征 | 路径 | 行为 |
|---|---|---|
| 单个 Tool Call + 参数完整 | 单步直达 | 执行 → 结果给 LLM → 生成回复 |
| 多个 Tool Call（无依赖） | 单步并行 | 并行执行 → 合并结果给 LLM |
| 多个 Tool Call（有依赖） | ReAct | 按依赖顺序逐步执行 |
| 单个 Tool Call + 后续意图 | ReAct | 执行 → 观察 → 继续循环 |
| 无 Tool Call | 直接返回 | 不进入执行流程 |

### ReAct 约束
- 最大步数：5 轮
- 写操作打断：遇到写操作暂停，展示确认
- 超时：单步 Tool 执行 3s 超时

### System Prompt 指令
告知 LLM：
- 能一次完成的，输出单个 Tool Call
- 需要多个独立数据的，输出多个并行 Tool Call
- 只有后续参数依赖前一步结果时，才输出单个 Tool Call 并说明后续计划
