package model

import "time"

type MemoryLayer string

const (
	MemoryLayerL1 MemoryLayer = "L1"
	MemoryLayerL2 MemoryLayer = "L2"
	MemoryLayerL3 MemoryLayer = "L3"
)

type MemorySource string

const (
	MemorySourceUserStated    MemorySource = "user_stated"
	MemorySourceSystemInferred MemorySource = "system_inferred"
	MemorySourceToolResult    MemorySource = "tool_result"
)

type Memory struct {
	ID              uint64       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID          uint64       `gorm:"index:idx_user_layer;index:idx_user_fact_type;not null" json:"user_id"`
	Layer           MemoryLayer  `gorm:"type:varchar(4);not null;index:idx_user_layer" json:"layer"`
	Content         string       `gorm:"type:text;not null" json:"content"`
	FactType        string       `gorm:"type:varchar(32);index:idx_user_fact_type" json:"fact_type,omitempty"`
	Metadata        string       `gorm:"type:json" json:"metadata,omitempty"`
	RelatedOpLogIDs string       `gorm:"type:json" json:"related_op_log_ids,omitempty"`
	Source          MemorySource `gorm:"type:varchar(20);not null" json:"source"`
	EmbeddingID     string       `gorm:"type:varchar(64)" json:"embedding_id,omitempty"`
	IsVerified      bool         `gorm:"not null;default:false" json:"is_verified"`
	IsDeleted       bool         `gorm:"not null;default:false" json:"-"`
	CreatedAt       time.Time    `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time    `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Memory) TableName() string {
	return "memories"
}
