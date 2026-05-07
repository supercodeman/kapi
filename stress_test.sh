#!/bin/bash
# =============================================================================
# 咔皮记账 并发压力测试脚本
# 用法: ./stress_test.sh [并发数] [每并发请求数]
# 默认: 10 并发 × 5 请求
# 依赖: curl, jq, bc
# =============================================================================

BASE_URL="${3:-http://localhost:8080}"
CONCURRENCY="${1:-10}"
REQUESTS_PER="${2:-5}"
TOTAL=$((CONCURRENCY * REQUESTS_PER))

echo "============================================="
echo " 咔皮记账 压力测试"
echo " 并发数: $CONCURRENCY"
echo " 每并发请求数: $REQUESTS_PER"
echo " 总请求数: $TOTAL"
echo " 目标: $BASE_URL"
echo "============================================="

# 注册并登录获取 token
TIMESTAMP=$(date +%s)
USER="stress_${TIMESTAMP}"
curl -s -X POST "$BASE_URL/api/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$USER\",\"password\":\"test123456\"}" > /dev/null 2>&1

TOKEN=$(curl -s -X POST "$BASE_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$USER\",\"password\":\"test123456\"}" | jq -r '.data.token')

if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  echo "登录失败，退出"
  exit 1
fi
echo "测试用户: $USER (token ok)"
echo ""

# 毫秒时间戳（macOS 兼容）
now_ms() { python3 -c "import time; print(int(time.time()*1000))"; }

# 结果目录
RESULT_DIR=$(mktemp -d)

# =============================================
# 测试1: CRUD API 并发（轻量级，验证基础吞吐）
# =============================================
echo "--- 测试1: CRUD API 并发 ---"

crud_worker() {
  local id=$1
  local count=$2
  local latencies=""
  for i in $(seq 1 $count); do
    start=$(now_ms)
    resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/bills" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $TOKEN" \
      -d "{\"bill_type\":\"expense\",\"amount\":$((RANDOM % 100 + 1)),\"category\":\"食饮\",\"date\":\"$(date +%Y-%m-%d)\"}")
    end=$(now_ms)
    http_code=$(echo "$resp" | tail -1)
    elapsed=$((end - start))
    latencies="$latencies $elapsed"
    if [ "$http_code" != "200" ]; then
      echo "FAIL" >> "$RESULT_DIR/crud_fail"
    else
      echo "OK" >> "$RESULT_DIR/crud_ok"
    fi
    echo "$elapsed" >> "$RESULT_DIR/crud_latency"
  done
}

for i in $(seq 1 $CONCURRENCY); do
  crud_worker $i $REQUESTS_PER &
done
wait

CRUD_OK=$(wc -l < "$RESULT_DIR/crud_ok" 2>/dev/null | tr -d ' ' || echo 0)
CRUD_FAIL=$(cat "$RESULT_DIR/crud_fail" 2>/dev/null | wc -l | tr -d ' ' || echo 0)
if [ -f "$RESULT_DIR/crud_latency" ]; then
  CRUD_P50=$(sort -n "$RESULT_DIR/crud_latency" | awk "NR==int($(wc -l < "$RESULT_DIR/crud_latency")*0.5)+1")
  CRUD_P95=$(sort -n "$RESULT_DIR/crud_latency" | awk "NR==int($(wc -l < "$RESULT_DIR/crud_latency")*0.95)+1")
  CRUD_MAX=$(sort -n "$RESULT_DIR/crud_latency" | tail -1)
else
  CRUD_P50=0; CRUD_P95=0; CRUD_MAX=0
fi

echo "  成功: $CRUD_OK / 失败: $CRUD_FAIL"
echo "  延迟 P50: ${CRUD_P50}ms / P95: ${CRUD_P95}ms / Max: ${CRUD_MAX}ms"
echo ""

# =============================================
# 测试2: AI 对话并发（重量级，验证 LLM 并发）
# =============================================
echo "--- 测试2: AI 对话并发 (同步 API) ---"

CHAT_CONCURRENCY=$((CONCURRENCY > 5 ? 5 : CONCURRENCY))
CHAT_REQUESTS=2

chat_worker() {
  local id=$1
  local count=$2
  local messages=("午饭30元" "今天花了多少" "本月预算还剩多少" "帮我记一笔咖啡15元")
  for i in $(seq 1 $count); do
    msg="${messages[$((RANDOM % ${#messages[@]}))]}"
    start=$(now_ms)
    resp=$(curl -s -w "\n%{http_code}" --max-time 60 -X POST "$BASE_URL/api/chat" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $TOKEN" \
      -d "{\"message\":\"$msg\",\"page_context\":\"首页\",\"session_id\":$id}")
    end=$(now_ms)
    http_code=$(echo "$resp" | tail -1)
    elapsed=$((end - start))
    echo "$elapsed" >> "$RESULT_DIR/chat_latency"
    if [ "$http_code" = "200" ]; then
      echo "OK" >> "$RESULT_DIR/chat_ok"
    else
      echo "FAIL $http_code" >> "$RESULT_DIR/chat_fail"
    fi
  done
}

for i in $(seq 1 $CHAT_CONCURRENCY); do
  chat_worker $i $CHAT_REQUESTS &
done
wait

CHAT_OK=$(wc -l < "$RESULT_DIR/chat_ok" 2>/dev/null | tr -d ' ' || echo 0)
CHAT_FAIL=$(cat "$RESULT_DIR/chat_fail" 2>/dev/null | wc -l | tr -d ' ' || echo 0)
if [ -f "$RESULT_DIR/chat_latency" ]; then
  CHAT_P50=$(sort -n "$RESULT_DIR/chat_latency" | awk "NR==int($(wc -l < "$RESULT_DIR/chat_latency")*0.5)+1")
  CHAT_P95=$(sort -n "$RESULT_DIR/chat_latency" | awk "NR==int($(wc -l < "$RESULT_DIR/chat_latency")*0.95)+1")
  CHAT_MAX=$(sort -n "$RESULT_DIR/chat_latency" | tail -1)
else
  CHAT_P50=0; CHAT_P95=0; CHAT_MAX=0
fi

echo "  并发: $CHAT_CONCURRENCY × $CHAT_REQUESTS = $((CHAT_CONCURRENCY * CHAT_REQUESTS)) 请求"
echo "  成功: $CHAT_OK / 失败: $CHAT_FAIL"
echo "  延迟 P50: ${CHAT_P50}ms / P95: ${CHAT_P95}ms / Max: ${CHAT_MAX}ms"
echo ""

# =============================================
# 汇总
# =============================================
echo "============================================="
echo " 测试结果汇总"
echo "============================================="
echo ""
echo " CRUD API:"
echo "   成功率: $CRUD_OK/$TOTAL"
echo "   P50: ${CRUD_P50}ms  P95: ${CRUD_P95}ms  Max: ${CRUD_MAX}ms"
echo "   目标: P95 < 100ms"
echo ""
echo " AI 对话:"
echo "   成功率: $CHAT_OK/$((CHAT_CONCURRENCY * CHAT_REQUESTS))"
echo "   P50: ${CHAT_P50}ms  P95: ${CHAT_P95}ms  Max: ${CHAT_MAX}ms"
echo "   目标: P95 < 12000ms (12s)"
echo ""

# 判定
PASS=true
if [ "$CRUD_FAIL" -gt 0 ]; then
  echo " ⚠ CRUD 有失败请求"
  PASS=false
fi
if [ "$CHAT_FAIL" -gt 0 ]; then
  echo " ⚠ AI 对话有失败请求"
  PASS=false
fi
if [ "$CRUD_P95" -gt 100 ] 2>/dev/null; then
  echo " ⚠ CRUD P95 超过 100ms"
fi
if [ "$CHAT_P95" -gt 12000 ] 2>/dev/null; then
  echo " ⚠ AI 对话 P95 超过 12s"
fi

rm -rf "$RESULT_DIR"

if $PASS; then
  echo ""
  echo " ✅ 压测通过"
else
  echo ""
  echo " ❌ 存在失败，需排查"
  exit 1
fi
