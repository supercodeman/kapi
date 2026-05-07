package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const cacheTTL = 25 * time.Hour

type CachedEngine struct {
	engine *Engine
	rdb    *redis.Client
}

func NewCachedEngine(engine *Engine, rdb *redis.Client) *CachedEngine {
	return &CachedEngine{engine: engine, rdb: rdb}
}

func (c *CachedEngine) GetCategorySummary(ctx context.Context, userID uint64, startDate, endDate string) ([]BillSummary, error) {
	key := fmt.Sprintf("cache:cat_summary:%d:%s:%s", userID, startDate, endDate)

	if data, err := c.rdb.Get(ctx, key).Bytes(); err == nil {
		var result []BillSummary
		if json.Unmarshal(data, &result) == nil {
			return result, nil
		}
	}

	result, err := c.engine.GetCategorySummary(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	if data, err := json.Marshal(result); err == nil {
		c.rdb.Set(ctx, key, data, cacheTTL)
	}
	return result, nil
}

func (c *CachedEngine) GetMonthComparison(ctx context.Context, userID uint64, year, month int) (*PeriodComparison, error) {
	key := fmt.Sprintf("cache:month_comp:%d:%d:%d", userID, year, month)

	if data, err := c.rdb.Get(ctx, key).Bytes(); err == nil {
		var result PeriodComparison
		if json.Unmarshal(data, &result) == nil {
			return &result, nil
		}
	}

	result, err := c.engine.GetMonthComparison(ctx, userID, year, month)
	if err != nil {
		return nil, err
	}

	if data, err := json.Marshal(result); err == nil {
		c.rdb.Set(ctx, key, data, cacheTTL)
	}
	return result, nil
}

func (c *CachedEngine) GetBudgetExecution(ctx context.Context, userID uint64, year, month int) ([]BudgetExecution, error) {
	key := fmt.Sprintf("cache:budget_exec:%d:%d:%d", userID, year, month)

	if data, err := c.rdb.Get(ctx, key).Bytes(); err == nil {
		var result []BudgetExecution
		if json.Unmarshal(data, &result) == nil {
			return result, nil
		}
	}

	result, err := c.engine.GetBudgetExecution(ctx, userID, year, month)
	if err != nil {
		return nil, err
	}

	if data, err := json.Marshal(result); err == nil {
		c.rdb.Set(ctx, key, data, cacheTTL)
	}
	return result, nil
}

func (c *CachedEngine) GetTotalSpent(ctx context.Context, userID uint64, startDate, endDate string) (float64, error) {
	key := fmt.Sprintf("cache:total_spent:%d:%s:%s", userID, startDate, endDate)

	if val, err := c.rdb.Get(ctx, key).Float64(); err == nil {
		return val, nil
	}

	result, err := c.engine.GetTotalSpent(ctx, userID, startDate, endDate)
	if err != nil {
		return 0, err
	}

	c.rdb.Set(ctx, key, result, cacheTTL)
	return result, nil
}

// InvalidateUser 写操作后清除该用户的所有分析缓存
func (c *CachedEngine) InvalidateUser(ctx context.Context, userID uint64) {
	pattern := fmt.Sprintf("cache:*:%d:*", userID)
	iter := c.rdb.Scan(ctx, 0, pattern, 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if len(keys) > 0 {
		if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
			log.Printf("failed to invalidate cache for user %d: %v", userID, err)
		}
	}
}
