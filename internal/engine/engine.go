package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sangchenglong/kapi/internal/memory"
	"github.com/sangchenglong/kapi/internal/model"
	"github.com/sangchenglong/kapi/internal/skill"
)

const maxReActSteps = 5

type ChatRequest struct {
	UserID      uint64 `json:"user_id"`
	SessionID   uint64 `json:"session_id"`
	Message     string `json:"message"`
	PageContext string `json:"page_context"`
}

type ChatResponse struct {
	Display       string         `json:"display"`
	Thinking      string         `json:"thinking,omitempty"`
	DataRef       map[string]any `json:"data_ref,omitempty"`
	Actions       []Action       `json:"actions,omitempty"`
	Plan          *ExecutionPlan `json:"plan,omitempty"`
	AffectedPages []string       `json:"affected_pages,omitempty"`
	StartTime     int64          `json:"start_time"`
	EndTime       int64          `json:"end_time"`
	DurationMs    int64          `json:"duration_ms"`
}

type Action struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	Target  string `json:"target,omitempty"`
}

type ExecutionPlan struct {
	Steps            int    `json:"steps"`
	EstimatedSeconds int    `json:"estimated_seconds"`
	Description      string `json:"description"`
}

type ToolResult struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
	Source  string         `json:"source"`
}

type Engine struct {
	llm          LLMProvider
	skillReg     *skill.Registry
	memMgr       *memory.MemoryManager
	toolExecutor *ToolExecutor
	promptBase   string
	pending      *PendingStore
	llmSem       chan struct{}
}

func NewEngine(llm LLMProvider, skillReg *skill.Registry, memMgr *memory.MemoryManager, toolExec *ToolExecutor, promptBase string, maxConcurrent int) *Engine {
	if maxConcurrent <= 0 {
		maxConcurrent = 20
	}
	return &Engine{
		llm:          llm,
		skillReg:     skillReg,
		memMgr:       memMgr,
		toolExecutor: toolExec,
		promptBase:   promptBase,
		pending:      NewPendingStore(),
		llmSem:       make(chan struct{}, maxConcurrent),
	}
}

func (e *Engine) llmChatSync(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	select {
	case e.llmSem <- struct{}{}:
		defer func() { <-e.llmSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return e.llm.ChatSync(ctx, messages, tools)
}

func (e *Engine) llmChatStream(ctx context.Context, messages []Message, tools []Tool) (<-chan StreamChunk, error) {
	select {
	case e.llmSem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ch, err := e.llm.Chat(ctx, messages, tools)
	if err != nil {
		<-e.llmSem
		return nil, err
	}
	wrappedCh := make(chan StreamChunk, 64)
	go func() {
		defer func() { <-e.llmSem }()
		defer close(wrappedCh)
		for chunk := range ch {
			wrappedCh <- chunk
		}
	}()
	return wrappedCh, nil
}

func (e *Engine) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	startTime := time.Now()

	if e.llm == nil {
		return &ChatResponse{Display: "AI 服务尚未配置，请设置 LLM Provider 后重试。"}, nil
	}

	// PendingAction 快速路径
	if isCancelMessage(req.Message) {
		e.pending.Clear(req.UserID, req.SessionID)
		resp := &ChatResponse{Display: "好的，已取消。"}
		e.setTiming(resp, startTime)
		e.persistConversation(ctx, req, resp.Display)
		return resp, nil
	}
	if action := e.pending.Get(req.UserID, req.SessionID); action != nil {
		if isPendingParamAnswer(req.Message) {
			result, err := e.executePendingAction(ctx, req, action, startTime)
			if err == nil {
				return result, nil
			}
			log.Printf("pending action failed, falling through to LLM: %v", err)
		} else {
			// 不像参数回答（可能是追问/闲聊），放回 PendingAction，走正常 LLM
			e.pending.Set(action.UserID, action.SessionID, action.ToolName, action.Params)
		}
	}

	// 并行预加载所有上下文
	pl := e.preloadContext(ctx, req)
	systemPrompt := e.buildSystemPrompt(req.PageContext, pl.skillSummary, pl.memories)

	messages := make([]Message, 0, 2+len(pl.historyMessages))
	messages = append(messages, Message{Role: "system", Content: systemPrompt})
	messages = append(messages, pl.historyMessages...)
	messages = append(messages, Message{Role: "user", Content: req.Message})

	tools := e.toolExecutor.GetToolDefinitionsForPage(e.skillReg.GetToolsForPage(req.PageContext))

	// 从历史消息中提取最近的 user 消息（用于多轮补参场景的金额校验）
	var recentUserMsgs []string
	for i := len(pl.historyMessages) - 1; i >= 0 && len(recentUserMsgs) < 3; i-- {
		if pl.historyMessages[i].Role == "user" {
			recentUserMsgs = append(recentUserMsgs, pl.historyMessages[i].Content)
		}
	}
	ctx = ContextWithRequest(ctx, req.SessionID, req.Message, recentUserMsgs)

	// 检测确认意图：上一轮 assistant 是 need_confirm 追问 + 当前用户消息是确认
	if isUserConfirming(pl.historyMessages, req.Message) {
		ctx = ContextWithConfirmation(ctx)
	}

	// RAG 预检索：消费模式提示注入 System Prompt
	if pl.ragHint != "" {
		messages[0].Content += "\n\n## 消费习惯提示\n" + pl.ragHint
	}

	// 第一轮 LLM 调用：意图识别 + 参数提取
	llmCtx, llmCancel := withLLMTimeout(ctx)
	defer llmCancel()
	resp, err := e.llmChatSync(llmCtx, messages, tools)
	if err != nil {
		result := llmFallbackResponse(err)
		e.setTiming(result, startTime)
		return result, nil
	}

	var result *ChatResponse

	if len(resp.ToolCalls) > 0 {
		// LLM 输出了 Tool Call → 走 Workflow 执行
		result, err = e.executeWorkflow(ctx, req, resp, messages, tools, startTime)
	} else {
		// LLM 没有输出 Tool Call → 检查是否应该有 Tool Call
		result, err = e.handleNoToolCall(ctx, req, resp, messages, tools, startTime)
	}

	if err != nil {
		return nil, err
	}

	e.setTiming(result, startTime)
	e.persistConversation(ctx, req, result.Display)
	e.extractFacts(ctx, req)
	go e.updateUserProfile(context.Background(), req.UserID)
	return result, nil
}

// executePendingAction 直接执行待确认操作，跳过 LLM
// 支持单笔（Params）和多笔（BatchParams）
func (e *Engine) executePendingAction(ctx context.Context, req *ChatRequest, action *PendingAction, startTime time.Time) (*ChatResponse, error) {
	ctx = ContextWithRequest(ctx, req.SessionID, req.Message, nil)
	ctx = ContextWithConfirmation(ctx)

	// 多笔批量执行
	if len(action.BatchParams) > 0 {
		return e.executeBatchPendingAction(ctx, req, action, startTime)
	}

	// 单笔执行
	mergePendingParams(action, req.Message)

	argsJSON, _ := json.Marshal(action.Params)
	tc := ToolCall{
		ID:        "pending_confirm",
		Name:      action.ToolName,
		Arguments: string(argsJSON),
	}

	result, err := e.toolExecutor.Execute(ctx, req.UserID, tc)
	if err != nil {
		return nil, err
	}

	e.saveToolMemory(ctx, req.UserID, result)

	resp := &ChatResponse{
		Display:       result.Message,
		AffectedPages: e.getAffectedPages([]ToolResult{*result}),
	}
	resp.DataRef = map[string]any{}
	for k, v := range result.Data {
		resp.DataRef[k] = v
	}

	e.setTiming(resp, startTime)
	e.persistConversation(ctx, req, resp.Display)
	return resp, nil
}

// executeBatchPendingAction 批量执行多笔待确认操作
func (e *Engine) executeBatchPendingAction(ctx context.Context, req *ChatRequest, action *PendingAction, startTime time.Time) (*ChatResponse, error) {
	var msgParts []string
	var allResults []ToolResult

	for i, params := range action.BatchParams {
		argsJSON, _ := json.Marshal(params)
		tc := ToolCall{
			ID:        fmt.Sprintf("batch_%d", i),
			Name:      "create_bill",
			Arguments: string(argsJSON),
		}
		result, err := e.toolExecutor.Execute(ctx, req.UserID, tc)
		if err != nil {
			msgParts = append(msgParts, fmt.Sprintf("第 %d 笔失败：%v", i+1, err))
			continue
		}
		e.saveToolMemory(ctx, req.UserID, result)
		allResults = append(allResults, *result)
		msgParts = append(msgParts, result.Message)
	}

	display := strings.Join(msgParts, "\n")
	resp := &ChatResponse{
		Display:       display,
		AffectedPages: e.getAffectedPages(allResults),
	}

	e.setTiming(resp, startTime)
	e.persistConversation(ctx, req, resp.Display)
	return resp, nil
}

// executeWorkflow 执行 Tool 调用链，支持多步 ReAct
func (e *Engine) executeWorkflow(ctx context.Context, req *ChatRequest, firstResp *Message, messages []Message, tools []Tool, startTime time.Time) (*ChatResponse, error) {
	var toolResults []ToolResult

	// 意图纠正：如果检测到确定性意图但 LLM 调用了错误的工具，直接纠正
	// 必须在 append messages 之前执行，确保 messages 中的 ToolCall ID 与后续 tool result 一致
	correctToolCalls(req.Message, &firstResp.ToolCalls)

	messages = append(messages, *firstResp)

	// 执行第一轮 Tool Calls
	// 如果有 create_bills_batch，优先执行它，跳过单笔 create_bill（避免冲突）
	hasBatch := false
	for _, tc := range firstResp.ToolCalls {
		if tc.Name == "create_bills_batch" {
			hasBatch = true
			break
		}
	}

	// 多笔合并：LLM 调用了多次 create_bill 时，合并为 batch 统一确认
	if !hasBatch {
		var createBillCalls []ToolCall
		var otherCalls []ToolCall
		for _, tc := range firstResp.ToolCalls {
			if tc.Name == "create_bill" {
				createBillCalls = append(createBillCalls, tc)
			} else {
				otherCalls = append(otherCalls, tc)
			}
		}

		// 检测用户消息中的金额数量，如果 LLM 调用的 create_bill 数量不足，重试
		userAmounts := ParseChineseNumber(req.Message)
		if len(createBillCalls) >= 1 && len(userAmounts) >= 2 && len(createBillCalls) < len(userAmounts) {
			// 重试要求使用 batch
			retryMessages := make([]Message, len(messages)-1)
			copy(retryMessages, messages[:len(messages)-1])
			retryMessages = append(retryMessages, Message{
				Role:    "user",
				Content: "用户报了多笔消费，请使用 create_bills_batch 工具一次性记录所有消费，不要遗漏任何一笔。",
			})
			retryCtx, retryCancel := withLLMTimeout(ctx)
			retryResp, retryErr := e.llmChatSync(retryCtx, retryMessages, tools)
			retryCancel()
			if retryErr == nil && len(retryResp.ToolCalls) > 0 {
				// 检查重试结果是否有 batch 或更多 create_bill
				retryBillCount := 0
				retryHasBatch := false
				for _, tc := range retryResp.ToolCalls {
					if tc.Name == "create_bills_batch" {
						retryHasBatch = true
					}
					if tc.Name == "create_bill" {
						retryBillCount++
					}
				}
				if retryHasBatch || retryBillCount > len(createBillCalls) {
					return e.executeWorkflow(ctx, req, retryResp, retryMessages, tools, startTime)
				}
			}
			// 重试失败，用已有的 createBillCalls 构建 batch（至少不丢笔）
			// 即使只有1笔也走 batch 确认流程，让用户知道可能有遗漏
		}

		if len(createBillCalls) > 1 {
			// 多笔 create_bill → 合并为 batch PendingAction
			var batchParams []map[string]any
			for _, tc := range createBillCalls {
				var params map[string]any
				json.Unmarshal([]byte(tc.Arguments), &params)
				batchParams = append(batchParams, params)
			}
			// 构建确认消息
			var msgParts []string
			var total float64
			for i, p := range batchParams {
				amount, _ := ToFloat64(p["amount"])
				merchant, _ := p["merchant"].(string)
				category := NormalizeCategoryV2(getString(p, "category"), merchant, "").Category
				p["category"] = category
				p["date"] = NormalizeDate(getString(p, "date"))
				total += amount
				msgParts = append(msgParts, fmt.Sprintf("  %d. %s ¥%.2f（%s）", i+1, merchant, amount, category))
			}
			date := NormalizeDate(getString(batchParams[0], "date"))
			// 日期异常提示
			dateHint := ""
			if t, err := time.ParseInLocation("2006-01-02", date, time.Local); err == nil {
				now := time.Now()
				today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
				if t.After(today) {
					dateHint = fmt.Sprintf("\n⚠️ 日期 %s（未来日期）", date)
				} else if days := int(today.Sub(t).Hours() / 24); days > 30 {
					dateHint = fmt.Sprintf("\n⚠️ 日期 %s（%d 天前）", date, days)
				}
			}
			msg := fmt.Sprintf("检测到 %d 笔消费，共 ¥%.2f：\n%s%s\n确认全部记录吗？",
				len(batchParams), total, strings.Join(msgParts, "\n"), dateHint)

			action := &PendingAction{
				UserID:      req.UserID,
				SessionID:   req.SessionID,
				ToolName:    "create_bill",
				BatchParams: batchParams,
				ExpiresAt:   time.Now().Add(5 * time.Minute),
			}
			e.pending.mu.Lock()
			e.pending.store[pendingKey(req.UserID, req.SessionID)] = action
			e.pending.mu.Unlock()

			// 执行非 create_bill 的其他工具
			for _, tc := range otherCalls {
				e.executeSingleTool(ctx, req.UserID, tc, &messages)
			}
			return &ChatResponse{Display: msg}, nil
		}
	}

	for _, tc := range firstResp.ToolCalls {
		if hasBatch && tc.Name == "create_bill" {
			continue
		}
		tr := e.executeSingleTool(ctx, req.UserID, tc, &messages)
		if tr != nil {
			toolResults = append(toolResults, *tr)
		}
	}

	// 如果有 need_confirm 的结果，存储 PendingAction 并返回追问
	for i, tr := range toolResults {
		if tr.Status == "need_confirm" {
			// 找到对应的 ToolCall，提取参数用于后续直接执行
			if i < len(firstResp.ToolCalls) {
				tc := firstResp.ToolCalls[i]
				var params map[string]any
				json.Unmarshal([]byte(tc.Arguments), &params)
				// 如果 need_confirm 的 data 中有补充参数（如 RAG 预填的 amount），合并进去
				for k, v := range tr.Data {
					if k == "_batch_params" {
						continue
					}
					if _, exists := params[k]; !exists {
						params[k] = v
					}
				}
				// 提取批量参数
				var batchParams []map[string]any
				if bp, ok := tr.Data["_batch_params"].([]map[string]any); ok {
					batchParams = bp
				} else if bpAny, ok := tr.Data["_batch_params"].([]any); ok {
					for _, item := range bpAny {
						if m, ok := item.(map[string]any); ok {
							batchParams = append(batchParams, m)
						}
					}
				} else {
				}
				action := &PendingAction{
					UserID:      req.UserID,
					SessionID:   req.SessionID,
					ToolName:    tc.Name,
					Params:      params,
					BatchParams: batchParams,
					ExpiresAt:   time.Now().Add(5 * time.Minute),
				}
				e.pending.mu.Lock()
				e.pending.store[pendingKey(req.UserID, req.SessionID)] = action
				e.pending.mu.Unlock()
			}
			return &ChatResponse{Display: tr.Message}, nil
		}
	}

	// 写操作快速返回：Tool 全部成功且都是写操作，直接用 Tool message 回复，跳过第二次 LLM
	if hasWriteToolResult(toolResults) && allToolsSucceeded(toolResults) {
		display := buildToolMessageReply(toolResults)
		result := &ChatResponse{
			Display:       display,
			AffectedPages: e.getAffectedPages(toolResults),
			DataRef:       e.collectDataRef(toolResults),
		}
		return result, nil
	}

	// 查询操作快速返回：Tool 全部成功且都是已知的查询 Tool，直接用 Tool message 回复
	// 对于自带结论的工具（如 check_budget_affordability），即使是疑问句也直接返回
	if canFastReturnRead(toolResults) && (!isQuestionMessage(req.Message) || hasConclusivedTool(toolResults)) {
		display := buildReadToolMessageReply(toolResults)
		result := &ChatResponse{
			Display: display,
			DataRef: e.collectDataRef(toolResults),
		}
		return result, nil
	}

	// ReAct 循环：后续轮次
	for step := 1; step < maxReActSteps; step++ {
		stepCtx, stepCancel := withLLMTimeout(ctx)
		// 追加简短回复指令
		stepMessages := make([]Message, len(messages))
		copy(stepMessages, messages)
		stepMessages = append(stepMessages, Message{
			Role:    "user",
			Content: "根据工具返回的数据简洁回复，不要做额外计算，不要长篇分析。",
		})
		resp, err := e.llmChatSync(stepCtx, stepMessages, tools)
		stepCancel()
		if err != nil {
			// ReAct 中间步骤失败，用已有的 Tool 结果兜底
			if len(toolResults) > 0 {
				return e.buildFromToolMessages(toolResults, req.PageContext), nil
			}
			return nil, fmt.Errorf("llm chat step %d failed: %w", step, err)
		}

		if len(resp.ToolCalls) == 0 {
			// LLM 生成了最终回复
			result := e.buildResponse(resp.Content, toolResults, req.PageContext)
			result = e.validateResponse(result, toolResults)

			// 硬约束：如果有写操作 Tool 成功执行，检查 LLM 回复中是否有 Tool 数据中不存在的金额
			// 如果有，说明 LLM 自己做了计算，用 Tool message 替换
			if hasWriteToolResult(toolResults) && containsUnverifiedAmount(result.Display, toolResults) {
				result.Display = buildToolMessageReply(toolResults)
			}

			// 硬约束：数值敏感的查询工具（预算可承受性等），LLM 不得篡改结论
			if hasNumericQueryResult(toolResults) && containsUnverifiedAmount(result.Display, toolResults) {
				result.Display = buildToolMessageReply(toolResults)
			}

			return result, nil
		}

		messages = append(messages, *resp)
		for _, tc := range resp.ToolCalls {
			tr := e.executeSingleTool(ctx, req.UserID, tc, &messages)
			if tr != nil {
				toolResults = append(toolResults, *tr)
			}
		}
	}

	// 达到最大步数，用 Tool 的 Message 拼接回复
	return e.buildFromToolMessages(toolResults, req.PageContext), nil
}

// executeSingleTool 执行单个 Tool 并更新消息列表
func (e *Engine) executeSingleTool(ctx context.Context, userID uint64, tc ToolCall, messages *[]Message) *ToolResult {
	result, err := e.toolExecutor.Execute(ctx, userID, tc)
	if err != nil {
		*messages = append(*messages, Message{
			Role:       "tool",
			Content:    fmt.Sprintf(`{"status":"failed","message":"%s"}`, err.Error()),
			ToolCallID: tc.ID,
		})
		return nil
	}

	// need_confirm 状态不保存记忆，不算成功执行
	if result.Status == "need_confirm" {
		*messages = append(*messages, Message{
			Role:       "tool",
			Content:    fmt.Sprintf(`{"status":"need_confirm","message":"%s"}`, result.Message),
			ToolCallID: tc.ID,
		})
		return result
	}

	e.saveToolMemory(ctx, userID, result)

	resultJSON, _ := json.Marshal(map[string]any{
		"status":  result.Status,
		"message": result.Message,
		"data":    result.Data,
	})
	*messages = append(*messages, Message{
		Role:       "tool",
		Content:    string(resultJSON),
		ToolCallID: tc.ID,
	})
	return result
}

// handleNoToolCall 处理 LLM 没有输出 Tool Call 的情况
func (e *Engine) handleNoToolCall(ctx context.Context, req *ChatRequest, resp *Message, messages []Message, tools []Tool, startTime time.Time) (*ChatResponse, error) {
	result := e.buildResponse(resp.Content, nil, req.PageContext)
	display := result.Display

	// 追问回答场景：上一轮 assistant 是追问，当前用户消息是短回答
	// 此时 LLM 可能没调 Tool 而是自己回复了，应该强制重试
	if isFollowUpAnswer(messages, req.Message) {
		retryMessages := make([]Message, len(messages))
		copy(retryMessages, messages)
		retryMessages = append(retryMessages, Message{
			Role:    "user",
			Content: "请根据我提供的信息，调用对应的工具完成操作。",
		})

		retryCtx, retryCancel := withLLMTimeout(ctx)
		retryResp, err := e.llmChatSync(retryCtx, retryMessages, tools)
		retryCancel()
		if err == nil && len(retryResp.ToolCalls) > 0 {
			return e.executeWorkflow(ctx, req, retryResp, retryMessages, tools, startTime)
		}
		// 重试也失败，放行 LLM 的原始回复（可能是追问或说明）
		if strings.Contains(display, "？") || strings.Contains(display, "?") {
			result = e.validateResponse(result, nil)
			return result, nil
		}
	}

	// 先检查操作声明（优先级最高）：有操作声明但没调 Tool → 走重试逻辑
	if containsOperationClaim(display) {
		retryMessages := make([]Message, len(messages))
		copy(retryMessages, messages)
		retryMessages = append(retryMessages, Message{
			Role:    "user",
			Content: "请务必调用对应的工具来查询或执行这个操作，不要自己编造数据，不要直接回复。",
		})

		retryCtx, retryCancel := withLLMTimeout(ctx)
		retryResp, err := e.llmChatSync(retryCtx, retryMessages, tools)
		retryCancel()
		if err == nil && len(retryResp.ToolCalls) > 0 {
			return e.executeWorkflow(ctx, req, retryResp, retryMessages, tools, startTime)
		}

		return &ChatResponse{Display: "抱歉，操作未能完成，请重新描述你的需求，例如：'帮我记一笔午饭30元'"}, nil
	}

	// 如果 LLM 在追问（有问号且无操作声明）→ 允许直接回复
	// 同时启动追问保护：如果用户消息含操作意图，存 PendingAction 保护上下文
	if strings.Contains(display, "？") || strings.Contains(display, "?") {
		if e.messageNeedsTool(req.Message) {
			params := map[string]any{}
			numbers := ParseChineseNumber(req.Message)
			if len(numbers) > 0 {
				params["amount"] = numbers[0]
			}
			if strings.Contains(req.Message, "收入") {
				params["bill_type"] = "income"
			} else {
				params["bill_type"] = "expense"
			}
			action := &PendingAction{
				UserID:    req.UserID,
				SessionID: req.SessionID,
				ToolName:  "create_bill",
				Params:    params,
				ExpiresAt: time.Now().Add(5 * time.Minute),
			}
			e.pending.mu.Lock()
			e.pending.store[pendingKey(req.UserID, req.SessionID)] = action
			e.pending.mu.Unlock()
		}
		result = e.validateResponse(result, nil)
		return result, nil
	}

	// 检查是否应该有 Tool Call（用户意图涉及数据操作或数据查询）
	needsTool := e.messageNeedsTool(req.Message)

	if needsTool {
		// 重试1：在消息中追加强制指令，要求 LLM 必须调用 Tool
		retryMessages := make([]Message, len(messages))
		copy(retryMessages, messages)
		retryMessages = append(retryMessages, Message{
			Role:    "user",
			Content: "请务必调用对应的工具来查询或执行这个操作，不要自己编造数据，不要直接回复。",
		})

		retryCtx2, retryCancel2 := withLLMTimeout(ctx)
		retryResp, err := e.llmChatSync(retryCtx2, retryMessages, tools)
		retryCancel2()
		if err == nil && len(retryResp.ToolCalls) > 0 {
			return e.executeWorkflow(ctx, req, retryResp, retryMessages, tools, startTime)
		}

		// 重试2：清除历史，只用 system prompt + 用户消息，排除历史干扰
		cleanMessages := []Message{
			messages[0],
			{Role: "user", Content: req.Message},
		}
		retryCtx3, retryCancel3 := withLLMTimeout(ctx)
		retryResp2, err := e.llmChatSync(retryCtx3, cleanMessages, tools)
		retryCancel3()
		if err == nil && len(retryResp2.ToolCalls) > 0 {
			return e.executeWorkflow(ctx, req, retryResp2, cleanMessages, tools, startTime)
		}

		return &ChatResponse{Display: "抱歉，操作未能完成，请重新描述你的需求，例如：'帮我记一笔午饭30元'"}, nil
	}

	// 非操作性意图（闲聊、问候等），允许直接回复
	result = e.validateResponse(result, nil)
	return result, nil
}

// isFollowUpAnswer 检测当前消息是否是对上一轮追问的回答
// 条件：上一轮 assistant 消息包含问号（追问），且当前用户消息是短回答（≤10字，无问号）
func isFollowUpAnswer(messages []Message, userMsg string) bool {
	runes := []rune(strings.TrimSpace(userMsg))
	if len(runes) > 10 || len(runes) == 0 {
		return false
	}
	if strings.Contains(userMsg, "？") || strings.Contains(userMsg, "?") {
		return false
	}
	// 找最后一条 assistant 消息
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			content := messages[i].Content
			return strings.Contains(content, "？") || strings.Contains(content, "?")
		}
		if messages[i].Role == "user" {
			break
		}
	}
	return false
}

// messageNeedsTool 判断用户消息是否可能需要 Tool Call（操作类或查询类意图）
// 用于流式路径决定是否立即推送 content chunks
func (e *Engine) messageNeedsTool(msg string) bool {
	keywords := []string{
		"花了", "消费", "买了", "支出", "收入",
		"记一笔", "记账", "帮我记",
		"设置预算", "创建预算", "调整预算", "修改预算", "删除预算",
		"添加资产", "创建资产", "修改资产", "删除资产",
		"转账",
		"分析", "统计", "汇总", "趋势", "环比", "同比",
		"花了多少", "消费了多少", "还剩多少", "预算执行", "预算情况",
		"账单", "明细", "查询", "查看",
		"净资产", "负债",
		"消费习惯", "消费模式", "消费规律",
		"能不能买", "够不够", "买得起", "超支", "预算够",
	}
	for _, kw := range keywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	// 消息中包含金额（数字+元/块/¥）→ 大概率是记账意图
	if len(ParseChineseNumber(msg)) > 0 {
		return true
	}
	return false
}

// validateResponse 校验 LLM 回复，拦截虚假操作声明
func (e *Engine) validateResponse(resp *ChatResponse, toolResults []ToolResult) *ChatResponse {
	hasSuccessTool := false
	for _, tr := range toolResults {
		if tr.Status == "success" {
			hasSuccessTool = true
			break
		}
	}

	if !hasSuccessTool && containsOperationClaim(resp.Display) {
		resp.Display = "抱歉，操作未能完成，请重新描述你的需求。"
		resp.Thinking = ""
		return resp
	}

	return resp
}

// containsOperationClaim 检查文本中是否包含操作完成的声明或自行判断重复的声明
func containsOperationClaim(display string) bool {
	claims := []string{
		"已记录", "记好了", "已创建", "已修改", "已调整", "已删除", "已设置",
		"记录了", "创建了", "修改了", "删除了", "设置了", "调整了",
		"为你记录", "帮你记录", "帮你创建", "帮你修改", "帮你删除",
		"记上了", "记好啦", "记录成功", "创建成功", "修改成功", "删除成功",
		"已存在", "已经有了", "已经存在", "已经记录", "记账已存在",
		"已为您记录", "已为你记录", "为您记过",
	}
	for _, claim := range claims {
		if strings.Contains(display, claim) {
			return true
		}
	}
	return false
}

// buildFromToolMessages 当 ReAct 达到最大步数时，用 Tool Message 拼接回复
func (e *Engine) buildFromToolMessages(toolResults []ToolResult, page string) *ChatResponse {
	var parts []string
	for _, tr := range toolResults {
		if tr.Message != "" {
			parts = append(parts, tr.Message)
		}
	}
	display := strings.Join(parts, "\n")
	if display == "" {
		display = "处理完成。"
	}
	resp := &ChatResponse{
		Display:       display,
		AffectedPages: e.getAffectedPages(toolResults),
	}
	resp.DataRef = e.collectDataRef(toolResults)
	return resp
}

// collectDataRef 从所有 ToolResult 中收集数据引用
func (e *Engine) collectDataRef(toolResults []ToolResult) map[string]any {
	allData := make(map[string]any)
	for _, tr := range toolResults {
		for k, v := range tr.Data {
			allData[k] = v
		}
	}
	return allData
}

// extractFacts 从用户输入中提取 L2 事实记忆
func (e *Engine) extractFacts(ctx context.Context, req *ChatRequest) {
	if e.memMgr == nil {
		return
	}
	factPatterns := []struct {
		keywords []string
		factType string
	}{
		{[]string{"每月", "月租", "房租", "月供"}, "monthly_expense"},
		{[]string{"工资", "月薪", "薪水", "年薪"}, "income"},
		{[]string{"信用卡", "储蓄卡", "银行卡", "花呗", "白条"}, "asset_info"},
		{[]string{"还款日", "还款", "每月还"}, "periodic"},
		{[]string{"生日", "纪念日", "周年"}, "anniversary"},
		{[]string{"打算买", "想买", "计划买", "准备买", "过几天买"}, "plan"},
		{[]string{"每周", "每天", "每年"}, "periodic"},
		{[]string{"发薪日", "发工资日", "几号发"}, "periodic"},
	}

	for _, pattern := range factPatterns {
		for _, kw := range pattern.keywords {
			if strings.Contains(req.Message, kw) {
				mem := &model.Memory{
					UserID:          req.UserID,
					Layer:           model.MemoryLayerL2,
					Content:         req.Message,
					FactType:        pattern.factType,
					Source:          model.MemorySourceUserStated,
					Metadata:        "{}",
					RelatedOpLogIDs: "[]",
				}
				if err := e.memMgr.SaveMemory(ctx, mem); err != nil {
					log.Printf("failed to save fact memory: %v", err)
				}
				return
			}
		}
	}
}

// buildRAGHint 根据用户消息检索消费模式，生成 System Prompt 提示
func (e *Engine) buildRAGHint(ctx context.Context, req *ChatRequest) string {
	return e.toolExecutor.ragFiller.BuildRAGHint(ctx, req.UserID, req.Message)
}

func (e *Engine) retrieveMemories(ctx context.Context, req *ChatRequest) (string, error) {
	if e.memMgr == nil {
		return "", nil
	}

	var parts []string

	profile, err := e.memMgr.GetUserProfile(ctx, req.UserID)
	if err == nil && len(profile) > 0 {
		var profileParts []string
		for _, m := range profile {
			profileParts = append(profileParts, m.Content)
		}
		parts = append(parts, "用户画像："+strings.Join(profileParts, "；"))
	}

	facts, err := e.memMgr.SearchFacts(ctx, req.UserID, req.Message, 5)
	if err == nil && len(facts) > 0 {
		var factParts []string
		for _, m := range facts {
			factParts = append(factParts, m.Content)
		}
		parts = append(parts, "相关事实："+strings.Join(factParts, "；"))
	}

	episodes, err := e.memMgr.SearchEpisodes(ctx, req.UserID, req.Message, 3)
	if err == nil && len(episodes) > 0 {
		var epParts []string
		for _, m := range episodes {
			epParts = append(epParts, m.Content)
		}
		parts = append(parts, "相关情景："+strings.Join(epParts, "；"))
	}

	// 对话历史已通过消息列表注入，不再放在 System Prompt 中

	return strings.Join(parts, "\n\n"), nil
}

// preloadResult 聚合所有 pre-LLM I/O 的结果
type preloadResult struct {
	memories        string
	historyMessages []Message
	ragHint         string
	skillSummary    string
}

// preloadContext 并行加载所有 pre-LLM 上下文（记忆、历史、RAG、技能）
func (e *Engine) preloadContext(ctx context.Context, req *ChatRequest) *preloadResult {
	result := &preloadResult{}
	result.skillSummary = e.skillReg.GenerateSummary(req.PageContext)

	var wg sync.WaitGroup
	wg.Add(2)

	// g1: 记忆加载（共享 Embedding，并行 Milvus 搜索）
	go func() {
		defer wg.Done()
		if e.memMgr == nil {
			return
		}
		var parts []string
		var profileDone, factsDone sync.WaitGroup
		var profile []model.Memory
		var facts, episodes []model.Memory

		profileDone.Add(1)
		go func() {
			defer profileDone.Done()
			p, err := e.memMgr.GetUserProfile(ctx, req.UserID)
			if err == nil {
				profile = p
			}
		}()

		factsDone.Add(1)
		go func() {
			defer factsDone.Done()
			f, ep, err := e.memMgr.SearchFactsAndEpisodes(ctx, req.UserID, req.Message, 5, 3)
			if err != nil {
				log.Printf("SearchFactsAndEpisodes failed: %v", err)
			}
			facts = f
			episodes = ep
		}()

		profileDone.Wait()
		if len(profile) > 0 {
			var pp []string
			for _, m := range profile {
				pp = append(pp, m.Content)
			}
			parts = append(parts, "用户画像："+strings.Join(pp, "；"))
		}

		factsDone.Wait()
		if len(facts) > 0 {
			var fp []string
			for _, m := range facts {
				fp = append(fp, m.Content)
			}
			parts = append(parts, "相关事实："+strings.Join(fp, "；"))
		}
		if len(episodes) > 0 {
			var ep []string
			for _, m := range episodes {
				ep = append(ep, m.Content)
			}
			parts = append(parts, "相关情景："+strings.Join(ep, "；"))
		}
		result.memories = strings.Join(parts, "\n\n")
	}()

	// g2: 对话历史（Redis）+ RAG 提示（MySQL）
	go func() {
		defer wg.Done()
		var histWg sync.WaitGroup
		histWg.Add(1)
		go func() {
			defer histWg.Done()
			if e.memMgr == nil {
				return
			}
			convHistory, err := e.memMgr.GetRecentConversation(ctx, req.UserID, req.SessionID, 6)
			if err == nil && len(convHistory) > 0 {
				result.historyMessages = buildHistoryMessages(convHistory)
			}
		}()

		if e.toolExecutor.ragFiller != nil {
			result.ragHint = e.toolExecutor.ragFiller.BuildRAGHint(ctx, req.UserID, req.Message)
		}

		histWg.Wait()
	}()

	wg.Wait()
	return result
}

func (e *Engine) buildSystemPrompt(page, skillSummary, memories string) string {
	var sb strings.Builder
	sb.WriteString(e.promptBase)

	now := time.Now()
	weekdays := []string{"日", "一", "二", "三", "四", "五", "六"}
	sb.WriteString(fmt.Sprintf("\n\n## 当前时间\n今天是 %s，星期%s", now.Format("2006-01-02"), weekdays[now.Weekday()]))

	sb.WriteString("\n\n## 当前页面\n")
	sb.WriteString(page)
	sb.WriteString("\n\n## 可用技能\n")
	sb.WriteString(skillSummary)
	if memories != "" {
		sb.WriteString("\n\n## 用户记忆\n")
		sb.WriteString(memories)
	} else {
		sb.WriteString("\n\n## 新用户引导\n")
		sb.WriteString("这是新用户，尚无历史数据。首次交互时友好介绍你的功能，引导用户开始记账。可以建议用户试试：记一笔消费、查看预算、了解资产状况。")
	}
	sb.WriteString("\n\n## 输出格式要求\n")
	sb.WriteString("回复简洁自然。引用数值时确保与工具返回一致。")
	return sb.String()
}

var thinkTagRegex = regexp.MustCompile(`(?s)<think>.*?</think>`)
var thinkExtractRegex = regexp.MustCompile(`(?s)<think>(.*?)</think>`)

func (e *Engine) buildResponse(content string, toolResults []ToolResult, page string) *ChatResponse {
	var thinking string
	matches := thinkExtractRegex.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) > 1 {
			if thinking != "" {
				thinking += "\n"
			}
			thinking += strings.TrimSpace(m[1])
		}
	}

	cleaned := thinkTagRegex.ReplaceAllString(content, "")
	cleaned = strings.TrimSpace(cleaned)

	resp := &ChatResponse{
		Display:  cleaned,
		Thinking: thinking,
	}

	if len(toolResults) > 0 {
		resp.DataRef = e.validateDataRef(cleaned, toolResults)
		resp.AffectedPages = e.getAffectedPages(toolResults)
	}

	return resp
}

var toolPageMap = map[string][]string{
	"create_bill":  {"账单详情", "报表", "首页"},
	"update_bill":  {"账单详情", "报表"},
	"delete_bill":  {"账单详情", "报表", "首页"},
	"create_budget": {"预算"},
	"update_budget": {"预算"},
	"delete_budget": {"预算"},
	"create_asset":  {"资产管理"},
	"update_asset":  {"资产管理"},
	"delete_asset":  {"资产管理"},
	"transfer":      {"资产管理", "账单详情"},
}

func (e *Engine) getAffectedPages(toolResults []ToolResult) []string {
	seen := make(map[string]bool)
	var pages []string
	for _, tr := range toolResults {
		if ps, ok := toolPageMap[tr.Name]; ok {
			for _, p := range ps {
				if !seen[p] {
					seen[p] = true
					pages = append(pages, p)
				}
			}
		}
	}
	return pages
}

func (e *Engine) setTiming(resp *ChatResponse, startTime time.Time) {
	endTime := time.Now()
	resp.StartTime = startTime.UnixMilli()
	resp.EndTime = endTime.UnixMilli()
	resp.DurationMs = endTime.Sub(startTime).Milliseconds()
}

func (e *Engine) persistConversation(ctx context.Context, req *ChatRequest, assistantReply string) {
	if e.memMgr == nil {
		return
	}

	// 不存储失败回复，避免污染对话历史导致 LLM 学到错误模式
	if strings.Contains(assistantReply, "操作未能完成") || strings.Contains(assistantReply, "服务暂时不可用") || strings.Contains(assistantReply, "响应超时") {
		return
	}

	now := time.Now()
	ts := now.Unix()
	todayStr := now.Format("2006-01-02")

	// 存储 assistant 回复前，将相对日期替换为绝对日期，避免后续检索时产生歧义
	normalizedReply := strings.ReplaceAll(assistantReply, "今天", todayStr)
	normalizedReply = strings.ReplaceAll(normalizedReply, "今日", todayStr)

	if err := e.memMgr.AddConversationMessage(ctx, req.UserID, req.SessionID, &memory.ConversationMessage{
		Role: "user", Content: req.Message, PageContext: req.PageContext, CreatedAt: ts,
	}); err != nil {
		log.Printf("failed to persist user message: %v", err)
	}
	if err := e.memMgr.AddConversationMessage(ctx, req.UserID, req.SessionID, &memory.ConversationMessage{
		Role: "assistant", Content: normalizedReply, PageContext: req.PageContext, CreatedAt: ts,
	}); err != nil {
		log.Printf("failed to persist assistant message: %v", err)
	}
}

// GetConversationHistory 获取用户对话历史，供 Handler 层调用
func (e *Engine) GetConversationHistory(ctx context.Context, userID, sessionID uint64, limit int) ([]memory.ConversationMessage, error) {
	if e.memMgr == nil {
		return nil, nil
	}
	return e.memMgr.GetRecentConversation(ctx, userID, sessionID, limit)
}

// saveToolMemory 在写操作 Tool 执行成功后，自动保存 L3 情景记忆
func (e *Engine) saveToolMemory(ctx context.Context, userID uint64, toolResult *ToolResult) {
	if e.memMgr == nil {
		return
	}
	writeTools := map[string]bool{
		"create_bill": true, "delete_bill": true, "update_bill": true,
		"create_budget": true, "update_budget": true, "delete_budget": true,
		"create_asset": true, "update_asset": true, "delete_asset": true,
		"transfer": true,
	}
	if !writeTools[toolResult.Name] {
		return
	}
	// 用 Tool 的 Message（结构化自然语言）作为记忆内容，便于语义检索
	content := toolResult.Message
	if content == "" {
		content = fmt.Sprintf("操作：%s", toolResult.Name)
	}
	mem := &model.Memory{
		UserID:          userID,
		Layer:           model.MemoryLayerL3,
		Content:         content,
		Source:          model.MemorySourceToolResult,
		Metadata:        "{}",
		RelatedOpLogIDs: "[]",
	}
	if err := e.memMgr.SaveMemory(ctx, mem); err != nil {
		log.Printf("failed to save tool memory: %v", err)
	}
}

func (e *Engine) validateDataRef(content string, toolResults []ToolResult) map[string]any {
	allData := make(map[string]any)
	for _, tr := range toolResults {
		for k, v := range tr.Data {
			allData[k] = v
		}
	}

	validated := make(map[string]any)
	for k, v := range allData {
		if num, ok := ToFloat64(v); ok {
			str := formatNumber(num)
			if strings.Contains(content, str) {
				validated[k] = v
			}
		} else {
			validated[k] = v
		}
	}

	return validated
}

func ToFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	}
	return 0, false
}

func formatNumber(f float64) string {
	if f == math.Trunc(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}
