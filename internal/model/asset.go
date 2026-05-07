package model

import "time"

type Asset struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint64    `gorm:"index:idx_user_type;not null" json:"user_id"`
	Name      string    `gorm:"type:varchar(64);not null" json:"name"`
	Type      string    `gorm:"type:varchar(16);not null;index:idx_user_type" json:"type"`
	Balance   float64   `gorm:"type:decimal(14,2);not null;default:0" json:"balance"`
	IsDeleted bool      `gorm:"not null;default:false" json:"-"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Asset) TableName() string {
	return "assets"
}
