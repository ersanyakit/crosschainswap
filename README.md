# Exchange

Multi-chain DEX pool scanner, registry-based price API, websocket publisher and unsigned swap transaction builder.

This backend currently focuses on registered assets only. If an asset is not in `internal/config/defaults.go`, the scanner and price API do not use it. This is intentional: when you request `PEPPER`, the API only returns prices from chains and pools that match the registry deployments.

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

- `go run ./cmd/executor`: starts the all-in-one runtime, including API, websocket listener, scanner, and the placeholder exchange services
- `go run ./cmd/scanner`: runs only the scanner
- `go run ./cmd/api`: runs only the Fiber v3 HTTP API and websocket endpoint

## Requirements

- Go 1.24+
- PostgreSQL
- RPC endpoints for the chains you want to scan

The app auto-migrates the `pools` table when `cmd/api` or `cmd/scanner` connects to Postgres.

## Environment

Create a `.env` file in the repository root:

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/exchange?sslmode=disable

# API bind address. Default is :8080.
API_ADDR=:8080

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
- websocket price publisher
- Postgres price update listener
- pool scanner
- indexer, matcher, executor, settler, scheduler and worker heartbeat services

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

The scanner writes pools to Postgres and sends Postgres `NOTIFY` messages on the `price_updates` channel. The API listens to that channel and publishes websocket messages to clients.

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

The websocket only publishes after scanner writes trigger Postgres `NOTIFY`. Start both processes for live updates:

```bash
go run ./cmd/api
```

In another terminal:

```bash
SCANNER_INTERVAL=1s go run ./cmd/scanner
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

Assets live in `internal/config/defaults.go`.

Current important symbols:

- `PEPPER`
- `CHZ`
- `SOL`
- `ETH`
- `AVAX`
- `USDC`

To add a token:

1. Add it to the `assets` slice.
2. Add one deployment per chain with the correct address or Solana mint.
3. Make sure decimals are correct.
4. Add or update a market if you want it represented in market metadata.
5. Run the scanner again.

Do not duplicate token lists in scanner, API, or swap code. The registry is the source of truth.

## Performance Notes

- Use `SCANNER_MODE=` or leave it empty for targeted registry scans.
- Use `SCANNER_INTERVAL=1s` or lower only with reliable RPCs.
- EVM scanner reuses RPC and multicall clients per chain, so Ethereum Uniswap V1/V2/V3 do not reconnect for every venue.
- Aerodrome Slipstream uses active factory `tickSpacings()` and stores each pool's `tick_spacing`, so quote and transaction calls can use the correct Slipstream pool route.
- Solana public RPCs often throttle `getProgramAccounts`; use a paid/indexed Solana RPC for real-time operation.
- Websocket publish happens after DB save and Postgres `NOTIFY`, so API and scanner should point to the same `DATABASE_URL`.

## Tests

Run all tests:

```bash
go test ./...
```

This verifies compilation, storage conversion, pricing behavior, and swap adapter tests.
