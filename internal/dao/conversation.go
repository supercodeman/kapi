package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/sangchenglong/kapi/internal/model"
)

type ConversationDAO struct {
	db *gorm.DB
}

func NewConversationDAO(db *gorm.DB) *ConversationDAO {
	return &ConversationDAO{db: db}
}

func (d *ConversationDAO) Create(ctx context.Context, msg *model.ConversationHistory) error {
	return d.db.WithContext(ctx).Create(msg).Error
}

func (d *ConversationDAO) ListBySession(ctx context.Context, userID, sessionID uint64, limit int) ([]model.ConversationHistory, error) {
	var messages []model.ConversationHistory
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error
	// 反转为时间正序
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, err
}

func (d *ConversationDAO) BatchCreate(ctx context.Context, messages []model.ConversationHistory) error {
	if len(messages) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Create(&messages).Error
}
