package model

import "time"

type ConsumptionPattern struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	UserID      uint64    `gorm:"uniqueIndex:idx_cp_unique;not null" json:"user_id"`
	Merchant    string    `gorm:"uniqueIndex:idx_cp_unique;type:varchar(100)" json:"merchant"`
	Category    string    `gorm:"uniqueIndex:idx_cp_unique;type:varchar(50);not null" json:"category"`
	SubCategory string    `gorm:"type:varchar(50)" json:"sub_category"`
	LastAmount  float64   `gorm:"type:decimal(12,2)" json:"last_amount"`
	Count       int       `gorm:"column:cnt" json:"count"`
	RecentCount int       `gorm:"column:recent_cnt" json:"recent_count"`
	EffectScore float64   `gorm:"type:decimal(8,4)" json:"effect_score"`
	LastDate    string    `gorm:"type:varchar(10)" json:"last_date"`
	Frequency   string    `gorm:"type:varchar(20)" json:"frequency"`
	Disabled    bool      `gorm:"default:false" json:"disabled"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type SequencePattern struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	UserID        uint64    `gorm:"uniqueIndex:idx_sp_unique;not null" json:"user_id"`
	PrevCategory  string    `gorm:"uniqueIndex:idx_sp_unique;type:varchar(50);not null" json:"prev_category"`
	PrevMerchant  string    `gorm:"uniqueIndex:idx_sp_unique;type:varchar(100)" json:"prev_merchant"`
	NextCategory  string    `gorm:"uniqueIndex:idx_sp_unique;type:varchar(50);not null" json:"next_category"`
	NextMerchant  string    `gorm:"uniqueIndex:idx_sp_unique;type:varchar(100)" json:"next_merchant"`
	NextAvgAmount float64   `gorm:"type:decimal(12,2)" json:"next_avg_amount"`
	Count         int       `gorm:"column:cnt" json:"count"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
