package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

var ErrAssetNotFound = errors.New("asset not found")

type AssetService struct {
	assetDAO *dao.AssetDAO
}

func NewAssetService(assetDAO *dao.AssetDAO) *AssetService {
	return &AssetService{assetDAO: assetDAO}
}

func (s *AssetService) Create(ctx context.Context, userID uint64, name, assetType string, balance float64) (*model.Asset, error) {
	asset := &model.Asset{
		UserID:  userID,
		Name:    name,
		Type:    assetType,
		Balance: balance,
	}
	if err := s.assetDAO.Create(ctx, asset); err != nil {
		return nil, err
	}
	return asset, nil
}

func (s *AssetService) List(ctx context.Context, userID uint64) ([]model.Asset, error) {
	return s.assetDAO.List(ctx, userID)
}

func (s *AssetService) GetByID(ctx context.Context, userID, assetID uint64) (*model.Asset, error) {
	asset, err := s.assetDAO.GetByID(ctx, userID, assetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}
	return asset, nil
}

func (s *AssetService) Update(ctx context.Context, userID, assetID uint64, name, assetType string, balance float64) (*model.Asset, error) {
	asset, err := s.GetByID(ctx, userID, assetID)
	if err != nil {
		return nil, err
	}
	asset.Name = name
	asset.Type = assetType
	asset.Balance = balance
	if err := s.assetDAO.Update(ctx, asset); err != nil {
		return nil, err
	}
	return asset, nil
}

func (s *AssetService) Delete(ctx context.Context, userID, assetID uint64) error {
	if _, err := s.GetByID(ctx, userID, assetID); err != nil {
		return err
	}
	return s.assetDAO.SoftDelete(ctx, userID, assetID)
}

// UpdateBalance 原子更新资产余额（delta 可正可负）
func (s *AssetService) UpdateBalance(ctx context.Context, userID, assetID uint64, delta float64) error {
	return s.assetDAO.UpdateBalance(ctx, userID, assetID, delta)
}
