#!/usr/bin/env bash
set -Eeuo pipefail

EXCHANGE_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
GATEWAY_DIR="${GATEWAY_DIR:-$(CDPATH= cd -- "$EXCHANGE_DIR/../gateway" && pwd)}"

EXCHANGE_ENV_FILE="${EXCHANGE_ENV_FILE:-$EXCHANGE_DIR/.env}"
GATEWAY_ENV_FILE="${GATEWAY_ENV_FILE:-$GATEWAY_DIR/.env}"

log() {
  printf '[gateway-deposit-smoke] %s\n' "$*"
}

fail() {
  printf '[gateway-deposit-smoke] ERROR: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

env_value() {
  local file="$1"
  local key="$2"

  [[ -f "$file" ]] || return 0

  awk -F= -v key="$key" '
    $0 !~ /^[[:space:]]*#/ {
      name=$1
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", name)
      if (name == key) {
        value=$0
        sub(/^[^=]*=/, "", value)
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
        if (value ~ /^".*"$/ || value ~ /^\047.*\047$/) {
          value=substr(value, 2, length(value)-2)
        }
        print value
        exit
      }
    }
  ' "$file"
}

sql_literal() {
  local value="$1"
  value="${value//\'/\'\'}"
  printf "'%s'" "$value"
}

psql_at() {
  local dsn="$1"
  local sql="$2"
  psql "$dsn" -X -v ON_ERROR_STOP=1 -At -F $'\t' -c "$sql"
}

decrypt_gateway_secret() {
	local encrypted="$1"
	local master_key="$2"
	local tmp_dir
	local output

	tmp_dir="$(mktemp -d)"

	cat >"$tmp_dir/main.go" <<'GO'
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	encrypted := os.Getenv("SMOKE_ENCRYPTED_SECRET")
	masterKey := os.Getenv("SMOKE_MASTER_KEY")
	if encrypted == "" || masterKey == "" {
		fmt.Fprintln(os.Stderr, "encrypted secret and master key are required")
		os.Exit(1)
	}

	hash := sha256.Sum256([]byte(masterKey))
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		fmt.Fprintln(os.Stderr, "ciphertext too short")
		os.Exit(1)
	}
	nonce := ciphertext[:nonceSize]
	payload := ciphertext[nonceSize:]
	plain, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(string(plain))
}
GO

	if ! output="$(SMOKE_ENCRYPTED_SECRET="$encrypted" SMOKE_MASTER_KEY="$master_key" go run "$tmp_dir/main.go")"; then
		rm -rf "$tmp_dir"
		return 1
	fi
	rm -rf "$tmp_dir"
	printf '%s' "$output"
}

need_cmd psql
need_cmd jq
need_cmd curl
need_cmd openssl
need_cmd go

GATEWAY_DATABASE_URL="${GATEWAY_DATABASE_URL:-$(env_value "$GATEWAY_ENV_FILE" DATABASE_URL)}"
EXCHANGE_DATABASE_URL="${EXCHANGE_DATABASE_URL:-$(env_value "$EXCHANGE_ENV_FILE" DATABASE_URL)}"
GATEWAY_MASTER_KEY="${GATEWAY_MASTER_KEY:-$(env_value "$GATEWAY_ENV_FILE" MASTER_KEY)}"

[[ -n "$GATEWAY_DATABASE_URL" ]] || fail "GATEWAY_DATABASE_URL is not set and $GATEWAY_ENV_FILE has no DATABASE_URL"
[[ -n "$EXCHANGE_DATABASE_URL" ]] || fail "EXCHANGE_DATABASE_URL is not set and $EXCHANGE_ENV_FILE has no DATABASE_URL"
[[ -n "$GATEWAY_MASTER_KEY" ]] || fail "GATEWAY_MASTER_KEY is not set and $GATEWAY_ENV_FILE has no MASTER_KEY"

DOMAIN_ID="${SMOKE_DOMAIN_ID:-$(env_value "$EXCHANGE_ENV_FILE" PAYMENT_GATEWAY_DOMAIN_ID)}"
API_KEY="${SMOKE_API_KEY:-$(env_value "$EXCHANGE_ENV_FILE" PAYMENT_GATEWAY_API_KEY)}"
USER_ID="${SMOKE_USER_ID:-$(env_value "$EXCHANGE_DIR/frontend/.env" VITE_EXCHANGE_USER_ID)}"
USER_ID="${USER_ID:-demo-user}"
ASSET="${SMOKE_ASSET:-BTC}"
DECIMALS="${SMOKE_DECIMALS:-8}"
AMOUNT_RAW="${SMOKE_AMOUNT_RAW:-10000}"
EVENT_TYPE="${SMOKE_EVENT_TYPE:-deposit_confirmed}"
CALLBACK_URL="${SMOKE_CALLBACK_URL:-}"

[[ -n "$DOMAIN_ID" ]] || fail "SMOKE_DOMAIN_ID is not set and exchange .env has no PAYMENT_GATEWAY_DOMAIN_ID"
[[ "$DECIMALS" =~ ^[0-9]+$ ]] || fail "SMOKE_DECIMALS must be a non-negative integer"
[[ "$AMOUNT_RAW" =~ ^[1-9][0-9]*$ ]] || fail "SMOKE_AMOUNT_RAW must be a positive integer raw amount"

domain_filter="id = $(sql_literal "$DOMAIN_ID")::uuid"
if [[ -n "$API_KEY" ]]; then
  domain_filter="$domain_filter and api_key = $(sql_literal "$API_KEY")"
fi

domain_row="$(
  psql_at "$GATEWAY_DATABASE_URL" \
    "select id, webhook_url, webhook_secret from domains where $domain_filter limit 1;"
)"
[[ -n "$domain_row" ]] || fail "gateway domain not found for domain_id=$DOMAIN_ID"

IFS=$'\t' read -r FOUND_DOMAIN_ID DOMAIN_WEBHOOK_URL ENCRYPTED_WEBHOOK_SECRET <<<"$domain_row"
[[ -n "$ENCRYPTED_WEBHOOK_SECRET" ]] || fail "gateway domain webhook_secret is empty"
if [[ -z "$CALLBACK_URL" ]]; then
  CALLBACK_URL="$DOMAIN_WEBHOOK_URL"
fi
[[ -n "$CALLBACK_URL" ]] || fail "gateway domain webhook_url is empty"

WEBHOOK_SECRET="$(decrypt_gateway_secret "$ENCRYPTED_WEBHOOK_SECRET" "$GATEWAY_MASTER_KEY")"
[[ -n "$WEBHOOK_SECRET" ]] || fail "failed to decrypt gateway webhook secret"

exchange_webhook_secret="$(env_value "$EXCHANGE_ENV_FILE" PAYMENT_GATEWAY_WEBHOOK_SECRET)"
if [[ -z "$exchange_webhook_secret" ]]; then
  log "warning: exchange .env has no PAYMENT_GATEWAY_WEBHOOK_SECRET; the running exchange process must still have the gateway domain webhook secret in its environment"
elif [[ "$exchange_webhook_secret" != "$WEBHOOK_SECRET" ]]; then
  fail "exchange .env PAYMENT_GATEWAY_WEBHOOK_SECRET does not match the gateway domain webhook secret"
fi

before_available="$(
  psql_at "$EXCHANGE_DATABASE_URL" \
    "select coalesce((select available from exchange_balances where user_id = $(sql_literal "$USER_ID") and asset = $(sql_literal "$ASSET")), '0');"
)"

expected_amount="$(
  psql_at "$EXCHANGE_DATABASE_URL" \
    "select ($(sql_literal "$AMOUNT_RAW")::numeric / power(10::numeric, $(sql_literal "$DECIMALS")::integer))::numeric;"
)"

event_id="smoke-${EVENT_TYPE}-${ASSET}-$(date +%s)-$RANDOM"
tx_hash="manual-smoke-${event_id}"
body="$(
  jq -c -n \
    --arg event_id "$event_id" \
    --arg event_type "$EVENT_TYPE" \
    --arg user_id "$USER_ID" \
    --arg symbol "$ASSET" \
    --arg amount_raw "$AMOUNT_RAW" \
    --arg status "confirmed" \
    --arg tx_hash "$tx_hash" \
    --argjson decimals "$DECIMALS" \
    --argjson chain_id 0 \
    '{
      event_id: $event_id,
      event_type: $event_type,
      user_id: $user_id,
      symbol: $symbol,
      amount_raw: $amount_raw,
      decimals: $decimals,
      status: $status,
      chain_id: $chain_id,
      tx_hash: $tx_hash
    }'
)"

timestamp="$(date +%s)"
signature="$(
  printf '%s%s' "$timestamp" "$body" \
    | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" -hex \
    | awk '{print $2}'
)"

tmp_response="$(mktemp)"
trap 'rm -f "$tmp_response"' EXIT

log "domain=$FOUND_DOMAIN_ID event=$EVENT_TYPE user=$USER_ID asset=$ASSET raw=$AMOUNT_RAW callback=$CALLBACK_URL"

http_status="$(
  curl -sS -o "$tmp_response" -w '%{http_code}' \
    -X POST "$CALLBACK_URL" \
    -H "Content-Type: application/json" \
    -H "X-Gateway-Event: $EVENT_TYPE" \
    -H "X-Gateway-Event-Id: $event_id" \
    -H "X-Gateway-Timestamp: $timestamp" \
    -H "X-Gateway-Signature: sha256=$signature" \
    --data "$body"
)"

response_body="$(cat "$tmp_response")"
if [[ ! "$http_status" =~ ^2 ]]; then
  printf '%s\n' "$response_body" >&2
  fail "callback returned HTTP $http_status"
fi

action="$(jq -r '.action // empty' "$tmp_response")"
[[ "$action" == "deposit_settled" ]] || fail "callback action=$action, want deposit_settled; body=$response_body"

after_available="$(
  psql_at "$EXCHANGE_DATABASE_URL" \
    "select coalesce((select available from exchange_balances where user_id = $(sql_literal "$USER_ID") and asset = $(sql_literal "$ASSET")), '0');"
)"

delta_check="$(
  psql_at "$EXCHANGE_DATABASE_URL" \
    "select case when ($(sql_literal "$after_available")::numeric - $(sql_literal "$before_available")::numeric) = $(sql_literal "$expected_amount")::numeric then 'ok' else 'fail' end;"
)"

[[ "$delta_check" == "ok" ]] || fail "balance delta mismatch: before=$before_available after=$after_available expected_delta=$expected_amount"

log "ok: action=$action before=$before_available after=$after_available delta=$expected_amount event_id=$event_id"
