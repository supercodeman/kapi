package analytics

import (
	"context"
	"log"
	"math"
	"sort"
	"time"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

type PatternExtractor struct {
	billDAO    *dao.BillDAO
	patternDAO *dao.PatternDAO
}

func NewPatternExtractor(billDAO *dao.BillDAO, patternDAO *dao.PatternDAO) *PatternExtractor {
	return &PatternExtractor{billDAO: billDAO, patternDAO: patternDAO}
}

// ExtractConsumptionPatterns 从账单中提取 L5 消费模式
func (pe *PatternExtractor) ExtractConsumptionPatterns(ctx context.Context, userID uint64) error {
	bills, err := pe.billDAO.List(ctx, userID)
	if err != nil {
		return err
	}

	now := time.Now()
	type key struct{ merchant, category string }
	type entry struct {
		amounts     []float64
		weights     []float64
		dates       []string
		subCategory string
	}
	groups := make(map[key]*entry)

	for _, b := range bills {
		if b.BillType != "expense" || b.IsDeleted {
			continue
		}
		dateStr := b.Date
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}
		k := key{merchant: b.Merchant, category: b.Category}
		e, ok := groups[k]
		if !ok {
			e = &entry{}
			groups[k] = e
		}
		e.amounts = append(e.amounts, b.Amount)
		e.weights = append(e.weights, timeWeight(now, dateStr))
		e.dates = append(e.dates, dateStr)
		if b.SubCategory != "" {
			e.subCategory = b.SubCategory
		}
	}

	thirtyDaysAgo := now.AddDate(0, 0, -30).Format("2006-01-02")

	for k, e := range groups {
		if len(e.amounts) < 2 {
			continue
		}

		var effectScore float64
		var recentCount int
		for i, _ := range e.amounts {
			w := e.weights[i]
			effectScore += w
			if e.dates[i] >= thirtyDaysAgo {
				recentCount++
			}
		}

		// 按日期排序，取最近一次的金额
		type dated struct {
			date   string
			amount float64
		}
		var pairs []dated
		for i := range e.amounts {
			pairs = append(pairs, dated{e.dates[i], e.amounts[i]})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].date < pairs[j].date })
		lastAmount := pairs[len(pairs)-1].amount
		lastDate := pairs[len(pairs)-1].date
		freq := classifyFrequency(e.dates)

		pattern := &model.ConsumptionPattern{
			UserID:      userID,
			Merchant:    k.merchant,
			Category:    k.category,
			SubCategory: e.subCategory,
			LastAmount:  lastAmount,
			Count:       len(e.amounts),
			RecentCount: recentCount,
			EffectScore: math.Round(effectScore*10000) / 10000,
			LastDate:    lastDate,
			Frequency:   freq,
		}
		if err := pe.patternDAO.UpsertConsumptionPattern(ctx, pattern); err != nil {
			log.Printf("failed to upsert pattern for user %d: %v", userID, err)
		}
	}
	return nil
}

// ExtractSequencePatterns 从账单中提取 L6 序列模式
func (pe *PatternExtractor) ExtractSequencePatterns(ctx context.Context, userID uint64) error {
	ninetyDaysAgo := time.Now().AddDate(0, 0, -90).Format("2006-01-02")
	bills, err := pe.billDAO.ListByDateRange(ctx, userID, ninetyDaysAgo, time.Now().Format("2006-01-02"))
	if err != nil {
		return err
	}

	// 按天分组，每天内按 created_at 排序
	type dayBills struct {
		bills []model.Bill
	}
	days := make(map[string]*dayBills)
	for _, b := range bills {
		if b.BillType != "expense" || b.IsDeleted {
			continue
		}
		dateKey := b.Date
		if len(dateKey) > 10 {
			dateKey = dateKey[:10]
		}
		d, ok := days[dateKey]
		if !ok {
			d = &dayBills{}
			days[b.Date] = d
		}
		d.bills = append(d.bills, b)
	}

	type seqKey struct {
		prevCat, prevMerchant, nextCat, nextMerchant string
	}
	type seqEntry struct {
		amounts []float64
		count   int
	}
	seqs := make(map[seqKey]*seqEntry)

	for _, d := range days {
		bs := d.bills
		sort.Slice(bs, func(i, j int) bool {
			return bs[i].CreatedAt.Before(bs[j].CreatedAt)
		})
		for i := 0; i < len(bs)-1; i++ {
			k := seqKey{
				prevCat:      bs[i].Category,
				prevMerchant: bs[i].Merchant,
				nextCat:      bs[i+1].Category,
				nextMerchant: bs[i+1].Merchant,
			}
			e, ok := seqs[k]
			if !ok {
				e = &seqEntry{}
				seqs[k] = e
			}
			e.count++
			e.amounts = append(e.amounts, bs[i+1].Amount)
		}
	}

	for k, e := range seqs {
		if e.count < 3 {
			continue
		}
		var sum float64
		for _, a := range e.amounts {
			sum += a
		}
		avg := sum / float64(len(e.amounts))

		sp := &model.SequencePattern{
			UserID:        userID,
			PrevCategory:  k.prevCat,
			PrevMerchant:  k.prevMerchant,
			NextCategory:  k.nextCat,
			NextMerchant:  k.nextMerchant,
			NextAvgAmount: math.Round(avg*100) / 100,
			Count:         e.count,
		}
		if err := pe.patternDAO.UpsertSequencePattern(ctx, sp); err != nil {
			log.Printf("failed to upsert sequence for user %d: %v", userID, err)
		}
	}
	return nil
}

func timeWeight(now time.Time, dateStr string) float64 {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return 0.1
	}
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days <= 30:
		return 1.0
	case days <= 90:
		return 0.6
	case days <= 180:
		return 0.3
	default:
		return 0.1
	}
}

func classifyFrequency(dates []string) string {
	if len(dates) < 2 {
		return "occasional"
	}
	first, _ := time.Parse("2006-01-02", dates[0])
	last, _ := time.Parse("2006-01-02", dates[len(dates)-1])
	span := last.Sub(first).Hours() / 24
	if span <= 0 {
		return "occasional"
	}
	avgInterval := span / float64(len(dates)-1)
	switch {
	case avgInterval <= 2:
		return "daily"
	case avgInterval <= 10:
		return "weekly"
	case avgInterval <= 40:
		return "monthly"
	default:
		return "occasional"
	}
}
