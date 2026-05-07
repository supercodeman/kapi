#!/bin/bash
# 咔皮记账 AI 助手 — 综合测试脚本
# 用法：bash scripts/test_suite.sh

set -e
BASE="http://localhost:8080"
PASS=0
FAIL=0
TOTAL=0

# 颜色
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

check() {
  TOTAL=$((TOTAL+1))
  local name="$1"
  local condition="$2"
  if eval "$condition"; then
    echo -e "  ${GREEN}✓${NC} $name"
    PASS=$((PASS+1))
  else
    echo -e "  ${RED}✗${NC} $name"
    FAIL=$((FAIL+1))
  fi
}

chat() {
  local msg="$1"
  local page="$2"
  curl -s -X POST $BASE/api/chat \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "{\"message\":\"$msg\",\"page_context\":\"$page\",\"session_id\":1}" 2>/dev/null | \
    python3 -c "import sys,re,json; raw=re.sub(r'[\x00-\x1f]',' ',sys.stdin.read()); print(json.dumps(json.loads(raw)['data']))" 2>/dev/null
}

echo "=============================="
echo "咔皮记账 AI 助手 综合测试"
echo "=============================="
echo ""

# 0. 健康检查
echo "--- 健康检查 ---"
HEALTH=$(curl -s $BASE/health)
check "服务可用" "[ '$HEALTH' = '{\"status\":\"ok\"}' ]"

# 1. 注册 + 登录
echo ""
echo "--- 认证测试 ---"
REG=$(curl -s -X POST $BASE/api/auth/register -H "Content-Type: application/json" \
  -d '{"username":"suite_test_'$$'","password":"test123456"}')
check "注册成功" "echo '$REG' | grep -q 'success'"

LOGIN=$(curl -s -X POST $BASE/api/auth/login -H "Content-Type: application/json" \
  -d '{"username":"suite_test_'$$'","password":"test123456"}')
TOKEN=$(echo "$LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])" 2>/dev/null)
check "登录成功" "[ -n '$TOKEN' ]"

UNAUTH=$(curl -s $BASE/api/bills -H "Content-Type: application/json")
check "未认证拒绝" "echo '$UNAUTH' | grep -q '40100'"

# 2. 记账核心
echo ""
echo "--- 记账核心 (B系列) ---"

# B001 基础支出
RESP=$(chat "午饭30元" "记一笔")
check "B001 基础支出" "echo '$RESP' | grep -q 'success'"
DB_COUNT=$(mysql -u root -p'Super!123' kapi -sN -e "SELECT COUNT(*) FROM bills WHERE user_id=(SELECT id FROM users WHERE username='suite_test_$$') AND amount=30" 2>/dev/null)
check "B001 DB写入" "[ '$DB_COUNT' -ge 1 ]"

# B002 带商户
RESP=$(chat "星巴克咖啡38元" "记一笔")
check "B002 带商户" "echo '$RESP' | grep -q 'success'"
SUB_CAT=$(mysql -u root -p'Super!123' kapi -sN -e "SELECT sub_category FROM bills WHERE user_id=(SELECT id FROM users WHERE username='suite_test_$$') AND merchant='星巴克' ORDER BY id DESC LIMIT 1" 2>/dev/null)
check "B002 子分类=饮品" "[ '$SUB_CAT' = '饮品' ]"

# B004 收入
RESP=$(chat "发工资15000" "记一笔")
check "B004 收入记账" "echo '$RESP' | grep -q 'success'"
BILL_TYPE=$(mysql -u root -p'Super!123' kapi -sN -e "SELECT bill_type FROM bills WHERE user_id=(SELECT id FROM users WHERE username='suite_test_$$') AND amount=15000 ORDER BY id DESC LIMIT 1" 2>/dev/null)
check "B004 bill_type=income" "[ '$BILL_TYPE' = 'income' ]"

# B003 带日期
RESP=$(chat "昨天打车25元" "记一笔")
YESTERDAY=$(date -v-1d +%Y-%m-%d 2>/dev/null || date -d "yesterday" +%Y-%m-%d 2>/dev/null)
BILL_DATE=$(mysql -u root -p'Super!123' kapi -sN -e "SELECT date FROM bills WHERE user_id=(SELECT id FROM users WHERE username='suite_test_$$') AND amount=25 ORDER BY id DESC LIMIT 1" 2>/dev/null)
check "B003 日期=昨天" "[ '$BILL_DATE' = '$YESTERDAY' ]"

# 3. 预算管理
echo ""
echo "--- 预算管理 (P系列) ---"

RESP=$(chat "设置餐饮预算2000元" "预算")
check "P002 创建预算" "echo '$RESP' | grep -q 'success'"
BUDGET_AMT=$(mysql -u root -p'Super!123' kapi -sN -e "SELECT amount FROM budgets WHERE user_id=(SELECT id FROM users WHERE username='suite_test_$$') ORDER BY id DESC LIMIT 1" 2>/dev/null)
check "P002 金额=2000" "[ '$BUDGET_AMT' = '2000.00' ]"

# 4. 反幻觉验证
echo ""
echo "--- 反幻觉 (H系列) ---"

# H003 日期验证
TODAY=$(date +%Y-%m-%d)
RESP=$(chat "今天买了杯奶茶15元" "记一笔")
BILL_DATE=$(mysql -u root -p'Super!123' kapi -sN -e "SELECT date FROM bills WHERE user_id=(SELECT id FROM users WHERE username='suite_test_$$') AND amount=15 ORDER BY id DESC LIMIT 1" 2>/dev/null)
check "H003 日期=今天($TODAY)" "[ '$BILL_DATE' = '$TODAY' ]"

# 5. 多轮对话
echo ""
echo "--- 多轮对话 (M系列) ---"

RESP=$(chat "刚才那杯奶茶多少钱" "记一笔")
check "M001 上下文连贯" "echo '$RESP' | grep -q '15'"

# 6. OpLog 验证
echo ""
echo "--- 数据完整性 ---"

OPLOG_COUNT=$(mysql -u root -p'Super!123' kapi -sN -e "SELECT COUNT(*) FROM operation_logs WHERE user_id=(SELECT id FROM users WHERE username='suite_test_$$')" 2>/dev/null)
check "OpLog 有记录" "[ '$OPLOG_COUNT' -ge 1 ]"

TRIGGER=$(mysql -u root -p'Super!123' kapi -sN -e "SELECT trigger_text FROM operation_logs WHERE user_id=(SELECT id FROM users WHERE username='suite_test_$$') ORDER BY id DESC LIMIT 1" 2>/dev/null)
check "OpLog trigger_text 非空" "[ -n '$TRIGGER' ]"

MEMORY_COUNT=$(mysql -u root -p'Super!123' kapi -sN -e "SELECT COUNT(*) FROM memories WHERE user_id=(SELECT id FROM users WHERE username='suite_test_$$')" 2>/dev/null)
check "Memory 有记录" "[ '$MEMORY_COUNT' -ge 1 ]"

# 7. REST API 性能
echo ""
echo "--- REST API 性能 ---"

START=$(date +%s%N)
for i in $(seq 1 10); do
  curl -s $BASE/api/bills -H "Authorization: Bearer $TOKEN" -o /dev/null
done
END=$(date +%s%N)
AVG_MS=$(( (END - START) / 10000000 ))
check "GET /api/bills 平均 < 200ms" "[ $AVG_MS -lt 200 ]"

# 8. 多租户隔离
echo ""
echo "--- 多租户隔离 (T系列) ---"

curl -s -X POST $BASE/api/auth/register -H "Content-Type: application/json" \
  -d '{"username":"suite_other_'$$'","password":"test123456"}' > /dev/null 2>&1
TOKEN2=$(curl -s -X POST $BASE/api/auth/login -H "Content-Type: application/json" \
  -d '{"username":"suite_other_'$$'","password":"test123456"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])" 2>/dev/null)

OTHER_BILLS=$(curl -s $BASE/api/bills -H "Authorization: Bearer $TOKEN2" | python3 -c "import sys,json; d=json.load(sys.stdin)['data']; print(len(d) if isinstance(d,list) else 0)" 2>/dev/null)
check "T001 用户B看不到A的账单" "[ '$OTHER_BILLS' = '0' ]"

# 汇总
echo ""
echo "=============================="
echo -e "总计: $TOTAL | ${GREEN}通过: $PASS${NC} | ${RED}失败: $FAIL${NC}"
echo "=============================="

if [ $FAIL -gt 0 ]; then
  exit 1
fi
