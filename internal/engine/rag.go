package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/sangchenglong/kapi/internal/dao"
)

// RAGResult 是 RAG 检索的结果
type RAGResult struct {
	Amount     float64
	Category   string
	SubCategory string
	Merchant   string
	Confidence float64 // 0~1
	Source     string  // "L5_merchant" / "L5_category" / "L6_sequence"
	Hint       string  // 给用户的提示文本
}

// RAGFiller 负责从消费模式中补全记账参数
type RAGFiller struct {
	patternDAO *dao.PatternDAO
}

func NewRAGFiller(patternDAO *dao.PatternDAO) *RAGFiller {
	return &RAGFiller{patternDAO: patternDAO}
}

// FillParams 尝试从 L5 消费模式中补全缺失参数
// 只补全缺失参数，不覆盖用户已提供的
func (r *RAGFiller) FillParams(ctx context.Context, userID uint64, merchant, category string, amount float64) *RAGResult {
	// 优先按商户精确匹配
	if merchant != "" {
		p, err := r.patternDAO.FindByMerchant(ctx, userID, merchant)
		if err == nil && p != nil {
			confidence := calcConfidence(p.Count, p.EffectScore)
			result := &RAGResult{
				Amount:      p.LastAmount,
				Category:    p.Category,
				SubCategory: p.SubCategory,
				Merchant:    p.Merchant,
				Confidence:  confidence,
				Source:      "L5_merchant",
			}
			if confidence >= 0.8 {
				result.Hint = "根据你的消费习惯预填"
			} else {
				result.Hint = "根据你的消费记录推测"
			}
			return result
		}
	}

	// 按分类匹配
	if category != "" {
		patterns, err := r.patternDAO.FindByCategory(ctx, userID, category)
		if err == nil && len(patterns) > 0 {
			p := patterns[0]
			confidence := calcConfidence(p.Count, p.EffectScore)
			return &RAGResult{
				Amount:      p.LastAmount,
				Category:    p.Category,
				SubCategory: p.SubCategory,
				Merchant:    p.Merchant,
				Confidence:  confidence,
				Source:      "L5_category",
				Hint:        "根据你的消费记录推测",
			}
		}
	}

	return nil
}

// FillFromSequence 从 L6 序列模式中检索下一步消费
func (r *RAGFiller) FillFromSequence(ctx context.Context, userID uint64, prevCategory, prevMerchant string) *RAGResult {
	seqs, err := r.patternDAO.FindSequence(ctx, userID, prevCategory, prevMerchant)
	if err != nil || len(seqs) == 0 {
		return nil
	}
	s := seqs[0]
	return &RAGResult{
		Amount:      s.NextAvgAmount,
		Category:    s.NextCategory,
		Merchant:    s.NextMerchant,
		Confidence:  0.6,
		Source:      "L6_sequence",
		Hint:        "你之前经常在这之后消费",
	}
}

func calcConfidence(count int, effectScore float64) float64 {
	if count >= 5 && effectScore >= 2.0 {
		return 0.9
	}
	if count >= 3 && effectScore >= 1.5 {
		return 0.7
	}
	if count >= 2 && effectScore >= 1.0 {
		return 0.5
	}
	return 0.3
}

// BuildRAGHint 根据用户消息检索消费模式，生成注入 System Prompt 的提示
// 返回空字符串表示无匹配
func (r *RAGFiller) BuildRAGHint(ctx context.Context, userID uint64, userMsg string) string {
	if r == nil {
		return ""
	}

	// 尝试从用户消息中提取商户名或关键词
	patterns, err := r.patternDAO.ListByUser(ctx, userID)
	if err != nil || len(patterns) == 0 {
		return ""
	}

	// 按商户名匹配
	for _, p := range patterns {
		if p.Merchant != "" && strings.Contains(userMsg, p.Merchant) {
			return fmt.Sprintf(
				"用户在「%s」有 %d 次消费记录，上次 ¥%.2f，分类%s。用户没说金额时，直接调用 create_bill 工具，amount=%.2f，category=%s，不需要追问金额。",
				p.Merchant, p.Count, p.LastAmount, p.Category, p.LastAmount, p.Category)
		}
	}

	// 按分类关键词匹配
	for _, p := range patterns {
		if strings.Contains(userMsg, p.Category) {
			return fmt.Sprintf(
				"用户%s分类有 %d 次消费记录，上次 ¥%.2f。用户没说金额时，直接调用 create_bill 工具，amount=%.2f，不需要追问金额。",
				p.Category, p.Count, p.LastAmount, p.LastAmount)
		}
	}

	return ""
}
