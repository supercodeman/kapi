package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/6tail/lunar-go/calendar"
)

var lunarMonthMap = map[string]int{
	"正月": 1, "一月": 1, "二月": 2, "三月": 3, "四月": 4, "五月": 5,
	"六月": 6, "七月": 7, "八月": 8, "九月": 9, "十月": 10, "冬月": 11, "十一月": 11, "腊月": 12, "十二月": 12,
}

var lunarDayMap = map[string]int{
	"初一": 1, "初二": 2, "初三": 3, "初四": 4, "初五": 5,
	"初六": 6, "初七": 7, "初八": 8, "初九": 9, "初十": 10,
	"十一": 11, "十二": 12, "十三": 13, "十四": 14, "十五": 15,
	"十六": 16, "十七": 17, "十八": 18, "十九": 19, "二十": 20,
	"廿一": 21, "廿二": 22, "廿三": 23, "廿四": 24, "廿五": 25,
	"廿六": 26, "廿七": 27, "廿八": 28, "廿九": 29, "三十": 30,
}

func ConvertLunarToSolar(year, month, day int) (string, error) {
	lunar := calendar.NewLunar(year, month, day, 0, 0, 0)
	solar := lunar.GetSolar()
	return solar.ToYmd(), nil
}

func ParseLunarDate(text string) (year, month, day int, err error) {
	now := time.Now()
	year = now.Year()

	for k, v := range lunarMonthMap {
		if strings.Contains(text, k) {
			month = v
			break
		}
	}

	for k, v := range lunarDayMap {
		if strings.Contains(text, k) {
			day = v
			break
		}
	}

	if month > 0 && day == 0 {
		nums := extractNumbers(text)
		for _, n := range nums {
			if n >= 1 && n <= 30 {
				day = n
				break
			}
		}
	}

	if month == 0 || day == 0 {
		return 0, 0, 0, fmt.Errorf("cannot parse lunar date from: %s", text)
	}

	return year, month, day, nil
}

func extractNumbers(s string) []int {
	var nums []int
	var current string
	for _, r := range s {
		if r >= '0' && r <= '9' {
			current += string(r)
		} else {
			if current != "" {
				if n, err := strconv.Atoi(current); err == nil {
					nums = append(nums, n)
				}
				current = ""
			}
		}
	}
	if current != "" {
		if n, err := strconv.Atoi(current); err == nil {
			nums = append(nums, n)
		}
	}
	return nums
}
