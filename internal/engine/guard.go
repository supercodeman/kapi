package engine

import (
	"fmt"
	"strings"
	"time"
)

// needConfirmIndicators 是 need_confirm 追问消息中的特征词
var needConfirmIndicators = []string{
	"确认要记录吗", "确认要删除", "确认要",
	"确认记录吗", "金额是多少",
	"什么类型的收入",
}

// confirmKeywords 是用户确认意图的关键词
var confirmKeywords = []string{
	"确认", "记录吧", "记吧", "好的", "可以", "是的", "对", "没错",
	"确定", "就这样", "行", "嗯", "ok", "OK", "yes",
}

// isUserConfirming 检测当前是否是用户确认上一轮 need_confirm 的场景
func isUserConfirming(historyMessages []Message, currentMsg string) bool {
	// 找到最后一条 assistant 消息
	var lastAssistant string
	for i := len(historyMessages) - 1; i >= 0; i-- {
		if historyMessages[i].Role == "assistant" {
			lastAssistant = historyMessages[i].Content
			break
		}
	}
	if lastAssistant == "" {
		return false
	}

	// 上一轮 assistant 是否是 need_confirm 追问
	isNeedConfirm := false
	for _, indicator := range needConfirmIndicators {
		if strings.Contains(lastAssistant, indicator) {
			isNeedConfirm = true
			break
		}
	}
	if !isNeedConfirm {
		return false
	}

	// 当前用户消息是否包含确认意图
	for _, kw := range confirmKeywords {
		if strings.Contains(currentMsg, kw) {
			return true
		}
	}
	return false
}

// guardAmountAnomaly 检查金额是否异常（超过该分类历史最大值 3 倍）
// 返回 need_confirm 的 ToolResult 或 nil（通过）
func guardAmountAnomaly(amount float64, category string, historyMaxAmount float64) *ToolResult {
	if historyMaxAmount <= 0 {
		return nil
	}
	if amount > historyMaxAmount*3 {
		return &ToolResult{
			Name:   "create_bill",
			Status: "need_confirm",
			Message: fmt.Sprintf("这笔 ¥%.2f 超过了%s分类历史最高金额（¥%.2f）的 3 倍，确认要记录吗？",
				amount, category, historyMaxAmount),
			Data:   map[string]any{"amount": amount, "category": category, "max_history": historyMaxAmount},
			Source: "engine",
		}
	}
	return nil
}

// guardDateAnomaly 检查日期是否异常（超过 30 天前或在未来）
// amount/merchant/category 用于在确认消息中展示完整信息
func guardDateAnomaly(dateStr string, amount float64, merchant, category string) *ToolResult {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return nil
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	detail := fmt.Sprintf("%s %s ¥%.2f", merchant, category, amount)

	if t.After(today) {
		return &ToolResult{
			Name:    "create_bill",
			Status:  "need_confirm",
			Message: fmt.Sprintf("%s，日期 %s（未来日期），确认要记录吗？", detail, dateStr),
			Data:    map[string]any{"date": dateStr, "amount": amount, "merchant": merchant, "category": category},
			Source:  "engine",
		}
	}

	daysBefore := int(today.Sub(t).Hours() / 24)
	if daysBefore > 30 {
		return &ToolResult{
			Name:    "create_bill",
			Status:  "need_confirm",
			Message: fmt.Sprintf("%s，日期 %s（%d 天前），确认要记录吗？", detail, dateStr, daysBefore),
			Data:    map[string]any{"date": dateStr, "days_ago": daysBefore, "amount": amount, "merchant": merchant, "category": category},
			Source:  "engine",
		}
	}
	return nil
}

var cancelKeywords = []string{
	"取消", "不要了", "不需要", "算了", "不记了", "不用了", "不了",
}

func isConfirmMessage(msg string) bool {
	msg = strings.TrimSpace(msg)
	if len([]rune(msg)) > 20 {
		return false
	}
	for _, kw := range confirmKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

func isCancelMessage(msg string) bool {
	msg = strings.TrimSpace(msg)
	if len([]rune(msg)) > 20 {
		return false
	}
	for _, kw := range cancelKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// isPendingParamAnswer 判断用户消息是否像参数回答
// 正向匹配：确认词、分类名、纯金额
func isPendingParamAnswer(msg string) bool {
	msg = strings.TrimSpace(msg)
	runes := []rune(msg)

	// 超过 10 个字 → 不是简短参数
	if len(runes) > 10 {
		return false
	}

	// 包含疑问词 → 是追问而非参数回答
	questionWords := []string{"吗", "呢", "？", "?", "哪些", "什么", "怎么", "如何", "为什么"}
	for _, qw := range questionWords {
		if strings.Contains(msg, qw) {
			return false
		}
	}

	// 确认词
	for _, kw := range confirmKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}

	// 纯金额（"30"、"9.9元"）
	numbers := ParseChineseNumber(msg)
	if len(numbers) > 0 && len(runes) <= 6 {
		return true
	}

	// 精确匹配已知分类名（收入+支出）
	knownCategories := map[string]bool{
		"工资": true, "奖金": true, "兼职": true, "理财": true,
		"红包": true, "退款": true, "报销": true, "其他": true,
		"餐饮": true, "交通": true, "购物": true, "娱乐": true,
		"居住": true, "医疗": true, "教育": true, "通讯": true,
		"食饮": true, "日用": true, "服饰": true, "运动": true,
		"旅行": true, "社交": true, "宠物": true,
		"职业收入": true, "投资收入": true, "其他收入": true,
	}
	if knownCategories[msg] {
		return true
	}

	return false
}
