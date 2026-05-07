package engine

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/sangchenglong/kapi/internal/model"
)

// updateUserProfile 异步更新 L1 用户画像（每用户每天最多一次）
func (e *Engine) updateUserProfile(ctx context.Context, userID uint64) {
	if e.memMgr == nil {
		return
	}

	existing, _ := e.memMgr.GetUserProfile(ctx, userID)
	if len(existing) > 0 {
		for _, m := range existing {
			if m.CreatedAt.After(time.Now().Add(-24 * time.Hour)) {
				return
			}
		}
	}

	profile := e.buildProfile(ctx, userID)
	if profile == "" {
		return
	}

	mem := &model.Memory{
		UserID:          userID,
		Layer:           model.MemoryLayerL1,
		Content:         profile,
		FactType:        "user_profile",
		Source:          model.MemorySourceSystemInferred,
		Metadata:        "{}",
		RelatedOpLogIDs: "[]",
	}
	if err := e.memMgr.SaveMemory(ctx, mem); err != nil {
		log.Printf("failed to save L1 profile for user %d: %v", userID, err)
	}
}

func (e *Engine) buildProfile(ctx context.Context, userID uint64) string {
	bills, err := e.toolExecutor.billSvc.List(ctx, userID)
	if err != nil || len(bills) < 5 {
		return ""
	}

	// 统计分类频次和金额
	type catStat struct {
		count int
		total float64
	}
	stats := make(map[string]*catStat)
	var totalExpense float64
	var expenseCount int

	for _, b := range bills {
		if b.BillType != "expense" || b.IsDeleted {
			continue
		}
		expenseCount++
		totalExpense += b.Amount
		s, ok := stats[b.Category]
		if !ok {
			s = &catStat{}
			stats[b.Category] = s
		}
		s.count++
		s.total += b.Amount
	}

	if expenseCount < 5 {
		return ""
	}

	// 按频次排序取 Top 3
	type ranked struct {
		cat  string
		stat *catStat
	}
	var ranked_ []ranked
	for cat, s := range stats {
		ranked_ = append(ranked_, ranked{cat, s})
	}
	sort.Slice(ranked_, func(i, j int) bool {
		return ranked_[i].stat.count > ranked_[j].stat.count
	})

	var parts []string

	// 高频分类
	topN := 3
	if len(ranked_) < topN {
		topN = len(ranked_)
	}
	var topCats []string
	for i := 0; i < topN; i++ {
		topCats = append(topCats, ranked_[i].cat)
	}
	parts = append(parts, fmt.Sprintf("高频消费分类：%s", strings.Join(topCats, "、")))

	// 平均单笔消费
	avg := totalExpense / float64(expenseCount)
	parts = append(parts, fmt.Sprintf("平均单笔消费约 ¥%.0f", avg))

	// 总账单数
	parts = append(parts, fmt.Sprintf("累计 %d 笔支出记录", expenseCount))

	return strings.Join(parts, "；")
}
