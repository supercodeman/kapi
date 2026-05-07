package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatProvider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAICompatProvider(baseURL, apiKey, model string) *OpenAICompatProvider {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &OpenAICompatProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

type openaiRequest struct {
	Model    string           `json:"model"`
	Messages []openaiMessage  `json:"messages"`
	Tools    []openaiTool     `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
}

type openaiMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiFunction `json:"function"`
}

type openaiFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type openaiToolCall struct {
	Index    int                 `json:"index"`
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function openaiToolCallFunc  `json:"function"`
}

type openaiToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}

func (p *OpenAICompatProvider) ChatSync(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	reqBody := p.buildRequest(messages, tools, false)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(respBody))
	}

	var openaiResp openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return &Message{Role: "assistant", Content: ""}, nil
	}

	choice := openaiResp.Choices[0].Message
	msg := &Message{
		Role:    "assistant",
		Content: choice.Content,
	}

	for _, tc := range choice.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return msg, nil
}

func (p *OpenAICompatProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (<-chan StreamChunk, error) {
	reqBody := p.buildRequest(messages, tools, true)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("SSE stream goroutine panic: %v", r)
			}
		}()
		defer resp.Body.Close()
		defer close(ch)

		// tool_calls 增量拼接累积器
		var pendingToolCalls []ToolCall

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				// 流结束：如果有累积的 ToolCalls，先推送
				if len(pendingToolCalls) > 0 {
					ch <- StreamChunk{ToolCalls: pendingToolCalls}
				}
				ch <- StreamChunk{Done: true}
				return
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string          `json:"content"`
						ToolCalls []openaiToolCall `json:"tool_calls"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			// 纯文本 content 逐 chunk 推送
			if delta.Content != "" {
				ch <- StreamChunk{Content: delta.Content}
			}

			// tool_calls 按 index 增量拼接
			for _, tc := range delta.ToolCalls {
				idx := tc.Index
				// 扩展 pendingToolCalls 到足够长度
				for len(pendingToolCalls) <= idx {
					pendingToolCalls = append(pendingToolCalls, ToolCall{})
				}
				// id 和 name 只取第一次非空值
				if tc.ID != "" {
					pendingToolCalls[idx].ID = tc.ID
				}
				if tc.Function.Name != "" {
					pendingToolCalls[idx].Name = tc.Function.Name
				}
				// arguments 持续追加
				pendingToolCalls[idx].Arguments += tc.Function.Arguments
			}
		}
		// scanner 结束（连接断开等）：推送累积的 ToolCalls
		if len(pendingToolCalls) > 0 {
			ch <- StreamChunk{ToolCalls: pendingToolCalls}
		}
		ch <- StreamChunk{Done: true}
	}()

	return ch, nil
}

func (p *OpenAICompatProvider) buildRequest(messages []Message, tools []Tool, stream bool) openaiRequest {
	var oaiMsgs []openaiMessage
	for _, m := range messages {
		oaiMsg := openaiMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			oaiMsg.ToolCalls = append(oaiMsg.ToolCalls, openaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openaiToolCallFunc{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		oaiMsgs = append(oaiMsgs, oaiMsg)
	}

	var oaiTools []openaiTool
	for _, t := range tools {
		oaiTools = append(oaiTools, openaiTool{
			Type: "function",
			Function: openaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	return openaiRequest{
		Model:    p.model,
		Messages: oaiMsgs,
		Tools:    oaiTools,
		Stream:   stream,
	}
}
