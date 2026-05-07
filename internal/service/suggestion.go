package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sangchenglong/kapi/internal/analytics"
	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

// Suggestion 表示一条智能建议
type Suggestion struct {
	Type    string `json:"type"`    // budget_warning / no_record_today / periodic_reminder
	Title   string `json:"title"`
	Content string `json:"content"`
}

// SuggestionService 聚合多个数据源，生成用户建议
type SuggestionService struct {
	billDAO    *dao.BillDAO
	memoryDAO  *dao.MemoryDAO
	patternDAO *dao.PatternDAO
	analytics  analytics.Analyzer
}

// NewSuggestionService 创建 SuggestionService 实例
func NewSuggestionService(billDAO *dao.BillDAO, memoryDAO *dao.MemoryDAO, patternDAO *dao.PatternDAO, analyticsEng analytics.Analyzer) *SuggestionService {
	return &SuggestionService{
		billDAO:    billDAO,
		memoryDAO:  memoryDAO,
		patternDAO: patternDAO,
		analytics:  analyticsEng,
	}
}

// GetSuggestions 获取用户的智能建议列表
func (s *SuggestionService) GetSuggestions(ctx context.Context, userID uint64) ([]Suggestion, error) {
	suggestions := make([]Suggestion, 0, 4)

	// 1. 预算预警：当月预算执行率 > 80% 的生成提醒
	budgetSuggestions, err := s.checkBudgetWarnings(ctx, userID)
	if err != nil {
		log.Printf("failed to check budget warnings for user %d: %v", userID, err)
		// 非致命错误，继续检查其他建议
	}
	suggestions = append(suggestions, budgetSuggestions...)

	// 2. 今日未记账提醒
	noRecordSuggestion, err := s.checkTodayRecord(ctx, userID)
	if err != nil {
		log.Printf("failed to check today record for user %d: %v", userID, err)
	}
	if noRecordSuggestion != nil {
		suggestions = append(suggestions, *noRecordSuggestion)
	}

	// 3. 周期性提醒：检查 L2 层 periodic 类型事实
	periodicSuggestions, err := s.checkPeriodicReminders(ctx, userID)
	if err != nil {
		log.Printf("failed to check periodic reminders for user %d: %v", userID, err)
	}
	suggestions = append(suggestions, periodicSuggestions...)

	// 4. 序列模式推荐：基于最近一笔账单推荐下一步
	seqSuggestion := s.checkSequencePattern(ctx, userID)
	if seqSuggestion != nil {
		suggestions = append(suggestions, *seqSuggestion)
	}

	return suggestions, nil
}

// checkBudgetWarnings 检查预算执行率，> 80% 的生成预警
func (s *SuggestionService) checkBudgetWarnings(ctx context.Context, userID uint64) ([]Suggestion, error) {
	if s.analytics == nil {
		return nil, nil
	}

	now := time.Now()
	executions, err := s.analytics.GetBudgetExecution(ctx, userID, now.Year(), int(now.Month()))
	if err != nil {
		return nil, fmt.Errorf("get budget execution for user %d: %w", userID, err)
	}

	suggestions := make([]Suggestion, 0, len(executions))
	for _, exec := range executions {
		if exec.BudgetAmount <= 0 {
			continue
		}
		if exec.ExecutionRate > 80 {
			title := fmt.Sprintf("%s 预算预警", exec.Category)
			content := fmt.Sprintf("%s 已使用 %.0f%%（¥%.2f / ¥%.2f），剩余 ¥%.2f",
				exec.Category, exec.ExecutionRate, exec.SpentAmount, exec.BudgetAmount, exec.Remaining)
			suggestions = append(suggestions, Suggestion{
				Type:    "budget_warning",
				Title:   title,
				Content: content,
			})
		}
	}
	return suggestions, nil
}

// checkTodayRecord 检查今天是否有记账记录
func (s *SuggestionService) checkTodayRecord(ctx context.Context, userID uint64) (*Suggestion, error) {
	if s.billDAO == nil {
		return nil, nil
	}

	today := time.Now().Format("2006-01-02")
	bills, err := s.billDAO.ListByDateRange(ctx, userID, today, today)
	if err != nil {
		return nil, fmt.Errorf("list today bills for user %d: %w", userID, err)
	}

	if len(bills) == 0 {
		return &Suggestion{
			Type:    "no_record_today",
			Title:   "今日未记账",
			Content: "今天还没有记账哦，随手记一笔，养成好习惯~",
		}, nil
	}
	return nil, nil
}

// checkPeriodicReminders 检查 L2 层周期性事实，生成提醒
func (s *SuggestionService) checkPeriodicReminders(ctx context.Context, userID uint64) ([]Suggestion, error) {
	if s.memoryDAO == nil {
		return nil, nil
	}

	memories, err := s.memoryDAO.SearchByFactType(ctx, userID, "periodic")
	if err != nil {
		return nil, fmt.Errorf("search periodic facts for user %d: %w", userID, err)
	}

	if len(memories) == 0 {
		return nil, nil
	}

	suggestions := make([]Suggestion, 0, len(memories))
	for _, mem := range memories {
		if mem.Layer != model.MemoryLayerL2 {
			continue
		}
		suggestions = append(suggestions, Suggestion{
			Type:    "periodic_reminder",
			Title:   "周期性提醒",
			Content: mem.Content,
		})
	}
	return suggestions, nil
}

// checkSequencePattern 基于最近一笔账单查找序列模式推荐
func (s *SuggestionService) checkSequencePattern(ctx context.Context, userID uint64) *Suggestion {
	if s.patternDAO == nil || s.billDAO == nil {
		return nil
	}

	today := time.Now().Format("2006-01-02")
	bills, err := s.billDAO.ListByDateRange(ctx, userID, today, today)
	if err != nil || len(bills) == 0 {
		return nil
	}

	lastBill := bills[len(bills)-1]
	if lastBill.BillType != "expense" {
		return nil
	}

	seqs, err := s.patternDAO.FindSequence(ctx, userID, lastBill.Category, lastBill.Merchant)
	if err != nil || len(seqs) == 0 {
		return nil
	}

	seq := seqs[0]
	content := fmt.Sprintf("你刚记了%s，要记一笔%s吗？（约 ¥%.2f）",
		lastBill.Category, seq.NextCategory, seq.NextAvgAmount)
	if seq.NextMerchant != "" {
		content = fmt.Sprintf("你刚记了%s，要记一笔%s（%s）吗？（约 ¥%.2f）",
			lastBill.Category, seq.NextMerchant, seq.NextCategory, seq.NextAvgAmount)
	}

	return &Suggestion{
		Type:    "sequence_pattern",
		Title:   "关联消费提醒",
		Content: content,
	}
}
