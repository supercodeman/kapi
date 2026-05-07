package service

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

var ErrBudgetNotFound = errors.New("budget not found")

type BudgetService struct {
	budgetDAO *dao.BudgetDAO
}

func NewBudgetService(budgetDAO *dao.BudgetDAO) *BudgetService {
	return &BudgetService{budgetDAO: budgetDAO}
}

// Create 创建预算
// categories 是 JSON 数组字符串，如 ["食饮","社交"]
// Category 字段取 categories 中的第一个元素，兼容旧逻辑
func (s *BudgetService) Create(ctx context.Context, userID uint64, name, categories string, amount float64, period, budgetType string) (*model.Budget, error) {
	// 从 categories JSON 数组中提取第一个作为 Category（兼容旧数据）
	category := extractFirstCategory(categories)

	budget := &model.Budget{
		UserID:     userID,
		Name:       name,
		Category:   category,
		Categories: categories,
		Amount:     amount,
		Period:     period,
		BudgetType: budgetType,
	}
	if err := s.budgetDAO.Create(ctx, budget); err != nil {
		return nil, err
	}
	return budget, nil
}

func (s *BudgetService) List(ctx context.Context, userID uint64) ([]model.Budget, error) {
	return s.budgetDAO.List(ctx, userID)
}

func (s *BudgetService) GetByID(ctx context.Context, userID, budgetID uint64) (*model.Budget, error) {
	budget, err := s.budgetDAO.GetByID(ctx, userID, budgetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBudgetNotFound
		}
		return nil, err
	}
	return budget, nil
}

func (s *BudgetService) Update(ctx context.Context, userID, budgetID uint64, name, categories string, amount float64, period, budgetType string) (*model.Budget, error) {
	budget, err := s.GetByID(ctx, userID, budgetID)
	if err != nil {
		return nil, err
	}
	budget.Name = name
	budget.Category = extractFirstCategory(categories)
	budget.Categories = categories
	budget.Amount = amount
	budget.Period = period
	budget.BudgetType = budgetType
	if err := s.budgetDAO.Update(ctx, budget); err != nil {
		return nil, err
	}
	return budget, nil
}

func (s *BudgetService) Delete(ctx context.Context, userID, budgetID uint64) error {
	if _, err := s.GetByID(ctx, userID, budgetID); err != nil {
		return err
	}
	return s.budgetDAO.SoftDelete(ctx, userID, budgetID)
}

// extractFirstCategory 从 JSON 数组字符串中提取第一个元素
func extractFirstCategory(categoriesJSON string) string {
	var cats []string
	if err := json.Unmarshal([]byte(categoriesJSON), &cats); err == nil && len(cats) > 0 {
		return cats[0]
	}
	return categoriesJSON
}
