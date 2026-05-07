package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

type BudgetDAO struct {
	db *gorm.DB
}

func NewBudgetDAO(db *gorm.DB) *BudgetDAO {
	return &BudgetDAO{db: db}
}

func (d *BudgetDAO) activeScope() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_deleted = ?", false)
	}
}

func (d *BudgetDAO) Create(ctx context.Context, budget *model.Budget) error {
	return d.db.WithContext(ctx).Create(budget).Error
}

func (d *BudgetDAO) List(ctx context.Context, userID uint64) ([]model.Budget, error) {
	var budgets []model.Budget
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("user_id = ?", userID).
		Order("category ASC").
		Find(&budgets).Error
	return budgets, err
}

func (d *BudgetDAO) GetByID(ctx context.Context, userID, budgetID uint64) (*model.Budget, error) {
	var budget model.Budget
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("id = ? AND user_id = ?", budgetID, userID).
		First(&budget).Error
	if err != nil {
		return nil, err
	}
	return &budget, nil
}

func (d *BudgetDAO) Update(ctx context.Context, budget *model.Budget) error {
	return d.db.WithContext(ctx).Save(budget).Error
}

func (d *BudgetDAO) SoftDelete(ctx context.Context, userID, budgetID uint64) error {
	return d.db.WithContext(ctx).
		Model(&model.Budget{}).
		Where("id = ? AND user_id = ?", budgetID, userID).
		Update("is_deleted", true).Error
}
