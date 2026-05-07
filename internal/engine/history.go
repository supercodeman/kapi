package engine

import (
	"regexp"
	"strings"

	"github.com/sangchenglong/kapi/internal/memory"
)

// recentRounds 近期保留完整内容的对话轮数（1 轮 = 1 user + 1 assistant）
const recentRounds = 3

var (
	// 金额模式：¥30.00、¥1,200.50、30元、9.9块、30.00 元
	amountPattern = regexp.MustCompile(`¥[\d,]+\.?\d*|[\d,]+(\.\d+)?\s*[元块]`)

	// 统计模式：共12笔、总计¥205、环比17.8%、占45.2%、增长12%
	statsPattern = regexp.MustCompile(`共\s*\d+\s*笔|总[计共]?\s*¥?[\d,.]+|[环同]比[\d.]+%|占[\d.]+%|增长[\d.]+%|下降[\d.]+%`)

	// 预算执行模式：已花 ¥205.00、剩余 ¥300、超出预算 ¥120
	budgetExecPattern = regexp.MustCompile(`(?:已花|已用|剩余|超出预算?|还剩)\s*¥?[\d,.]+`)
)

// buildHistoryMessages 构建注入 LLM 的对话历史
// 近期 recentRounds 轮完整保留，更早的 assistant 消息做金额/统计脱敏
// 过滤掉失败回复及其对应的用户消息，避免 LLM 学到错误模式
func buildHistoryMessages(history []memory.ConversationMessage) []Message {
	if len(history) == 0 {
		return nil
	}

	// 先过滤掉失败回复对（user + failed assistant）
	filtered := make([]memory.ConversationMessage, 0, len(history))
	for i := 0; i < len(history); i++ {
		if history[i].Role == "assistant" && isFailedReply(history[i].Content) {
			// 跳过这条失败回复，同时移除前一条对应的 user 消息
			if len(filtered) > 0 && filtered[len(filtered)-1].Role == "user" {
				filtered = filtered[:len(filtered)-1]
			}
			continue
		}
		filtered = append(filtered, history[i])
	}

	if len(filtered) == 0 {
		return nil
	}

	// 计算近期消息的起始索引：最后 recentRounds*2 条为近期
	recentStart := len(filtered) - recentRounds*2
	if recentStart < 0 {
		recentStart = 0
	}

	messages := make([]Message, 0, len(filtered))
	for i, cm := range filtered {
		content := cm.Content
		if i < recentStart && cm.Role == "assistant" {
			content = sanitizeAssistantMessage(content)
		}
		messages = append(messages, Message{Role: cm.Role, Content: content})
	}
	return messages
}

// isFailedReply 判断 assistant 回复是否是失败/错误消息
func isFailedReply(content string) bool {
	return strings.Contains(content, "操作未能完成") ||
		strings.Contains(content, "服务暂时不可用") ||
		strings.Contains(content, "响应超时")
}

// sanitizeAssistantMessage 对远期 assistant 消息做数据脱敏
// 规则：
//  1. 包含问号的追问消息不脱敏（追问是上下文连贯的关键）
//  2. 用正则替换金额、统计数据、预算执行数据
//  3. 保留操作类型词和分类/商户名（语义上下文）
func sanitizeAssistantMessage(content string) string {
	if strings.Contains(content, "？") || strings.Contains(content, "?") {
		return content
	}

	result := content

	// 替换预算执行数据（先于金额模式，避免部分匹配）
	result = budgetExecPattern.ReplaceAllString(result, "[金额已省略]")

	// 替换统计数据
	result = statsPattern.ReplaceAllString(result, "[数据已省略，请通过工具查询]")

	// 替换金额
	result = amountPattern.ReplaceAllString(result, "¥***")

	// 清理连续的省略标记
	result = strings.ReplaceAll(result, "¥***，¥***", "¥***")
	result = strings.ReplaceAll(result, "[金额已省略]，[金额已省略]", "[金额已省略]")

	return result
}
