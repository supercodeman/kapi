package model

import "time"

type OperationLog struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         uint64    `gorm:"index:idx_user_created;not null" json:"user_id"`
	SessionID      uint64    `gorm:"not null" json:"session_id"`
	OperationType  string    `gorm:"type:varchar(16);not null" json:"operation_type"`
	TargetTable    string    `gorm:"type:varchar(64);not null;index:idx_target" json:"target_table"`
	TargetID       uint64    `gorm:"not null;index:idx_target" json:"target_id"`
	BeforeSnapshot string    `gorm:"type:json" json:"before_snapshot,omitempty"`
	AfterSnapshot  string    `gorm:"type:json" json:"after_snapshot,omitempty"`
	TriggerText    string    `gorm:"type:varchar(512)" json:"trigger_text,omitempty"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index:idx_user_created" json:"created_at"`
}

func (OperationLog) TableName() string {
	return "operation_logs"
}
