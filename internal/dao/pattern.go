package dao

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sangchenglong/kapi/internal/model"
)

type PatternDAO struct {
	db *gorm.DB
}

func NewPatternDAO(db *gorm.DB) *PatternDAO {
	return &PatternDAO{db: db}
}

// UpsertConsumptionPattern 插入或更新消费模式（按 user_id + merchant + category 唯一）
func (d *PatternDAO) UpsertConsumptionPattern(ctx context.Context, p *model.ConsumptionPattern) error {
	return d.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "merchant"}, {Name: "category"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_amount", "cnt", "recent_cnt", "effect_score", "last_date", "frequency", "sub_category", "updated_at"}),
		}).Create(p).Error
}

func (d *PatternDAO) ListByUser(ctx context.Context, userID uint64) ([]model.ConsumptionPattern, error) {
	var patterns []model.ConsumptionPattern
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND disabled = ?", userID, false).
		Order("effect_score DESC").
		Find(&patterns).Error
	return patterns, err
}

func (d *PatternDAO) FindByMerchant(ctx context.Context, userID uint64, merchant string) (*model.ConsumptionPattern, error) {
	var p model.ConsumptionPattern
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND merchant = ? AND disabled = ?", userID, merchant, false).
		Order("effect_score DESC").
		First(&p).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &p, err
}

func (d *PatternDAO) FindByCategory(ctx context.Context, userID uint64, category string) ([]model.ConsumptionPattern, error) {
	var patterns []model.ConsumptionPattern
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND category = ? AND disabled = ?", userID, category, false).
		Order("effect_score DESC").
		Find(&patterns).Error
	return patterns, err
}

func (d *PatternDAO) DisablePattern(ctx context.Context, userID, patternID uint64) error {
	return d.db.WithContext(ctx).
		Model(&model.ConsumptionPattern{}).
		Where("id = ? AND user_id = ?", patternID, userID).
		Update("disabled", true).Error
}

func (d *PatternDAO) DeleteByUser(ctx context.Context, userID uint64) error {
	return d.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.ConsumptionPattern{}).Error
}

// UpsertSequencePattern 插入或更新序列模式
func (d *PatternDAO) UpsertSequencePattern(ctx context.Context, p *model.SequencePattern) error {
	return d.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "prev_category"}, {Name: "prev_merchant"}, {Name: "next_category"}, {Name: "next_merchant"}},
			DoUpdates: clause.AssignmentColumns([]string{"next_avg_amount", "cnt", "updated_at"}),
		}).Create(p).Error
}

func (d *PatternDAO) FindSequence(ctx context.Context, userID uint64, prevCategory, prevMerchant string) ([]model.SequencePattern, error) {
	var patterns []model.SequencePattern
	q := d.db.WithContext(ctx).Where("user_id = ? AND prev_category = ? AND cnt >= 3", userID, prevCategory)
	if prevMerchant != "" {
		q = q.Where("prev_merchant = ?", prevMerchant)
	}
	err := q.Order("cnt DESC").Find(&patterns).Error
	return patterns, err
}

func (d *PatternDAO) ListSequencesByUser(ctx context.Context, userID uint64) ([]model.SequencePattern, error) {
	var patterns []model.SequencePattern
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND cnt >= 3", userID).
		Order("cnt DESC").
		Find(&patterns).Error
	return patterns, err
}

func (d *PatternDAO) DeleteSequencesByUser(ctx context.Context, userID uint64) error {
	return d.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.SequencePattern{}).Error
}
