package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

type BillSummary struct {
	Category   string  `json:"category"`
	Total      float64 `json:"total"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
	AvgAmount  float64 `json:"avg_amount"`
}

type PeriodComparison struct {
	CurrentTotal  float64 `json:"current_total"`
	PreviousTotal float64 `json:"previous_total"`
	Diff          float64 `json:"diff"`
	GrowthRate    float64 `json:"growth_rate"`
}

type BudgetExecution struct {
	Category     string  `json:"category"`
	BudgetAmount float64 `json:"budget_amount"`
	SpentAmount  float64 `json:"spent_amount"`
	Remaining    float64 `json:"remaining"`
	ExecutionRate float64 `json:"execution_rate"`
}

type Engine struct {
	db *gorm.DB
}

func NewEngine(db *gorm.DB) *Engine {
	return &Engine{db: db}
}

func (e *Engine) GetCategorySummary(ctx context.Context, userID uint64, startDate, endDate string) ([]BillSummary, error) {
	var results []BillSummary
	err := e.db.WithContext(ctx).
		Table("bills").
		Select("category, SUM(amount) as total, COUNT(*) as count").
		Where("user_id = ? AND is_deleted = ? AND bill_type = ? AND date >= ? AND date <= ?", userID, false, "expense", startDate, endDate).
		Group("category").
		Order("total DESC").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	var grandTotal float64
	for _, r := range results {
		grandTotal += r.Total
	}
	if grandTotal > 0 {
		for i := range results {
			results[i].Percentage = results[i].Total / grandTotal * 100
			if results[i].Count > 0 {
				results[i].AvgAmount = math.Round(results[i].Total/float64(results[i].Count)*100) / 100
			}
		}
	}
	return results, nil
}

// GenerateSpendingReport 基于分类汇总生成完整的消费分析报告（固定模板，不依赖 LLM）
func GenerateSpendingReport(summary []BillSummary, startDate, endDate string) string {
	if len(summary) == 0 {
		return fmt.Sprintf("%s 至 %s 没有找到消费记录。", startDate, endDate)
	}

	var totalAmount float64
	var totalCount int
	for _, s := range summary {
		totalAmount += s.Total
		totalCount += s.Count
	}

	var sb strings.Builder

	// 第一部分：消费概览
	sb.WriteString(fmt.Sprintf("%s 至 %s 消费分析：\n\n", startDate, endDate))
	sb.WriteString(fmt.Sprintf("总支出 ¥%.2f，共 %d 笔，涉及 %d 个分类。\n\n", totalAmount, totalCount, len(summary)))

	// 第二部分：分类明细
	sb.WriteString("各分类支出：\n")
	for _, s := range summary {
		sb.WriteString(fmt.Sprintf("- %s：¥%.2f（%d 笔，占 %.1f%%）\n", s.Category, s.Total, s.Count, s.Percentage))
	}

	// 第三部分：消费特征（固定 3 个维度）
	sb.WriteString("\n消费特征：\n")

	// 维度1：集中度（前 1~2 名占比）
	top := summary[0]
	if len(summary) >= 2 {
		top2Pct := top.Percentage + summary[1].Percentage
		if top.Percentage > 70 {
			sb.WriteString(fmt.Sprintf("- 集中度：%s 独占 %.1f%%，消费高度集中在单一分类\n", top.Category, top.Percentage))
		} else if top2Pct > 70 {
			sb.WriteString(fmt.Sprintf("- 集中度：%s + %s 合计占 %.1f%%，消费集中在两个分类\n", top.Category, summary[1].Category, top2Pct))
		} else {
			sb.WriteString("- 集中度：消费分布较均匀，无明显集中\n")
		}
	} else {
		sb.WriteString(fmt.Sprintf("- 集中度：仅 %s 一个分类\n", top.Category))
	}

	// 维度2：频次最高的分类
	maxFreq := summary[0]
	for _, s := range summary[1:] {
		if s.Count > maxFreq.Count {
			maxFreq = s
		}
	}
	sb.WriteString(fmt.Sprintf("- 最频繁：%s（%d 笔）\n", maxFreq.Category, maxFreq.Count))

	// 维度3：单笔金额最大的分类
	maxSingle := summary[0]
	for _, s := range summary[1:] {
		if s.Total/float64(s.Count) > maxSingle.Total/float64(maxSingle.Count) {
			maxSingle = s
		}
	}
	if maxSingle.Category != maxFreq.Category {
		sb.WriteString(fmt.Sprintf("- 大额支出：%s（¥%.2f / %d 笔）\n", maxSingle.Category, maxSingle.Total, maxSingle.Count))
	}

	return sb.String()
}

func (e *Engine) GetMonthComparison(ctx context.Context, userID uint64, year, month int) (*PeriodComparison, error) {
	currentStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	currentEnd := currentStart.AddDate(0, 1, -1)
	previousStart := currentStart.AddDate(0, -1, 0)
	previousEnd := previousStart.AddDate(0, 1, -1)

	var currentTotal, previousTotal float64

	if err := e.db.WithContext(ctx).
		Table("bills").
		Select("COALESCE(SUM(amount), 0)").
		Where("user_id = ? AND is_deleted = ? AND bill_type = 'expense' AND date >= ? AND date <= ?", userID, false, currentStart.Format("2006-01-02"), currentEnd.Format("2006-01-02")).
		Scan(&currentTotal).Error; err != nil {
		return nil, fmt.Errorf("query current month failed: %w", err)
	}

	if err := e.db.WithContext(ctx).
		Table("bills").
		Select("COALESCE(SUM(amount), 0)").
		Where("user_id = ? AND is_deleted = ? AND bill_type = 'expense' AND date >= ? AND date <= ?", userID, false, previousStart.Format("2006-01-02"), previousEnd.Format("2006-01-02")).
		Scan(&previousTotal).Error; err != nil {
		return nil, fmt.Errorf("query previous month failed: %w", err)
	}

	var growthRate float64
	if previousTotal > 0 {
		growthRate = (currentTotal - previousTotal) / previousTotal
	}

	return &PeriodComparison{
		CurrentTotal:  currentTotal,
		PreviousTotal: previousTotal,
		Diff:          currentTotal - previousTotal,
		GrowthRate:    growthRate,
	}, nil
}

func (e *Engine) GetBudgetExecution(ctx context.Context, userID uint64, year, month int) ([]BudgetExecution, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	endDate := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.Local).Format("2006-01-02")

	// 读取预算，支持新的 categories JSON 字段
	type budgetRow struct {
		ID         uint64
		Name       string
		Category   string
		Categories string
		Amount     float64
	}
	var budgets []budgetRow
	if err := e.db.WithContext(ctx).
		Table("budgets").
		Select("id, name, category, categories, amount").
		Where("user_id = ? AND period = ? AND is_deleted = ?", userID, "monthly", false).
		Scan(&budgets).Error; err != nil {
		return nil, fmt.Errorf("query budgets failed: %w", err)
	}

	// 查询当月所有支出的分类汇总
	type spentRow struct {
		Category string
		Spent    float64
	}
	var spents []spentRow
	if err := e.db.WithContext(ctx).
		Table("bills").
		Select("category, SUM(amount) as spent").
		Where("user_id = ? AND is_deleted = ? AND bill_type = 'expense' AND date >= ? AND date <= ?", userID, false, startDate, endDate).
		Group("category").
		Scan(&spents).Error; err != nil {
		return nil, fmt.Errorf("query spent failed: %w", err)
	}

	spentMap := make(map[string]float64)
	var totalSpent float64
	for _, s := range spents {
		spentMap[s.Category] = s.Spent
		totalSpent += s.Spent
	}

	var results []BudgetExecution
	for _, b := range budgets {
		var cats []string
		if b.Categories != "" && b.Categories != "null" && b.Categories != "[]" {
			_ = json.Unmarshal([]byte(b.Categories), &cats)
		}
		if len(cats) == 0 && b.Category != "" && b.Category != "总预算" {
			cats = []string{b.Category}
		}

		// 总预算：已花 = 当月所有支出之和
		var spent float64
		if len(cats) == 0 {
			spent = totalSpent
		} else {
			for _, cat := range cats {
				spent += spentMap[cat]
			}
		}

		var rate float64
		if b.Amount > 0 {
			rate = spent / b.Amount * 100
		}

		displayName := b.Name
		if displayName == "" {
			displayName = b.Category
		}

		results = append(results, BudgetExecution{
			Category:      displayName,
			BudgetAmount:  b.Amount,
			SpentAmount:   spent,
			Remaining:     b.Amount - spent,
			ExecutionRate: rate,
		})
	}
	return results, nil
}

func (e *Engine) GetTotalSpent(ctx context.Context, userID uint64, startDate, endDate string) (float64, error) {
	var total float64
	err := e.db.WithContext(ctx).
		Table("bills").
		Select("COALESCE(SUM(amount), 0)").
		Where("user_id = ? AND is_deleted = ? AND bill_type = 'expense' AND date >= ? AND date <= ?", userID, false, startDate, endDate).
		Scan(&total).Error
	return total, err
}
