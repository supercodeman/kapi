package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// SSEEvent 是推送给前端的 SSE 事件
type SSEEvent struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
}

// ChatStream 流式对话：Tool 调用链走同步，最终回复走流式 channel
// 返回的 channel 会依次推送：
//   - {type:"thinking", data:"正在处理..."} — 中间状态
//   - {type:"chunk", data:"文本片段"} — 流式文本
//   - {type:"done", data:"{完整 ChatResponse JSON}"} — 结束
func (e *Engine) ChatStream(ctx context.Context, req *ChatRequest) (<-chan SSEEvent, error) {
	ch := make(chan SSEEvent, 64)

	go func() {
		defer close(ch)
		e.chatStreamInternal(ctx, req, ch)
	}()

	return ch, nil
}

func (e *Engine) chatStreamInternal(ctx context.Context, req *ChatRequest, ch chan<- SSEEvent) {
	startTime := time.Now()

	if e.llm == nil {
		e.sendDone(ch, &ChatResponse{Display: "AI 服务尚未配置，请设置 LLM Provider 后重试。"}, startTime)
		return
	}

	// PendingAction 快速路径
	if isCancelMessage(req.Message) {
		e.pending.Clear(req.UserID, req.SessionID)
		resp := &ChatResponse{Display: "好的，已取消。"}
		e.setTiming(resp, startTime)
		e.persistConversation(ctx, req, resp.Display)
		e.streamText(ch, resp.Display)
		e.sendDone(ch, resp, startTime)
		return
	}
	if action := e.pending.Get(req.UserID, req.SessionID); action != nil {
		if isPendingParamAnswer(req.Message) {
			log.Printf("[PENDING] executing pending action for msg=%q, params=%v", req.Message, action.Params)
			result, err := e.executePendingAction(ctx, req, action, startTime)
			if err == nil {
				e.streamText(ch, result.Display)
				e.sendDone(ch, result, startTime)
				return
			}
			log.Printf("pending action failed in stream, falling through: %v", err)
		} else {
			log.Printf("[PENDING] msg=%q not param answer, putting back pending (params=%v)", req.Message, action.Params)
			e.pending.Set(action.UserID, action.SessionID, action.ToolName, action.Params)
		}
	} else {
		log.Printf("[PENDING] no pending action for user=%d session=%d msg=%q", req.UserID, req.SessionID, req.Message)
	}

	ch <- SSEEvent{Type: "thinking", Data: "正在理解你的意图..."}

	// 并行预加载所有上下文
	pl := e.preloadContext(ctx, req)
	systemPrompt := e.buildSystemPrompt(req.PageContext, pl.skillSummary, pl.memories)

	messages := make([]Message, 0, 2+len(pl.historyMessages))
	messages = append(messages, Message{Role: "system", Content: systemPrompt})
	messages = append(messages, pl.historyMessages...)
	messages = append(messages, Message{Role: "user", Content: req.Message})

	tools := e.toolExecutor.GetToolDefinitionsForPage(e.skillReg.GetToolsForPage(req.PageContext))

	var recentUserMsgs []string
	for i := len(pl.historyMessages) - 1; i >= 0 && len(recentUserMsgs) < 3; i-- {
		if pl.historyMessages[i].Role == "user" {
			recentUserMsgs = append(recentUserMsgs, pl.historyMessages[i].Content)
		}
	}
	ctx = ContextWithRequest(ctx, req.SessionID, req.Message, recentUserMsgs)

	if isUserConfirming(pl.historyMessages, req.Message) {
		ctx = ContextWithConfirmation(ctx)
	}

	// RAG 预检索：消费模式提示注入 System Prompt
	if pl.ragHint != "" {
		messages[0].Content += "\n\n## 消费习惯提示\n" + pl.ragHint
	}

	// 第一轮 LLM 调用（真流式）
	// 流式调用超时设置更长：LLM thinking + 回复可能超过 30s
	llmCtx, llmCancel := context.WithTimeout(ctx, 120*time.Second)
	defer llmCancel()

	// 判断是否可能需要 Tool Call（操作类意图）
	mayNeedTool := e.messageNeedsTool(req.Message)

	var fullContent strings.Builder
	var toolCalls []ToolCall

	if mayNeedTool {
		// 操作类意图：用同步调用确保 Tool Call 不丢失
		resp, err := e.llmChatSync(llmCtx, messages, tools)
		if err != nil {
			fb := llmFallbackResponse(err)
			ch <- SSEEvent{Type: "chunk", Data: fb.Display}
			e.sendDone(ch, fb, startTime)
			return
		}
		fullContent.WriteString(resp.Content)
		toolCalls = resp.ToolCalls
	} else {
		// 闲聊场景：用流式调用，边生成边推送
		streamCh, err := e.llmChatStream(llmCtx, messages, tools)
		if err != nil {
			fb := llmFallbackResponse(err)
			ch <- SSEEvent{Type: "chunk", Data: fb.Display}
			e.sendDone(ch, fb, startTime)
			return
		}

		// thinking 标签状态机
		inThinking := false
		thinkingDone := false
		var pendingBuf strings.Builder

		for chunk := range streamCh {
			if chunk.Content != "" {
				fullContent.WriteString(chunk.Content)

				if thinkingDone {
					ch <- SSEEvent{Type: "chunk", Data: chunk.Content}
				} else {
					pendingBuf.WriteString(chunk.Content)
					accumulated := pendingBuf.String()

					if !inThinking {
						if idx := strings.Index(accumulated, "<think>"); idx >= 0 {
							inThinking = true
							after := accumulated[idx+len("<think>"):]
							pendingBuf.Reset()
							if strings.Contains(after, "</think>") {
								endIdx := strings.Index(after, "</think>")
								thinkPart := after[:endIdx]
								if thinkPart != "" {
									ch <- SSEEvent{Type: "thinking", Data: strings.TrimSpace(thinkPart)}
								}
								inThinking = false
								thinkingDone = true
								rest := after[endIdx+len("</think>"):]
								rest = strings.TrimLeft(rest, "\n\r")
								if rest != "" {
									ch <- SSEEvent{Type: "chunk", Data: rest}
								}
							} else {
								after = strings.TrimLeft(after, "\n\r")
								if after != "" {
									ch <- SSEEvent{Type: "thinking", Data: after}
								}
								pendingBuf.Reset()
							}
						} else if !strings.Contains(accumulated, "<") {
							thinkingDone = true
							ch <- SSEEvent{Type: "chunk", Data: accumulated}
							pendingBuf.Reset()
						}
					} else {
						if idx := strings.Index(accumulated, "</think>"); idx >= 0 {
							thinkPart := accumulated[:idx]
							if thinkPart != "" {
								ch <- SSEEvent{Type: "thinking", Data: thinkPart}
							}
							inThinking = false
							thinkingDone = true
							rest := accumulated[idx+len("</think>"):]
							rest = strings.TrimLeft(rest, "\n\r")
							if rest != "" {
								ch <- SSEEvent{Type: "chunk", Data: rest}
							}
							pendingBuf.Reset()
						} else {
							ch <- SSEEvent{Type: "thinking", Data: accumulated}
							pendingBuf.Reset()
						}
					}
				}
			}
			if len(chunk.ToolCalls) > 0 {
				toolCalls = chunk.ToolCalls
			}
			if chunk.Done {
				break
			}
		}

		// 流结束时 thinking 仍未结束的异常处理
		if !thinkingDone && pendingBuf.Len() > 0 {
			content := thinkTagRegex.ReplaceAllString(pendingBuf.String(), "")
			content = strings.TrimSpace(content)
			if content != "" {
				ch <- SSEEvent{Type: "chunk", Data: content}
			}
		}
	}

	if len(toolCalls) > 0 {
		// 有 Tool Call → 构造 Message，进入 executeWorkflowStream
		ch <- SSEEvent{Type: "thinking", Data: "正在执行操作..."}
		resp := &Message{Role: "assistant", Content: fullContent.String(), ToolCalls: toolCalls}
		result, err := e.executeWorkflowStream(ctx, req, resp, messages, tools, startTime, ch)
		if err != nil {
			log.Printf("[STREAM] executeWorkflowStream error: %v", err)
			ch <- SSEEvent{Type: "error", Data: "操作执行失败"}
			return
		}
		e.setTiming(result, startTime)
		e.persistConversation(ctx, req, result.Display)
		e.extractFacts(ctx, req)
		go e.updateUserProfile(context.Background(), req.UserID)
		e.sendDone(ch, result, startTime)
		return
	}

	// 无 Tool Call：构造 Message，走 handleNoToolCall 逻辑
	resp := &Message{Role: "assistant", Content: fullContent.String()}
	if mayNeedTool {
		// 可能需要重试，走同步 handleNoToolCall
		result, err := e.handleNoToolCall(ctx, req, resp, messages, tools, startTime)
		if err != nil {
			ch <- SSEEvent{Type: "error", Data: "处理失败"}
			return
		}
		e.setTiming(result, startTime)
		e.persistConversation(ctx, req, result.Display)
		e.extractFacts(ctx, req)
		go e.updateUserProfile(context.Background(), req.UserID)
		e.streamText(ch, result.Display)
		e.sendDone(ch, result, startTime)
	} else {
		// 闲聊场景，content 已流式推送完毕
		result := e.buildResponse(fullContent.String(), nil, req.PageContext)
		result = e.validateResponse(result, nil)
		e.setTiming(result, startTime)
		e.persistConversation(ctx, req, result.Display)
		e.extractFacts(ctx, req)
		go e.updateUserProfile(context.Background(), req.UserID)
		e.sendDone(ch, result, startTime)
	}
}

// executeWorkflowStream 与 executeWorkflow 类似，但最终回复走真流式
func (e *Engine) executeWorkflowStream(ctx context.Context, req *ChatRequest, firstResp *Message, messages []Message, tools []Tool, startTime time.Time, ch chan<- SSEEvent) (*ChatResponse, error) {
	var toolResults []ToolResult

	// 意图纠正：如果检测到确定性意图但 LLM 调用了错误的工具，直接纠正
	// 必须在 append messages 之前执行，确保 messages 中的 ToolCall ID 与后续 tool result 一致
	correctToolCalls(req.Message, &firstResp.ToolCalls)

	messages = append(messages, *firstResp)

	// 如果有 create_bills_batch，优先执行它，跳过单笔 create_bill
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
			retryMessages := make([]Message, len(messages)-1)
			copy(retryMessages, messages[:len(messages)-1])
			retryMessages = append(retryMessages, Message{
				Role:    "user",
				Content: "用户报了多笔消费，请使用 create_bills_batch 工具一次性记录所有消费，不要遗漏任何一笔。",
			})
			retryCtx, retryCancel := withLLMTimeout(ctx)
			retryResp, retryErr := e.llm.ChatSync(retryCtx, retryMessages, tools)
			retryCancel()
			if retryErr == nil && len(retryResp.ToolCalls) > 0 {
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
					return e.executeWorkflowStream(ctx, req, retryResp, retryMessages, tools, startTime, ch)
				}
			}
		}

		if len(createBillCalls) > 1 {
			var batchParams []map[string]any
			for _, tc := range createBillCalls {
				var params map[string]any
				json.Unmarshal([]byte(tc.Arguments), &params)
				batchParams = append(batchParams, params)
			}
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

	for i, tr := range toolResults {
		if tr.Status == "need_confirm" {
			if i < len(firstResp.ToolCalls) {
				tc := firstResp.ToolCalls[i]
				var params map[string]any
				json.Unmarshal([]byte(tc.Arguments), &params)
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
				}
				action := &PendingAction{
					UserID:      req.UserID,
					SessionID:   req.SessionID,
					ToolName:    tc.Name,
					Params:      params,
					BatchParams: batchParams,
					ExpiresAt:   time.Now().Add(5 * time.Minute),
				}
				log.Printf("[PENDING] storing new pending from workflow: tool=%s params=%v", tc.Name, params)
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
		e.streamText(ch, result.Display)
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
		e.streamText(ch, result.Display)
		return result, nil
	}

	for step := 1; step < maxReActSteps; step++ {
		stepCtx, stepCancel := context.WithTimeout(ctx, 120*time.Second)

		// 追加简短回复指令，减少 thinking 时间
		streamMessages := make([]Message, len(messages))
		copy(streamMessages, messages)
		streamMessages = append(streamMessages, Message{
			Role:    "user",
			Content: "根据工具返回的数据简洁回复，不要做额外计算，不要长篇分析。",
		})

		streamCh2, err := e.llmChatStream(stepCtx, streamMessages, tools)
		if err != nil {
			stepCancel()
			return nil, fmt.Errorf("llm chat step %d failed: %w", step, err)
		}

		var stepContent strings.Builder
		var stepToolCalls []ToolCall
		// think 标签过滤（与首次调用相同逻辑）
		stepThinkingDone := false
		stepInThinking := false
		var stepPendingBuf strings.Builder

		for chunk := range streamCh2 {
			if chunk.Content != "" {
				stepContent.WriteString(chunk.Content)

				if stepThinkingDone {
					ch <- SSEEvent{Type: "chunk", Data: chunk.Content}
				} else {
					stepPendingBuf.WriteString(chunk.Content)
					accumulated := stepPendingBuf.String()

					if !stepInThinking {
						if idx := strings.Index(accumulated, "<think>"); idx >= 0 {
							stepInThinking = true
							after := accumulated[idx+len("<think>"):]
							stepPendingBuf.Reset()
							if endIdx := strings.Index(after, "</think>"); endIdx >= 0 {
								stepInThinking = false
								stepThinkingDone = true
								rest := strings.TrimLeft(after[endIdx+len("</think>"):], "\n\r")
								if rest != "" {
									ch <- SSEEvent{Type: "chunk", Data: rest}
								}
							} else {
								stepPendingBuf.Reset()
							}
						} else if !strings.Contains(accumulated, "<") {
							stepThinkingDone = true
							ch <- SSEEvent{Type: "chunk", Data: accumulated}
							stepPendingBuf.Reset()
						}
					} else {
						if idx := strings.Index(accumulated, "</think>"); idx >= 0 {
							stepInThinking = false
							stepThinkingDone = true
							rest := strings.TrimLeft(accumulated[idx+len("</think>"):], "\n\r")
							if rest != "" {
								ch <- SSEEvent{Type: "chunk", Data: rest}
							}
							stepPendingBuf.Reset()
						} else {
							stepPendingBuf.Reset()
						}
					}
				}
			}
			if len(chunk.ToolCalls) > 0 {
				stepToolCalls = chunk.ToolCalls
			}
			if chunk.Done {
				break
			}
		}
		stepCancel()

		// 流结束后处理残留
		if !stepThinkingDone && stepPendingBuf.Len() > 0 {
			content := thinkTagRegex.ReplaceAllString(stepPendingBuf.String(), "")
			content = strings.TrimSpace(content)
			if content != "" {
				ch <- SSEEvent{Type: "chunk", Data: content}
			}
		}

		if len(stepToolCalls) == 0 {
			// 最终回复已流式推送，构建 result 做验证
			fullText := stepContent.String()
			result := e.buildResponse(fullText, toolResults, req.PageContext)
			result = e.validateResponse(result, toolResults)
			// 如果验证发现金额不一致，用 Tool message 替换（此时已推送的内容可能不准确，但概率低）
			if hasNumericQueryResult(toolResults) && containsUnverifiedAmount(result.Display, toolResults) {
				result.Display = buildToolMessageReply(toolResults)
			}
			return result, nil
		}

		// 还有 Tool Call，继续 ReAct 循环
		resp := &Message{Role: "assistant", Content: stepContent.String(), ToolCalls: stepToolCalls}
		messages = append(messages, *resp)
		ch <- SSEEvent{Type: "thinking", Data: "正在继续处理..."}
		for _, tc := range stepToolCalls {
			tr := e.executeSingleTool(ctx, req.UserID, tc, &messages)
			if tr != nil {
				toolResults = append(toolResults, *tr)
			}
		}
	}

	result := e.buildFromToolMessages(toolResults, req.PageContext)
	e.streamText(ch, result.Display)
	return result, nil
}

// streamText 将文本分块推送，模拟流式输出
func (e *Engine) streamText(ch chan<- SSEEvent, text string) {
	runes := []rune(text)
	chunkSize := 4
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		ch <- SSEEvent{Type: "chunk", Data: string(runes[i:end])}
	}
}

func (e *Engine) sendDone(ch chan<- SSEEvent, result *ChatResponse, startTime time.Time) {
	e.setTiming(result, startTime)
	data, _ := json.Marshal(result)
	ch <- SSEEvent{Type: "done", Data: string(data)}
}
