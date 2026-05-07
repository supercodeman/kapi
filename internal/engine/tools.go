package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/sangchenglong/kapi/internal/model"
	"github.com/sangchenglong/kapi/internal/service"
)

type ToolHandler func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error)

type ToolExecutor struct {
	handlers        map[string]ToolHandler
	tools           []Tool
	opLogSvc        *service.OpLogService
	billSvc         *service.BillService
	ragFiller       *RAGFiller
	onWriteComplete func(ctx context.Context, userID uint64)
}

func (te *ToolExecutor) SetRAGFiller(filler *RAGFiller) {
	te.ragFiller = filler
}

// requestContextKey 用于在 context 中传递请求级别的 sessionID 和 triggerText
type requestContextKey struct{}

type requestContext struct {
	SessionID      uint64
	TriggerText    string
	RecentUserMsgs []string
	IsConfirmation bool
}

func (te *ToolExecutor) SetRequestContext(sessionID uint64, triggerText string) {
	// 已废弃：请使用 ContextWithRequest 将请求上下文注入 ctx
	// 保留此方法仅为兼容，不再写入共享字段
}

// ContextWithRequest 将请求级别的 sessionID、triggerText 和最近用户消息注入 context
func ContextWithRequest(ctx context.Context, sessionID uint64, triggerText string, recentUserMsgs []string) context.Context {
	return context.WithValue(ctx, requestContextKey{}, &requestContext{
		SessionID:      sessionID,
		TriggerText:    triggerText,
		RecentUserMsgs: recentUserMsgs,
	})
}

// ContextWithConfirmation 在已有 requestContext 上标记本轮为确认操作
func ContextWithConfirmation(ctx context.Context) context.Context {
	rc, ok := ctx.Value(requestContextKey{}).(*requestContext)
	if !ok || rc == nil {
		return ctx
	}
	rc.IsConfirmation = true
	return ctx
}

func isConfirmation(ctx context.Context) bool {
	rc, ok := ctx.Value(requestContextKey{}).(*requestContext)
	return ok && rc != nil && rc.IsConfirmation
}

// getRequestContext 从 context 中提取请求级别的上下文
func getRequestContext(ctx context.Context) (uint64, string) {
	rc, ok := ctx.Value(requestContextKey{}).(*requestContext)
	if !ok || rc == nil {
		return 0, ""
	}
	return rc.SessionID, rc.TriggerText
}

func getUserMessage(ctx context.Context) string {
	_, msg := getRequestContext(ctx)
	return msg
}

// getRecentUserMsgs 从 context 中提取最近的用户消息列表
func getRecentUserMsgs(ctx context.Context) []string {
	rc, ok := ctx.Value(requestContextKey{}).(*requestContext)
	if !ok || rc == nil {
		return nil
	}
	return rc.RecentUserMsgs
}

func NewToolExecutor(billSvc *service.BillService, budgetSvc *service.BudgetService, assetSvc *service.AssetService, opLogSvc *service.OpLogService) *ToolExecutor {
	te := &ToolExecutor{
		handlers: make(map[string]ToolHandler),
		opLogSvc: opLogSvc,
		billSvc:  billSvc,
	}

	te.register("create_bill", "创建一笔账单记录（支出或收入）", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bill_type": map[string]any{"type": "string", "description": "账单类型：expense（支出）或 income（收入），默认 expense"},
			"amount":    map[string]any{"type": "number", "description": "金额"},
			"category":  map[string]any{"type": "string", "description": "分类"},
			"merchant":  map[string]any{"type": "string", "description": "商户"},
			"date":      map[string]any{"type": "string", "description": "日期，格式 YYYY-MM-DD"},
			"note":      map[string]any{"type": "string", "description": "备注"},
			"asset_id":  map[string]any{"type": "integer", "description": "关联资产ID（可选）"},
		},
		"required": []string{"amount", "category", "date"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		amount, ok := ToFloat64(args["amount"])
		if !ok || amount <= 0 {
			return nil, fmt.Errorf("invalid amount")
		}

		// 金额来源校验：防止 LLM 从历史中编造金额（确认轮次跳过）
		userMsg := getUserMessage(ctx)
		recentMsgs := getRecentUserMsgs(ctx)


		// 单价×数量 或 多商品求和场景：Engine 计算总金额
		quantityTotal := float64(0)
		if !isConfirmation(ctx) {
			quantityTotal = calcTotalFromQuantity(userMsg)
			if quantityTotal > 0 {
				amount = quantityTotal
				args["amount"] = amount
			}
		}

		// 金额校验跳过条件：确认轮次、数量计算已得出总金额
		if !isConfirmation(ctx) && quantityTotal <= 0 && !amountFoundInContext(amount, userMsg, recentMsgs) {
			// 尝试 RAG 补全金额
			if te.ragFiller != nil {
				merchant := getString(args, "merchant")
				category := getString(args, "category")
				rag := te.ragFiller.FillParams(ctx, userID, merchant, category, amount)
				if rag != nil && rag.Confidence >= 0.5 {
					hint := rag.Hint
					if rag.Confidence >= 0.8 {
						hint = "根据你的消费习惯预填"
					}
					return &ToolResult{
						Name:   "create_bill",
						Status: "need_confirm",
						Message: fmt.Sprintf("%s %s，金额 ¥%.2f？（%s）\n回复\"确认\"记录，或告诉我正确金额。",
							merchant, rag.Category, rag.Amount, hint),
						Data: map[string]any{"amount": rag.Amount, "category": rag.Category, "merchant": rag.Merchant,
							"bill_type": getString(args, "bill_type"), "date": getString(args, "date")},
						Source: "engine",
					}, nil
				} else {
					return &ToolResult{
						Name:    "create_bill",
						Status:  "need_confirm",
						Message: "这笔消费金额是多少呢？",
						Data:    map[string]any{"category": getString(args, "category"), "merchant": getString(args, "merchant")},
						Source:  "engine",
					}, nil
				}
			} else {
				return &ToolResult{
					Name:    "create_bill",
					Status:  "need_confirm",
					Message: "这笔消费金额是多少呢？",
					Data:    map[string]any{"category": getString(args, "category"), "merchant": getString(args, "merchant")},
					Source:  "engine",
				}, nil
			}
		}

		billType := getString(args, "bill_type")
		if billType != "income" {
			billType = "expense"
		}
		merchant := getString(args, "merchant")
		catResult := NormalizeCategoryV2(getString(args, "category"), merchant, getUserMessage(ctx))
		// 如果 args 中已有 sub_category（PendingAction 补参），保留不覆盖
		if sc := getString(args, "sub_category"); sc != "" && catResult.SubCategory == "" {
			catResult.SubCategory = sc
		}

		// 收入类型校验：收入记账时如果分类模糊，追问类型（确认轮次跳过）
		if !isConfirmation(ctx) && billType == "income" && catResult.SubCategory == "" && isVagueIncomeCategory(catResult.Category) {
			return &ToolResult{
				Name:    "create_bill",
				Status:  "need_confirm",
				Message: "这笔是什么类型的收入？工资、兼职、理财还是其他？",
				Data:    map[string]any{"amount": amount},
				Source:  "engine",
			}, nil
		}

		// 收入分类自动修正 bill_type
		incomeCategories := map[string]bool{
			"职业收入": true, "投资收入": true, "其他收入": true,
			"工资": true, "奖金": true, "兼职": true,
			"理财收益": true, "股票分红": true, "房租收入": true,
			"红包": true, "退款": true, "报销": true,
		}
		if incomeCategories[catResult.Category] || incomeCategories[catResult.SubCategory] {
			billType = "income"
		}
		date := NormalizeDate(getString(args, "date"))
		note := getString(args, "note")

		// 业务规则兜底：日期异常检测（确认轮次跳过）
		if !isConfirmation(ctx) {
			if guard := guardDateAnomaly(date, amount, merchant, catResult.Category); guard != nil {
				return guard, nil
			}
		}

		// 业务规则兜底：金额异常检测（确认轮次跳过）
		if !isConfirmation(ctx) && billType == "expense" {
			allBills, listErr := billSvc.List(ctx, userID)
			if listErr == nil {
				var maxAmount float64
				for _, b := range allBills {
					if b.Category == catResult.Category && b.BillType == "expense" && b.Amount > maxAmount {
						maxAmount = b.Amount
					}
				}
				if guard := guardAmountAnomaly(amount, catResult.Category, maxAmount); guard != nil {
					return guard, nil
				}
			}
		}

		var assetID uint64
		if aid, ok := ToFloat64(args["asset_id"]); ok && aid > 0 {
			assetID = uint64(aid)
		}

		// 如果未指定资产账户，自动关联默认账户（用户的第一个 cash 类型资产）
		if assetID == 0 {
			assets, err := assetSvc.List(ctx, userID)
			if err == nil {
				for _, a := range assets {
					if a.Type == "cash" {
						assetID = a.ID
						break
					}
				}
			}
		}

		bill, err := billSvc.Create(ctx, userID, billType, amount, catResult.Category, catResult.SubCategory, merchant, date, note, assetID)
		if err != nil {
			return nil, err
		}

		te.logOperation(ctx, userID, "create", "bills", bill.ID, nil, bill)

		// 资产联动：创建账单成功后更新资产余额
		if assetID > 0 {
			delta := -amount // 支出扣减
			if billType == "income" {
				delta = amount // 收入增加
			}
			if err := assetSvc.UpdateBalance(ctx, userID, assetID, delta); err != nil {
				log.Printf("failed to update asset balance for user %d, asset %d: %v", userID, assetID, err)
			}
		}

		typeLabel := "支出"
		if billType == "income" {
			typeLabel = "收入"
		}
		catDisplay := catResult.Category
		if catResult.SubCategory != "" {
			catDisplay += "（" + catResult.SubCategory + "）"
		}
		msg := fmt.Sprintf("已记录%s：%s %s ¥%.2f 分类%s", typeLabel, date, merchant, amount, catDisplay)

		// 检查该分类当月预算执行情况
		budgets, budgetErr := budgetSvc.List(ctx, userID)
		if budgetErr == nil {
			for _, b := range budgets {
				if b.Category == catResult.Category && b.Period == "monthly" {
					// 计算当月该分类已花金额
					now := time.Now()
					monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
					monthEnd := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.Local).Format("2006-01-02")
					bills, listErr := billSvc.ListByDateRange(ctx, userID, monthStart, monthEnd)
					if listErr == nil {
						var spent float64
						for _, bi := range bills {
							if bi.Category == catResult.Category {
								spent += bi.Amount
							}
						}
						if spent > b.Amount {
							msg += fmt.Sprintf("（注意：%s分类本月已花 ¥%.2f，超出预算 ¥%.2f 共 ¥%.2f）",
								catResult.Category, spent, b.Amount, spent-b.Amount)
						}
					}
					break
				}
			}
		}

		return &ToolResult{
			Name:    "create_bill",
			Status:  "success",
			Message: msg,
			Data:    map[string]any{"id": bill.ID, "bill_type": bill.BillType, "amount": bill.Amount, "category": bill.Category, "sub_category": bill.SubCategory, "date": bill.Date, "merchant": bill.Merchant, "asset_id": bill.AssetID},
			Source:  "mysql",
		}, nil
	})

	te.register("list_bills", "查询用户账单列表，支持按时间范围、商户、分类、关键词筛选", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"start_date": map[string]any{"type": "string", "description": "开始日期 YYYY-MM-DD"},
			"end_date":   map[string]any{"type": "string", "description": "结束日期 YYYY-MM-DD"},
			"merchant":   map[string]any{"type": "string", "description": "商户名筛选（模糊匹配）"},
			"category":   map[string]any{"type": "string", "description": "分类筛选"},
			"keyword":    map[string]any{"type": "string", "description": "关键词筛选（匹配商户、分类、备注）"},
		},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		startDate := getString(args, "start_date")
		endDate := getString(args, "end_date")
		if startDate != "" {
			startDate = NormalizeDate(startDate)
		}
		if endDate != "" {
			endDate = NormalizeDate(endDate)
		}

		var bills []model.Bill
		var err error
		if startDate != "" || endDate != "" {
			bills, err = billSvc.ListByDateRange(ctx, userID, startDate, endDate)
		} else {
			bills, err = billSvc.List(ctx, userID)
		}
		if err != nil {
			return nil, err
		}

		// 按商户/分类/关键词过滤
		merchant := getString(args, "merchant")
		category := getString(args, "category")
		keyword := getString(args, "keyword")
		if merchant != "" || category != "" || keyword != "" {
			var filtered []model.Bill
			for _, b := range bills {
				if merchant != "" && !strings.Contains(b.Merchant, merchant) {
					continue
				}
				if category != "" && b.Category != category && !strings.Contains(b.Category, category) {
					continue
				}
				if keyword != "" && !strings.Contains(b.Merchant, keyword) && !strings.Contains(b.Category, keyword) && !strings.Contains(b.Note, keyword) && !strings.Contains(b.SubCategory, keyword) {
					continue
				}
				filtered = append(filtered, b)
			}
			bills = filtered
		}

		// 生成汇总信息
		var totalAmount float64
		for _, b := range bills {
			if b.BillType == "expense" {
				totalAmount += b.Amount
			}
		}

		msg := fmt.Sprintf("查询到 %d 条账单记录", len(bills))
		if len(bills) == 0 {
			msg = "没有找到符合条件的账单记录"
		} else {
			msg += fmt.Sprintf("，总支出 ¥%.2f", totalAmount)
		}

		return &ToolResult{
			Name:    "list_bills",
			Status:  "success",
			Message: msg,
			Data:    map[string]any{"bills": bills, "count": len(bills), "total_amount": totalAmount},
			Source:  "mysql",
		}, nil
	})

	te.register("list_budgets", "查询用户预算列表", map[string]any{
		"type": "object",
		"properties": map[string]any{},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		budgets, err := budgetSvc.List(ctx, userID)
		if err != nil {
			return nil, err
		}

		msg := fmt.Sprintf("查询到 %d 条预算记录", len(budgets))
		if len(budgets) == 0 {
			msg = "没有找到预算记录"
		}

		return &ToolResult{
			Name:    "list_budgets",
			Status:  "success",
			Message: msg,
			Data:    map[string]any{"budgets": budgets, "count": len(budgets)},
			Source:  "mysql",
		}, nil
	})

	te.register("create_budget", "创建新的预算", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        map[string]any{"type": "string", "description": "预算名称，如'日常餐饮预算'"},
			"categories":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "预算覆盖的分类数组，如[\"食饮\",\"社交\"]"},
			"amount":      map[string]any{"type": "number", "description": "预算金额"},
			"period":      map[string]any{"type": "string", "description": "周期：monthly 或 weekly，默认 monthly"},
			"budget_type": map[string]any{"type": "string", "description": "预算类型：essential（必要）或 optional（可选），默认 optional"},
		},
		"required": []string{"categories", "amount"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		// 解析 categories 数组并序列化为 JSON 字符串
		var categories []string
		if rawCats, ok := args["categories"]; ok {
			if catArr, ok := rawCats.([]any); ok {
				for _, c := range catArr {
					if s, ok := c.(string); ok && s != "" {
						categories = append(categories, NormalizeCategory(s))
					}
				}
			}
		}
		if len(categories) == 0 {
			return nil, fmt.Errorf("categories is required")
		}
		categoriesJSON, _ := json.Marshal(categories)

		amount, ok := ToFloat64(args["amount"])
		if !ok || amount <= 0 {
			return nil, fmt.Errorf("invalid amount")
		}
		name := getString(args, "name")
		if name == "" {
			name = categories[0] + "预算"
		}
		period := getString(args, "period")
		period = NormalizePeriod(period)
		if period == "" {
			period = "monthly"
		}
		budgetType := getString(args, "budget_type")
		if budgetType != "essential" {
			budgetType = "optional"
		}

		// 约束校验：分类预算之和不能超过总预算
		constraint, _ := checkBudgetConstraint(ctx, budgetSvc, userID, 0, amount)
		if msg := budgetConstraintMessage(constraint, name, amount); msg != nil {
			return msg, nil
		}

		budget, err := budgetSvc.Create(ctx, userID, name, string(categoriesJSON), amount, period, budgetType)
		if err != nil {
			return nil, err
		}
		te.logOperation(ctx, userID, "create", "budgets", budget.ID, nil, budget)
		return &ToolResult{
			Name:    "create_budget",
			Status:  "success",
			Message: fmt.Sprintf("已创建预算：%s ¥%.2f/%s", budget.Name, budget.Amount, periodToChinese(budget.Period)),
			Data:    map[string]any{"id": budget.ID, "name": budget.Name, "categories": categories, "amount": budget.Amount, "period": budget.Period, "budget_type": budget.BudgetType},
			Source:  "mysql",
		}, nil
	})

	te.register("update_budget", "修改已有预算", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"budget_id":   map[string]any{"type": "integer", "description": "预算ID（可选，如果不传则通过 category 查找）"},
			"category":    map[string]any{"type": "string", "description": "预算对应的分类名（如食饮、交通），用于查找预算"},
			"name":        map[string]any{"type": "string", "description": "预算名称"},
			"categories":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "预算覆盖的分类数组"},
			"amount":      map[string]any{"type": "number", "description": "新预算金额"},
			"period":      map[string]any{"type": "string", "description": "周期：monthly/weekly"},
			"budget_type": map[string]any{"type": "string", "description": "预算类型：essential/optional"},
		},
		"required": []string{"amount"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		budgetID, hasID := ToFloat64(args["budget_id"])
		if hasID && budgetID == 0 {
			hasID = false // budget_id=0 视为未传
		}
		amount, _ := ToFloat64(args["amount"])
		name := getString(args, "name")
		period := getString(args, "period")
		period = NormalizePeriod(period)
		if period == "" {
			period = "monthly"
		}
		budgetType := getString(args, "budget_type")
		if budgetType != "essential" {
			budgetType = "optional"
		}

		// 如果没传 budget_id，通过 category 或 name 查找
		if !hasID {
			categoryName := getString(args, "category")
			// 如果没传 category，尝试从 name 中提取
			if categoryName == "" && name != "" {
				categoryName = strings.TrimSuffix(name, "预算")
			}
			if categoryName == "" {
				return &ToolResult{
					Name: "update_budget", Status: "error",
					Message: "请指定要修改的预算分类", Source: "engine",
				}, nil
			}
			categoryName = NormalizeCategory(categoryName)
			budgets, err := budgetSvc.List(ctx, userID)
			if err != nil {
				return nil, err
			}
			log.Printf("[BUDGET] userID=%d, looking for category=%q, found %d budgets", userID, categoryName, len(budgets))
			for _, b := range budgets {
				log.Printf("[BUDGET]   id=%d name=%q categories=%q", b.ID, b.Name, b.Categories)
				if strings.Contains(b.Name, categoryName) || strings.Contains(b.Categories, categoryName) {
					budgetID = float64(b.ID)
					break
				}
			}
			if budgetID == 0 {
				// 未找到该预算，自动创建
				catJSON, _ := json.Marshal([]string{categoryName})
				newBudget, createErr := budgetSvc.Create(ctx, userID, categoryName+"预算", string(catJSON), amount, "monthly", "optional")
				if createErr != nil {
					return nil, createErr
				}
				return &ToolResult{
					Name:    "update_budget",
					Status:  "success",
					Message: fmt.Sprintf("未找到 %s 预算，已自动创建：%s 预算 ¥%.2f/月", categoryName, categoryName, amount),
					Data:    map[string]any{"id": newBudget.ID, "name": newBudget.Name, "amount": amount},
					Source:  "engine",
				}, nil
			}
		}

		// 解析 categories 数组
		var categories []string
		if rawCats, ok := args["categories"]; ok {
			if catArr, ok := rawCats.([]any); ok {
				for _, c := range catArr {
					if s, ok := c.(string); ok && s != "" {
						categories = append(categories, NormalizeCategory(s))
					}
				}
			}
		}
		categoriesJSON := "[]"
		if len(categories) > 0 {
			b, _ := json.Marshal(categories)
			categoriesJSON = string(b)
		}

		before, _ := budgetSvc.GetByID(ctx, userID, uint64(budgetID))

		// 未传的字段从原数据回填，避免空值覆盖
		if before != nil {
			if name == "" {
				name = before.Name
			}
			if len(categories) == 0 && before.Categories != "" && before.Categories != "[]" {
				categoriesJSON = before.Categories
			}
			if period == "monthly" && before.Period != "" {
				period = before.Period
			}
		}

		// 约束校验：更新分类预算时检查不超总预算（总预算本身不校验）
		if before != nil && !isTotalBudget(before) {
			constraint, _ := checkBudgetConstraint(ctx, budgetSvc, userID, uint64(budgetID), amount)
			if msg := budgetConstraintMessage(constraint, name, amount); msg != nil {
				msg.Name = "update_budget"
				return msg, nil
			}
		}

		budget, err := budgetSvc.Update(ctx, userID, uint64(budgetID), name, categoriesJSON, amount, period, budgetType)
		if err != nil {
			return nil, err
		}

		te.logOperation(ctx, userID, "update", "budgets", budget.ID, before, budget)

		periodLabel := periodToChinese(budget.Period)
		msg := fmt.Sprintf("%s已调整为 ¥%.2f/%s。", budget.Name, budget.Amount, periodLabel)

		// 自动查询调整后的预算执行情况，避免 LLM 自己推算
		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		monthEnd := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.Local).Format("2006-01-02")

		var cats []string
		if categoriesJSON != "" && categoriesJSON != "[]" {
			_ = json.Unmarshal([]byte(categoriesJSON), &cats)
		}
		if len(cats) == 0 && budget.Category != "" {
			cats = []string{budget.Category}
		}

		if len(cats) > 0 {
			bills, listErr := billSvc.ListByDateRange(ctx, userID, monthStart, monthEnd)
			if listErr == nil {
				var spent float64
				for _, b := range bills {
					for _, cat := range cats {
						if b.Category == cat && b.BillType == "expense" {
							spent += b.Amount
						}
					}
				}
				remaining := budget.Amount - spent
				msg += fmt.Sprintf(" 本月已花 ¥%.2f，剩余 ¥%.2f。", spent, remaining)
			}
		}

		// 总预算调整时，附带分类预算之和信息
		if isTotalBudget(budget) {
			allBudgets, _ := budgetSvc.List(ctx, userID)
			var catSum float64
			for _, ab := range allBudgets {
				if !isTotalBudget(&ab) {
					catSum += ab.Amount
				}
			}
			msg += fmt.Sprintf(" 当前分类预算合计 ¥%.2f，总预算余量 ¥%.2f。", catSum, budget.Amount-catSum)
		}

		return &ToolResult{
			Name:    "update_budget",
			Status:  "success",
			Message: msg,
			Data:    map[string]any{"id": budget.ID, "name": budget.Name, "categories": categories, "amount": budget.Amount, "period": budget.Period, "budget_type": budget.BudgetType},
			Source:  "mysql",
		}, nil
	})

	te.register("list_assets", "查询用户资产列表", map[string]any{
		"type": "object",
		"properties": map[string]any{},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		assets, err := assetSvc.List(ctx, userID)
		if err != nil {
			return nil, err
		}
		var totalBalance float64
		for _, a := range assets {
			if a.Type == "debt" || a.Type == "credit" {
				totalBalance -= a.Balance
			} else {
				totalBalance += a.Balance
			}
		}
		msg := fmt.Sprintf("查询到 %d 条资产记录，净资产 ¥%.2f", len(assets), totalBalance)
		if len(assets) == 0 {
			msg = "没有找到资产记录"
		}

		return &ToolResult{
			Name:    "list_assets",
			Status:  "success",
			Message: msg,
			Data:    map[string]any{"assets": assets, "count": len(assets), "net_balance": totalBalance},
			Source:  "mysql",
		}, nil
	})

	// delete_bill — 删除账单（含资产余额恢复），首次调用返回确认预览
	te.register("delete_bill", "删除一笔账单记录。首次调用不传 confirmed 参数时返回待删除内容预览，用户确认后再次调用并传 confirmed=true 执行删除", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bill_id":   map[string]any{"type": "integer", "description": "账单ID"},
			"confirmed": map[string]any{"type": "boolean", "description": "是否已确认删除，首次调用不传或传 false"},
		},
		"required": []string{"bill_id"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		billID, ok := ToFloat64(args["bill_id"])
		if !ok || billID <= 0 {
			return nil, fmt.Errorf("invalid bill_id")
		}
		before, _ := billSvc.GetByID(ctx, userID, uint64(billID))

		// 未确认时返回预览
		confirmed, _ := args["confirmed"].(bool)
		if !confirmed {
			msg := fmt.Sprintf("确认要删除账单 #%d 吗？", uint64(billID))
			if before != nil {
				msg = fmt.Sprintf("确认要删除这笔账单吗？\n%s %s %s ¥%.2f", before.Date, before.Category, before.Merchant, before.Amount)
			}
			return &ToolResult{
				Name: "delete_bill", Status: "need_confirm", Message: msg,
				Data: map[string]any{"bill_id": uint64(billID)}, Source: "engine",
			}, nil
		}

		if err := billSvc.Delete(ctx, userID, uint64(billID)); err != nil {
			return nil, err
		}
		te.logOperation(ctx, userID, "delete", "bills", uint64(billID), before, nil)

		// 资产联动：删除账单时恢复资产余额（反向操作）
		if before != nil && before.AssetID > 0 {
			delta := before.Amount // 支出删除 → 余额恢复（加回）
			if before.BillType == "income" {
				delta = -before.Amount // 收入删除 → 余额恢复（扣减）
			}
			if err := assetSvc.UpdateBalance(ctx, userID, before.AssetID, delta); err != nil {
				log.Printf("failed to restore asset balance on delete for user %d, asset %d: %v", userID, before.AssetID, err)
			}
		}

		msg := fmt.Sprintf("已删除账单 #%d", uint64(billID))
		if before != nil {
			msg = fmt.Sprintf("已删除账单：%s %s ¥%.2f", before.Date, before.Category, before.Amount)
		}

		return &ToolResult{
			Name:    "delete_bill",
			Status:  "success",
			Message: msg,
			Data:    map[string]any{"id": uint64(billID), "deleted": true},
			Source:  "mysql",
		}, nil
	})

	// update_bill — 修改账单（含资产余额差值调整）
	te.register("update_bill", "修改一笔账单记录", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bill_id":  map[string]any{"type": "integer", "description": "账单ID"},
			"amount":   map[string]any{"type": "number", "description": "金额"},
			"category": map[string]any{"type": "string", "description": "分类"},
			"merchant": map[string]any{"type": "string", "description": "商户"},
			"date":     map[string]any{"type": "string", "description": "日期，格式 YYYY-MM-DD"},
			"note":     map[string]any{"type": "string", "description": "备注"},
		},
		"required": []string{"bill_id", "amount", "category", "date"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		billID, ok := ToFloat64(args["bill_id"])
		if !ok || billID <= 0 {
			return nil, fmt.Errorf("invalid bill_id")
		}
		amount, ok := ToFloat64(args["amount"])
		if !ok || amount <= 0 {
			return nil, fmt.Errorf("invalid amount")
		}
		category, _ := args["category"].(string)
		category = NormalizeCategory(category)
		if category == "" {
			return nil, fmt.Errorf("category is required")
		}
		date, _ := args["date"].(string)
		date = NormalizeDate(date)
		merchant, _ := args["merchant"].(string)
		note, _ := args["note"].(string)

		before, _ := billSvc.GetByID(ctx, userID, uint64(billID))

		bill, err := billSvc.Update(ctx, userID, uint64(billID), amount, category, merchant, date, note)
		if err != nil {
			return nil, err
		}
		te.logOperation(ctx, userID, "update", "bills", bill.ID, before, bill)

		// 资产联动：修改金额时调整资产余额差值
		if before != nil && before.AssetID > 0 {
			amountDiff := amount - before.Amount
			if amountDiff != 0 {
				delta := -amountDiff // 支出增加 → 余额减少
				if before.BillType == "income" {
					delta = amountDiff // 收入增加 → 余额增加
				}
				if err := assetSvc.UpdateBalance(ctx, userID, before.AssetID, delta); err != nil {
					log.Printf("failed to adjust asset balance on update for user %d, asset %d: %v", userID, before.AssetID, err)
				}
			}
		}

		return &ToolResult{
			Name:    "update_bill",
			Status:  "success",
			Message: fmt.Sprintf("已更新账单：%s %s ¥%.2f 分类%s", bill.Date, merchant, bill.Amount, bill.Category),
			Data:    map[string]any{"id": bill.ID, "amount": bill.Amount, "category": bill.Category, "date": bill.Date},
			Source:  "mysql",
		}, nil
	})

	// delete_budget — 删除预算，首次调用返回确认预览
	te.register("delete_budget", "删除一条预算。首次调用不传 confirmed 时返回预览，确认后传 confirmed=true 执行", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"budget_id": map[string]any{"type": "integer", "description": "预算ID"},
			"confirmed": map[string]any{"type": "boolean", "description": "是否已确认删除"},
		},
		"required": []string{"budget_id"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		budgetID, ok := ToFloat64(args["budget_id"])
		if !ok || budgetID <= 0 {
			return nil, fmt.Errorf("invalid budget_id")
		}
		before, _ := budgetSvc.GetByID(ctx, userID, uint64(budgetID))

		confirmed, _ := args["confirmed"].(bool)
		if !confirmed {
			msg := fmt.Sprintf("确认要删除预算 #%d 吗？", uint64(budgetID))
			if before != nil {
				msg = fmt.Sprintf("确认要删除预算「%s」（¥%.2f/%s）吗？", before.Name, before.Amount, periodToChinese(before.Period))
			}
			return &ToolResult{
				Name: "delete_budget", Status: "need_confirm", Message: msg,
				Data: map[string]any{"budget_id": uint64(budgetID)}, Source: "engine",
			}, nil
		}

		if err := budgetSvc.Delete(ctx, userID, uint64(budgetID)); err != nil {
			return nil, err
		}
		te.logOperation(ctx, userID, "delete", "budgets", uint64(budgetID), before, nil)

		msg := fmt.Sprintf("已删除预算 #%d", uint64(budgetID))
		if before != nil {
			msg = fmt.Sprintf("已删除预算：%s ¥%.2f/%s", before.Category, before.Amount, periodToChinese(before.Period))
		}

		return &ToolResult{
			Name:    "delete_budget",
			Status:  "success",
			Message: msg,
			Data:    map[string]any{"id": uint64(budgetID), "deleted": true},
			Source:  "mysql",
		}, nil
	})

	// create_asset — 创建资产
	te.register("create_asset", "创建一条资产记录", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":    map[string]any{"type": "string", "description": "资产名称"},
			"type":    map[string]any{"type": "string", "description": "资产类型：cash/credit/investment/debt"},
			"balance": map[string]any{"type": "number", "description": "余额"},
		},
		"required": []string{"name", "type", "balance"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		name, _ := args["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("name is required")
		}
		assetType, _ := args["type"].(string)
		assetType = NormalizeAssetType(assetType)
		if assetType == "" {
			return nil, fmt.Errorf("type is required")
		}
		balance, ok := ToFloat64(args["balance"])
		if !ok {
			return nil, fmt.Errorf("invalid balance")
		}
		asset, err := assetSvc.Create(ctx, userID, name, assetType, balance)
		if err != nil {
			return nil, err
		}
		te.logOperation(ctx, userID, "create", "assets", asset.ID, nil, asset)
		return &ToolResult{
			Name:    "create_asset",
			Status:  "success",
			Message: fmt.Sprintf("已创建资产：%s（%s）余额 ¥%.2f", asset.Name, asset.Type, asset.Balance),
			Data:    map[string]any{"id": asset.ID, "name": asset.Name, "type": asset.Type, "balance": asset.Balance},
			Source:  "mysql",
		}, nil
	})

	// update_asset — 修改资产
	te.register("update_asset", "修改一条资产记录", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"asset_id": map[string]any{"type": "integer", "description": "资产ID"},
			"name":     map[string]any{"type": "string", "description": "资产名称"},
			"type":     map[string]any{"type": "string", "description": "资产类型：cash/credit/investment/debt"},
			"balance":  map[string]any{"type": "number", "description": "余额"},
		},
		"required": []string{"asset_id", "name", "type", "balance"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		assetID, ok := ToFloat64(args["asset_id"])
		if !ok || assetID <= 0 {
			return nil, fmt.Errorf("invalid asset_id")
		}
		name, _ := args["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("name is required")
		}
		assetType, _ := args["type"].(string)
		assetType = NormalizeAssetType(assetType)
		if assetType == "" {
			return nil, fmt.Errorf("type is required")
		}
		balance, ok := ToFloat64(args["balance"])
		if !ok {
			return nil, fmt.Errorf("invalid balance")
		}

		before, _ := assetSvc.GetByID(ctx, userID, uint64(assetID))

		asset, err := assetSvc.Update(ctx, userID, uint64(assetID), name, assetType, balance)
		if err != nil {
			return nil, err
		}
		te.logOperation(ctx, userID, "update", "assets", asset.ID, before, asset)
		return &ToolResult{
			Name:    "update_asset",
			Status:  "success",
			Message: fmt.Sprintf("已更新资产：%s（%s）余额 ¥%.2f", asset.Name, asset.Type, asset.Balance),
			Data:    map[string]any{"id": asset.ID, "name": asset.Name, "type": asset.Type, "balance": asset.Balance},
			Source:  "mysql",
		}, nil
	})

	// delete_asset — 删除资产，首次调用返回确认预览
	te.register("delete_asset", "删除一条资产记录。首次调用不传 confirmed 时返回预览，确认后传 confirmed=true 执行", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"asset_id":  map[string]any{"type": "integer", "description": "资产ID"},
			"confirmed": map[string]any{"type": "boolean", "description": "是否已确认删除"},
		},
		"required": []string{"asset_id"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		assetID, ok := ToFloat64(args["asset_id"])
		if !ok || assetID <= 0 {
			return nil, fmt.Errorf("invalid asset_id")
		}
		before, _ := assetSvc.GetByID(ctx, userID, uint64(assetID))

		confirmed, _ := args["confirmed"].(bool)
		if !confirmed {
			msg := fmt.Sprintf("确认要删除资产 #%d 吗？", uint64(assetID))
			if before != nil {
				msg = fmt.Sprintf("确认要删除资产「%s」（%s，余额 ¥%.2f）吗？", before.Name, before.Type, before.Balance)
			}
			return &ToolResult{
				Name: "delete_asset", Status: "need_confirm", Message: msg,
				Data: map[string]any{"asset_id": uint64(assetID)}, Source: "engine",
			}, nil
		}

		if err := assetSvc.Delete(ctx, userID, uint64(assetID)); err != nil {
			return nil, err
		}
		te.logOperation(ctx, userID, "delete", "assets", uint64(assetID), before, nil)

		msg := fmt.Sprintf("已删除资产 #%d", uint64(assetID))
		if before != nil {
			msg = fmt.Sprintf("已删除资产：%s（%s）", before.Name, before.Type)
		}

		return &ToolResult{
			Name:    "delete_asset",
			Status:  "success",
			Message: msg,
			Data:    map[string]any{"id": uint64(assetID), "deleted": true},
			Source:  "mysql",
		}, nil
	})

	// transfer — 在两个资产账户之间转账
	te.register("transfer", "在两个资产账户之间转账", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from_asset_id": map[string]any{"type": "integer", "description": "转出资产ID"},
			"to_asset_id":   map[string]any{"type": "integer", "description": "转入资产ID"},
			"amount":        map[string]any{"type": "number", "description": "转账金额"},
			"note":          map[string]any{"type": "string", "description": "备注"},
			"category":      map[string]any{"type": "string", "description": "分类，默认金融"},
		},
		"required": []string{"from_asset_id", "to_asset_id", "amount"},
	}, func(ctx context.Context, userID uint64, args map[string]any) (*ToolResult, error) {
		fromAssetID, ok := ToFloat64(args["from_asset_id"])
		if !ok || fromAssetID <= 0 {
			return nil, fmt.Errorf("invalid from_asset_id")
		}
		toAssetID, ok := ToFloat64(args["to_asset_id"])
		if !ok || toAssetID <= 0 {
			return nil, fmt.Errorf("invalid to_asset_id")
		}
		amount, ok := ToFloat64(args["amount"])
		if !ok || amount <= 0 {
			return nil, fmt.Errorf("invalid amount")
		}
		note := getString(args, "note")
		category := getString(args, "category")
		if category == "" {
			category = "金融"
		}
		date := time.Now().Format("2006-01-02")

		// 创建 transfer 类型账单
		bill, err := billSvc.Create(ctx, userID, "transfer", amount, category, "转账", "", date, note, uint64(fromAssetID))
		if err != nil {
			return nil, fmt.Errorf("create transfer bill failed: %w", err)
		}

		// 更新 to_asset_id（BillService.Create 不设置 ToAssetID，需要手动更新）
		// 这里直接通过 Update 来补充 ToAssetID 信息不太合适，先在 bill 对象上记录
		// 实际上 transfer 的 to_asset_id 需要在 bill 上记录
		// 由于 BillService.Update 不支持修改 asset 字段，这里用 logOperation 记录完整信息

		// 转出资产扣减
		if err := assetSvc.UpdateBalance(ctx, userID, uint64(fromAssetID), -amount); err != nil {
			return nil, fmt.Errorf("deduct from_asset failed: %w", err)
		}
		// 转入资产增加
		if err := assetSvc.UpdateBalance(ctx, userID, uint64(toAssetID), amount); err != nil {
			return nil, fmt.Errorf("add to_asset failed: %w", err)
		}

		te.logOperation(ctx, userID, "create", "bills", bill.ID, nil, map[string]any{
			"id": bill.ID, "bill_type": "transfer", "amount": amount,
			"from_asset_id": uint64(fromAssetID), "to_asset_id": uint64(toAssetID),
			"category": category, "note": note,
		})

		return &ToolResult{
			Name:    "transfer",
			Status:  "success",
			Message: fmt.Sprintf("已完成转账 ¥%.2f", amount),
			Data: map[string]any{
				"id": bill.ID, "amount": amount,
				"from_asset_id": uint64(fromAssetID), "to_asset_id": uint64(toAssetID),
			},
			Source: "mysql",
		}, nil
	})

	return te
}

func (te *ToolExecutor) logOperation(ctx context.Context, userID uint64, opType, table string, targetID uint64, before, after interface{}) {
	if te.opLogSvc == nil {
		return
	}
	sessionID, triggerText := getRequestContext(ctx)
	if err := te.opLogSvc.LogOperation(ctx, userID, sessionID, opType, table, targetID, before, after, triggerText); err != nil {
		log.Printf("failed to log operation for user %d, table %s, target %d: %v", userID, table, targetID, err)
	}
	// 写操作完成后触发缓存失效
	if te.onWriteComplete != nil {
		te.onWriteComplete(ctx, userID)
	}
}

// SetOnWriteComplete 设置写操作完成后的回调（用于缓存失效等）
func (te *ToolExecutor) SetOnWriteComplete(fn func(ctx context.Context, userID uint64)) {
	te.onWriteComplete = fn
}

func (te *ToolExecutor) register(name, desc string, params any, handler ToolHandler) {
	te.handlers[name] = handler
	te.tools = append(te.tools, Tool{
		Name:        name,
		Description: desc,
		Parameters:  params,
	})
}

func (te *ToolExecutor) Register(name, desc string, params any, handler ToolHandler) {
	te.register(name, desc, params, handler)
}

func (te *ToolExecutor) Execute(ctx context.Context, userID uint64, tc ToolCall) (*ToolResult, error) {
	handler, ok := te.handlers[tc.Name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", tc.Name)
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}

	result, err := handler(ctx, userID, args)
	if err != nil {
		return nil, err
	}
	result.ID = tc.ID
	return result, nil
}

func (te *ToolExecutor) GetToolDefinitions() []Tool {
	result := make([]Tool, len(te.tools))
	copy(result, te.tools)
	return result
}

// GetToolDefinitionsForPage 返回指定页面可用的 Tool 子集
// 逻辑：收集该页面所有 Skill 的 mandatory_tool，加上通用 Tool（list_bills 等）
func (te *ToolExecutor) GetToolDefinitionsForPage(pageTools map[string]bool) []Tool {
	if len(pageTools) == 0 {
		return te.GetToolDefinitions()
	}
	var result []Tool
	for _, t := range te.tools {
		if pageTools[t.Name] {
			result = append(result, t)
		}
	}
	if len(result) == 0 {
		return te.GetToolDefinitions()
	}
	return result
}

func getString(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

// GetString 导出版本
func GetString(args map[string]any, key string) string {
	return getString(args, key)
}

// periodToChinese 将英文周期转为中文显示
func periodToChinese(period string) string {
	switch period {
	case "weekly":
		return "周"
	case "yearly":
		return "年"
	case "daily":
		return "日"
	default:
		return "月"
	}
}
