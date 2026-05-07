package model

import "time"

type Session struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint64    `gorm:"index:idx_user_status;not null" json:"user_id"`
	LastActiveAt time.Time `gorm:"not null" json:"last_active_at"`
	PageContext string    `gorm:"type:varchar(32);not null;default:'首页'" json:"page_context"`
	Status      string    `gorm:"type:varchar(16);not null;default:active;index:idx_user_status" json:"status"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Session) TableName() string {
	return "sessions"
}
