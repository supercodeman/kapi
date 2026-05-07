package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

type OperationLogDAO struct {
	db *gorm.DB
}

func NewOperationLogDAO(db *gorm.DB) *OperationLogDAO {
	return &OperationLogDAO{db: db}
}

func (d *OperationLogDAO) Create(ctx context.Context, log *model.OperationLog) error {
	return d.db.WithContext(ctx).Create(log).Error
}

func (d *OperationLogDAO) ListByUser(ctx context.Context, userID uint64, limit int) ([]model.OperationLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var logs []model.OperationLog
	err := d.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

func (d *OperationLogDAO) GetByID(ctx context.Context, userID, logID uint64) (*model.OperationLog, error) {
	var opLog model.OperationLog
	err := d.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", logID, userID).
		First(&opLog).Error
	if err != nil {
		return nil, err
	}
	return &opLog, nil
}

func (d *OperationLogDAO) ListByTarget(ctx context.Context, userID uint64, table string, targetID uint64) ([]model.OperationLog, error) {
	var logs []model.OperationLog
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND target_table = ? AND target_id = ?", userID, table, targetID).
		Order("created_at DESC").
		Find(&logs).Error
	return logs, err
}
