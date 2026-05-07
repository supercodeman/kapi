package engine

import (
	"context"
	"strings"
	"time"
)

const llmTimeout = 30 * time.Second

// withLLMTimeout 为 LLM 调用创建带超时的 context
func withLLMTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, llmTimeout)
}

// isTimeoutError 判断是否是超时错误
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "context deadline exceeded") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "Timeout")
}

// llmFallbackResponse 生成 LLM 不可用时的降级响应
func llmFallbackResponse(err error) *ChatResponse {
	if isTimeoutError(err) {
		return &ChatResponse{Display: "AI 响应超时了，请稍后再试，或者简化一下你的问题。"}
	}
	return &ChatResponse{Display: "AI 服务暂时不可用，请稍后再试。"}
}
