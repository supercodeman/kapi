package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

var ErrBillNotFound = errors.New("bill not found")

type BillService struct {
	billDAO *dao.BillDAO
}

func NewBillService(billDAO *dao.BillDAO) *BillService {
	return &BillService{billDAO: billDAO}
}

func (s *BillService) Create(ctx context.Context, userID uint64, billType string, amount float64, category, subCategory, merchant, date, note string, assetID uint64) (*model.Bill, error) {
	if billType == "" {
		billType = "expense"
	}
	bill := &model.Bill{
		UserID:      userID,
		BillType:    billType,
		Amount:      amount,
		Category:    category,
		SubCategory: subCategory,
		Merchant:    merchant,
		AssetID:     assetID,
		Date:        date,
		Note:        note,
	}
	if err := s.billDAO.Create(ctx, bill); err != nil {
		return nil, err
	}
	return bill, nil
}

func (s *BillService) List(ctx context.Context, userID uint64) ([]model.Bill, error) {
	return s.billDAO.List(ctx, userID)
}

func (s *BillService) ListByDateRange(ctx context.Context, userID uint64, startDate, endDate string) ([]model.Bill, error) {
	return s.billDAO.ListByDateRange(ctx, userID, startDate, endDate)
}

func (s *BillService) GetByID(ctx context.Context, userID, billID uint64) (*model.Bill, error) {
	bill, err := s.billDAO.GetByID(ctx, userID, billID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBillNotFound
		}
		return nil, err
	}
	return bill, nil
}

func (s *BillService) Update(ctx context.Context, userID, billID uint64, amount float64, category, merchant, date, note string) (*model.Bill, error) {
	bill, err := s.GetByID(ctx, userID, billID)
	if err != nil {
		return nil, err
	}
	bill.Amount = amount
	bill.Category = category
	bill.Merchant = merchant
	bill.Date = date
	bill.Note = note
	if err := s.billDAO.Update(ctx, bill); err != nil {
		return nil, err
	}
	return bill, nil
}

func (s *BillService) Delete(ctx context.Context, userID, billID uint64) error {
	if _, err := s.GetByID(ctx, userID, billID); err != nil {
		return err
	}
	return s.billDAO.SoftDelete(ctx, userID, billID)
}
