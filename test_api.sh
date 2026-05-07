#!/bin/bash
# =============================================================================
# 咔皮记账 API 端到端集成测试脚本
# 用法: ./test_api.sh [BASE_URL]
# 默认 BASE_URL: http://localhost:8080
# 依赖: curl, jq
# =============================================================================

BASE_URL="${1:-http://localhost:8080}"
TIMESTAMP=$(date +%s)
TEST_USER="e2e_test_${TIMESTAMP}"
TEST_PASS="test_pass_123"
TOKEN=""
ASSET_ID=""
BILL_ID_1=""
BILL_ID_2=""
BUDGET_ID=""
TODAY=$(date +%Y-%m-%d)

# 计数器
PASS_COUNT=0
FAIL_COUNT=0
TOTAL_COUNT=0

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# -----------------------------------------------------------------------------
# 前置检查
# -----------------------------------------------------------------------------
if ! command -v jq &> /dev/null; then
    echo -e "${RED}[ERROR] jq 未安装，请先安装: brew install jq${NC}"
    exit 1
fi

if ! command -v curl &> /dev/null; then
    echo -e "${RED}[ERROR] curl 未安装${NC}"
    exit 1
fi

echo "============================================="
echo " 咔皮记账 API E2E 测试"
echo " BASE_URL: ${BASE_URL}"
echo " 测试用户: ${TEST_USER}"
echo " 日期: $(date '+%Y-%m-%d %H:%M:%S')"
echo "============================================="
echo ""

# -----------------------------------------------------------------------------
# 辅助函数
# -----------------------------------------------------------------------------

# 执行测试并检查结果
# 参数: test_name, http_method, url, data(可选), extra_check(可选)
run_test() {
    local test_name="$1"
    local http_status="$2"
    local response_body="$3"

    TOTAL_COUNT=$((TOTAL_COUNT + 1))
    echo -n "[TEST ${TOTAL_COUNT}] ${test_name} ... "

    # 检查 HTTP 状态码
    if [[ "$http_status" -lt 200 || "$http_status" -ge 300 ]]; then
        echo -e "${RED}FAIL${NC} (HTTP ${http_status})"
        echo "  响应: ${response_body}"
        FAIL_COUNT=$((FAIL_COUNT + 1))
        return 1
    fi

    # 检查响应中的 code 字段
    local code
    code=$(echo "$response_body" | jq -r '.code // empty' 2>/dev/null)
    if [[ "$code" != "0" ]]; then
        local msg
        msg=$(echo "$response_body" | jq -r '.message // empty' 2>/dev/null)
        echo -e "${RED}FAIL${NC} (code=${code}, message=${msg})"
        echo "  响应: ${response_body}"
        FAIL_COUNT=$((FAIL_COUNT + 1))
        return 1
    fi

    echo -e "${GREEN}PASS${NC}"
    PASS_COUNT=$((PASS_COUNT + 1))
    return 0
}

# 带认证的 GET 请求
auth_get() {
    local url="$1"
    curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        "${BASE_URL}${url}"
}

# 带认证的 POST 请求
auth_post() {
    local url="$1"
    local data="$2"
    curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        -d "${data}" \
        "${BASE_URL}${url}"
}

# 带认证的 PUT 请求
auth_put() {
    local url="$1"
    local data="$2"
    curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        -X PUT \
        -d "${data}" \
        "${BASE_URL}${url}"
}

# 带认证的 DELETE 请求
auth_delete() {
    local url="$1"
    curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        -X DELETE \
        "${BASE_URL}${url}"
}

# 从 curl 输出中分离 body 和 status_code
parse_response() {
    local raw="$1"
    RESP_BODY=$(echo "$raw" | sed '$d')
    RESP_CODE=$(echo "$raw" | tail -1)
}

# =============================================================================
# 测试用例 1: 注册测试用户
# =============================================================================
echo ""
echo "--- 认证测试 ---"

RAW=$(curl -s -w "\n%{http_code}" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"${TEST_USER}\",\"password\":\"${TEST_PASS}\"}" \
    "${BASE_URL}/api/auth/register")
parse_response "$RAW"
run_test "注册测试用户 ${TEST_USER}" "$RESP_CODE" "$RESP_BODY"
REGISTER_OK=$?

# =============================================================================
# 测试用例 2: 登录获取 token
# =============================================================================
if [[ $REGISTER_OK -ne 0 ]]; then
    echo -e "${YELLOW}[WARN] 注册失败，尝试直接登录（用户可能已存在）${NC}"
fi

RAW=$(curl -s -w "\n%{http_code}" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"${TEST_USER}\",\"password\":\"${TEST_PASS}\"}" \
    "${BASE_URL}/api/auth/login")
parse_response "$RAW"
run_test "登录获取 token" "$RESP_CODE" "$RESP_BODY"
LOGIN_OK=$?

if [[ $LOGIN_OK -eq 0 ]]; then
    TOKEN=$(echo "$RESP_BODY" | jq -r '.data.token')
    if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
        echo -e "${RED}[ERROR] 登录成功但未获取到 token，后续测试将跳过${NC}"
        TOKEN=""
    fi
else
    echo -e "${RED}[ERROR] 登录失败，后续需要认证的测试将全部失败${NC}"
fi

# =============================================================================
# 测试用例 3: 创建资产（现金账户）
# =============================================================================
echo ""
echo "--- 资产测试 ---"

RAW=$(auth_post "/api/assets" '{"name":"E2E测试现金","type":"cash","balance":10000}')
parse_response "$RAW"
run_test "创建资产（现金账户）" "$RESP_CODE" "$RESP_BODY"
if [[ $? -eq 0 ]]; then
    ASSET_ID=$(echo "$RESP_BODY" | jq -r '.data.id // .data.ID // empty')
    echo "  资产 ID: ${ASSET_ID}"
fi

# =============================================================================
# 测试用例 4: 创建账单（支出）
# =============================================================================
echo ""
echo "--- 账单测试 ---"

BILL_DATA_1="{\"bill_type\":\"expense\",\"amount\":50.5,\"category\":\"餐饮\",\"sub_category\":\"午餐\",\"merchant\":\"测试商户\",\"date\":\"${TODAY}\",\"note\":\"E2E测试支出\"}"
if [[ -n "$ASSET_ID" && "$ASSET_ID" != "null" ]]; then
    BILL_DATA_1="{\"bill_type\":\"expense\",\"amount\":50.5,\"category\":\"餐饮\",\"sub_category\":\"午餐\",\"merchant\":\"测试商户\",\"date\":\"${TODAY}\",\"note\":\"E2E测试支出\",\"asset_id\":${ASSET_ID}}"
fi

RAW=$(auth_post "/api/bills" "$BILL_DATA_1")
parse_response "$RAW"
run_test "创建账单（支出 50.5 元）" "$RESP_CODE" "$RESP_BODY"
if [[ $? -eq 0 ]]; then
    BILL_ID_1=$(echo "$RESP_BODY" | jq -r '.data.id // .data.ID // empty')
    echo "  账单 ID: ${BILL_ID_1}"
fi

# =============================================================================
# 测试用例 5: 创建账单（收入）
# =============================================================================
BILL_DATA_2="{\"bill_type\":\"income\",\"amount\":8000,\"category\":\"工资\",\"date\":\"${TODAY}\",\"note\":\"E2E测试收入\"}"

RAW=$(auth_post "/api/bills" "$BILL_DATA_2")
parse_response "$RAW"
run_test "创建账单（收入 8000 元）" "$RESP_CODE" "$RESP_BODY"
if [[ $? -eq 0 ]]; then
    BILL_ID_2=$(echo "$RESP_BODY" | jq -r '.data.id // .data.ID // empty')
    echo "  账单 ID: ${BILL_ID_2}"
fi

# =============================================================================
# 测试用例 6: 查询账单列表，验证有 2 条
# =============================================================================
RAW=$(auth_get "/api/bills")
parse_response "$RAW"
run_test "查询账单列表" "$RESP_CODE" "$RESP_BODY"

if [[ $? -eq 0 ]]; then
    BILL_COUNT=$(echo "$RESP_BODY" | jq '.data | length' 2>/dev/null)
    TOTAL_COUNT=$((TOTAL_COUNT + 1))
    echo -n "[TEST ${TOTAL_COUNT}] 验证账单数量 >= 2 ... "
    if [[ "$BILL_COUNT" -ge 2 ]] 2>/dev/null; then
        echo -e "${GREEN}PASS${NC} (共 ${BILL_COUNT} 条)"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo -e "${RED}FAIL${NC} (期望 >= 2, 实际 ${BILL_COUNT})"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
fi

# =============================================================================
# 测试用例 7: 创建预算
# =============================================================================
echo ""
echo "--- 预算测试 ---"

RAW=$(auth_post "/api/budgets" '{"name":"E2E餐饮预算","categories":"[\"餐饮\"]","amount":2000,"period":"monthly","budget_type":"essential"}')
parse_response "$RAW"
run_test "创建预算（餐饮 2000 元/月）" "$RESP_CODE" "$RESP_BODY"
if [[ $? -eq 0 ]]; then
    BUDGET_ID=$(echo "$RESP_BODY" | jq -r '.data.id // .data.ID // empty')
    echo "  预算 ID: ${BUDGET_ID}"
fi

# =============================================================================
# 测试用例 8: 查询预算列表
# =============================================================================
RAW=$(auth_get "/api/budgets")
parse_response "$RAW"
run_test "查询预算列表" "$RESP_CODE" "$RESP_BODY"

if [[ $? -eq 0 ]]; then
    BUDGET_COUNT=$(echo "$RESP_BODY" | jq '.data | length' 2>/dev/null)
    echo "  预算数量: ${BUDGET_COUNT}"
fi

# =============================================================================
# 测试用例 9: AI 对话 - 记账
# =============================================================================
echo ""
echo "--- AI 对话测试 ---"

RAW=$(auth_post "/api/chat" '{"message":"午饭30元","page_context":"home","session_id":1}')
parse_response "$RAW"
run_test "AI 对话：记账（午饭30元）" "$RESP_CODE" "$RESP_BODY"

# =============================================================================
# 测试用例 10: AI 对话 - 查询
# =============================================================================
RAW=$(auth_post "/api/chat" '{"message":"今天花了多少","page_context":"home","session_id":1}')
parse_response "$RAW"
run_test "AI 对话：查询（今天花了多少）" "$RESP_CODE" "$RESP_BODY"

# =============================================================================
# 测试用例 11: 查询对话历史
# =============================================================================
RAW=$(auth_get "/api/chat/history?session_id=1&limit=20")
parse_response "$RAW"
run_test "查询对话历史" "$RESP_CODE" "$RESP_BODY"

if [[ $? -eq 0 ]]; then
    HISTORY_COUNT=$(echo "$RESP_BODY" | jq '.data | length' 2>/dev/null)
    TOTAL_COUNT=$((TOTAL_COUNT + 1))
    echo -n "[TEST ${TOTAL_COUNT}] 验证对话历史有记录 ... "
    if [[ "$HISTORY_COUNT" -gt 0 ]] 2>/dev/null; then
        echo -e "${GREEN}PASS${NC} (共 ${HISTORY_COUNT} 条)"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo -e "${RED}FAIL${NC} (期望 > 0, 实际 ${HISTORY_COUNT})"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
fi

# =============================================================================
# 测试用例 12: 查询建议
# =============================================================================
echo ""
echo "--- 建议测试 ---"

RAW=$(auth_get "/api/suggestions")
parse_response "$RAW"
run_test "查询智能建议" "$RESP_CODE" "$RESP_BODY"

# =============================================================================
# 测试结果汇总
# =============================================================================
echo ""
echo "============================================="
echo " 测试结果汇总"
echo "============================================="
echo -e " 总计: ${TOTAL_COUNT}"
echo -e " 通过: ${GREEN}${PASS_COUNT}${NC}"
echo -e " 失败: ${RED}${FAIL_COUNT}${NC}"
echo "============================================="

if [[ $FAIL_COUNT -gt 0 ]]; then
    echo -e "${RED}存在失败的测试用例！${NC}"
    exit 1
else
    echo -e "${GREEN}所有测试通过！${NC}"
    exit 0
fi