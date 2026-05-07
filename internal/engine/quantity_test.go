package engine

import "testing"

func TestCalcTotalFromQuantity(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want float64
	}{
		// 基本场景
		{"单价×数量", "咖啡9.9一杯 买了4杯", 39.6},
		{"买了N个", "面包5元一个 买了3个", 15},
		{"共N份", "奶茶12元 共3份", 36},
		{"中文数字数量", "买了四杯瑞幸咖啡，每一杯9.9", 39.6},
		{"带日期", "3月21日 买了四杯瑞幸咖啡，每一杯9.9", 39.6},

		// 中文数量词
		{"N瓶", "矿泉水2元一瓶 买了6瓶", 12},
		{"N包", "薯片8.5一包 买了2包", 17},
		{"三盒", "鸡蛋15一盒 买了三盒", 45},

		// 不是数量场景
		{"普通记账", "午饭30元", 0},
		{"只有金额", "咖啡15", 0},
		{"单个商品", "星巴克38元", 0},
		{"数量为1不触发", "咖啡9.9一杯 买了1杯", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcTotalFromQuantity(tt.msg)
			if got != tt.want {
				t.Errorf("calcTotalFromQuantity(%q) = %.2f, want %.2f", tt.msg, got, tt.want)
			}
		})
	}
}

func TestCalcTotalFromSum(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want float64
	}{
		{"两个不同价格", "肯德基2个汉堡，一个是8.8，一个是10.6", 19.4},
		{"一个X一个Y", "一个15一个20", 35},
		{"不是求和场景", "午饭30元", 0},
		{"单个商品", "咖啡9.9", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcTotalFromSum(tt.msg)
			if got != tt.want {
				t.Errorf("calcTotalFromSum(%q) = %.2f, want %.2f", tt.msg, got, tt.want)
			}
		})
	}
}
