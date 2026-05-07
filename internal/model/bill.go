package model

import "time"

type Bill struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint64    `gorm:"index:idx_user_date;index:idx_user_category;not null" json:"user_id"`
	BillType    string    `gorm:"type:varchar(8);not null;default:expense" json:"bill_type"`
	Amount      float64   `gorm:"type:decimal(12,2);not null" json:"amount"`
	Category    string    `gorm:"type:varchar(32);not null;index:idx_user_category" json:"category"`
	SubCategory string    `gorm:"type:varchar(32)" json:"sub_category"`
	Merchant    string    `gorm:"type:varchar(128)" json:"merchant"`
	AssetID     uint64    `gorm:"not null;default:0" json:"asset_id"`
	ToAssetID   uint64    `gorm:"not null;default:0" json:"to_asset_id"`
	Date        string    `gorm:"type:date;not null;index:idx_user_date" json:"date"`
	Note        string    `gorm:"type:varchar(255)" json:"note"`
	IsDeleted   bool      `gorm:"not null;default:false" json:"-"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Bill) TableName() string {
	return "bills"
}
