package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// PendingAction 表示一个等待用户确认的操作（支持单笔或多笔）
type PendingAction struct {
	UserID      uint64
	SessionID   uint64
	ToolName    string
	Params      map[string]any
	BatchParams []map[string]any // 多笔操作时使用
	ExpiresAt   time.Time
}

// PendingStore 管理待确认操作，按 userID:sessionID 存储
type PendingStore struct {
	mu    sync.RWMutex
	store map[string]*PendingAction
}

func NewPendingStore() *PendingStore {
	return &PendingStore{store: make(map[string]*PendingAction)}
}

func pendingKey(userID, sessionID uint64) string {
	return fmt.Sprintf("%d:%d", userID, sessionID)
}

// Set 存储一个待确认操作（5 分钟过期）
func (ps *PendingStore) Set(userID, sessionID uint64, toolName string, params map[string]any) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.store[pendingKey(userID, sessionID)] = &PendingAction{
		UserID:    userID,
		SessionID: sessionID,
		ToolName:  toolName,
		Params:    params,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
}

// Get 获取并删除待确认操作（一次性消费）
func (ps *PendingStore) Get(userID, sessionID uint64) *PendingAction {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	key := pendingKey(userID, sessionID)
	action, ok := ps.store[key]
	if !ok {
		return nil
	}
	delete(ps.store, key)
	if time.Now().After(action.ExpiresAt) {
		return nil
	}
	return action
}

// Clear 清除指定用户的待确认操作
func (ps *PendingStore) Clear(userID, sessionID uint64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.store, pendingKey(userID, sessionID))
}

// mergePendingParams 把用户消息作为补充参数合并进 PendingAction
func mergePendingParams(action *PendingAction, userMsg string) {
	msg := strings.TrimSpace(userMsg)
	if msg == "" {
		return
	}

	// 尝试从用户消息中提取金额——用户说了金额就用用户的，覆盖预填值
	numbers := ParseChineseNumber(msg)
	if len(numbers) > 0 {
		action.Params["amount"] = numbers[0]
	}

	// 如果是 create_bill 且 category 为空或模糊，用用户消息作为 category
	if action.ToolName == "create_bill" {
		cat, _ := action.Params["category"].(string)
		if isVagueIncomeCategory(cat) || cat == "" {
			result := NormalizeCategoryV2(msg, "", msg)
			billType, _ := action.Params["bill_type"].(string)
			// 收入场景下，"其他" 映射为 "其他收入"
			if billType == "income" && result.Category == "其他" {
				result.Category = "其他收入"
			}
			action.Params["category"] = result.Category
			if result.SubCategory != "" {
				action.Params["sub_category"] = result.SubCategory
			}
		}
	}
}
