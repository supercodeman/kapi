# 收入类型确认流程 — 技术设计文档

## 1. 业务背景

- 用户记录收入时经常不指定具体类型（如"记一笔收入500"）
- 系统需要追问收入类型以正确分类
- 收入分类体系：职业收入（工资/奖金/兼职）、投资收入（理财收益/股票分红/房租收入）、其他收入（红包/退款/报销）

## 2. 触发条件

```go
// tools.go:185
if !isConfirmation(ctx) && billType == "income" && catResult.SubCategory == "" && isVagueIncomeCategory(catResult.Category)
```

- billType 为 income
- 子分类为空
- 一级分类为模糊分类（"其他"、"其他收入"、"收入"、""）
- 非确认轮次

## 3. 模糊分类定义

```go
// validate.go
func isVagueIncomeCategory(category string) bool {
    vague := map[string]bool{
        "其他": true, "其他收入": true, "收入": true, "": true,
    }
    return vague[category]
}
```

## 4. 完整流程

### 4.1 首轮：触发追问

1. 用户说"记一笔收入500"
2. LLM 调用 create_bill{bill_type:"income", amount:500, category:"收入"}
3. NormalizeCategoryV2("收入") → {Category:"收入"} (validParentCategories 中没有"收入")
   - 实际上 NormalizeCategoryV2 对 "收入" 的处理：不在 validParentCategories → 不在 keywordCategoryMap → 不在 categoryAliases → 返回 {Category:"收入"}
4. isVagueIncomeCategory("收入") = true
5. 返回 need_confirm: "这笔是什么类型的收入？工资、兼职、理财还是其他？"
6. PendingAction 存储: {amount:500, bill_type:"income", category:"收入", date:"2026-05-06"}

### 4.2 用户直接回答类型

1. 用户说"工资"
2. isPendingParamAnswer("工资"):
   - 长度 2 ≤ 10 ✓
   - 无疑问词 ✓
   - knownCategories["工资"] = true → 返回 true
3. executePendingAction:
   - ContextWithConfirmation(ctx)
   - mergePendingParams: NormalizeCategoryV2("工资","","工资") → {Category:"职业收入", SubCategory:"工资"}
   - action.Params["category"] = "职业收入", action.Params["sub_category"] = "工资"
4. Tool 执行: isConfirmation=true → 跳过追问
5. 成功记录

### 4.3 用户回答"其他"

1. 用户说"其他"
2. isPendingParamAnswer("其他"):
   - knownCategories["其他"] = true → 返回 true
3. executePendingAction:
   - mergePendingParams: NormalizeCategoryV2("其他","","其他") → {Category:"其他"}
   - billType=="income" && result.Category=="其他" → 映射为 "其他收入"
   - action.Params["category"] = "其他收入"
4. Tool 执行: isConfirmation=true → 跳过追问
5. 成功记录（分类为"其他收入"）

### 4.4 用户插入无关问题

1. 用户说"都有哪些类型呢"
2. isPendingParamAnswer("都有哪些类型呢"):
   - 长度 7 ≤ 10 ✓
   - 包含疑问词"呢" → 返回 false
3. PendingAction 放回 PendingStore
4. 走正常 LLM 流程，LLM 回答类型列表
5. 下一轮用户说"红包":
   - isPendingParamAnswer("红包") → knownCategories["红包"] = true → true
   - executePendingAction 正常执行

## 5. isPendingParamAnswer 判断逻辑

```go
func isPendingParamAnswer(msg string) bool {
    // 1. 长度限制：>10字 → false
    // 2. 疑问词过滤：含"吗/呢/？/?/哪些/什么/怎么/如何/为什么" → false
    // 3. 确认词匹配：含"确认/好的/可以/是的/对/行..." → true
    // 4. 纯金额匹配：ParseChineseNumber 有结果且≤6字 → true
    // 5. 精确分类名匹配：knownCategories 白名单 → true
    // 6. 以上都不满足 → false
}
```

### 5.1 knownCategories 白名单

```go
knownCategories := map[string]bool{
    // 收入分类
    "工资": true, "奖金": true, "兼职": true, "理财": true,
    "红包": true, "退款": true, "报销": true, "其他": true,
    // 支出分类
    "餐饮": true, "交通": true, "购物": true, "娱乐": true,
    "居住": true, "医疗": true, "教育": true, "通讯": true,
    "食饮": true, "日用": true, "服饰": true, "运动": true,
    "旅行": true, "社交": true, "宠物": true,
    // 收入一级分类
    "职业收入": true, "投资收入": true, "其他收入": true,
}
```

## 6. mergePendingParams 中的收入分类处理

```go
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
```

## 7. 收入分类自动修正

```go
// tools.go:196-204
incomeCategories := map[string]bool{
    "职业收入": true, "投资收入": true, "其他收入": true,
    "工资": true, "奖金": true, "兼职": true,
    "理财收益": true, "股票分红": true, "房租收入": true,
    "红包": true, "退款": true, "报销": true,
}
if incomeCategories[catResult.Category] || incomeCategories[catResult.SubCategory] {
    billType = "income"
}
```

## 8. 已修复的问题

### 8.1 "其他" 不被识别为有效回答（已修复）

- 原因：旧版 isPendingParamAnswer 使用 NormalizeCategoryV2 做模糊匹配，"其他" 返回 Category="其他"，不满足 `!= "其他"` 条件
- 修复：改用 knownCategories 精确匹配白名单

### 8.2 "其他包含哪些类型" 被误判为参数回答（已修复）

- 原因：NormalizeCategoryV2("其他包含哪些类型") 中 "包" 匹配了 keywordCategoryMap 中的 "包":{"购物","服饰"}
- 修复：1) 改用精确匹配替代模糊匹配 2) 增加疑问词前置过滤

### 8.3 "红包" 在 NormalizeCategoryV2 中映射为支出分类（已修复）

- 原因：keywordCategoryMap 中 "红包":{"社交","礼物红包"} 是支出分类
- 修复：mergePendingParams 中收入场景的分类映射逻辑 + isConfirmation 跳过追问

## 9. 注意事项

- 修改 isPendingParamAnswer 时必须同时考虑 engine.go 和 stream.go 两个入口
- knownCategories 白名单需要与 model/seed.go 中的分类定义保持同步
- 疑问词列表需要覆盖常见的中文疑问表达
