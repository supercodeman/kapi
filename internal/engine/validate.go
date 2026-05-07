package engine

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// amountFoundInContext 检查金额是否能在用户消息上下文中找到
// 三层校验：直接匹配 → 乘积匹配 → 上一条消息匹配
func amountFoundInContext(amount float64, userMsg string, recentUserMsgs []string) bool {
	numbers := ParseChineseNumber(userMsg)

	// 第一层：直接匹配
	for _, n := range numbers {
		if amountMatch(n, amount) {
			return true
		}
	}

	// 第二层：乘积匹配（单价×数量场景的兜底）
	if len(numbers) >= 2 {
		for i := 0; i < len(numbers); i++ {
			for j := 0; j < len(numbers); j++ {
				if i != j && numbers[i] > 0 && numbers[j] > 1 {
					if amountMatch(numbers[i]*numbers[j], amount) {
						return true
					}
				}
			}
		}
	}

	// 第三层：求和匹配（多商品相加场景的兜底）
	// 检查 amount 是否等于消息中若干数字的和（2~5个数字的组合）
	if len(numbers) >= 2 && amountMatchesSum(numbers, amount) {
		return true
	}

	// 第三层：上一条用户消息
	if len(recentUserMsgs) > 0 {
		prevNumbers := ParseChineseNumber(recentUserMsgs[0])
		for _, n := range prevNumbers {
			if amountMatch(n, amount) {
				return true
			}
		}
	}

	return false
}

func amountMatch(found, expected float64) bool {
	if found == expected {
		return true
	}
	diff := math.Abs(found - expected)
	return diff < 0.01
}

// amountMatchesSum 检查 target 是否等于 numbers 中 2~5 个数字的和
func amountMatchesSum(numbers []float64, target float64) bool {
	n := len(numbers)
	if n < 2 || n > 20 {
		return false
	}
	// 限制组合数量，只检查 2~4 个数字的组合
	maxK := 4
	if n < maxK {
		maxK = n
	}
	for k := 2; k <= maxK; k++ {
		if checkSumCombination(numbers, k, 0, 0, target) {
			return true
		}
	}
	return false
}

func checkSumCombination(numbers []float64, k, start int, currentSum, target float64) bool {
	if k == 0 {
		return amountMatch(currentSum, target)
	}
	for i := start; i <= len(numbers)-k; i++ {
		if checkSumCombination(numbers, k-1, i+1, currentSum+numbers[i], target) {
			return true
		}
	}
	return false
}

func isVagueIncomeCategory(category string) bool {
	vague := map[string]bool{
		"其他": true, "其他收入": true, "收入": true, "": true,
	}
	return vague[category]
}

var writeToolNames = map[string]bool{
	"create_bill": true, "update_bill": true, "delete_bill": true,
	"create_budget": true, "update_budget": true, "delete_budget": true,
	"create_asset": true, "update_asset": true, "delete_asset": true,
	"transfer": true,
}

// hasWriteToolResult 检查是否有写操作 Tool 成功执行
func hasWriteToolResult(toolResults []ToolResult) bool {
	for _, tr := range toolResults {
		if tr.Status == "success" && writeToolNames[tr.Name] {
			return true
		}
	}
	return false
}

// allToolsSucceeded 检查所有 Tool 是否都执行成功
func allToolsSucceeded(toolResults []ToolResult) bool {
	for _, tr := range toolResults {
		if tr.Status != "success" {
			return false
		}
	}
	return len(toolResults) > 0
}

// numericQueryTools 数值敏感的查询工具，LLM 不得对其结果做二次计算
var numericQueryTools = map[string]bool{
	"check_budget_affordability": true,
	"get_budget_execution":       true,
	"get_category_summary":       true,
	"get_month_comparison":       true,
}

// hasNumericQueryResult 检查是否有数值敏感的查询工具成功执行
func hasNumericQueryResult(toolResults []ToolResult) bool {
	for _, tr := range toolResults {
		if tr.Status == "success" && numericQueryTools[tr.Name] {
			return true
		}
	}
	return false
}

var amountRegex = regexp.MustCompile(`¥([\d,]+\.?\d*)`)

// containsUnverifiedAmount 检查 LLM 回复中是否包含 Tool 数据中不存在的金额
func containsUnverifiedAmount(display string, toolResults []ToolResult) bool {
	// 收集所有 Tool message 中的金额
	var toolAmounts []float64
	for _, tr := range toolResults {
		for _, m := range amountRegex.FindAllStringSubmatch(tr.Message, -1) {
			if len(m) > 1 {
				s := strings.ReplaceAll(m[1], ",", "")
				if f, err := strconv.ParseFloat(s, 64); err == nil {
					toolAmounts = append(toolAmounts, f)
				}
			}
		}
	}

	// 计算 Tool 金额的总和（允许 LLM 展示合计）
	var toolSum float64
	for _, a := range toolAmounts {
		toolSum += a
	}

	// 提取 LLM 回复中的金额
	for _, m := range amountRegex.FindAllStringSubmatch(display, -1) {
		if len(m) > 1 {
			s := strings.ReplaceAll(m[1], ",", "")
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				continue
			}
			found := false
			for _, ta := range toolAmounts {
				if math.Abs(f-ta) < 0.01 {
					found = true
					break
				}
			}
			// 允许 Tool 金额的总和
			if !found && len(toolAmounts) > 1 && math.Abs(f-toolSum) < 0.01 {
				found = true
			}
			if !found {
				return true
			}
		}
	}
	return false
}

// buildToolMessageReply 用写操作 Tool 的 message 作为回复
func buildToolMessageReply(toolResults []ToolResult) string {
	// 收集所有写操作 Tool 的成功消息
	var writeMessages []string
	for _, tr := range toolResults {
		if tr.Status == "success" && writeToolNames[tr.Name] && tr.Message != "" {
			writeMessages = append(writeMessages, tr.Message)
		}
	}
	if len(writeMessages) > 0 {
		return strings.Join(writeMessages, "\n")
	}
	// 兜底：取最后一个成功 Tool 的 message
	for i := len(toolResults) - 1; i >= 0; i-- {
		if toolResults[i].Status == "success" && toolResults[i].Message != "" {
			return toolResults[i].Message
		}
	}
	return "操作已完成。"
}

// readToolsForFastReturn 可以直接用 Message 回复的查询类 Tool
var readToolsForFastReturn = map[string]bool{
	"list_bills":                  true,
	"get_category_summary":        true,
	"get_budget_execution":        true,
	"get_month_comparison":        true,
	"get_total_spent":             true,
	"check_budget_affordability":  true,
}

// conclusivedTools 自带结论的工具，即使用户是疑问句也可以直接返回
var conclusivedTools = map[string]bool{
	"check_budget_affordability": true,
}

// hasConclusivedTool 检查 toolResults 中是否包含自带结论的工具
func hasConclusivedTool(toolResults []ToolResult) bool {
	for _, tr := range toolResults {
		if conclusivedTools[tr.Name] {
			return true
		}
	}
	return false
}

// buildReadToolMessageReply 用查询类 Tool 的 message 拼接回复
func buildReadToolMessageReply(toolResults []ToolResult) string {
	var parts []string
	for _, tr := range toolResults {
		if tr.Status == "success" && tr.Message != "" {
			parts = append(parts, tr.Message)
		}
	}
	if len(parts) == 0 {
		return "没有找到相关记录。"
	}
	return strings.Join(parts, "\n\n")
}

// canFastReturnRead 判断查询结果是否可以直接快速返回
func canFastReturnRead(toolResults []ToolResult) bool {
	if len(toolResults) == 0 {
		return false
	}
	for _, tr := range toolResults {
		if tr.Status != "success" {
			return false
		}
		if !readToolsForFastReturn[tr.Name] {
			return false
		}
	}
	return true
}
