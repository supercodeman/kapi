package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

type AssetDAO struct {
	db *gorm.DB
}

func NewAssetDAO(db *gorm.DB) *AssetDAO {
	return &AssetDAO{db: db}
}

func (d *AssetDAO) activeScope() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_deleted = ?", false)
	}
}

func (d *AssetDAO) Create(ctx context.Context, asset *model.Asset) error {
	return d.db.WithContext(ctx).Create(asset).Error
}

func (d *AssetDAO) List(ctx context.Context, userID uint64) ([]model.Asset, error) {
	var assets []model.Asset
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("user_id = ?", userID).
		Order("type ASC, name ASC").
		Find(&assets).Error
	return assets, err
}

func (d *AssetDAO) GetByID(ctx context.Context, userID, assetID uint64) (*model.Asset, error) {
	var asset model.Asset
	err := d.db.WithContext(ctx).Scopes(d.activeScope()).
		Where("id = ? AND user_id = ?", assetID, userID).
		First(&asset).Error
	if err != nil {
		return nil, err
	}
	return &asset, nil
}

func (d *AssetDAO) Update(ctx context.Context, asset *model.Asset) error {
	return d.db.WithContext(ctx).Save(asset).Error
}

func (d *AssetDAO) SoftDelete(ctx context.Context, userID, assetID uint64) error {
	return d.db.WithContext(ctx).
		Model(&model.Asset{}).
		Where("id = ? AND user_id = ?", assetID, userID).
		Update("is_deleted", true).Error
}

// UpdateBalance 原子更新资产余额（delta 可正可负）
func (d *AssetDAO) UpdateBalance(ctx context.Context, userID, assetID uint64, delta float64) error {
	return d.db.WithContext(ctx).
		Model(&model.Asset{}).
		Where("id = ? AND user_id = ? AND is_deleted = ?", assetID, userID, false).
		Update("balance", gorm.Expr("balance + ?", delta)).Error
}
