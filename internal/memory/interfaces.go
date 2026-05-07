package memory

import (
	"context"

	"github.com/sangchenglong/kapi/internal/model"
)

type Store interface {
	SaveMemory(ctx context.Context, memory *model.Memory) error
	UpdateMemory(ctx context.Context, memory *model.Memory) error
	DeleteMemory(ctx context.Context, userID, memoryID uint64) error
}

type Retriever interface {
	GetUserProfile(ctx context.Context, userID uint64) ([]model.Memory, error)
	SearchFacts(ctx context.Context, userID uint64, query string, topK int) ([]model.Memory, error)
	SearchEpisodes(ctx context.Context, userID uint64, query string, topK int) ([]model.Memory, error)
	GetRecentConversation(ctx context.Context, userID, sessionID uint64, limit int) ([]ConversationMessage, error)
}
