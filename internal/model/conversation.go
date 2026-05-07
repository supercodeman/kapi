package model

import "time"

type ConversationHistory struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint64    `gorm:"index:idx_user_session;not null" json:"user_id"`
	SessionID   uint64    `gorm:"index:idx_user_session;index:idx_session_created;not null" json:"session_id"`
	Role        string    `gorm:"type:varchar(16);not null" json:"role"`
	Content     string    `gorm:"type:text;not null" json:"content"`
	PageContext string    `gorm:"type:varchar(32);not null" json:"page_context"`
	ToolCalls   string    `gorm:"type:json" json:"tool_calls,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index:idx_session_created" json:"created_at"`
}

func (ConversationHistory) TableName() string {
	return "conversation_history"
}
