package engine

import (
	"regexp"
	"strconv"
)

// 中文数字字符 → 数值映射
var chineseDigitMap = map[rune]float64{
	'零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
	'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
}

// 中文单位 → 数值映射
var chineseUnitMap = map[rune]float64{
	'十': 10, '百': 100, '千': 1000, '万': 10000, '亿': 100000000,
}

// 单位层级：用于省略单位推断（万后省略=千，千后省略=百，百后省略=十）
var unitNextLevel = map[rune]float64{
	'万': 1000, '千': 100, '百': 10, '十': 1,
}

// arabicNumberRegex 匹配阿拉伯数字（含小数）
var arabicNumberRegex = regexp.MustCompile(`\d+(?:\.\d+)?`)

// ParseChineseNumber 从字符串中提取所有数字（阿拉伯数字 + 中文数字）
func ParseChineseNumber(s string) []float64 {
	if s == "" {
		return nil
	}

	var results []float64

	// 1. 提取阿拉伯数字
	arabicMatches := arabicNumberRegex.FindAllString(s, -1)
	for _, m := range arabicMatches {
		if f, err := strconv.ParseFloat(m, 64); err == nil {
			results = append(results, f)
		}
	}

	// 2. 提取中文数字
	chineseResults := extractChineseNumbers(s)
	results = append(results, chineseResults...)

	return results
}

// extractChineseNumbers 从字符串中提取所有中文数字片段并解析
func extractChineseNumbers(s string) []float64 {
	var results []float64
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		// 跳过非中文数字字符
		if !isChineseNumberRune(runes[i]) {
			i++
			continue
		}

		// 找到一段连续的中文数字（含"块""毛""元"等货币单位）
		j := i
		for j < len(runes) && isChineseNumberOrCurrencyRune(runes[j]) {
			j++
		}

		segment := runes[i:j]
		if len(segment) > 0 {
			val := parseChineseSegment(segment)
			if val >= 0 {
				results = append(results, val)
			}
		}
		i = j
	}

	return results
}

// parseChineseSegment 解析一段中文数字 rune 序列
// 支持：三千五百二十八、两万三、一百五、九块九、三毛、半
func parseChineseSegment(runes []rune) float64 {
	if len(runes) == 0 {
		return -1
	}

	// 处理"半"
	if len(runes) == 1 && runes[0] == '半' {
		return 0.5
	}

	// 分离整数部分和小数部分（以"块""元"为分隔）
	intPart, decPart := splitByCurrency(runes)

	var result float64

	// 解析整数部分
	if len(intPart) > 0 {
		result = parseChineseInteger(intPart)
		if result < 0 {
			return -1
		}
	}

	// 解析小数部分（"块"后面的数字 → 十分位，"毛"→ 十分位）
	if len(decPart) > 0 {
		dec := parseDecimalPart(decPart)
		if dec >= 0 {
			result += dec
		}
	}

	return result
}

// splitByCurrency 以"块""元"为界分割整数部分和小数部分
func splitByCurrency(runes []rune) (intPart, decPart []rune) {
	for i, r := range runes {
		if r == '块' || r == '元' {
			intPart = runes[:i]
			if i+1 < len(runes) {
				decPart = runes[i+1:]
			}
			return
		}
	}

	// 没有"块/元"，检查是否以"毛/角"开头（如"三毛"→ 0.3）
	// 仅当整段只有一个数字+毛/角时，视为纯小数
	for i, r := range runes {
		if r == '毛' || r == '角' {
			if i == 1 {
				if _, ok := chineseDigitMap[runes[0]]; ok {
					return nil, runes[:i+1]
				}
			}
		}
	}

	return runes, nil
}

// parseDecimalPart 解析小数部分（"块"后面的中文数字 → 十分位）
func parseDecimalPart(runes []rune) float64 {
	if len(runes) == 0 {
		return 0
	}

	// "半" → 0.5
	if runes[0] == '半' {
		return 0.5
	}

	// 取第一个数字字符作为十分位
	for _, r := range runes {
		if d, ok := chineseDigitMap[r]; ok {
			return d / 10.0
		}
	}
	return 0
}

// parseChineseInteger 解析中文整数部分
// 支持省略单位推断：两万三 → 23000，一百五 → 150
func parseChineseInteger(runes []rune) float64 {
	if len(runes) == 0 {
		return 0
	}

	// 特殊处理：以"十"开头（如"十五" → 15）
	if runes[0] == '十' {
		result := 10.0
		if len(runes) > 1 {
			if d, ok := chineseDigitMap[runes[1]]; ok {
				result += d
			}
		}
		return result
	}

	return parseWithUnits(runes)
}

// parseWithUnits 解析带单位的中文数字
// 处理：三千五百二十八、两万三（省略推断）、一千五（省略推断）
func parseWithUnits(runes []rune) float64 {
	var result float64
	var current float64
	var lastUnit rune
	hasUnit := false

	for _, r := range runes {
		if d, ok := chineseDigitMap[r]; ok {
			current = d
		} else if u, ok := chineseUnitMap[r]; ok {
			hasUnit = true
			if current == 0 && r != '十' {
				// "零百" 之类不合法，跳过
				lastUnit = r
				continue
			}
			result += current * u
			current = 0
			lastUnit = r
		}
	}

	// 处理尾部数字（省略单位推断）
	if current > 0 {
		if hasUnit && lastUnit != 0 {
			// 省略单位推断：万后省略=千，千后省略=百，百后省略=十
			if nextLevel, ok := unitNextLevel[lastUnit]; ok {
				result += current * nextLevel
			} else {
				result += current
			}
		} else {
			// 没有任何单位，纯数字字符（如"五"→ 5）
			result += current
		}
	}

	return result
}

// isChineseNumberRune 判断是否为中文数字相关字符
func isChineseNumberRune(r rune) bool {
	_, isDigit := chineseDigitMap[r]
	_, isUnit := chineseUnitMap[r]
	return isDigit || isUnit || r == '半'
}

// isChineseNumberOrCurrencyRune 判断是否为中文数字或货币单位字符
func isChineseNumberOrCurrencyRune(r rune) bool {
	return isChineseNumberRune(r) || r == '块' || r == '元' || r == '毛' || r == '角'
}
