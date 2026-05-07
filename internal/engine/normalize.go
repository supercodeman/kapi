package engine

import (
	"strings"
	"time"
)

var validAssetTypes = map[string]bool{
	"cash": true, "credit": true, "investment": true, "debt": true,
}

var validPeriods = map[string]bool{
	"monthly": true, "weekly": true,
}

var relativeDateMap = map[string]int{
	"今天": 0, "今日": 0,
	"昨天": -1, "昨日": -1,
	"前天": -2, "前日": -2,
	"大前天": -3,
	"明天": 1, "明日": 1,
	"后天": 2,
	"大后天": 3,
}

var weekdayMap = map[string]time.Weekday{
	"周一": time.Monday, "星期一": time.Monday,
	"周二": time.Tuesday, "星期二": time.Tuesday,
	"周三": time.Wednesday, "星期三": time.Wednesday,
	"周四": time.Thursday, "星期四": time.Thursday,
	"周五": time.Friday, "星期五": time.Friday,
	"周六": time.Saturday, "星期六": time.Saturday,
	"周日": time.Sunday, "星期日": time.Sunday, "星期天": time.Sunday,
}

// NormalizeDate 标准化日期参数
// 处理：空值默认今天、相对日期、年份修正、上周/下周
func NormalizeDate(dateStr string) string {
	now := time.Now()
	today := now.Format("2006-01-02")

	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return today
	}

	// 相对日期
	for keyword, offset := range relativeDateMap {
		if strings.Contains(dateStr, keyword) {
			return now.AddDate(0, 0, offset).Format("2006-01-02")
		}
	}

	// 上周X / 这周X / 下周X
	for keyword, wd := range weekdayMap {
		if strings.Contains(dateStr, keyword) {
			target := findWeekday(now, wd, dateStr)
			return target.Format("2006-01-02")
		}
	}

	// 尝试解析标准格式
	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		// 年份修正：如果与当前年份差距超过 1 年，修正为当前年份
		if abs(t.Year()-now.Year()) > 1 {
			t = time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		}
		return t.Format("2006-01-02")
	}

	// 无法解析，返回今天
	return today
}

func findWeekday(now time.Time, target time.Weekday, hint string) time.Time {
	current := now.Weekday()
	diff := int(target) - int(current)

	if strings.Contains(hint, "上周") || strings.Contains(hint, "上个") {
		diff -= 7
	} else if strings.Contains(hint, "下周") || strings.Contains(hint, "下个") {
		diff += 7
	} else {
		// "这周"或无前缀，取本周
		if diff < 0 {
			diff += 7
		}
	}
	return now.AddDate(0, 0, diff)
}

type CategoryResult struct {
	Category    string
	SubCategory string
}

// 一级分类列表
var validParentCategories = map[string]bool{
	"食饮": true, "居住": true, "交通": true, "通讯": true, "医疗": true,
	"购物": true, "娱乐": true, "社交": true, "教育": true, "宠物": true,
	"金融": true, "其他": true,
	"职业收入": true, "投资收入": true, "其他收入": true,
}

// 商户 → (子分类, 一级分类)
var merchantCategoryMap = map[string]CategoryResult{
	"星巴克": {"食饮", "饮品"}, "瑞幸": {"食饮", "饮品"},
	"麦当劳": {"食饮", "三餐"}, "肯德基": {"食饮", "三餐"},
	"海底捞": {"食饮", "三餐"}, "必胜客": {"食饮", "三餐"},
	"美团": {"食饮", "三餐"}, "饿了么": {"食饮", "三餐"},
	"滴滴": {"交通", "打车"}, "高德打车": {"交通", "打车"},
	"中石化": {"交通", "加油/充电"}, "中石油": {"交通", "加油/充电"},
	"淘宝": {"购物", "其他购物"}, "京东": {"购物", "数码"},
	"拼多多": {"购物", "其他购物"}, "天猫": {"购物", "其他购物"},
	"优衣库": {"购物", "服饰"}, "ZARA": {"购物", "服饰"},
	"万达影城": {"娱乐", "电影/演出"}, "猫眼": {"娱乐", "电影/演出"},
}

// 关键词 → (一级分类, 子分类)
var keywordCategoryMap = map[string]CategoryResult{
	"午饭": {"食饮", "三餐"}, "早餐": {"食饮", "三餐"}, "晚饭": {"食饮", "三餐"},
	"午餐": {"食饮", "三餐"}, "早饭": {"食饮", "三餐"}, "晚餐": {"食饮", "三餐"},
	"外卖": {"食饮", "三餐"}, "吃饭": {"食饮", "三餐"},
	"咖啡": {"食饮", "饮品"}, "奶茶": {"食饮", "饮品"}, "饮料": {"食饮", "饮品"},
	"水果": {"食饮", "零食水果"}, "零食": {"食饮", "零食水果"},
	"面包": {"食饮", "零食水果"}, "蛋糕": {"食饮", "零食水果"},
	"买菜": {"食饮", "食材"}, "超市": {"食饮", "食材"},
	"打车": {"交通", "打车"}, "出租车": {"交通", "打车"}, "网约车": {"交通", "打车"},
	"地铁": {"交通", "公共交通"}, "公交": {"交通", "公共交通"}, "公交车": {"交通", "公共交通"},
	"加油": {"交通", "加油/充电"}, "充电": {"交通", "加油/充电"},
	"停车": {"交通", "停车"}, "洗车": {"交通", "车辆保养"}, "保养": {"交通", "车辆保养"},
	"房租": {"居住", "房租/房贷"}, "房贷": {"居住", "房租/房贷"},
	"水电": {"居住", "水电燃气"}, "电费": {"居住", "水电燃气"}, "水费": {"居住", "水电燃气"}, "燃气": {"居住", "水电燃气"},
	"物业": {"居住", "物业"},
	"话费": {"通讯", "话费"}, "网费": {"通讯", "网费"}, "流量": {"通讯", "话费"},
	"会员": {"通讯", "会员订阅"}, "订阅": {"通讯", "会员订阅"},
	"看病": {"医疗", "门诊"}, "挂号": {"医疗", "门诊"},
	"药": {"医疗", "药品"}, "体检": {"医疗", "体检"}, "保险": {"医疗", "保险"},
	"衣服": {"购物", "服饰"}, "鞋": {"购物", "服饰"}, "包": {"购物", "服饰"},
	"手机": {"购物", "数码"}, "电脑": {"购物", "数码"},
	"电影": {"娱乐", "电影/演出"}, "演出": {"娱乐", "电影/演出"},
	"游戏": {"娱乐", "游戏"}, "健身": {"娱乐", "运动健身"}, "旅行": {"娱乐", "旅行"}, "旅游": {"娱乐", "旅行"},
	"聚餐": {"社交", "聚餐请客"}, "请客": {"社交", "聚餐请客"},
	"红包": {"社交", "礼物红包"}, "礼物": {"社交", "礼物红包"}, "份子钱": {"社交", "人情往来"},
	"培训": {"教育", "课程培训"}, "课程": {"教育", "课程培训"}, "书": {"教育", "书籍"},
	"信用卡还款": {"金融", "信用卡还款"}, "还款": {"金融", "信用卡还款"},
	"贷款": {"金融", "贷款还款"},
	"工资": {"职业收入", "工资"}, "奖金": {"职业收入", "奖金"}, "兼职": {"职业收入", "兼职"},
	"理财收益": {"投资收入", "理财收益"}, "分红": {"投资收入", "股票分红"},
	"退款": {"其他收入", "退款"}, "报销": {"其他收入", "报销"},
}

// NormalizeCategoryV2 标准化分类，返回一级分类和子分类
// userMessage 是用户原话，用于关键词二次推断
func NormalizeCategoryV2(category, merchant, userMessage string) CategoryResult {
	category = strings.TrimSpace(category)
	merchant = strings.TrimSpace(merchant)

	// 优先用商户匹配
	if merchant != "" {
		for key, result := range merchantCategoryMap {
			if strings.Contains(merchant, key) {
				return result
			}
		}
	}

	// 如果 category 本身是一级分类
	if validParentCategories[category] {
		return CategoryResult{Category: category}
	}

	// 关键词匹配（用 category + merchant + 用户原话 合并搜索）
	// 优先匹配更长的关键词，避免"红包"被"包"先匹配
	searchText := category + " " + merchant + " " + userMessage
	var bestMatch CategoryResult
	bestMatchLen := 0
	for keyword, result := range keywordCategoryMap {
		if strings.Contains(searchText, keyword) && len(keyword) > bestMatchLen {
			bestMatch = result
			bestMatchLen = len(keyword)
		}
	}
	if bestMatchLen > 0 {
		return bestMatch
	}

	// 兜底：如果 category 不在一级分类中，尝试模糊匹配到最近的一级分类
	categoryAliases := map[string]string{
		"日用": "购物", "生活": "居住", "餐饮": "食饮", "饮食": "食饮",
		"出行": "交通", "运动": "娱乐", "服饰": "购物", "数码": "购物",
	}
	if mapped, ok := categoryAliases[category]; ok {
		return CategoryResult{Category: mapped}
	}

	if category != "" {
		return CategoryResult{Category: category}
	}
	return CategoryResult{Category: "其他"}
}

// NormalizeCategory 兼容旧接口，只返回一级分类
func NormalizeCategory(category string) string {
	result := NormalizeCategoryV2(category, "", "")
	return result.Category
}

// NormalizePeriod 标准化周期参数
func NormalizePeriod(period string) string {
	period = strings.TrimSpace(strings.ToLower(period))
	if validPeriods[period] {
		return period
	}
	if strings.Contains(period, "周") || strings.Contains(period, "week") {
		return "weekly"
	}
	return "monthly"
}

// NormalizeAssetType 标准化资产类型
func NormalizeAssetType(assetType string) string {
	assetType = strings.TrimSpace(strings.ToLower(assetType))
	if validAssetTypes[assetType] {
		return assetType
	}
	typeAliases := map[string]string{
		"现金": "cash", "储蓄": "cash", "储蓄卡": "cash", "银行卡": "cash",
		"信用卡": "credit", "花呗": "credit",
		"投资": "investment", "基金": "investment", "股票": "investment", "理财": "investment",
		"负债": "debt", "贷款": "debt", "借款": "debt", "房贷": "debt", "车贷": "debt",
	}
	for alias, t := range typeAliases {
		if strings.Contains(assetType, alias) {
			return t
		}
	}
	return "cash"
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
