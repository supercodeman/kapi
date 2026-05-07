package engine

import (
	"regexp"
	"strings"
)

// 量词集合（记账场景常用）
const unitChars = `杯个份瓶包袋盒箱碗碟盘张套件双只条块斤两升辆艘匹`

// 匹配"买了X杯"、"共X份"等数量表达（X > 1）
var buyQtyPattern = regexp.MustCompile(`(?:买了|共|一共)\s*([一二三四五六七八九十两百\d]+)\s*[` + unitChars + `]`)

// 匹配单价表达
var unitPricePatterns = []*regexp.Regexp{
	regexp.MustCompile(`[每一]\s*[` + unitChars + `]\s*(\d+(?:\.\d+)?)`),
	regexp.MustCompile(`(\d+(?:\.\d+)?)\s*[元块]?\s*[一每/]\s*[` + unitChars + `]`),
}

func calcTotalFromQuantity(userMsg string) float64 {
	// 1. 提取数量
	qtyMatch := buyQtyPattern.FindStringSubmatch(userMsg)
	if len(qtyMatch) < 2 {
		return 0
	}
	qtyNums := ParseChineseNumber(qtyMatch[1])
	if len(qtyNums) == 0 || qtyNums[0] < 2 {
		return 0
	}
	qty := qtyNums[0]

	// 2. 提取单价
	var price float64
	for _, p := range unitPricePatterns {
		m := p.FindStringSubmatch(userMsg)
		if len(m) >= 2 {
			if f, ok := ToFloat64(m[1]); ok && f > 0 {
				price = f
				break
			}
		}
	}

	// 兜底：去掉日期后从剩余文本中找金额
	if price <= 0 {
		cleaned := regexp.MustCompile(`\d{1,4}[年月日号/-]\d{0,2}[月日号/-]?\d{0,2}[日号]?`).ReplaceAllString(userMsg, "")
		// 去掉已匹配的数量部分
		cleaned = strings.Replace(cleaned, qtyMatch[0], "", 1)
		nums := ParseChineseNumber(cleaned)
		for _, n := range nums {
			if n > 0 && n != qty {
				price = n
				break
			}
		}
	}

	if price <= 0 {
		return 0
	}

	return price * qty
}

// 多商品求和模式："一个是X，一个是Y"、"X和Y"、"X加Y"
var multiItemPatterns = []*regexp.Regexp{
	regexp.MustCompile(`一[个杯份瓶包][是]?\s*(\d+(?:\.\d+)?)[,，\s]*一[个杯份瓶包][是]?\s*(\d+(?:\.\d+)?)`),
	regexp.MustCompile(`(\d+(?:\.\d+)?)\s*[和加+]\s*(\d+(?:\.\d+)?)`),
}

// calcTotalFromSum 识别多商品求和场景，返回总金额
func calcTotalFromSum(userMsg string) float64 {
	// 去掉日期部分
	cleaned := regexp.MustCompile(`\d{1,4}[年月日号/-]\d{0,2}[月日号/-]?\d{0,2}[日号]?`).ReplaceAllString(userMsg, "")

	for _, p := range multiItemPatterns {
		matches := p.FindAllStringSubmatch(cleaned, -1)
		if len(matches) > 0 {
			var total float64
			seen := make(map[float64]bool)
			for _, m := range matches {
				for i := 1; i < len(m); i++ {
					if f, ok := ToFloat64(m[i]); ok && f > 0 && !seen[f] {
						total += f
						seen[f] = true
					}
				}
			}
			if total > 0 {
				return total
			}
		}
	}

	// 兜底：包含"N个"且有多个金额数字，求和
	if strings.Contains(userMsg, "个") || strings.Contains(userMsg, "份") {
		nums := ParseChineseNumber(cleaned)
		// 过滤掉可能是数量的整数（和"个"/"份"相邻的）
		var prices []float64
		for _, n := range nums {
			if n > 0 && n < 10000 {
				prices = append(prices, n)
			}
		}
		if len(prices) >= 2 && len(prices) <= 5 {
			// 检查是否有数量词暗示这些是不同商品的价格
			qtyMatch := buyQtyPattern.FindStringSubmatch(userMsg)
			if len(qtyMatch) >= 2 {
				qtyNums := ParseChineseNumber(qtyMatch[1])
				if len(qtyNums) > 0 && int(qtyNums[0]) == len(prices) {
					var total float64
					for _, p := range prices {
						total += p
					}
					return total
				}
			}
		}
	}

	return 0
}

// ParsedItem 表示从用户消息中解析出的一笔消费
type ParsedItem struct {
	Name   string
	Amount float64
}

// parseMultipleItems 从用户消息中识别多笔不同商品的消费
// 返回 nil 表示不是多笔场景
func parseMultipleItems(userMsg string) []ParsedItem {
	// 去掉日期
	cleaned := regexp.MustCompile(`\d{1,4}[年月日号/-]\d{0,2}[月日号/-]?\d{0,2}[日号]?`).ReplaceAllString(userMsg, "")

	// 模式1："一杯咖啡9.9，一份便当21"
	// 按逗号/顿号/分号拆分
	parts := regexp.MustCompile(`[,，;；、]`).Split(cleaned, -1)
	if len(parts) < 2 {
		return nil
	}

	var items []ParsedItem
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		nums := ParseChineseNumber(part)
		if len(nums) == 0 {
			continue
		}
		// 取最后一个数字作为金额（通常金额在末尾）
		amount := nums[len(nums)-1]
		if amount <= 0 {
			continue
		}
		// 提取商品名：去掉数字和量词前缀
		name := regexp.MustCompile(`^[一二三四五六七八九十两\d]+\s*[` + unitChars + `]\s*`).ReplaceAllString(part, "")
		name = regexp.MustCompile(`[\d.]+\s*[元块]?\s*$`).ReplaceAllString(name, "")
		name = strings.TrimSpace(name)
		if name == "" {
			name = "消费"
		}
		items = append(items, ParsedItem{Name: name, Amount: amount})
	}

	if len(items) < 2 {
		return nil
	}
	return items
}