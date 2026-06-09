#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
MARKET="${MARKET:-USDC/USD}"
TRADED_ASSET="${TRADED_ASSET:-USDC}"
QUOTE_ASSET="${QUOTE_ASSET:-USD}"
BUYER_ID="${BUYER_ID:-demo-buyer}"
SELLER_ID="${SELLER_ID:-demo-seller}"
DEMO_USER_ID="${DEMO_USER_ID:-demo-user}"
STEP_DELAY="${STEP_DELAY:-2}"
ORDERBOOK_DEPTH="${ORDERBOOK_DEPTH:-20}"
CANCEL_AT_END="${CANCEL_AT_END:-0}"
PSQL="${PSQL:-psql}"
PGHOST="${PGHOST:-127.0.0.1}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-exchange}"
PGPASSWORD="${PGPASSWORD:-test}"
export PGPASSWORD

market_query="${MARKET//\//%2F}"

log() {
  printf '[usdc-usd-sim] %s\n' "$*" >&2
}

sleep_step() {
  if [[ "$STEP_DELAY" != "0" ]]; then
    sleep "$STEP_DELAY"
  fi
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
    "$@"
}

reset_usdc_usd_state() {
  log "Resetting ${MARKET} orderbook/history and demo balances"
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
  (:'buyer_id', :'quote_asset', 1000, 0, 0, NOW()),
  (:'buyer_id', :'traded_asset', 0, 0, 0, NOW()),
  (:'seller_id', :'quote_asset', 0, 0, 0, NOW()),
  (:'seller_id', :'traded_asset', 1000, 0, 0, NOW()),
  (:'demo_user_id', :'quote_asset', 1000, 0, 0, NOW()),
  (:'demo_user_id', :'traded_asset', 1000, 0, 0, NOW())
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

require_api() {
  log "Checking exchange API at ${API_BASE_URL}"
  curl -fsS "${API_BASE_URL}/health" >/dev/null
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

print_balances() {
  local user_id="$1"
  curl -fsS "${API_BASE_URL}/v1/users/${user_id}/balances" |
    jq -r --arg quote "$QUOTE_ASSET" --arg traded "$TRADED_ASSET" '
      map(select(.asset == $quote or .asset == $traded)) |
      .[] |
      "  " + .asset + " available=" + .available + " locked=" + .locked + " pending=" + .pending
    '
}

place_order() {
  local user_id="$1"
  local side="$2"
  local price="$3"
  local quantity="$4"
  local label="$5"
  local client_order_id="sim_${label}_$(date +%s%N)"
  local payload
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
    }'
  )"

  local response body status order_id order_status remaining trade_count
  response="$(curl -sS -w $'\n%{http_code}' \
    -H 'Content-Type: application/json' \
    -d "$payload" \
    "${API_BASE_URL}/v1/orders")"
  status="${response##*$'\n'}"
  body="${response%$'\n'*}"
  if [[ "$status" -lt 200 || "$status" -ge 300 ]]; then
    printf '%s\n' "$body" >&2
    log "Order failed: ${side} ${quantity} ${TRADED_ASSET} @ ${price} ${QUOTE_ASSET} (${status})"
    return 1
  fi

  order_id="$(jq -r '.order.id' <<<"$body")"
  order_status="$(jq -r '.order.status' <<<"$body")"
  remaining="$(jq -r '.order.remaining_quantity' <<<"$body")"
  trade_count="$(jq -r '.trades | length' <<<"$body")"
  log "${label}: ${side} ${quantity} ${TRADED_ASSET} @ ${price} ${QUOTE_ASSET} -> ${order_status}, remaining=${remaining}, trades=${trade_count}, id=${order_id}"
  printf '%s\n' "$order_id"
}

cancel_order() {
  local user_id="$1"
  local order_id="$2"
  local response body status
  response="$(curl -sS -w $'\n%{http_code}' -X DELETE "${API_BASE_URL}/v1/orders/${order_id}?user_id=${user_id}")"
  status="${response##*$'\n'}"
  body="${response%$'\n'*}"
  if [[ "$status" -lt 200 || "$status" -ge 300 ]]; then
    printf '%s\n' "$body" >&2
    log "Cancel failed: ${order_id} (${status})"
    return 1
  fi
  log "Canceled ${order_id}: $(jq -r '.status' <<<"$body")"
}

main() {
  if [[ "$MARKET" != "USDC/USD" ]]; then
    log "MARKET=${MARKET}; this script is tuned for USDC/USD, override only when you intentionally test another USD pair."
  fi

  reset_usdc_usd_state
  require_api

  log "Open http://localhost:3002/trade?market=USDCUSD before running with STEP_DELAY>0 to watch the book move."
  log "Initial ${MARKET} orderbook"
  print_book
  sleep_step

  ask_1="$(place_order "$SELLER_ID" "sell" "1.01" "100" "ask_1")"
  print_book
  sleep_step

  ask_2="$(place_order "$SELLER_ID" "sell" "1.02" "150" "ask_2")"
  print_book
  sleep_step

  ask_3="$(place_order "$SELLER_ID" "sell" "1.03" "200" "ask_3")"
  print_book
  sleep_step

  bid_1="$(place_order "$BUYER_ID" "buy" "0.99" "100" "bid_1")"
  print_book
  sleep_step

  bid_2="$(place_order "$BUYER_ID" "buy" "0.98" "200" "bid_2")"
  print_book
  sleep_step

  place_order "$BUYER_ID" "buy" "1.02" "250" "aggressive_buy_sweeps_1_01_and_1_02" >/dev/null
  refresh_market_stats
  log "After aggressive BUY: asks 1.01 and 1.02 must be gone; ask 1.03 and both bids remain."
  print_book
  sleep_step

  place_order "$SELLER_ID" "sell" "0.98" "250" "aggressive_sell_sweeps_0_99_and_partial_0_98" >/dev/null
  refresh_market_stats
  log "After aggressive SELL: bid 0.99 must be gone; bid 0.98 must remain with 50 ${TRADED_ASSET}; ask 1.03 remains."
  print_book

  log "${BUYER_ID} balances"
  print_balances "$BUYER_ID"
  log "${SELLER_ID} balances"
  print_balances "$SELLER_ID"
  log "${DEMO_USER_ID} manual-test balances"
  print_balances "$DEMO_USER_ID"

  if [[ "$CANCEL_AT_END" == "1" ]]; then
    sleep_step
    cancel_order "$BUYER_ID" "$bid_2"
    cancel_order "$SELLER_ID" "$ask_3"
    refresh_market_stats
    log "After final cancels, book must be empty."
    print_book
  else
    log "Leaving remaining live book levels for visual inspection: bid_2=${bid_2}, ask_3=${ask_3}. Set CANCEL_AT_END=1 to clean them."
    log "Filled order ids for reference: ask_1=${ask_1}, ask_2=${ask_2}, bid_1=${bid_1}"
  fi
}

main "$@"
