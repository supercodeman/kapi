package model

import "time"

type Category struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint64    `gorm:"index:idx_user_parent;not null;default:0" json:"user_id"`
	ParentID  uint64    `gorm:"index:idx_user_parent;not null;default:0" json:"parent_id"`
	Name      string    `gorm:"type:varchar(32);not null" json:"name"`
	Icon      string    `gorm:"type:varchar(16)" json:"icon"`
	BillType  string    `gorm:"type:varchar(8);not null;default:expense" json:"bill_type"`
	Necessity string    `gorm:"type:varchar(16)" json:"necessity"`
	SortOrder int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Category) TableName() string {
	return "categories"
}
