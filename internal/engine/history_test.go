package engine

import (
	"testing"

	"github.com/sangchenglong/kapi/internal/memory"
)

func TestSanitizeAssistantMessage_AmountRemoval(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{
			name:    "记账确认含金额",
			input:   "已记录：今天午饭 ¥30.00，分类食饮（三餐）。",
			want:    "已记录：今天午饭 ¥***，分类食饮（三餐）。",
			changed: true,
		},
		{
			name:    "元为单位的金额",
			input:   "已记录：咖啡 9.9元，分类食饮（饮品）。",
			want:    "已记录：咖啡 ¥***，分类食饮（饮品）。",
			changed: true,
		},
		{
			name:    "块为单位的金额",
			input:   "已记录：打车 15块，分类交通。",
			want:    "已记录：打车 ¥***，分类交通。",
			changed: true,
		},
		{
			name:    "千分位金额",
			input:   "本月总支出 ¥12,345.67",
			want:    "本月总支出 ¥***",
			changed: true,
		},
		{
			name:    "追问消息不脱敏",
			input:   "请问是什么类型的收入？工资、奖金、兼职还是其他？",
			want:    "请问是什么类型的收入？工资、奖金、兼职还是其他？",
			changed: false,
		},
		{
			name:    "英文问号追问也不脱敏",
			input:   "What amount? ¥30.00",
			want:    "What amount? ¥30.00",
			changed: false,
		},
		{
			name:    "无金额的普通回复",
			input:   "好的，已帮你切换到预算页面。",
			want:    "好的，已帮你切换到预算页面。",
			changed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeAssistantMessage(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeAssistantMessage(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeAssistantMessage_StatsRemoval(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "笔数统计",
			input: "2026-04-28 至 2026-04-30 共 12 笔账单",
			want:  "2026-04-28 至 2026-04-30 [数据已省略，请通过工具查询]账单",
		},
		{
			name:  "环比增长",
			input: "本月支出环比17.8%增长",
			want:  "本月支出[数据已省略，请通过工具查询]增长",
		},
		{
			name:  "预算执行",
			input: "食饮预算已花 ¥205.00，超出预算 ¥120.00。",
			want:  "食饮预算[金额已省略]。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeAssistantMessage(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeAssistantMessage(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildHistoryMessages_LayeredInjection(t *testing.T) {
	history := []memory.ConversationMessage{
		{Role: "user", Content: "午饭30元"},
		{Role: "assistant", Content: "已记录：午饭 ¥30.00，分类食饮。"},
		{Role: "user", Content: "咖啡9.9元"},
		{Role: "assistant", Content: "已记录：咖啡 ¥9.90，分类食饮（饮品）。"},
		{Role: "user", Content: "这个月花了多少"},
		{Role: "assistant", Content: "本月总支出 ¥1,234.56，共 45 笔。"},
		{Role: "user", Content: "设置餐饮预算500元"},
		{Role: "assistant", Content: "已创建餐饮预算 ¥500.00/月。"},
		{Role: "user", Content: "今天还花了什么"},
		{Role: "assistant", Content: "今天共 3 笔消费，总计 ¥69.90。"},
	}

	messages := buildHistoryMessages(history)

	if len(messages) != 10 {
		t.Fatalf("expected 10 messages, got %d", len(messages))
	}

	// 前 4 条（第 1-2 轮）应该被脱敏
	if messages[0].Content != "午饭30元" {
		t.Errorf("user message should not be sanitized, got: %s", messages[0].Content)
	}
	if messages[1].Content == "已记录：午饭 ¥30.00，分类食饮。" {
		t.Errorf("old assistant message should be sanitized, got original: %s", messages[1].Content)
	}
	if messages[3].Content == "已记录：咖啡 ¥9.90，分类食饮（饮品）。" {
		t.Errorf("old assistant message should be sanitized, got original: %s", messages[3].Content)
	}

	// 最后 6 条（第 3-5 轮）应该完整保留
	if messages[5].Content != "本月总支出 ¥1,234.56，共 45 笔。" {
		t.Errorf("recent assistant should be preserved, got: %s", messages[5].Content)
	}
	if messages[9].Content != "今天共 3 笔消费，总计 ¥69.90。" {
		t.Errorf("most recent assistant should be preserved, got: %s", messages[9].Content)
	}
}

func TestBuildHistoryMessages_LessThan3Rounds(t *testing.T) {
	history := []memory.ConversationMessage{
		{Role: "user", Content: "午饭30元"},
		{Role: "assistant", Content: "已记录：午饭 ¥30.00，分类食饮。"},
		{Role: "user", Content: "咖啡9.9元"},
		{Role: "assistant", Content: "已记录：咖啡 ¥9.90，分类食饮（饮品）。"},
	}

	messages := buildHistoryMessages(history)

	if messages[1].Content != "已记录：午饭 ¥30.00，分类食饮。" {
		t.Errorf("with <3 rounds, all should be preserved, got: %s", messages[1].Content)
	}
	if messages[3].Content != "已记录：咖啡 ¥9.90，分类食饮（饮品）。" {
		t.Errorf("with <3 rounds, all should be preserved, got: %s", messages[3].Content)
	}
}

func TestBuildHistoryMessages_Empty(t *testing.T) {
	messages := buildHistoryMessages(nil)
	if messages != nil {
		t.Errorf("expected nil for empty history, got %v", messages)
	}
}
