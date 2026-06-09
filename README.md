# Exchange

Multi-chain DEX pool scanner, registry-based price API, websocket publisher and unsigned swap transaction builder.

This backend currently focuses on registered assets only. Assets are loaded from the payment gateway registry first, with `internal/config/defaults.go` used as a local fallback. If an asset is not in the active registry, the scanner and price API do not use it. This is intentional: when you request `PEPPER`, the API only returns prices from chains and pools that match the registry deployments.

## What Is Included

Chains:

- Chiliz Chain
- Solana Mainnet
- Base
- Ethereum
- Avalanche C-Chain

DEX venues:

- Chiliz: KayenSwap, KewlSwap, DiviSwap
- Solana: Raydium, Orca, Meteora
- Base: Aerodrome Classic, Aerodrome Slipstream
- Ethereum: Uniswap V1, Uniswap V2, Uniswap V3
- Avalanche: Pangolin, Trader Joe legacy V1

Runtime commands:

- `go run ./cmd/executor`: starts the all-in-one runtime, including API, websocket listener, scanner, frontend dev server, and the placeholder exchange services
- `go run ./cmd/scanner`: runs only the scanner
- `go run ./cmd/api`: runs only the Fiber v3 HTTP API and websocket endpoint
- `go run ./cmd/matcher`: runs the async matcher worker for `MATCHING_MODE=async`
- `go run ./cmd/worker`: dispatches durable outbox events to delivery channels
- `go run ./cmd/migrate`: runs database migrations/backfills as a standalone job

## Requirements

- Go 1.24+
- PostgreSQL
- RPC endpoints for the chains you want to scan

The app auto-migrates the `pools` table when `cmd/api` or `cmd/scanner` connects to Postgres.
For production-style microservice deployments, run `cmd/migrate` first and start services with `AUTO_MIGRATE=false`.

## Environment

Create a `.env` file in the repository root:

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/exchange?sslmode=disable

# API bind address. Default is :8080.
API_ADDR=:8080

# OIDC auth. OIDC_ISSUER_URL is the provider discovery issuer URL.
# Keep OIDC_CLIENT_SECRET out of source control.
OIDC_PROVIDER_NAME=RESEARCHCAVE
OIDC_ISSUER_URL=
OIDC_CLIENT_ID=kewlswapexchange
OIDC_CLIENT_SECRET=
OIDC_REDIRECT_URI=http://localhost:8080/auth/oidc/callback
OIDC_SCOPES=openid,profile,email,roles
OIDC_SESSION_SECRET=
OIDC_SESSION_TTL=12h
OIDC_COOKIE_SECURE=false

# Payment gateway integration. Gateway swagger is usually at http://localhost:3001/swagger/index.html.
PAYMENT_GATEWAY_BASE_URL=http://localhost:3001
PAYMENT_GATEWAY_MERCHANT_ID=
PAYMENT_GATEWAY_DOMAIN_ID=
# Optional legacy value. Wallet creation sends product_id equal to PAYMENT_GATEWAY_DOMAIN_ID.
PAYMENT_GATEWAY_PRODUCT_ID=
PAYMENT_GATEWAY_API_KEY=
PAYMENT_GATEWAY_API_SECRET=
# Alias accepted by the exchange backend for gateway X-API-Secret signing.
PAYMENT_GATEWAY_SECRET_KEY=
PAYMENT_GATEWAY_WEBHOOK_SECRET=
PAYMENT_GATEWAY_TIMEOUT=10s
PAYMENT_GATEWAY_SECRET=

# Scanner mode:
# - default/empty: only scan pairs containing registry assets
# - full: scan full factories where the scanner supports full scans
SCANNER_MODE=

# Optional repeat interval. Empty means run once and exit.
# For live updates use values like 1s, 2s, 500ms.
SCANNER_INTERVAL=1s

# Chain RPCs. Comma-separated. Use paid/indexed RPCs for low latency.
CHILIZ_RPC_URLS=https://rpc.chiliz.com,https://chiliz.publicnode.com
SOLANA_RPC_URLS=https://api.mainnet-beta.solana.com
BASE_RPC_URLS=https://base-rpc.publicnode.com,https://mainnet.base.org
ETHEREUM_RPC_URLS=https://ethereum-rpc.publicnode.com,https://eth.llamarpc.com
AVALANCHE_RPC_URLS=https://api.avax.network/ext/bc/C/rpc

# Base Aerodrome overrides. Defaults are already in code.
AERODROME_SLIPSTREAM_FACTORY_ADDRESS=0x5e7BB104d84c7CB9B682AaC2F3d509f5F406809A
AERODROME_SLIPSTREAM_ROUTER_ADDRESS=0xBE6D8f0d05cC4be24d5167a3eF062215bE6D18a5
AERODROME_SLIPSTREAM_QUOTER_ADDRESS=0x254cF9E1E6e233aa1AC962CB9B05b2cfeAaE15b0
AERODROME_SLIPSTREAM_POSITION_MANAGER_ADDRESS=0x827922686190790b37229fd06084350E74485b72

# Chiliz routers. Kayen has a verified default; Kewl/Divi should be set if you want swap transactions.
KAYENSWAP_ROUTER_ADDRESS=0x1918EbB39492C8b98865c5E53219c3f1AE79e76F
KEWLSWAP_ROUTER_ADDRESS=
DIVISWAP_ROUTER_ADDRESS=

# Ethereum Uniswap overrides. Defaults are already in code.
UNISWAP_V1_FACTORY_ADDRESS=0xc0a47dFe034B400B47bDaD5FecDa2621de6c4d95
UNISWAP_V2_FACTORY_ADDRESS=0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f
UNISWAP_V2_ROUTER_ADDRESS=0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D
UNISWAP_V3_FACTORY_ADDRESS=0x1F98431c8aD98523631AE4a59f267346ea31F984
UNISWAP_V3_ROUTER_ADDRESS=0xE592427A0AEce92De3Edee1F18E0157C05861564
UNISWAP_V3_QUOTER_ADDRESS=0x61fFE014bA17989E743c5F6cB21bF9697530B21e

# Avalanche overrides. Defaults are already in code.
PANGOLIN_FACTORY_ADDRESS=0xefa94de7a5529449c8a6857d5b3b61e4c03ee475
PANGOLIN_ROUTER_ADDRESS=0xe54ca86531e17ef3616d22ca28b0d458b6c89106
TRADERJOE_FACTORY_ADDRESS=0x9Ad6C38BE94206cA50bb0d90783181662f0Cfa10
TRADERJOE_ROUTER_ADDRESS=0x60aE616a2155Ee3d9A68541Ba4544862310933d4
```

Public RPCs are enough for development, but not enough for second-by-second trading. For Solana, Base, Ethereum and Avalanche low latency, set your own fast RPCs in the env vars above.

## Start Postgres

Any local Postgres is fine. Example with Docker:

```bash
docker run --name exchange-postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=exchange \
  -p 5432:5432 \
  -d postgres:16
```

Then set:

```bash
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/exchange?sslmode=disable'
```

## Start Everything From Executor

Use this when you want one process to bring up the whole local runtime:

```bash
go run ./cmd/executor
```

This starts:

- Fiber v3 API
- frontend dev server on `http://localhost:3002`
- websocket price publisher
- event backend price/update listener
- pool scanner
- matcher worker, outbox worker plus indexer, executor, settler and scheduler heartbeat services

Frontend options:

```bash
go run ./cmd/executor --frontend=dev
go run ./cmd/executor --frontend=build
go run ./cmd/executor --frontend=off
go run ./cmd/executor --frontend=dev --frontend-port=3002
```

The frontend directory is resolved from the repository root, so both commands below work:

```bash
go run ./cmd/executor
cd cmd/executor && go run .
```

In all-in-one mode, if `SCANNER_INTERVAL` is empty, the runtime sets it to `1s` so the scanner keeps running. If you want a different scan cadence:

```bash
SCANNER_INTERVAL=2s go run ./cmd/executor
```

The API is available at:

```text
http://localhost:8080
```

## Scan Pools Only

Default mode scans only registry assets. This is the fast path and should be used for live prices:

```bash
go run ./cmd/scanner
```

Run continuously and publish websocket updates whenever prices change:

```bash
SCANNER_INTERVAL=1s go run ./cmd/scanner
```

Standalone `cmd/scanner` runs once and exits when `SCANNER_INTERVAL` is empty. The all-in-one executor runtime keeps it alive by default.

Full scan mode:

```bash
SCANNER_MODE=full go run ./cmd/scanner
```

Full scan is only supported by scanners that can enumerate factories efficiently, such as Uniswap V2-compatible factories and Aerodrome. Uniswap V1 and V3 use tracked registry scans because the scanner asks the factory directly for pools matching registered assets.

The scanner writes pools to Postgres and enqueues durable outbox events for the `price_updates` channel. The worker dispatches those events, and the API listens to the channel and publishes websocket messages to clients.

For microservice-style deployments, scanner replicas coordinate with DB-backed leases by default. Disable with `SCANNER_LEASES=false` only for local troubleshooting. Price updates are written to the durable outbox; run `cmd/worker` to dispatch them to the `price_updates` delivery channel.

## Start Matcher

The API keeps synchronous matching by default. To run matching as a separate service, start the API and matcher with async matching enabled:

```bash
MATCHING_MODE=async go run ./cmd/api
MATCHING_MODE=async go run ./cmd/matcher
go run ./cmd/worker
```

In async mode the API accepts orders, reserves balances, writes `exchange_match_jobs`, and returns the order with `pending_match` status. The matcher claims jobs with `FOR UPDATE SKIP LOCKED`, executes the existing matching engine, and writes websocket payloads to the durable outbox. The worker dispatches outbox rows to the selected event backend.

Production-style startup should run migrations once, then disable automatic migrations in service replicas:

```bash
go run ./cmd/migrate
AUTO_MIGRATE=false MATCHING_MODE=async go run ./cmd/api
AUTO_MIGRATE=false MATCHING_MODE=async go run ./cmd/matcher
AUTO_MIGRATE=false SCANNER_INTERVAL=1s go run ./cmd/scanner
AUTO_MIGRATE=false go run ./cmd/worker
```

Event delivery backend is selected with `EVENT_BACKEND`; supported values are `postgres`, `redis`, `nats`, and `kafka`. Use the same backend settings on API and worker:

```bash
EVENT_BACKEND=postgres
EVENT_BACKEND=redis REDIS_URL=redis://localhost:6379/0
EVENT_BACKEND=nats NATS_URL=nats://localhost:4222
EVENT_BACKEND=kafka KAFKA_BROKERS=localhost:9092
```

See `docs/microservices.md` for full backend configuration.

## Start API

```bash
go run ./cmd/api
```

Default address:

```text
http://localhost:8080
```

Health check:

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{"status":"ok"}
```

## Price API

Get all registry-based prices for an asset symbol:

```bash
curl http://localhost:8080/v1/prices/PEPPER
```

Equivalent endpoint:

```bash
curl http://localhost:8080/v1/assets/PEPPER/prices
```

Example shape:

```json
{
  "symbol": "PEPPER",
  "prices": [
    {
      "chain_key": "chiliz",
      "venue_key": "kayenswap",
      "pool_id": "0x...",
      "base_symbol": "PEPPER",
      "base_asset_id": "0x60f397acbcfb8f4e3234c659a3e10867e6fa6b67",
      "quote_symbol": "CHZ",
      "quote_asset_id": "0x677f7e16c7dd57be1d4c8ad1244883214953dc47",
      "price": "0.0123",
      "reserve_base": "1000000000000000000",
      "reserve_quote": "12300000000000000",
      "pool_kind": "v2"
    }
  ]
}
```

Notes:

- Symbol matching is case-insensitive: `PEPPER`, `pepper`, `Pepper` work.
- Unknown assets return 404.
- Pools with unknown quote assets are ignored.
- V2-style pools use reserves.
- V3 pools use `sqrtPriceX96` if reserves are not available.
- `ETH` and `AVAX` are represented by their wrapped EVM deployments in the registry (`WETH` and `WAVAX`) so DEX prices and swaps use token contract addresses.

## Websocket Price Updates

Connect:

```bash
wscat -c ws://localhost:8080/ws/prices
```

When the scanner saves pools, the API publishes messages like:

```json
{
  "type": "prices.updated",
  "data": {
    "symbol": "PEPPER",
    "prices": []
  }
}
```

The websocket only publishes after scanner writes are dispatched by the outbox worker. Start API, scanner, and worker for live updates:

```bash
go run ./cmd/api
```

In another terminal:

```bash
SCANNER_INTERVAL=1s go run ./cmd/scanner
```

In another terminal:

```bash
go run ./cmd/worker
```

## Swap API

The swap API returns quotes, ERC20 approve calldata, and unsigned transaction data. It does not sign or broadcast transactions. Your wallet or execution service must sign and send the returned transaction.

Supported swap paths:

- Uniswap V2-compatible venues: KayenSwap, Pangolin, Trader Joe, Uniswap V2, and configured V2-style DEXs
- Aerodrome Classic through the V2-style executor
- Uniswap V3 through the configured V3 quoter and router
- Aerodrome Slipstream through its tick-spacing based quoter and router

Uniswap V1 is currently used for price scanning. Its swap flow is exchange-contract based and is not wired into the router-based swap API.

### Quote

```bash
curl -X POST http://localhost:8080/v1/swaps/quote \
  -H 'Content-Type: application/json' \
  -d '{
    "chain_key": "chiliz",
    "venue_key": "kayenswap",
    "token_in_symbol": "PEPPER",
    "token_out_symbol": "CHZ",
    "amount_in": "1000000000000000000",
    "slippage_bps": 50
  }'
```

Response:

```json
{
  "chain_key": "chiliz",
  "venue_key": "kayenswap",
  "venue_kind": "uniswap_v2",
  "pool_id": "0x...",
  "token_in": "0x60f397acbcfb8f4e3234c659a3e10867e6fa6b67",
  "token_out": "0x677f7e16c7dd57be1d4c8ad1244883214953dc47",
  "amount_in": "1000000000000000000",
  "amount_out": "123",
  "min_out": "122",
  "fee_bps": 30
}
```

### Approve

Use this before swapping ERC20 tokens:

```bash
curl -X POST http://localhost:8080/v1/swaps/approve \
  -H 'Content-Type: application/json' \
  -d '{
    "chain_key": "chiliz",
    "venue_key": "kayenswap",
    "token_in_symbol": "PEPPER",
    "amount_in": "1000000000000000000"
  }'
```

Response contains unsigned ERC20 `approve(spender, amount)` calldata:

```json
{
  "chain_key": "chiliz",
  "venue_key": "kayenswap",
  "venue_kind": "uniswap_v2",
  "to": "0x60f397acbcfb8f4e3234c659a3e10867e6fa6b67",
  "data": "0x095ea7b3...",
  "value": "0"
}
```

### Build Swap Transaction

```bash
curl -X POST http://localhost:8080/v1/swaps/transaction \
  -H 'Content-Type: application/json' \
  -d '{
    "chain_key": "chiliz",
    "venue_key": "kayenswap",
    "token_in_symbol": "PEPPER",
    "token_out_symbol": "CHZ",
    "amount_in": "1000000000000000000",
    "recipient": "0xYourWalletAddress",
    "sender": "0xYourWalletAddress",
    "slippage_bps": 50
  }'
```

Response:

```json
{
  "quote": {
    "chain_key": "chiliz",
    "venue_key": "kayenswap",
    "venue_kind": "uniswap_v2",
    "pool_id": "0x...",
    "token_in": "0x...",
    "token_out": "0x...",
    "amount_in": "1000000000000000000",
    "amount_out": "123",
    "min_out": "122",
    "fee_bps": 30
  },
  "evm": {
    "chain_key": "chiliz",
    "venue_key": "kayenswap",
    "venue_kind": "uniswap_v2",
    "to": "0x1918EbB39492C8b98865c5E53219c3f1AE79e76F",
    "data": "0x...",
    "value": "0"
  }
}
```

Send `evm.to`, `evm.data`, and `evm.value` to your wallet/signing layer. The backend does not hold private keys.

## OIDC Authentication

OIDC is enabled only when `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URI`, and `OIDC_SESSION_SECRET` are set. `OIDC_PROVIDER_NAME` is a display name; it is not the discovery URL. Use your provider issuer URL in `OIDC_ISSUER_URL`.

Start login:

```text
http://localhost:8080/auth/oidc/login
```

After the provider redirects to `/auth/oidc/callback`, the API verifies the ID token and writes an HTTP-only `exchange_session` cookie. Check the current session:

```bash
curl -b cookies.txt http://localhost:8080/auth/me
```

Logout:

```bash
curl -X POST -b cookies.txt http://localhost:8080/auth/logout
```

When OIDC is enabled, order, balance, withdrawal history and wallet read endpoints require the session cookie. The authenticated OIDC `sub` is used as the exchange `user_id`; request body or path `user_id` values cannot impersonate another user. Gateway mutation endpoints still use `X-Gateway-Secret`.

## Limit Order Protocol

The exchange module includes an internal limit order protocol with:

- asset/market based order books
- persisted price levels
- buy/sell limit matching
- buy/sell market matching with IOC execution
- stop-limit activation
- price-time priority
- decimal precision validation up to 18 places
- optional last-trade price band protection
- deterministic core matching engine
- required `client_order_id` idempotency per user
- row locks while matching and canceling
- incremental price-level refresh instead of full book rebuilds
- GORM-managed orders, trades and price level tables

All amounts and prices are sent as decimal strings. Do not send floats from clients. Every order must include a unique `client_order_id` for that `user_id`; retrying the same request with the same pair returns the existing order instead of creating a duplicate.

By default, limit prices are not restricted by a last-trade price band. Configure `EXCHANGE_PRICE_BAND_BPS` only when an environment needs a protective band; for example `1000` means 10%, and `0` disables the band. If a market has no trades yet, the band is not applied.

### Place Limit Order

The user must have enough available balance before placing an order. Buy orders lock the quote asset at `price * quantity`; sell orders lock the base asset at `quantity`.

```bash
curl -X POST http://localhost:8080/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{
    "client_order_id": "my-order-1",
    "user_id": "user-a",
    "market": "PEPPER/USDC",
    "side": "buy",
    "type": "limit",
    "time_in_force": "gtc",
    "price": "0.000000001",
    "quantity": "1000000"
  }'
```

The response contains the accepted order and any trades created immediately:

```json
{
  "order": {
    "id": "ord_...",
    "market": "PEPPER/USDC",
    "side": "buy",
    "type": "limit",
    "status": "open",
    "price": "0.000000001",
    "quantity": "1000000",
    "filled_quantity": "0",
    "remaining_quantity": "1000000"
  },
  "trades": []
}
```

### Place Market Order

Market orders are immediate-or-cancel (`ioc`) and never rest on the order book. The `price` field is still required as a protection price: for market buys it is the maximum acceptable execution price, and for market sells it is the minimum acceptable execution price. Any unfilled remainder expires and its locked balance is released.

Market buy:

```bash
curl -X POST http://localhost:8080/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{
    "client_order_id": "my-market-buy-1",
    "user_id": "user-a",
    "market": "PEPPER/USDC",
    "side": "buy",
    "type": "market",
    "price": "0.0000000012",
    "quantity": "1000000"
  }'
```

Market sell:

```bash
curl -X POST http://localhost:8080/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{
    "client_order_id": "my-market-sell-1",
    "user_id": "user-a",
    "market": "PEPPER/USDC",
    "side": "sell",
    "type": "market",
    "price": "0.0000000008",
    "quantity": "1000000"
  }'
```

### User Balances

Exchange balances are tracked per `user_id` and asset:

- `available`: can be used for new orders
- `locked`: reserved by open limit/stop-limit orders and in-flight market execution
- `pending`: reported by the payment gateway before final settlement

The exchange database owns user balances. Gateway callbacks only move amounts between `pending`, `available`, and withdrawal state after the exchange verifies the callback.

The deposit screen can request a static deposit address from the payment gateway on demand. The exchange stores only that address metadata in its own wallet table so the user can reuse the same deposit address; it does not read balances from the gateway and it does not auto-create gateway wallets on login. Static deposit address generation requires:

- `PAYMENT_GATEWAY_BASE_URL`
- `PAYMENT_GATEWAY_MERCHANT_ID`
- `PAYMENT_GATEWAY_DOMAIN_ID`
- `PAYMENT_GATEWAY_API_SECRET` or `PAYMENT_GATEWAY_SECRET_KEY` when the gateway requires signed requests

`PAYMENT_GATEWAY_MERCHANT_ID` and `PAYMENT_GATEWAY_DOMAIN_ID` must be UUIDs.

The legacy wallet sync endpoint is kept for compatibility, but it only returns exchange-stored wallet address records and does not call the gateway:

```bash
curl -X POST -b cookies.txt http://localhost:8080/v1/users/YOUR_OIDC_SUB/wallets/sync
```

The exchange keeps only registry-known chains and validates address format before persisting.

Manual gateway wallet registration is still supported:

```bash
curl -X PUT http://localhost:8080/v1/users/user-a/wallets \
  -H 'X-Gateway-Secret: your-secret' \
  -H 'Content-Type: application/json' \
  -d '{
    "chain_key": "chiliz",
    "address": "0xGatewayGeneratedDepositAddress"
  }'
```

List gateway-registered wallets:

```bash
curl http://localhost:8080/v1/users/user-a/wallets
```

List balances:

```bash
curl http://localhost:8080/v1/users/user-a/balances
```

Gateway marks an incoming deposit as pending:

```bash
curl -X POST http://localhost:8080/v1/users/user-a/deposits/pending \
  -H 'X-Gateway-Secret: your-secret' \
  -H 'Content-Type: application/json' \
  -d '{
    "asset": "USDC",
    "amount": "100"
  }'
```

Gateway confirms the pending deposit into available balance:

```bash
curl -X POST http://localhost:8080/v1/users/user-a/deposits/settle \
  -H 'X-Gateway-Secret: your-secret' \
  -H 'Content-Type: application/json' \
  -d '{
    "asset": "USDC",
    "amount": "100"
  }'
```

For production callbacks, set the gateway domain `WebhookURL` to the unified exchange endpoint. The gateway posts both raw deposit/transaction events and payment status events to this same URL and identifies the event with `X-Gateway-Event`.

```text
http://localhost:8080/v1/payment-gateway/callback
```

The integration guide requires signed webhooks and the exchange rejects unsigned callbacks. HMAC is verified with `PAYMENT_GATEWAY_WEBHOOK_SECRET`. The signature is `HMAC-SHA256(timestamp + raw_body)` and may include the `sha256=` prefix.

Supported unified callback events:

- `native_transfer`, `token_transfer`, `erc20_transfer`, `spl_transfer`, `deposit_detected`: marks deposit pending unless the payload status is settled
- `deposit_confirmed`, `manual_test_deposit`, `payment_succeeded`: settles the deposit
- `payment_failed`, `payment_expired`: acknowledged and ignored for balances
- `payout_completed`, `payout_succeeded`, `withdrawal_completed`: completes a withdrawal if the payload includes the exchange `withdrawal_id`
- `payout_failed`, `payout_canceled`, `withdrawal_failed`, `withdrawal_canceled`: cancels a withdrawal if the payload includes the exchange `withdrawal_id`

Deposit callback example:

```bash
BODY='{
  "event_id": "gateway-event-1",
  "payment_id": "payment-1",
  "event_type": "payment_succeeded",
  "user_id": "user-a",
  "symbol": "USDC",
  "expected_amount_raw": "100000000",
  "decimals": 6,
  "status": "paid",
  "chain_id": 8453,
  "tx_hash": "0xabc"
}'
TS=$(date +%s)
SIG=$(printf "%s%s" "$TS" "$BODY" | openssl dgst -sha256 -hmac "$PAYMENT_GATEWAY_WEBHOOK_SECRET" -hex | awk '{print $2}')
curl -X POST http://localhost:8080/v1/payment-gateway/callback \
  -H "X-Gateway-Event: payment_succeeded" \
  -H "X-Gateway-Timestamp: $TS" \
  -H "X-Gateway-Signature: sha256=$SIG" \
  -H 'Content-Type: application/json' \
  -d "$BODY"
```

Accepted deposit statuses:

- pending: `pending`, `awaiting_payment`, `processing`
- settled: `paid`, `confirmed`, `complete`, `completed`, `settled`, `success`, `succeeded`
- ignored/canceled: `cancelled`, `canceled`, `failed`, `rejected`, `expired`

The deposit callback is idempotent. The exchange derives deterministic balance event IDs from `event_id`, `payment_id`, `track_id`, `order_id`, or `tx_hash`, so the same gateway event cannot credit funds twice. Prefer `amount` as a human decimal. If the gateway sends only raw token units, send `amount_raw` and `decimals`.

Local smoke test without admin login:

```bash
./scripts/smoke_gateway_deposit.sh
```

The script reads the gateway domain from exchange `.env`, decrypts the gateway domain webhook secret with the gateway `MASTER_KEY`, sends a signed BTC `deposit_confirmed` callback to the configured domain webhook URL, then verifies the exchange DB balance increased. Useful overrides:

```bash
SMOKE_USER_ID=demo-user SMOKE_AMOUNT_RAW=10000 ./scripts/smoke_gateway_deposit.sh
SMOKE_EVENT_TYPE=manual_test_deposit ./scripts/smoke_gateway_deposit.sh
```

Request a withdrawal. This moves the amount from `available` to `pending` until the payment gateway completes or cancels it:

```bash
curl -X POST http://localhost:8080/v1/users/user-a/withdrawals \
  -H 'Content-Type: application/json' \
  -d '{
    "asset": "USDC",
    "amount": "25",
    "chain_key": "chiliz",
    "address": "0xDestinationAddress"
  }'
```

Gateway completes a withdrawal:

```bash
curl -X POST http://localhost:8080/v1/withdrawals/wd_your_withdrawal_id/complete \
  -H 'X-Gateway-Secret: your-secret'
```

Gateway cancels a withdrawal and returns the pending amount to available balance:

```bash
curl -X POST http://localhost:8080/v1/withdrawals/wd_your_withdrawal_id/cancel \
  -H 'X-Gateway-Secret: your-secret'
```

Withdrawal callback example. The current gateway does not send payout webhooks yet, but the exchange endpoint is ready when that is added:

```bash
curl -X POST http://localhost:8080/v1/payment-gateway/callback \
  -H "X-Gateway-Event: payout_completed" \
  -H "X-Gateway-Timestamp: $TS" \
  -H "X-Gateway-Signature: sha256=$SIG" \
  -H 'Content-Type: application/json' \
  -d '{
    "withdrawal_id": "wd_your_withdrawal_id",
    "status": "completed",
    "tx_hash": "0xabc"
  }'
```

Withdrawal callback statuses `completed`, `settled`, `success`, and `succeeded` complete the withdrawal. `canceled`, `cancelled`, `failed`, `rejected`, and `expired` cancel it and return the pending amount to available balance.

Set `PAYMENT_GATEWAY_SECRET` in production so gateway mutation endpoints require `X-Gateway-Secret`.

### Place Stop-Limit Order

Buy stop-limit triggers when `last_price >= stop_price`. Sell stop-limit triggers when `last_price <= stop_price`.

```bash
curl -X POST http://localhost:8080/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{
    "client_order_id": "my-stop-order-1",
    "user_id": "user-b",
    "market": "PEPPER/USDC",
    "side": "sell",
    "type": "stop_limit",
    "time_in_force": "gtc",
    "stop_price": "0.0000000009",
    "price": "0.00000000085",
    "quantity": "500000"
  }'
```

### Trigger Stop Orders

In production this should be called by the market data/matching loop when last trade price changes. For manual testing:

```bash
curl -X POST http://localhost:8080/v1/orders/triggers \
  -H 'Content-Type: application/json' \
  -d '{
    "market": "PEPPER/USDC",
    "last_price": "0.0000000009"
  }'
```

### Get Order

```bash
curl http://localhost:8080/v1/orders/ord_your_order_id
```

### Cancel Order

```bash
curl -X DELETE 'http://localhost:8080/v1/orders/ord_your_order_id?user_id=user-a'
```

### Order Book

```bash
curl 'http://localhost:8080/v1/orderbook/PEPPER%2FUSDC?depth=50'
```

Response:

```json
{
  "market": "PEPPER/USDC",
  "bids": [
    {
      "market": "PEPPER/USDC",
      "side": "buy",
      "price": "0.000000001",
      "quantity": "1000000",
      "order_count": 1
    }
  ],
  "asks": []
}
```

### History and Chart Data

User order history:

```bash
curl 'http://localhost:8080/v1/users/user-a/orders?market=PEPPER%2FUSDC&limit=100'
```

Filter by status:

```bash
curl 'http://localhost:8080/v1/users/user-a/orders?market=PEPPER%2FUSDC&status=filled&limit=100'
```

User trade history:

```bash
curl 'http://localhost:8080/v1/users/user-a/trades?market=PEPPER%2FUSDC&limit=100'
```

Recent market trades:

```bash
curl 'http://localhost:8080/v1/markets/PEPPER%2FUSDC/trades?limit=100'
```

OHLC candles for charts:

```bash
curl 'http://localhost:8080/v1/markets/PEPPER%2FUSDC/candles?interval=1m&limit=500'
```

Supported candle intervals are `1m`, `5m`, `15m`, `1h`, `4h`, and `1d`. Candles are persisted when trades are created, so chart reads do not need to rebuild OHLC from raw trades on every request.

### Exchange Websocket Events

Connect the UI to:

```text
ws://localhost:8080/ws/orders
```

`/ws` and `/ws/prices` use the same hub. Exchange lifecycle events are published after the database transaction commits:

- `exchange.order_accepted`
- `exchange.order_updated`
- `exchange.order_filled`
- `exchange.order_expired`
- `exchange.order_canceled`
- `exchange.trades_created`
- `exchange.orderbook_updated`
- `exchange.deposit_pending`
- `exchange.deposit_settled`
- `exchange.withdrawal_requested`
- `exchange.withdrawal_completed`
- `exchange.withdrawal_canceled`
- `exchange.wallet_registered`

Example payload:

```json
{
  "type": "exchange.order_filled",
  "market": "PEPPER/USDC",
  "user_id": "user-a",
  "order": {
    "id": "ord_...",
    "status": "filled",
    "remaining_quantity": "0"
  },
  "trades": [
    {
      "id": "trd_...",
      "price": "0.000000001",
      "quantity": "1000000"
    }
  ],
  "created_at": "2026-06-06T10:00:00Z"
}
```

Deposit settlement event example:

```json
{
  "type": "exchange.deposit_settled",
  "user_id": "user-a",
  "balance": {
    "user_id": "user-a",
    "asset": "USDC",
    "available": "100",
    "locked": "0",
    "pending": "0"
  },
  "created_at": "2026-06-06T10:00:00Z"
}
```

### Swagger

Open Swagger UI:

```text
http://localhost:8080/swagger
```

OpenAPI JSON:

```text
http://localhost:8080/swagger.json
```

## Base Aerodrome Examples

Scan Base registry assets across Aerodrome Classic and Slipstream:

```bash
BASE_RPC_URLS=https://your-fast-base-rpc.example \
go run ./cmd/scanner
```

Get Base ETH prices after scanning:

```bash
curl http://localhost:8080/v1/prices/ETH
```

Aerodrome Slipstream quote example:

```bash
curl -X POST http://localhost:8080/v1/swaps/quote \
  -H 'Content-Type: application/json' \
  -d '{
    "chain_key": "base",
    "venue_key": "aerodrome_slipstream_base",
    "token_in_symbol": "ETH",
    "token_out_symbol": "USDC",
    "amount_in": "1000000000000000000",
    "slippage_bps": 50
  }'
```

Aerodrome Slipstream transaction example:

```bash
curl -X POST http://localhost:8080/v1/swaps/transaction \
  -H 'Content-Type: application/json' \
  -d '{
    "chain_key": "base",
    "venue_key": "aerodrome_slipstream_base",
    "token_in_symbol": "ETH",
    "token_out_symbol": "USDC",
    "amount_in": "1000000000000000000",
    "recipient": "0xYourWalletAddress",
    "sender": "0xYourWalletAddress",
    "slippage_bps": 50
  }'
```

## Ethereum Examples

Scan registry assets on Ethereum:

```bash
ETHEREUM_RPC_URLS=https://your-fast-ethereum-rpc.example \
go run ./cmd/scanner
```

Get ETH prices after scanning:

```bash
curl http://localhost:8080/v1/prices/ETH
```

Uniswap V3 quote example:

```bash
curl -X POST http://localhost:8080/v1/swaps/quote \
  -H 'Content-Type: application/json' \
  -d '{
    "chain_key": "ethereum",
    "venue_key": "uniswap_v3_ethereum",
    "token_in_symbol": "ETH",
    "token_out_symbol": "USDC",
    "amount_in": "1000000000000000000",
    "slippage_bps": 50
  }'
```

## Avalanche Examples

Scan Avalanche registry assets:

```bash
AVALANCHE_RPC_URLS=https://your-fast-avalanche-rpc.example \
go run ./cmd/scanner
```

Get AVAX prices:

```bash
curl http://localhost:8080/v1/prices/AVAX
```

Pangolin quote:

```bash
curl -X POST http://localhost:8080/v1/swaps/quote \
  -H 'Content-Type: application/json' \
  -d '{
    "chain_key": "avalanche",
    "venue_key": "pangolin_avalanche",
    "token_in_symbol": "AVAX",
    "token_out_symbol": "USDC",
    "amount_in": "1000000000000000000",
    "slippage_bps": 50
  }'
```

Trader Joe quote:

```bash
curl -X POST http://localhost:8080/v1/swaps/quote \
  -H 'Content-Type: application/json' \
  -d '{
    "chain_key": "avalanche",
    "venue_key": "traderjoe_avalanche",
    "token_in_symbol": "AVAX",
    "token_out_symbol": "USDC",
    "amount_in": "1000000000000000000",
    "slippage_bps": 50
  }'
```

## Registry Rules

Assets are loaded from the payment gateway registry at `PAYMENT_GATEWAY_BASE_URL` (`/api/v1/common/assets`). The local list in `internal/config/defaults.go` is only a fallback when the gateway registry is unavailable.

Current important symbols:

- `PEPPER`
- `CHZ`
- `SOL`
- `ETH`
- `AVAX`
- `USDC`

To add a token:

1. Add it to the gateway asset registry.
2. Add one deployment per chain with the correct token address or Solana mint.
3. Make sure decimals and logo URLs are correct.
4. Restart the exchange services so they reload the gateway registry.
5. Run the scanner again.

Do not duplicate token lists in scanner, API, or swap code. The gateway registry is the source of truth; `defaults.go` remains the development fallback.

## Performance Notes

- Use `SCANNER_MODE=` or leave it empty for targeted registry scans.
- Use `SCANNER_INTERVAL=1s` or lower only with reliable RPCs.
- EVM scanner reuses RPC and multicall clients per chain, so Ethereum Uniswap V1/V2/V3 do not reconnect for every venue.
- Aerodrome Slipstream uses active factory `tickSpacings()` and stores each pool's `tick_spacing`, so quote and transaction calls can use the correct Slipstream pool route.
- Solana public RPCs often throttle `getProgramAccounts`; use a paid/indexed Solana RPC for real-time operation.
- Websocket publish happens after DB save, outbox enqueue, and worker dispatch, so API, scanner, matcher, and worker should point to the same `DATABASE_URL`.

## Tests

Run all tests:

```bash
go test ./...
```

This verifies compilation, storage conversion, pricing behavior, and swap adapter tests.
