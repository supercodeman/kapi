package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

type BillDAO struct {
	db *gorm.DB
}

func NewBillDAO(db *gorm.DB) *BillDAO {
	return &BillDAO{db: db}
}

func (d *BillDAO) Create(ctx context.Context, bill *model.Bill) error {
	return d.db.WithContext(ctx).Create(bill).Error
}

func (d *BillDAO) List(ctx context.Context, userID uint64) ([]model.Bill, error) {
	var bills []model.Bill
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND is_deleted = ?", userID, false).
		Order("date DESC, id DESC").
		Find(&bills).Error
	return bills, err
}

func (d *BillDAO) ListByDateRange(ctx context.Context, userID uint64, startDate, endDate string) ([]model.Bill, error) {
	var bills []model.Bill
	query := d.db.WithContext(ctx).Where("user_id = ? AND is_deleted = ?", userID, false)
	if startDate != "" {
		query = query.Where("date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("date <= ?", endDate)
	}
	err := query.Order("date DESC, id DESC").Find(&bills).Error
	return bills, err
}

func (d *BillDAO) GetByID(ctx context.Context, userID, billID uint64) (*model.Bill, error) {
	var bill model.Bill
	err := d.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_deleted = ?", billID, userID, false).
		First(&bill).Error
	if err != nil {
		return nil, err
	}
	return &bill, nil
}

func (d *BillDAO) Update(ctx context.Context, bill *model.Bill) error {
	return d.db.WithContext(ctx).Save(bill).Error
}

func (d *BillDAO) SoftDelete(ctx context.Context, userID, billID uint64) error {
	return d.db.WithContext(ctx).
		Model(&model.Bill{}).
		Where("id = ? AND user_id = ?", billID, userID).
		Update("is_deleted", true).Error
}
