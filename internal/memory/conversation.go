package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type ConversationMessage struct {
	Role        string `json:"role"`
	Content     string `json:"content"`
	PageContext string `json:"page_context"`
	CreatedAt   int64  `json:"created_at"`
}

type ConversationStore struct {
	rdb        *redis.Client
	maxHistory int
	ttl        time.Duration
}

func NewConversationStore(rdb *redis.Client, maxHistory int, ttl time.Duration) *ConversationStore {
	if maxHistory <= 0 {
		maxHistory = 50
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &ConversationStore{rdb: rdb, maxHistory: maxHistory, ttl: ttl}
}

func (s *ConversationStore) sessionKey(userID, sessionID uint64) string {
	return fmt.Sprintf("session:%d:%d", userID, sessionID)
}

func (s *ConversationStore) AddMessage(ctx context.Context, userID, sessionID uint64, msg *ConversationMessage) error {
	key := s.sessionKey(userID, sessionID)
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	pipe := s.rdb.Pipeline()
	pipe.RPush(ctx, key, data)
	pipe.LTrim(ctx, key, int64(-s.maxHistory), -1)
	pipe.Expire(ctx, key, s.ttl)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *ConversationStore) GetRecent(ctx context.Context, userID, sessionID uint64, limit int) ([]ConversationMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	key := s.sessionKey(userID, sessionID)
	results, err := s.rdb.LRange(ctx, key, int64(-limit), -1).Result()
	if err != nil {
		return nil, err
	}

	messages := make([]ConversationMessage, 0, len(results))
	for _, r := range results {
		var msg ConversationMessage
		if err := json.Unmarshal([]byte(r), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *ConversationStore) GetAll(ctx context.Context, userID, sessionID uint64) ([]ConversationMessage, error) {
	key := s.sessionKey(userID, sessionID)
	results, err := s.rdb.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	messages := make([]ConversationMessage, 0, len(results))
	for _, r := range results {
		var msg ConversationMessage
		if err := json.Unmarshal([]byte(r), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *ConversationStore) Clear(ctx context.Context, userID, sessionID uint64) error {
	key := s.sessionKey(userID, sessionID)
	return s.rdb.Del(ctx, key).Err()
}
