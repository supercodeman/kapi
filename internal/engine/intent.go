package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DetectedIntent 表示从用户消息中确定性检测到的意图
type DetectedIntent struct {
	RequiredTool string         // 必须调用的工具名
	Params       map[string]any // 从用户消息中提取的确定性参数
}

// 预算修改模式：匹配 "将/把 X预算 调整到/改成/设为 N" 等
var budgetModifyPatterns = []*regexp.Regexp{
	// "将食饮预算调整到2700" / "把交通预算改成500"
	regexp.MustCompile(`(?:将|把)(.+?)预算(?:调整到|调到|改成|改为|设为|设置为|调整为)(\d+\.?\d*)`),
	// "食饮预算调整到2700" / "交通预算改为500"
	regexp.MustCompile(`(.+?)预算(?:调整到|调到|改成|改为|设为|设置为|调整为)(\d+\.?\d*)`),
	// "调整食饮预算到2700" / "修改交通预算为500"
	regexp.MustCompile(`(?:调整|修改)(.+?)预算(?:到|为)(\d+\.?\d*)`),
}

// 预算删除模式
var budgetDeletePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:删除|删掉|取消)(.+?)预算`),
}

// 预算可承受性检查模式："能不能买X" / "还能买X吗" / "买X会超支吗"
var budgetAffordabilityPatterns = []*regexp.Regexp{
	// "我还能买个5000块的相机吗" / "能买5000的东西吗"
	regexp.MustCompile(`(?:还能|能不能|能否|可以)买.{0,6}?(\d+\.?\d*)`),
	// "买个5000的相机会超支吗"
	regexp.MustCompile(`买.{0,6}?(\d+\.?\d*).{0,10}?(?:超支|超预算|够不够|够吗)`),
	// "5000块的相机买得起吗"
	regexp.MustCompile(`(\d+\.?\d*).{0,10}?(?:买得起|负担得起|承受得了)`),
}

// affordabilityCategoryPatterns 从消息中提取消费分类
var affordabilityCategoryPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:相机|电脑|手机|平板|耳机|键盘|显示器|电视|冰箱|洗衣机)`), // 购物/数码
	regexp.MustCompile(`(?:机票|酒店|旅游|出行)`),                                // 旅行/交通
	regexp.MustCompile(`(?:吃饭|餐厅|外卖|奶茶|咖啡)`),                              // 餐饮
}

// detectIntent 从用户消息中检测确定性意图
// 只在高置信度时返回结果（明确的动词+对象+参数），避免误判
func detectIntent(msg string) *DetectedIntent {
	msg = strings.TrimSpace(msg)

	// 预算修改意图
	for _, p := range budgetModifyPatterns {
		if matches := p.FindStringSubmatch(msg); len(matches) >= 3 {
			category := strings.TrimSpace(matches[1])
			amounts := ParseChineseNumber(matches[2])
			if len(amounts) > 0 && category != "" {
				return &DetectedIntent{
					RequiredTool: "update_budget",
					Params: map[string]any{
						"category": category,
						"amount":   amounts[0],
					},
				}
			}
		}
	}

	// 预算删除意图
	for _, p := range budgetDeletePatterns {
		if matches := p.FindStringSubmatch(msg); len(matches) >= 2 {
			category := strings.TrimSpace(matches[1])
			if category != "" {
				return &DetectedIntent{
					RequiredTool: "delete_budget",
					Params: map[string]any{
						"category": category,
					},
				}
			}
		}
	}

	// 预算可承受性检查意图："能不能买5000的相机"
	for _, p := range budgetAffordabilityPatterns {
		if matches := p.FindStringSubmatch(msg); len(matches) >= 2 {
			amount, err := strconv.ParseFloat(matches[1], 64)
			if err == nil && amount > 0 {
				category := guessAffordabilityCategory(msg)
				return &DetectedIntent{
					RequiredTool: "check_budget_affordability",
					Params: map[string]any{
						"planned_amount": amount,
						"category":       category,
					},
				}
			}
		}
	}

	return nil
}

// guessAffordabilityCategory 从消息中猜测消费分类
func guessAffordabilityCategory(msg string) string {
	categoryMap := map[string]string{
		"相机": "购物", "电脑": "购物", "手机": "购物", "平板": "购物",
		"耳机": "购物", "键盘": "购物", "显示器": "购物", "电视": "购物",
		"冰箱": "购物", "洗衣机": "购物", "衣服": "购物", "鞋": "购物",
		"机票": "交通", "酒店": "旅行", "旅游": "旅行",
		"吃饭": "食饮", "餐厅": "食饮", "外卖": "食饮",
		"电影": "娱乐", "游戏": "娱乐", "演唱会": "娱乐",
	}
	for keyword, cat := range categoryMap {
		if strings.Contains(msg, keyword) {
			return cat
		}
	}
	return "购物" // 默认购物
}

// shouldCorrectToolCall 检查 LLM 的 ToolCall 是否与检测到的意图冲突
// 如果 LLM 调用了正确的工具且参数合理，不纠正
// 如果 LLM 调用了错误的工具，或者调用了正确工具但关键参数为空/零值，纠正
func shouldCorrectToolCall(intent *DetectedIntent, toolCalls []ToolCall) bool {
	for _, tc := range toolCalls {
		if tc.Name == intent.RequiredTool {
			// 工具名正确，检查关键参数是否合理
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				return true // 参数解析失败，纠正
			}
			switch intent.RequiredTool {
			case "check_budget_affordability":
				pa, ok1 := ToFloat64(args["planned_amount"])
				a, ok2 := ToFloat64(args["amount"])
				if (!ok1 || pa == 0) && (!ok2 || a == 0) {
					return true // 金额为空，纠正
				}
			case "update_budget":
				// 检查 amount 是否正确
				a, ok := ToFloat64(args["amount"])
				if !ok || a == 0 {
					return true
				}
				// 检查 category 是否是字符串（LM 可能传了数组）
				if cat, ok := args["category"].(string); !ok || cat == "" {
					return true
				}
			}
			return false // 工具和参数都合理，不纠正
		}
	}
	return true // LLM 没有调用正确的工具，需要纠正
}

// buildCorrectedToolCall 根据检测到的意图构建正确的 ToolCall
func buildCorrectedToolCall(intent *DetectedIntent) ToolCall {
	argsJSON, _ := json.Marshal(intent.Params)
	return ToolCall{
		ID:        generateToolCallID(),
		Name:      intent.RequiredTool,
		Arguments: string(argsJSON),
	}
}

// generateToolCallID 生成合法的 tool_call_id
func generateToolCallID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "toolu_" + hex.EncodeToString(b)
}

// correctToolCalls 意图纠正入口：检测意图，如果 LLM 调用了错误的工具则纠正
// 返回 true 表示进行了纠正
func correctToolCalls(msg string, toolCalls *[]ToolCall) bool {
	intent := detectIntent(msg)
	if intent == nil {
		return false
	}
	if !shouldCorrectToolCall(intent, *toolCalls) {
		return false
	}
	// 纠正：用正确的 ToolCall 替换
	corrected := buildCorrectedToolCall(intent)
	*toolCalls = []ToolCall{corrected}
	return true
}

// formatIntentDescription 用于日志
func formatIntentDescription(intent *DetectedIntent) string {
	if intent == nil {
		return "none"
	}
	return fmt.Sprintf("tool=%s params=%v", intent.RequiredTool, intent.Params)
}

// isQuestionMessage 检测用户消息是否是疑问句（需要分析判断而非直接展示数据）
func isQuestionMessage(msg string) bool {
	questionSuffixes := []string{"吗", "吗？", "呢", "呢？", "？", "?"}
	for _, s := range questionSuffixes {
		if strings.HasSuffix(strings.TrimSpace(msg), s) {
			return true
		}
	}
	questionKeywords := []string{"能不能", "够不够", "会不会", "是不是", "有没有", "多少", "怎么样", "如何"}
	for _, kw := range questionKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
