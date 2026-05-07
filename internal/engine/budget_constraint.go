package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sangchenglong/kapi/internal/model"
	"github.com/sangchenglong/kapi/internal/service"
)

// budgetConstraint 预算约束校验结果
type budgetConstraint struct {
	TotalBudget     float64
	CategorySum     float64
	Remaining       float64
	HasTotalBudget  bool
}

// checkBudgetConstraint 校验分类预算之和是否超过总预算
// excludeID: 更新场景下排除当前预算 ID（避免重复计算）
// newAmount: 本次要创建/更新的金额
func checkBudgetConstraint(ctx context.Context, budgetSvc *service.BudgetService, userID uint64, excludeID uint64, newAmount float64) (*budgetConstraint, error) {
	budgets, err := budgetSvc.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	var totalBudgetAmount float64
	var categoryBudgetSum float64
	hasTotalBudget := false

	for _, b := range budgets {
		if isTotalBudget(&b) {
			totalBudgetAmount = b.Amount
			hasTotalBudget = true
			continue
		}
		if b.ID == excludeID {
			continue
		}
		categoryBudgetSum += b.Amount
	}

	if !hasTotalBudget {
		return &budgetConstraint{HasTotalBudget: false}, nil
	}

	newSum := categoryBudgetSum + newAmount
	return &budgetConstraint{
		TotalBudget:    totalBudgetAmount,
		CategorySum:    newSum,
		Remaining:      totalBudgetAmount - newSum,
		HasTotalBudget: true,
	}, nil
}

// isTotalBudget 判断是否是总预算
func isTotalBudget(b *model.Budget) bool {
	if b.Name == "总预算" || b.Category == "总预算" {
		return true
	}
	var cats []string
	if b.Categories != "" {
		json.Unmarshal([]byte(b.Categories), &cats)
	}
	return len(cats) == 0 && (b.Category == "" || b.Category == "总预算" || b.Category == "[]")
}

// budgetConstraintMessage 生成约束校验的提示消息
func budgetConstraintMessage(c *budgetConstraint, name string, amount float64) *ToolResult {
	if !c.HasTotalBudget {
		return nil
	}
	if c.Remaining >= 0 {
		return nil
	}
	overAmount := -c.Remaining
	return &ToolResult{
		Name:   "create_budget",
		Status: "need_confirm",
		Message: fmt.Sprintf("%s ¥%.2f 会导致分类预算总额（¥%.2f）超过总预算（¥%.2f），超出 ¥%.2f。\n请先将总预算调整到 ¥%.0f 以上，或减少其他分类预算。",
			name, amount, c.CategorySum, c.TotalBudget, overAmount, c.CategorySum),
		Data:   map[string]any{"category_sum": c.CategorySum, "total_budget": c.TotalBudget, "over": overAmount},
		Source: "engine",
	}
}
