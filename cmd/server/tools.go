package main

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/sangchenglong/kapi/internal/analytics"
	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/engine"
	"github.com/sangchenglong/kapi/internal/service"
	"github.com/sangchenglong/kapi/internal/utils"
)

// ToolDeps 注册 Tool 所需的依赖
type ToolDeps struct {
	AnalyticsEng *analytics.CachedEngine
	OpLogSvc     *service.OpLogService
	PatternDAO   *dao.PatternDAO
	BillSvc      *service.BillService
	AssetSvc     *service.AssetService
}

// RegisterTools 注册所有扩展 Tool（Analytics、预算、模式管理等）
func RegisterTools(toolExec *engine.ToolExecutor, deps ToolDeps) {
	registerAnalyticsTools(toolExec, deps)
	registerBudgetTools(toolExec, deps)
	registerUtilTools(toolExec)
	registerBatchTools(toolExec, deps)
	registerPatternTools(toolExec, deps)
}

// registerAnalyticsTools 注册分析类 Tool
func registerAnalyticsTools(toolExec *engine.ToolExecutor, deps ToolDeps) {
	// get_category_summary: 分类消费汇总
	toolExec.Register("get_category_summary", "获取指定时间范围内的分类消费汇总", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"start_date": map[string]any{"type": "string", "description": "开始日期 YYYY-MM-DD"},
			"end_date":   map[string]any{"type": "string", "description": "结束日期 YYYY-MM-DD"},
		},
		"required": []string{"start_date", "end_date"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		startDate, _ := args["start_date"].(string)
		endDate, _ := args["end_date"].(string)
		summary, err := deps.AnalyticsEng.GetCategorySummary(ctx, userID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		msg := analytics.GenerateSpendingReport(summary, startDate, endDate)
		return &engine.ToolResult{Name: "get_category_summary", Status: "success", Message: msg, Data: map[string]any{"summary": summary}, Source: "mysql"}, nil
	})

	// get_month_comparison: 月度环比对比
	toolExec.Register("get_month_comparison", "获取月度环比对比数据", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"year":  map[string]any{"type": "integer", "description": "年份"},
			"month": map[string]any{"type": "integer", "description": "月份"},
		},
		"required": []string{"year", "month"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		year, _ := engine.ToFloat64(args["year"])
		month, _ := engine.ToFloat64(args["month"])
		comp, err := deps.AnalyticsEng.GetMonthComparison(ctx, userID, int(year), int(month))
		if err != nil {
			return nil, err
		}

		msg := fmt.Sprintf("%d年%d月支出 ¥%.2f，上月 ¥%.2f，环比%.1f%%",
			int(year), int(month), comp.CurrentTotal, comp.PreviousTotal, comp.GrowthRate*100)

		return &engine.ToolResult{Name: "get_month_comparison", Status: "success", Message: msg, Data: map[string]any{
			"current_total": comp.CurrentTotal, "previous_total": comp.PreviousTotal,
			"diff": comp.Diff, "growth_rate": comp.GrowthRate,
		}, Source: "mysql"}, nil
	})

	// get_budget_execution: 预算执行情况
	toolExec.Register("get_budget_execution", "获取预算执行情况", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"year":  map[string]any{"type": "integer", "description": "年份"},
			"month": map[string]any{"type": "integer", "description": "月份"},
		},
		"required": []string{"year", "month"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		year, _ := engine.ToFloat64(args["year"])
		month, _ := engine.ToFloat64(args["month"])
		exec, err := deps.AnalyticsEng.GetBudgetExecution(ctx, userID, int(year), int(month))
		if err != nil {
			return nil, err
		}
		msg := fmt.Sprintf("%d年%d月预算执行情况：\n", int(year), int(month))
		if len(exec) == 0 {
			msg = fmt.Sprintf("%d年%d月没有找到预算记录", int(year), int(month))
		} else {
			overCount := 0
			activeCount := 0
			for _, e := range exec {
				status := "正常"
				if e.SpentAmount > e.BudgetAmount {
					status = "超支"
					overCount++
				}
				if e.SpentAmount > 0 {
					activeCount++
				}
				msg += fmt.Sprintf("- %s：预算 ¥%.2f，已花 ¥%.2f，剩余 ¥%.2f，执行率 %.1f%%（%s）\n",
					e.Category, e.BudgetAmount, e.SpentAmount, e.Remaining, e.ExecutionRate, status)
			}
			msg += fmt.Sprintf("\n汇总：共 %d 项预算，%d 项有支出，%d 项超支。",
				len(exec), activeCount, overCount)
		}

		return &engine.ToolResult{Name: "get_budget_execution", Status: "success", Message: msg, Data: map[string]any{"executions": exec}, Source: "mysql"}, nil
	})

	// get_operation_history: 操作历史记录
	toolExec.Register("get_operation_history", "获取用户操作历史记录", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{"type": "integer", "description": "返回条数，默认20"},
		},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		limit := 20
		if l, ok := engine.ToFloat64(args["limit"]); ok {
			limit = int(l)
		}
		logs, err := deps.OpLogSvc.GetHistory(ctx, userID, limit)
		if err != nil {
			return nil, err
		}

		msg := fmt.Sprintf("查询到 %d 条操作记录", len(logs))
		if len(logs) == 0 {
			msg = "没有找到操作历史记录"
		}

		return &engine.ToolResult{Name: "get_operation_history", Status: "success", Message: msg, Data: map[string]any{"logs": logs, "count": len(logs)}, Source: "mysql"}, nil
	})
	// get_total_spent: 总消费金额
	toolExec.Register("get_total_spent", "获取指定时间范围内的总消费金额", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"start_date": map[string]any{"type": "string", "description": "开始日期 YYYY-MM-DD"},
			"end_date":   map[string]any{"type": "string", "description": "结束日期 YYYY-MM-DD"},
		},
		"required": []string{"start_date", "end_date"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		startDate, _ := args["start_date"].(string)
		endDate, _ := args["end_date"].(string)
		total, err := deps.AnalyticsEng.GetTotalSpent(ctx, userID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		return &engine.ToolResult{Name: "get_total_spent", Status: "success", Message: fmt.Sprintf("%s 至 %s 总支出 ¥%.2f", startDate, endDate, total), Data: map[string]any{"total_spent": total, "start_date": startDate, "end_date": endDate}, Source: "mysql"}, nil
	})
}

// registerBudgetTools 注册预算类 Tool
func registerBudgetTools(toolExec *engine.ToolExecutor, deps ToolDeps) {
	// check_budget_affordability: 预算可承受性检查
	toolExec.Register("check_budget_affordability", "检查预算是否足够支付一笔计划消费，返回剩余/超支金额和建议", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category":       map[string]any{"type": "string", "description": "消费分类"},
			"planned_amount": map[string]any{"type": "number", "description": "计划消费金额"},
			"amount":         map[string]any{"type": "number", "description": "计划消费金额（同 planned_amount）"},
		},
		"required": []string{"category", "planned_amount"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		category, _ := args["category"].(string)
		plannedAmount, ok := engine.ToFloat64(args["planned_amount"])
		if !ok || plannedAmount == 0 {
			// 容错：LM 可能传了 amount 而不是 planned_amount
			plannedAmount, _ = engine.ToFloat64(args["amount"])
		}

		now := time.Now()
		exec, err := deps.AnalyticsEng.GetBudgetExecution(ctx, userID, now.Year(), int(now.Month()))
		if err != nil {
			return nil, err
		}
		var matched *analytics.BudgetExecution
		for i, e := range exec {
			if strings.Contains(e.Category, category) || strings.Contains(category, e.Category) {
				matched = &exec[i]
				break
			}
		}

		if matched == nil {
			return &engine.ToolResult{
				Name: "check_budget_affordability", Status: "success",
				Message: fmt.Sprintf("没有找到%s相关的预算，无法评估", category),
				Data:    map[string]any{"has_budget": false},
				Source:  "mysql",
			}, nil
		}

		afterRemaining := matched.Remaining - plannedAmount
		affordable := afterRemaining >= 0

		var msg string
		budgetLabel := matched.Category
		if !strings.HasSuffix(budgetLabel, "预算") {
			budgetLabel += "预算"
		}
		if affordable {
			msg = fmt.Sprintf("%s ¥%.2f，已花 ¥%.2f，剩余 ¥%.2f。购买 ¥%.2f 后还剩 ¥%.2f，预算充足。",
				budgetLabel, matched.BudgetAmount, matched.SpentAmount, matched.Remaining,
				plannedAmount, afterRemaining)
		} else {
			overAmount := -afterRemaining
			suggestedBudget := math.Ceil((matched.SpentAmount+plannedAmount)/1000) * 1000
			msg = fmt.Sprintf("%s ¥%.2f，已花 ¥%.2f，剩余 ¥%.2f。购买 ¥%.2f 会超支 ¥%.2f。建议将预算调整到 ¥%.0f。",
				budgetLabel, matched.BudgetAmount, matched.SpentAmount, matched.Remaining,
				plannedAmount, overAmount, suggestedBudget)
		}

		return &engine.ToolResult{
			Name: "check_budget_affordability", Status: "success", Message: msg,
			Data: map[string]any{
				"budget_name":     matched.Category,
				"budget_amount":   matched.BudgetAmount,
				"spent_amount":    matched.SpentAmount,
				"remaining":       matched.Remaining,
				"planned_amount":  plannedAmount,
				"after_remaining": afterRemaining,
				"affordable":      affordable,
			},
			Source: "mysql",
		}, nil
	})
}

// registerUtilTools 注册工具类 Tool
func registerUtilTools(toolExec *engine.ToolExecutor) {
	// convert_lunar_date: 农历转公历
	toolExec.Register("convert_lunar_date", "将农历/阴历日期转换为公历日期", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"lunar_text": map[string]any{"type": "string", "description": "农历日期文本，如'二月初五'、'腊月二十三'"},
			"year":       map[string]any{"type": "integer", "description": "年份，默认今年"},
		},
		"required": []string{"lunar_text"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		lunarText, _ := args["lunar_text"].(string)
		year, month, day, err := utils.ParseLunarDate(lunarText)
		if err != nil {
			return nil, err
		}
		if y, ok := engine.ToFloat64(args["year"]); ok && y > 0 {
			year = int(y)
		}
		solarDate, err := utils.ConvertLunarToSolar(year, month, day)
		if err != nil {
			return nil, err
		}
		return &engine.ToolResult{
			Name:    "convert_lunar_date",
			Status:  "success",
			Message: fmt.Sprintf("农历%d月%d日 对应公历 %s", month, day, solarDate),
			Data:    map[string]any{"lunar": fmt.Sprintf("农历%d月%d日", month, day), "solar": solarDate, "year": year},
			Source:  "computed",
		}, nil
	})
}

// registerBatchTools 注册批量操作类 Tool
func registerBatchTools(toolExec *engine.ToolExecutor, deps ToolDeps) {
	// create_bills_batch: 批量记账
	toolExec.Register("create_bills_batch", "批量记录多笔不同的消费（用户一次性报了多笔不同商品的消费时使用）", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"amount":   map[string]any{"type": "number", "description": "金额"},
						"category": map[string]any{"type": "string", "description": "分类"},
						"merchant": map[string]any{"type": "string", "description": "商户/商品名"},
					},
				},
				"description": "消费明细列表",
			},
			"date": map[string]any{"type": "string", "description": "日期 YYYY-MM-DD"},
		},
		"required": []string{"items"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		rawItems, ok := args["items"].([]any)
		if !ok || len(rawItems) == 0 {
			return nil, fmt.Errorf("items is required")
		}
		date := engine.NormalizeDate(engine.GetString(args, "date"))

		var msgParts []string
		var total float64
		var batchParams []map[string]any

		for i, raw := range rawItems {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			amount, _ := engine.ToFloat64(item["amount"])
			if amount <= 0 {
				continue
			}
			merchant, _ := item["merchant"].(string)
			category, _ := item["category"].(string)
			catResult := engine.NormalizeCategoryV2(category, merchant, merchant)

			batchParams = append(batchParams, map[string]any{
				"bill_type": "expense",
				"amount":    amount,
				"category":  catResult.Category,
				"merchant":  merchant,
				"date":      date,
			})
			msgParts = append(msgParts, fmt.Sprintf("  %d. %s ¥%.2f（%s）", i+1, merchant, amount, catResult.Category))
			total += amount
		}

		if len(batchParams) == 0 {
			return nil, fmt.Errorf("no valid items")
		}

		// 日期异常提示（合并到确认消息中）
		dateHint := ""
		if date != "" {
			t, err := time.ParseInLocation("2006-01-02", date, time.Local)
			if err == nil {
				now := time.Now()
				today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
				if t.After(today) {
					dateHint = fmt.Sprintf("\n⚠️ 日期 %s（未来日期）", date)
				} else if days := int(today.Sub(t).Hours() / 24); days > 30 {
					dateHint = fmt.Sprintf("\n⚠️ 日期 %s（%d 天前）", date, days)
				}
			}
		}

		msg := fmt.Sprintf("检测到 %d 笔消费，共 ¥%.2f：\n%s%s\n确认全部记录吗？",
			len(batchParams), total, strings.Join(msgParts, "\n"), dateHint)

		return &engine.ToolResult{
			Name: "create_bills_batch", Status: "need_confirm", Message: msg,
			Data:   map[string]any{"_batch_params": batchParams, "batch_count": len(batchParams), "total": total},
			Source: "engine",
		}, nil
	})
}

// registerPatternTools 注册消费模式管理类 Tool
func registerPatternTools(toolExec *engine.ToolExecutor, deps ToolDeps) {
	// list_my_patterns: 查看消费模式
	toolExec.Register("list_my_patterns", "查看系统记住的用户消费模式和习惯", map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		patterns, err := deps.PatternDAO.ListByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		if len(patterns) == 0 {
			return &engine.ToolResult{Name: "list_my_patterns", Status: "success", Message: "目前还没有记住你的消费习惯，多记几笔账后我会自动学习", Data: map[string]any{"patterns": []any{}, "count": 0}, Source: "mysql"}, nil
		}
		msg := fmt.Sprintf("已记住你 %d 个消费习惯：\n\n", len(patterns))
		for _, p := range patterns {
			name := p.Category
			if p.Merchant != "" {
				name = p.Merchant + "（" + p.Category + "）"
			}
			msg += fmt.Sprintf("- %s：上次 ¥%.2f，共 %d 次\n", name, p.LastAmount, p.Count)
		}
		items := make([]map[string]any, 0, len(patterns))
		for _, p := range patterns {
			items = append(items, map[string]any{
				"id": p.ID, "merchant": p.Merchant, "category": p.Category,
				"last_amount": p.LastAmount, "count": p.Count, "frequency": p.Frequency,
			})
		}
		return &engine.ToolResult{Name: "list_my_patterns", Status: "success", Message: msg, Data: map[string]any{"patterns": items, "count": len(patterns)}, Source: "mysql"}, nil
	})

	// delete_pattern: 删除/禁用消费模式
	toolExec.Register("delete_pattern", "删除或禁用指定的消费模式", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern_id": map[string]any{"type": "integer", "description": "消费模式ID"},
		},
		"required": []string{"pattern_id"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*engine.ToolResult, error) {
		patternID, ok := engine.ToFloat64(args["pattern_id"])
		if !ok || patternID <= 0 {
			return nil, fmt.Errorf("invalid pattern_id")
		}
		if err := deps.PatternDAO.DisablePattern(ctx, userID, uint64(patternID)); err != nil {
			return nil, err
		}
		return &engine.ToolResult{Name: "delete_pattern", Status: "success", Message: fmt.Sprintf("已禁用消费模式 #%d，不再用于预填", uint64(patternID)), Data: map[string]any{"id": uint64(patternID)}, Source: "mysql"}, nil
	})
}
