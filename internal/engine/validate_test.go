package engine

import (
	"strings"
	"testing"
)

func TestAmountFoundInContext(t *testing.T) {
	tests := []struct {
		name      string
		amount    float64
		userMsg   string
		recent    []string
		want      bool
	}{
		{"当前消息有金额", 30, "午饭30元", nil, true},
		{"当前消息中文数字", 9.9, "咖啡九块九", nil, true},
		{"上一条消息有金额", 9.9, "饮品", []string{"咖啡9.9元"}, true},
		{"都没有金额", 50, "咖啡", []string{"好的"}, false},
		{"金额不匹配", 100, "午饭30元", nil, false},
		// 乘积匹配
		{"乘积匹配：9.9×4=39.6", 39.6, "咖啡9.9一杯买了4杯", nil, true},
		{"乘积匹配：15×3=45", 45, "鸡蛋15一盒买了3盒", nil, true},
		{"乘积匹配：LLM算对了", 36, "奶茶12元共3份", nil, true},
		{"乘积不匹配：LLM编造", 50, "咖啡9.9一杯买了4杯", nil, false},
		// 求和匹配
		{"求和匹配：8.8+10.6=19.4", 19.4, "肯德基2个汉堡一个8.8一个10.6", nil, true},
		{"求和匹配：三个数", 50, "买了苹果10香蕉20橘子20", nil, true},
		{"求和不匹配", 100, "肯德基2个汉堡一个8.8一个10.6", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := amountFoundInContext(tt.amount, tt.userMsg, tt.recent); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsOperationClaim(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"已记录：午饭 ¥30.00", true},
		{"帮你创建了预算", true},
		{"已删除账单", true},
		{"请问你要记什么？", false},
		{"本月消费 ¥1234", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := containsOperationClaim(tt.input); got != tt.want {
				t.Errorf("containsOperationClaim(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsVagueIncomeCategory(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"其他", true},
		{"其他收入", true},
		{"收入", true},
		{"", true},
		{"工资", false},
		{"兼职", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isVagueIncomeCategory(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsUnverifiedAmount(t *testing.T) {
	toolResults := []ToolResult{
		{Name: "update_budget", Status: "success", Message: "教育预算已调整为 ¥10,000.00/月。 本月已花 ¥3,877.50，剩余 ¥6,122.50。"},
	}

	tests := []struct {
		name    string
		display string
		want    bool
	}{
		{
			"LLM 转述 Tool 数据（正确）",
			"教育预算已调整为 ¥10,000.00/月。本月已花 ¥3,877.50，剩余 ¥6,122.50。",
			false,
		},
		{
			"LLM 自己算了购买后剩余（错误）",
			"教育预算已调整为 ¥10,000.00/月。购买 ¥6,000.00 的会员后，预算还剩 ¥4,000.00，足够覆盖。",
			true, // ¥4,000.00 不在 Tool 数据中
		},
		{
			"LLM 自己算了另一个错误值",
			"教育预算已调整为 ¥10,000.00/月。剩余 ¥122.50。",
			true, // ¥122.50 不在 Tool 数据中
		},
		{
			"无金额的回复",
			"教育预算已调整成功。",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsUnverifiedAmount(tt.display, toolResults); got != tt.want {
				t.Errorf("containsUnverifiedAmount = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildToolMessageReply(t *testing.T) {
	tests := []struct {
		name    string
		results []ToolResult
		want    string
	}{
		{
			"只有写操作 Tool",
			[]ToolResult{
				{Name: "update_budget", Status: "success", Message: "教育预算已调整为 ¥10,000.00/月。 本月已花 ¥3,877.50，剩余 ¥6,122.50。"},
			},
			"教育预算已调整为 ¥10,000.00/月。 本月已花 ¥3,877.50，剩余 ¥6,122.50。",
		},
		{
			"混合查询和写操作 — 应只取写操作",
			[]ToolResult{
				{Name: "list_budgets", Status: "success", Message: "查询到 7 条预算记录"},
				{Name: "update_budget", Status: "success", Message: "教育预算已调整为 ¥10,000.00/月。"},
			},
			"教育预算已调整为 ¥10,000.00/月。",
		},
		{
			"多个写操作 — 取最后一个",
			[]ToolResult{
				{Name: "create_bill", Status: "success", Message: "已记录支出 ¥600.00"},
				{Name: "update_budget", Status: "success", Message: "教育预算已调整为 ¥10,000.00/月。"},
			},
			"教育预算已调整为 ¥10,000.00/月。",
		},
		{
			"空结果",
			[]ToolResult{},
			"操作已完成。",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildToolMessageReply(tt.results)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasWriteToolResult(t *testing.T) {
	tests := []struct {
		name    string
		results []ToolResult
		want    bool
	}{
		{"有写操作", []ToolResult{{Name: "update_budget", Status: "success"}}, true},
		{"只有查询", []ToolResult{{Name: "list_budgets", Status: "success"}}, false},
		{"写操作失败", []ToolResult{{Name: "update_budget", Status: "failed"}}, false},
		{"空", []ToolResult{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasWriteToolResult(tt.results); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBudgetUpdateScenario_FullFlow(t *testing.T) {
	// 模拟完整场景：用户问预算够不够 → 调整预算 → LLM 自己算了错误值 → 被硬拦截
	toolResults := []ToolResult{
		{Name: "check_budget_affordability", Status: "success",
			Message: "教育预算 ¥7,000.00，已花 ¥3,877.50，剩余 ¥3,122.50。购买 ¥6,000.00 会超支 ¥2,877.50。建议将预算调整到 ¥10,000。"},
		{Name: "list_budgets", Status: "success",
			Message: "查询到 7 条预算记录"},
		{Name: "update_budget", Status: "success",
			Message: "教育预算已调整为 ¥10,000.00/月。 本月已花 ¥3,877.50，剩余 ¥6,122.50。"},
	}

	// LLM 自己算了 "还剩 ¥4,000"（错误：10000-6000=4000，忽略了已花的3877.50）
	llmDisplay := "教育预算已调整为 ¥10,000.00/月。购买 ¥6,000.00 的 minimax 会员后，预算还剩 ¥4,000.00，足够覆盖。"

	// 应该检测到 ¥4,000.00 不在 Tool 数据中
	if !containsUnverifiedAmount(llmDisplay, toolResults) {
		t.Error("should detect ¥4,000.00 as unverified amount")
	}

	// 硬拦截后应该用 update_budget 的 message（最后一个写操作）
	reply := buildToolMessageReply(toolResults)
	if !strings.Contains(reply, "¥6,122.50") {
		t.Errorf("reply should contain correct remaining ¥6,122.50, got: %s", reply)
	}
	if strings.Contains(reply, "查询到 7 条") {
		t.Error("reply should not contain list_budgets message")
	}
}
