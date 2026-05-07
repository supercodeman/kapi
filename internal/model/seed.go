package model

import "gorm.io/gorm"

func SeedCategories(db *gorm.DB) error {
	var count int64
	db.Model(&Category{}).Where("user_id = 0").Count(&count)
	if count > 0 {
		return nil
	}

	expenseCategories := []struct {
		Name      string
		Icon      string
		Necessity string
		Subs      []string
	}{
		{"食饮", "🍽", "necessary", []string{"三餐", "饮品", "零食水果", "食材"}},
		{"居住", "🏠", "necessary", []string{"房租/房贷", "水电燃气", "物业", "家居用品"}},
		{"交通", "🚗", "necessary", []string{"公共交通", "打车", "加油/充电", "停车", "车辆保养"}},
		{"通讯", "📱", "necessary", []string{"话费", "网费", "会员订阅"}},
		{"医疗", "🏥", "necessary", []string{"门诊", "药品", "体检", "保险"}},
		{"购物", "🛒", "optional", []string{"服饰", "数码", "日用品", "其他购物"}},
		{"娱乐", "🎮", "optional", []string{"电影/演出", "游戏", "运动健身", "旅行"}},
		{"社交", "🤝", "optional", []string{"聚餐请客", "礼物红包", "人情往来"}},
		{"教育", "📚", "optional", []string{"课程培训", "书籍", "考试"}},
		{"宠物", "🐾", "optional", []string{"宠物食品", "宠物医疗", "宠物用品"}},
		{"金融", "💳", "necessary", []string{"信用卡还款", "贷款还款", "理财亏损", "手续费"}},
		{"其他", "📌", "optional", nil},
	}

	var order int
	for _, cat := range expenseCategories {
		order++
		parent := Category{
			UserID: 0, ParentID: 0, Name: cat.Name, Icon: cat.Icon,
			BillType: "expense", Necessity: cat.Necessity, SortOrder: order,
		}
		if err := db.Create(&parent).Error; err != nil {
			return err
		}
		for _, sub := range cat.Subs {
			order++
			child := Category{
				UserID: 0, ParentID: parent.ID, Name: sub, Icon: "",
				BillType: "expense", Necessity: cat.Necessity, SortOrder: order,
			}
			if err := db.Create(&child).Error; err != nil {
				return err
			}
		}
	}

	incomeCategories := []struct {
		Name string
		Subs []string
	}{
		{"职业收入", []string{"工资", "奖金", "兼职"}},
		{"投资收入", []string{"理财收益", "股票分红", "房租收入"}},
		{"其他收入", []string{"红包", "退款", "报销"}},
	}

	for _, cat := range incomeCategories {
		order++
		parent := Category{
			UserID: 0, ParentID: 0, Name: cat.Name, Icon: "💰",
			BillType: "income", SortOrder: order,
		}
		if err := db.Create(&parent).Error; err != nil {
			return err
		}
		for _, sub := range cat.Subs {
			order++
			child := Category{
				UserID: 0, ParentID: parent.ID, Name: sub, Icon: "",
				BillType: "income", SortOrder: order,
			}
			if err := db.Create(&child).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
