# Matching Engine Test Report

Date: 2026-06-10

Scope: `internal/core/matching`, `internal/app/orders`, and `internal/adapters/storage/postgres`.

## Current Behavior

- Order book bid levels sort high-to-low; ask levels sort low-to-high.
- Same-price priority is sequence based FIFO.
- Market orders are IOC. If liquidity is insufficient, the filled portion settles and the remaining quantity expires; market orders do not rest.
- Trade price is the maker/resting order price.
- Matching and balance math use decimal strings backed by `math/big.Rat` at 18 decimal places; no `float64` was found on the matching/wallet settlement path.
- Same-user self-trade is prevented with EXPIRE_TAKER behavior. The incoming crossing order expires without trade generation and remaining locked funds are released.
- Wallet accounting uses `ExchangeBalance` plus append-only `ExchangeBalanceEvent` records. There is no separate double-entry ledger table.
- CEX trading fees are not implemented in the matching/wallet settlement path.

## Risks Found

1. Fee accounting is absent. Maker/taker fee calculation, fee asset selection, fee rounding, and exchange fee account crediting cannot be validated yet.
2. Validation is incomplete for exchange-grade rules: tick size, step size, min notional, max order size, max open orders, suspended users, and market buy quote budget are not modeled.
3. `account_id` and `trade_group_id` do not exist on orders, so STP can only be enforced by `user_id`.
4. Ledger semantics are balance-event based, not a strict double-entry journal. `EventRelease` is used for both locked debit and available credit in cancel flows, so event replay requires contextual pairing.
5. `available + locked = total` is implicit because there is no persisted total field.
6. The concurrency test exposed a real DB race in sequence initialization. Order and command-log sequence allocation now seed rows with `ON CONFLICT DO NOTHING` before `FOR UPDATE` locking.

## Tests Added

| Test | File | Risk covered |
| --- | --- | --- |
| `TestOrderBookSortsBidsAndAsksByPrice` | `internal/core/matching/book_security_test.go` | Bid/ask price priority |
| `TestOrderBookNeverCrossed` | `internal/core/matching/book_security_test.go` | Persistent crossed book prevention |
| `TestCancelPreventsFutureMatch` | `internal/core/matching/book_security_test.go` | Canceled orders cannot trade later |
| `TestSelfTradeIsPreventedWithExpireTaker` | `internal/core/matching/book_security_test.go` | Same-user self-trade prevention |
| `TestMarketBookRandomFlowMaintainsInvariants` | `internal/core/matching/book_security_test.go` | Random operation invariant checks |
| `FuzzMarketBookNeverCrossed` | `internal/core/matching/book_security_test.go` | Fuzz/property crossed-book invariant |
| `TestDecimalPrecisionDoesNotLeakFunds` | `internal/core/decimal/decimal_security_test.go` | Exact decimal math, no IEEE-754 leakage |
| `TestDecimalRepeatedDustAddsExactly` | `internal/core/decimal/decimal_security_test.go` | Repeated dust additions conserve value |
| `TestDustPartialFillsConserveExactQuantity` | `internal/core/matching/precision_recovery_security_test.go` | 10,000 dust fills conserve base/quote totals |
| `TestSnapshotJournalReplayIsDeterministic` | `internal/core/matching/precision_recovery_security_test.go` | Snapshot plus result replay determinism |
| `TestLimitBuyLocksQuoteBalanceIntegration` | `internal/app/orders/accounting_security_integration_test.go` | Limit buy quote lock and reservation |
| `TestLimitSellLocksBaseBalanceIntegration` | `internal/app/orders/accounting_security_integration_test.go` | Limit sell base lock and reservation |
| `TestCancelUnlocksRemainingBalanceIntegration` | `internal/app/orders/accounting_security_integration_test.go` | Partial fill cancel unlocks remaining funds |
| `TestLedgerBalancesMatchWalletBalancesIntegration` | `internal/app/orders/accounting_security_integration_test.go` | Trade balance events match wallet state |
| `TestConcurrentOrdersDoNotDoubleSpendIntegration` | `internal/app/orders/accounting_security_integration_test.go` | Concurrent order placement cannot overspend one balance |
| `TestSamePriceConcurrentOrdersKeepSequenceFIFOIntegration` | `internal/app/orders/accounting_security_integration_test.go` | Concurrent same-price orders match by sequence FIFO |
| `TestCancelVsMatchRaceDoesNotDoubleSettleIntegration` | `internal/app/orders/accounting_security_integration_test.go` | Cancel-vs-fill race cannot double release/settle |
| `TestOrderCreationAndLockRollbackIntegration` | `internal/app/orders/accounting_security_integration_test.go` | Order insert plus balance lock rollback atomicity |
| `TestAtomicSettlementRollbackIntegration` | `internal/app/orders/accounting_security_integration_test.go` | Trade/order/balance settlement rollback atomicity |
| `TestSelfTradeIsPreventedIntegration` | `internal/app/orders/accounting_security_integration_test.go` | End-to-end same-user STP with fund release |

Pre-existing tests also cover limit matching invariants, decimal dust, settlement plans, command idempotency, snapshot/recovery, and market order lifecycle.

## Commands

```sh
go test ./...
go test -race ./...
go test -cover ./...
go test -fuzz=FuzzMarketBookNeverCrossed ./internal/core/matching
go test -fuzz=FuzzMatchLimitAccountingInvariants ./internal/core/matching
```

## Coverage Snapshot

- `internal/core/decimal`: 50.0%
- `internal/core/matching`: 78.9%
- `internal/app/orders`: 36.3%
- `internal/adapters/storage/postgres`: 16.9%
