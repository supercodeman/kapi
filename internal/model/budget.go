package model

import "time"

type Budget struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint64    `gorm:"index:idx_user_period;not null" json:"user_id"`
	Name       string    `gorm:"type:varchar(64)" json:"name"`
	Category   string    `gorm:"type:varchar(32)" json:"category"`                       // 兼容旧数据
	Categories string    `gorm:"type:json" json:"categories"`                             // JSON 数组，如 ["食饮","社交"]
	Amount     float64   `gorm:"type:decimal(12,2);not null" json:"amount"`
	Period     string    `gorm:"type:varchar(16);not null;default:monthly;index:idx_user_period" json:"period"`
	BudgetType string    `gorm:"type:varchar(16);default:optional" json:"budget_type"`    // optional / essential
	IsDeleted  bool      `gorm:"not null;default:false" json:"-"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Budget) TableName() string {
	return "budgets"
}
