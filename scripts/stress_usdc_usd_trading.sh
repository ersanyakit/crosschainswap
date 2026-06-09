#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
MARKET="${MARKET:-USDC/USD}"
TRADED_ASSET="${TRADED_ASSET:-USDC}"
QUOTE_ASSET="${QUOTE_ASSET:-USD}"
BUYER_ID="${BUYER_ID:-stress-buyer}"
SELLER_ID="${SELLER_ID:-stress-seller}"
DEMO_USER_ID="${DEMO_USER_ID:-demo-user}"
CYCLES="${CYCLES:-1000}"
ORDER_DELAY_MS="${ORDER_DELAY_MS:-0}"
RESET_STATE="${RESET_STATE:-1}"
PRINT_EVERY="${PRINT_EVERY:-100}"
ORDERBOOK_DEPTH="${ORDERBOOK_DEPTH:-12}"
STRESS_BALANCE="${STRESS_BALANCE:-1000000}"
DEMO_BALANCE="${DEMO_BALANCE:-1000}"
PSQL="${PSQL:-psql}"
PGHOST="${PGHOST:-127.0.0.1}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-exchange}"
PGPASSWORD="${PGPASSWORD:-test}"
export PGPASSWORD

market_query="${MARKET//\//%2F}"

log() {
  printf '[usdc-usd-stress] %s\n' "$*" >&2
}

psql_exec() {
  "$PSQL" \
    -h "$PGHOST" \
    -p "$PGPORT" \
    -U "$PGUSER" \
    -d "$PGDATABASE" \
    -v ON_ERROR_STOP=1 \
    -v market="$MARKET" \
    -v traded_asset="$TRADED_ASSET" \
    -v quote_asset="$QUOTE_ASSET" \
    -v buyer_id="$BUYER_ID" \
    -v seller_id="$SELLER_ID" \
    -v demo_user_id="$DEMO_USER_ID" \
    -v stress_balance="$STRESS_BALANCE" \
    -v demo_balance="$DEMO_BALANCE" \
    "$@"
}

reset_state() {
  log "Resetting ${MARKET} orderbook/history and seeding stress balances"
  psql_exec <<'SQL'
BEGIN;

INSERT INTO exchange_markets (
  symbol, base_asset, quote_asset, chain_keys, enabled,
  last_price, change_24h, high_24h, low_24h, volume_24h,
  created_at, updated_at
)
VALUES (
  :'market', :'traded_asset', :'quote_asset', '', true,
  0, 0, 0, 0, 0,
  NOW(), NOW()
)
ON CONFLICT (symbol) DO UPDATE SET
  base_asset = EXCLUDED.base_asset,
  quote_asset = EXCLUDED.quote_asset,
  enabled = true,
  last_price = 0,
  change_24h = 0,
  high_24h = 0,
  low_24h = 0,
  volume_24h = 0,
  updated_at = NOW();

DELETE FROM exchange_order_events WHERE market = :'market';
DELETE FROM exchange_trades WHERE market = :'market';
DELETE FROM exchange_candles WHERE market = :'market';
DELETE FROM exchange_price_levels WHERE market = :'market';
DELETE FROM exchange_match_jobs WHERE market = :'market';
DELETE FROM exchange_active_orders WHERE market = :'market';
DELETE FROM exchange_order_command_logs WHERE market = :'market';
DELETE FROM exchange_match_event_logs WHERE market = :'market';
DELETE FROM exchange_order_commands WHERE market = :'market';
DELETE FROM exchange_reservations WHERE market = :'market';
DELETE FROM exchange_orders WHERE market = :'market';
DELETE FROM exchange_order_command_sequences WHERE market = :'market';
DELETE FROM exchange_order_sequences WHERE market = :'market';
DELETE FROM exchange_matcher_snapshots WHERE market = :'market';
DELETE FROM exchange_projection_offsets WHERE market = :'market';

DELETE FROM exchange_balance_events
WHERE user_id IN (:'buyer_id', :'seller_id', :'demo_user_id');

DELETE FROM exchange_balances
WHERE user_id IN (:'buyer_id', :'seller_id', :'demo_user_id');

INSERT INTO exchange_balances (user_id, asset, available, locked, pending, updated_at) VALUES
  (:'buyer_id', :'quote_asset', :'stress_balance'::numeric, 0, 0, NOW()),
  (:'buyer_id', :'traded_asset', :'stress_balance'::numeric, 0, 0, NOW()),
  (:'seller_id', :'quote_asset', :'stress_balance'::numeric, 0, 0, NOW()),
  (:'seller_id', :'traded_asset', :'stress_balance'::numeric, 0, 0, NOW()),
  (:'demo_user_id', :'quote_asset', :'demo_balance'::numeric, 0, 0, NOW()),
  (:'demo_user_id', :'traded_asset', :'demo_balance'::numeric, 0, 0, NOW())
ON CONFLICT (user_id, asset) DO UPDATE SET
  available = EXCLUDED.available,
  locked = 0,
  pending = 0,
  updated_at = NOW();

COMMIT;
SQL
}

refresh_market_stats() {
  psql_exec <<'SQL' >/dev/null
WITH recent AS (
  SELECT price::numeric AS price, quantity::numeric AS quantity, created_at, ctid
  FROM exchange_trades
  WHERE market = :'market'
    AND created_at >= NOW() - INTERVAL '24 hours'
),
stats AS (
  SELECT
    (SELECT price FROM recent ORDER BY created_at DESC, ctid DESC LIMIT 1) AS last_price,
    (SELECT price FROM recent ORDER BY created_at ASC, ctid ASC LIMIT 1) AS first_price,
    MAX(price) AS high_24h,
    MIN(price) AS low_24h,
    COALESCE(SUM(quantity), 0) AS volume_24h
  FROM recent
)
UPDATE exchange_markets
SET
  last_price = COALESCE(stats.last_price, 0),
  change_24h = CASE
    WHEN COALESCE(stats.first_price, 0) > 0
    THEN ((COALESCE(stats.last_price, 0) - stats.first_price) * 100 / stats.first_price)
    ELSE 0
  END,
  high_24h = COALESCE(stats.high_24h, 0),
  low_24h = COALESCE(stats.low_24h, 0),
  volume_24h = COALESCE(stats.volume_24h, 0),
  updated_at = NOW()
FROM stats
WHERE symbol = :'market';
SQL
}

require_tools() {
  command -v jq >/dev/null || {
    log "jq is required"
    exit 1
  }
  curl -fsS "${API_BASE_URL}/health" >/dev/null
}

sleep_between_orders() {
  if [[ "$ORDER_DELAY_MS" != "0" ]]; then
    sleep "$(awk "BEGIN { printf \"%.3f\", ${ORDER_DELAY_MS}/1000 }")"
  fi
}

price_for_cycle() {
  local cycle="$1"
  local side="$2"
  local offset
  offset=$((cycle % 25))
  if [[ "$side" == "ask" ]]; then
    awk -v offset="$offset" 'BEGIN { printf "%.8f", 1.0001 + (offset * 0.0001) }'
  else
    awk -v offset="$offset" 'BEGIN { printf "%.8f", 0.9999 - (offset * 0.0001) }'
  fi
}

quantity_for_cycle() {
  local cycle="$1"
  local offset=$((cycle % 9))
  awk -v offset="$offset" 'BEGIN { printf "%.8f", 0.10 + (offset * 0.01) }'
}

place_order() {
  local user_id="$1"
  local side="$2"
  local price="$3"
  local quantity="$4"
  local label="$5"
  local client_order_id="stress_${label}_$(date +%s%N)"
  local payload response body status order_status

  payload="$(jq -nc \
    --arg client_order_id "$client_order_id" \
    --arg user_id "$user_id" \
    --arg market "$MARKET" \
    --arg side "$side" \
    --arg price "$price" \
    --arg quantity "$quantity" \
    '{
      client_order_id: $client_order_id,
      user_id: $user_id,
      market: $market,
      side: $side,
      type: "limit",
      time_in_force: "gtc",
      price: $price,
      quantity: $quantity
    }')"

  response="$(curl -sS -w $'\n%{http_code}' \
    -H 'Content-Type: application/json' \
    -d "$payload" \
    "${API_BASE_URL}/v1/orders")"
  status="${response##*$'\n'}"
  body="${response%$'\n'*}"
  if [[ "$status" -lt 200 || "$status" -ge 300 ]]; then
    printf '%s\n' "$body" >&2
    log "Order failed: ${label} ${side} ${quantity} ${TRADED_ASSET} @ ${price} ${QUOTE_ASSET} (${status})"
    return 1
  fi
  order_status="$(jq -r '.order.status' <<<"$body")"
  printf '%s\n' "$order_status"
}

print_book() {
  curl -fsS "${API_BASE_URL}/v1/orderbook?market=${market_query}&depth=${ORDERBOOK_DEPTH}" |
    jq -r '
      "ASKS " + (.asks | length | tostring),
      (.asks[]? | "  " + .price + "  " + .quantity),
      "BIDS " + (.bids | length | tostring),
      (.bids[]? | "  " + .price + "  " + .quantity)
    '
}

print_summary() {
  psql_exec <<'SQL'
SELECT
  (SELECT COUNT(*) FROM exchange_orders WHERE market = :'market') AS orders,
  (SELECT COUNT(*) FROM exchange_trades WHERE market = :'market') AS trades,
  (SELECT COUNT(*) FROM exchange_active_orders WHERE market = :'market') AS active_orders,
  (SELECT COUNT(*) FROM exchange_price_levels WHERE market = :'market') AS price_levels,
  (SELECT COUNT(*) FROM exchange_balances WHERE available < 0 OR locked < 0 OR pending < 0) AS negative_balance_rows;

SELECT user_id, asset, available, locked, pending
FROM exchange_balances
WHERE user_id IN (:'buyer_id', :'seller_id', :'demo_user_id')
  AND asset IN (:'quote_asset', :'traded_asset')
ORDER BY user_id, asset;

SELECT symbol, last_price, change_24h, high_24h, low_24h, volume_24h
FROM exchange_markets
WHERE symbol = :'market';
SQL
}

main() {
  if [[ "$MARKET" != "USDC/USD" ]]; then
    log "MARKET=${MARKET}; this stress script is tuned for USDC/USD."
  fi

  require_tools
  if [[ "$RESET_STATE" == "1" ]]; then
    reset_state
  fi

  log "Open http://localhost:3002/trade?market=USDCUSD to watch websocket/orderbook updates."
  log "Starting ${CYCLES} rapid cycles: 4 real API orders per cycle, 2 matches per cycle."

  local start_ns
  start_ns="$(date +%s%N)"

  for ((cycle = 1; cycle <= CYCLES; cycle++)); do
    ask_price="$(price_for_cycle "$cycle" "ask")"
    bid_price="$(price_for_cycle "$cycle" "bid")"
    quantity="$(quantity_for_cycle "$cycle")"

    place_order "$SELLER_ID" "sell" "$ask_price" "$quantity" "maker_ask_${cycle}" >/dev/null
    sleep_between_orders
    place_order "$BUYER_ID" "buy" "$ask_price" "$quantity" "taker_buy_${cycle}" >/dev/null
    sleep_between_orders
    place_order "$BUYER_ID" "buy" "$bid_price" "$quantity" "maker_bid_${cycle}" >/dev/null
    sleep_between_orders
    place_order "$SELLER_ID" "sell" "$bid_price" "$quantity" "taker_sell_${cycle}" >/dev/null
    sleep_between_orders

    if (( cycle % PRINT_EVERY == 0 )); then
      log "cycle=${cycle}/${CYCLES}"
      print_book
    fi
  done

  refresh_market_stats

  local end_ns duration_ns
  end_ns="$(date +%s%N)"
  duration_ns=$((end_ns - start_ns))
  awk -v cycles="$CYCLES" -v ns="$duration_ns" '
    BEGIN {
      seconds = ns / 1000000000
      orders = cycles * 4
      trades = cycles * 2
      printf "[usdc-usd-stress] Completed %d API orders and %d trades in %.2fs (%.2f orders/s, %.2f trades/s)\n", orders, trades, seconds, orders / seconds, trades / seconds > "/dev/stderr"
    }
  '
  print_summary
}

main "$@"
