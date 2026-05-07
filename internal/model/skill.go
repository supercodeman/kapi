package model

import "time"

type Skill struct {
	ID                   uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name                 string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"name"`
	Page                 string    `gorm:"type:json;not null" json:"page"`
	Type                 string    `gorm:"type:varchar(8);not null" json:"type"`
	TriggerDesc          string    `gorm:"type:varchar(255);not null" json:"trigger_desc"`
	ParamsSchema         string    `gorm:"type:json;not null" json:"params_schema"`
	PromptTemplate       string    `gorm:"type:text;not null" json:"prompt_template"`
	Status               string    `gorm:"type:varchar(16);not null;default:enabled;index:idx_status" json:"status"`
	OverridesFileVersion string    `gorm:"type:varchar(32)" json:"overrides_file_version"`
	GrayRatio            int       `gorm:"type:tinyint unsigned;not null;default:100" json:"gray_ratio"`
	Version              string    `gorm:"type:varchar(32);not null" json:"version"`
	CreatedAt            time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Skill) TableName() string {
	return "skills"
}
