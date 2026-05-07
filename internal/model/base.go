package model

import "gorm.io/gorm"

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&Bill{},
		&Budget{},
		&Asset{},
		&Session{},
		&Memory{},
		&ConversationHistory{},
		&Skill{},
		&OperationLog{},
		&Category{},
		&ConsumptionPattern{},
		&SequencePattern{},
	)
}
