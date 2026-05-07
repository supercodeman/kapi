package analytics

import "context"

// Analyzer 是分析引擎的接口，Engine 和 CachedEngine 都实现它
type Analyzer interface {
	GetCategorySummary(ctx context.Context, userID uint64, startDate, endDate string) ([]BillSummary, error)
	GetMonthComparison(ctx context.Context, userID uint64, year, month int) (*PeriodComparison, error)
	GetBudgetExecution(ctx context.Context, userID uint64, year, month int) ([]BudgetExecution, error)
	GetTotalSpent(ctx context.Context, userID uint64, startDate, endDate string) (float64, error)
}
