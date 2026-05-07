package engine

import "testing"

func TestGuardAmountAnomaly(t *testing.T) {
	tests := []struct {
		name       string
		amount     float64
		maxHistory float64
		wantConfirm bool
	}{
		{"正常金额", 30, 50, false},
		{"刚好3倍", 150, 50, false},
		{"超过3倍", 151, 50, true},
		{"无历史数据", 1000, 0, false},
		{"大额但历史也大", 3000, 2000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := guardAmountAnomaly(tt.amount, "食饮", tt.maxHistory)
			if tt.wantConfirm && result == nil {
				t.Error("expected need_confirm, got nil")
			}
			if !tt.wantConfirm && result != nil {
				t.Errorf("expected nil, got %s", result.Message)
			}
		})
	}
}

func TestGuardDateAnomaly(t *testing.T) {
	tests := []struct {
		name        string
		date        string
		wantConfirm bool
	}{
		{"今天", "2026-04-30", false},
		{"昨天", "2026-04-29", false},
		{"30天前", "2026-03-31", false},
		{"31天前", "2026-03-30", true},
		{"未来日期", "2026-05-15", true},
		{"无效日期", "invalid", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := guardDateAnomaly(tt.date, 19.4, "肯德基", "食饮")
			if tt.wantConfirm && result == nil {
				t.Error("expected need_confirm, got nil")
			}
			if !tt.wantConfirm && result != nil {
				t.Errorf("expected nil, got %s", result.Message)
			}
		})
	}
}

func TestIsUserConfirming(t *testing.T) {
	tests := []struct {
		name       string
		history    []Message
		currentMsg string
		want       bool
	}{
		{
			"确认金额异常",
			[]Message{{Role: "assistant", Content: "这笔 ¥600.00 超过了教育分类历史最高金额的 3 倍，确认要记录吗？"}},
			"记录吧",
			true,
		},
		{
			"确认删除",
			[]Message{{Role: "assistant", Content: "确认要删除这笔账单吗？"}},
			"确认",
			true,
		},
		{
			"非确认场景",
			[]Message{{Role: "assistant", Content: "已记录：午饭 ¥30.00"}},
			"再记一笔",
			false,
		},
		{
			"上轮追问但用户拒绝",
			[]Message{{Role: "assistant", Content: "这笔 ¥600.00 超过了历史最高，确认要记录吗？"}},
			"算了不记了",
			false,
		},
		{
			"空历史",
			[]Message{},
			"确认",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUserConfirming(tt.history, tt.currentMsg); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
