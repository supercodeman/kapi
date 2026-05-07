package engine

import "testing"

func TestNormalizeCategoryV2_MerchantMatch(t *testing.T) {
	tests := []struct {
		category, merchant string
		wantCat, wantSub   string
	}{
		{"", "星巴克", "食饮", "饮品"},
		{"", "肯德基", "食饮", "三餐"},
		{"", "滴滴出行", "交通", "打车"},
		{"", "中石化加油站", "交通", "加油/充电"},
	}
	for _, tt := range tests {
		t.Run(tt.merchant, func(t *testing.T) {
			r := NormalizeCategoryV2(tt.category, tt.merchant, "")
			if r.Category != tt.wantCat || r.SubCategory != tt.wantSub {
				t.Errorf("got %s/%s, want %s/%s", r.Category, r.SubCategory, tt.wantCat, tt.wantSub)
			}
		})
	}
}

func TestNormalizeCategoryV2_KeywordMatch(t *testing.T) {
	tests := []struct {
		name, category, userMsg string
		wantCat                 string
	}{
		{"午饭关键词", "食饮", "午饭30元", "食饮"},
		{"咖啡关键词", "", "咖啡9.9", "食饮"},
		{"打车关键词", "", "打车去公司", "交通"},
		{"房租关键词", "", "交房租3500", "居住"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NormalizeCategoryV2(tt.category, "", tt.userMsg)
			if r.Category != tt.wantCat {
				t.Errorf("got %s, want %s", r.Category, tt.wantCat)
			}
		})
	}
}

func TestNormalizeCategoryV2_Alias(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"餐饮", "食饮"},
		{"日用", "购物"},
		{"出行", "交通"},
		{"unknown_cat", "unknown_cat"},
		{"", "其他"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := NormalizeCategoryV2(tt.input, "", "")
			if r.Category != tt.want {
				t.Errorf("got %s, want %s", r.Category, tt.want)
			}
		})
	}
}

func TestNormalizeDate(t *testing.T) {
	tests := []struct {
		name, input string
		notEmpty    bool
	}{
		{"空值返回今天", "", true},
		{"标准格式", "2026-04-30", true},
		{"今天", "今天", true},
		{"昨天", "昨天", true},
		{"无法解析", "abc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeDate(tt.input)
			if tt.notEmpty && result == "" {
				t.Error("expected non-empty date")
			}
		})
	}
}

func TestNormalizeAssetType(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"cash", "cash"},
		{"credit", "credit"},
		{"现金", "cash"},
		{"信用卡", "credit"},
		{"基金", "investment"},
		{"房贷", "debt"},
		{"unknown", "cash"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeAssetType(tt.input); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNormalizePeriod(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"monthly", "monthly"},
		{"weekly", "weekly"},
		{"周", "weekly"},
		{"", "monthly"},
		{"random", "monthly"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizePeriod(tt.input); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
