# Enterprise Exchange Matching, Orderbook and Market Data Roadmap

## Purpose

This document defines the roadmap for moving the exchange from the current
Postgres-query-driven matching model to an enterprise-grade architecture that
can support millions of users, millions of orders, high-frequency orderbook
updates, low-latency matching, and scalable chart/market data.

The target system must handle:

- Millions of historical orders.
- Large active order books per market.
- Market and limit orders without stale orderbook state.
- Websocket orderbook deltas at high frequency.
- Low-latency chart reads.
- Reliable recovery after matcher restart.
- Atomic balances, fills, trades, and order state.
- Durable fill confirmation without waiting on synchronous Postgres commits.
- Snapshot and replay recovery with sub-second RTO targets for hot markets.
- Idempotent order command processing under retries and duplicate delivery.
- Strong pre-match wallet reservation and post-match ledger settlement.
- No negative balances, no overfills, no floating-point epsilon issues.

## Current State

The current exchange module uses normal Postgres tables managed by GORM
AutoMigrate. There is no TimescaleDB, hypertable, continuous aggregate, native
partitioning, or dedicated active-order table.

Relevant current tables:

- `exchange_orders`
- `exchange_trades`
- `exchange_candles`
- `exchange_price_levels`
- `exchange_order_events`
- `exchange_balance_events`
- `exchange_match_jobs`

Current positive points:

- There is a deterministic core matching function.
- There is an `exchange_price_levels` projection table for orderbook snapshots.
- Orderbook reads do not always aggregate from `exchange_orders`.
- Trades and candles are persisted.
- There is an async matcher mode using match jobs.
- Matching, balance settlement, trade insert, order events, and price-level
  refresh are inside a transaction.

Current risks:

- The current persistence boundary is a synchronous Postgres transaction. This
  is safe for correctness at small scale but becomes a throughput bottleneck for
  high-volume matching.
- There is no durable replicated event log that can act as the hot-path commit
  boundary for accepted commands and match results.
- Matching candidates are still queried from Postgres on each match.
- `exchange_orders` contains both active and historical orders.
- Filled/canceled/expired history growth can slow active matching queries.
- Moving only to smaller Postgres tables without introducing the durable event
  log first can shift the bottleneck to row-level locks on active-order and
  orderbook projection rows.
- Existing live DB index for `idx_orders_book` may not match the current code's
  `sequence_id` price-time priority requirement.
- `RebuildPriceLevels` only loads up to 1000 open orders per side, which is not
  safe for large books.
- Candle updates happen inside the order/trade transaction and update multiple
  intervals per trade.
- TimescaleDB is not used for trade/candle/event time-series data.
- Matcher recovery depends on DB reload/rebuild instead of a fast book snapshot
  plus event replay.
- Idempotency is not yet modeled as durable command state restored after
  matcher restart.
- Poison-pill commands are not yet isolated with retry limits, quarantine
  events, or a dead-letter queue.
- Cancel command idempotency and terminal-order response caching are not yet
  modeled.
- Cancel-replace/amend race behavior is not yet explicitly modeled for orders
  that fill before the replace command reaches the matcher.
- Snapshot freshness is not yet tied to event-log retention. A stale snapshot
  plus expired stream data can make recovery impossible.
- Wallet reservation and matching are not yet separated into a fast account
  actor plus durable reservation-ledger flow.
- If wallet reservation remains a synchronous RDBMS transaction per order, the
  bottleneck moves from matcher persistence to wallet locking.
- Websocket currently invalidates/pulls snapshots more than it should; the
  target should publish deterministic deltas.

## Core Design Decision

Order matching should not be driven by large Postgres candidate queries, and
the matcher must not wait for one Postgres commit per match batch in the hot
path.

The professional architecture is:

```text
API
  -> wallet/risk account actor reservation
  -> durable reservation event append
  -> durable order command log
  -> market-specific matching engine actor
  -> in-memory orderbook
  -> durable match event log append
  -> websocket/user confirmation after durable append
  -> async DB projection workers
  -> Redis/orderbook snapshots / market data workers / audit storage
```

The hot-path commit boundary should be a durable replicated append-only log,
not a synchronous Postgres transaction. Postgres remains essential, but as a
read model, audit store, reconciliation store, and operational query surface.

The authoritative source for match results should be:

```text
durable command log + durable match event log + periodic matcher snapshots
```

Postgres projections can lag slightly, but the durable event log must not lose
accepted orders or confirmed fills.

Important sequencing rule:

Do not spend multiple phases optimizing the old Postgres-driven matching path
with new hot tables before the durable log foundation exists. Active-order and
orderbook-level tables should be introduced as projections of events, not as a
new synchronous bottleneck that the matcher depends on forever.

## Target Architecture

### 1. Market-Owned Matching Actors

Each market has exactly one active matching owner at a time:

```text
BTC/USD matcher
ETH/USD matcher
USDC/USD matcher
```

Each matcher owns:

- In-memory bids book.
- In-memory asks book.
- Per-price FIFO queues.
- Per-market sequence.
- Last applied event sequence.
- Snapshot/version for orderbook deltas.

Only one matcher may mutate a market book at a time. This gives deterministic
price-time priority and prevents races.

### 2. In-Memory Orderbook Data Structures

Use exact decimal/integer-scaled quantities and prices. Do not use floats.

Recommended memory model:

```text
Book
  bids: ordered price tree desc
  asks: ordered price tree asc
  levels[side][price] -> PriceLevel
  orders[order_id] -> OrderRef

PriceLevel
  price
  total_remaining_quantity
  order_count
  fifo_queue

OrderRef
  order_id
  user_id
  side
  price
  original_quantity
  remaining_quantity
  filled_quantity
  sequence_id
```

Rules:

- Best bid is first key in bid tree.
- Best ask is first key in ask tree.
- FIFO order inside a price level is preserved by `sequence_id`.
- Market orders never rest in memory or DB active book.
- Partially filled limit orders remain active with reduced
  `remaining_quantity`.
- Fully filled/canceled/expired orders are removed from active memory and active
  storage.

### 3. Durable Event Log as the Hot-Path Commit Boundary

The matcher may calculate fills in RAM, but a fill must not be considered
confirmed until the result is appended to a durable replicated log.

Unsafe boundary:

```text
matcher RAM fill -> user confirmation
```

Safe boundary:

```text
matcher RAM fill
  -> append match event to durable replicated log
  -> append success
  -> user/websocket confirmation
  -> async Postgres projections
```

Candidate technologies:

- Kafka / Redpanda
- NATS JetStream
- Apache Pulsar
- custom replicated WAL
- Aeron + replicated journal for very low latency systems

The event log must support:

- Per-market ordered streams.
- Durable append acknowledgment.
- Replay from sequence.
- Retention long enough for recovery and projection rebuilds.
- Idempotent producer/consumer behavior.
- Backpressure visibility.

Important rule:

Postgres projection lag must not block matching. If Postgres is temporarily
slow, the matcher can continue as long as the durable event log and wallet
reservation guarantees hold.

### 4. Order Command Queue and Idempotency

Every order command must be idempotent. Duplicate delivery is normal in
distributed systems.

This applies to every command type, not only new orders:

- new order
- cancel order
- replace/amend order
- trigger/expire order
- administrative force-cancel

Each new-order command should include:

```text
command_id
client_order_id
user_id
market
side
type
price
quantity
reservation_id
request_timestamp
```

Each cancel command should include:

```text
command_id
client_cancel_id
user_id
market
order_id or client_order_id
request_timestamp
```

Each replace/amend command should include:

```text
command_id
client_replace_id
user_id
market
order_id or client_order_id
new_price
new_quantity
additional_reservation_id when required
request_timestamp
```

The matcher must keep an in-memory duplicate filter for recent commands, but
that is not enough. Idempotency state must be recoverable after restart.

Recommended approach:

- Command stream is durable and ordered per market.
- Matcher assigns or consumes a per-market `command_sequence`.
- Every command stores a payload fingerprint. The same id with a different
  payload is rejected.
- The durable event log records command outcome:
  - accepted
  - rejected
  - filled
  - partially filled
  - expired
  - canceled
  - already canceled
  - already filled
  - already expired
  - replace accepted
  - replace rejected
  - command quarantined
- On restart, matcher restores processed command ids from snapshot plus replay.
- API idempotency can query command outcome by `client_order_id`.

Duplicate command behavior:

- Same `client_order_id` and same payload returns the original outcome.
- Same `client_order_id` with different payload is rejected.
- Same `command_id` is ignored after first successful processing.
- Same cancel command returns the original cancel outcome.
- Cancel for an already canceled order returns idempotent success, not
  `order_not_found`.
- Cancel for an already filled or expired order returns the terminal state and
  must not release funds a second time.
- Same replace command returns the original replace outcome.
- Replace for an already filled, canceled, or expired order is rejected
  idempotently and must not open a new order.

The matcher must retain a bounded terminal-order cache for recently filled,
canceled, and expired orders. The cache can be restored from snapshot plus
event replay. This prevents duplicate cancel commands from producing confusing
or unsafe responses after the active order has already been removed from RAM.

Cancel-replace/amend behavior:

- Replace is one atomic matcher command. The API must not implement it as a
  client-side cancel call followed by a separate new-order call.
- If the target order is terminal before the replace command is applied, the
  replace command returns the terminal state and does not create a new order.
- If the target order is partially filled, replace applies only to the
  remaining quantity.
- If replace increases required quote/base reservation, the command must
  reference a valid additional reservation before entering the matcher.
- If replace reduces required reservation, the unused reservation is released by
  a durable release event.
- Price-changing replace normally loses original FIFO priority and receives a
  new sequence unless the product explicitly supports a reduce-only amend that
  preserves priority.

Poison-pill command handling:

- Commands must pass schema, version, size, decimal-scale, market, and
  reservation validation before they can mutate matcher state.
- Matcher processing must have a panic/recover boundary that records the
  crashing command sequence and restarts from the last safe snapshot/replay
  point.
- If the same `command_sequence_id` crashes or fails deterministically more
  than the configured threshold, it must be quarantined and sent to a DLQ.
- Quarantine must append a durable `command_quarantined` event and release any
  reservation that can be safely released.
- A quarantined command must be visible to audit/admin tooling with its payload
  fingerprint, error class, retry count, and market sequence.
- If state corruption cannot be excluded, the market enters circuit-breaker
  mode instead of automatically skipping the command.

### 5. Wallet Reservation and Balance Safety

The matcher must not receive orders that are not funded.

Correct flow:

```text
API
  -> Wallet/Risk account actor validates available balance in memory
  -> reservation event appended to durable wallet log
  -> reservation acknowledged after durable append
  -> order command references reservation_id
  -> matcher only consumes reserved funds
  -> fill events settle reserved funds
  -> cancel/expire events release reserved funds
```

Wallet state should be ledger-based and exactly decimal/integer-scaled. For
scale, the target wallet service should use account actors or user/asset
shards, not one global relational table lock per order.

Recommended account actor model:

```text
account actor: user_id + asset
  available
  locked
  pending
  recent reservation ids
  last ledger sequence
```

Reservation flow:

1. Route reservation command to the account actor for `user_id + asset`.
2. Actor checks in-memory `available`.
3. Actor appends `reservation_created` to a durable replicated wallet stream.
4. After append ack, actor mutates memory and returns `reservation_id`.
5. Async projection workers update SQL ledger/balance read models.

This keeps the reservation path safe without moving the synchronous Postgres
bottleneck from the matcher into the wallet service.

Recommended wallet ledger events:

- deposit_pending
- deposit_settled
- withdrawal_requested
- reservation_created
- reservation_released
- trade_debited
- trade_credited
- fee_debited

Balance invariants:

- `available >= 0`
- `locked >= 0`
- `pending >= 0`
- `available + locked + pending` changes only by valid ledger events
- matcher can never spend more than `reservation_id` permits

The API may still perform fast pre-checks before queueing commands, but the
authoritative reservation decision must come from the wallet actor and durable
wallet event append. A pre-check filter alone is not a correctness boundary.

### 6. Active Orders Table

Add a dedicated active order projection table. Do not rely on the historical
`orders` table for active matching.

Important:

`exchange_active_orders` is a read model and recovery fallback. It should be
written by event-log projection workers. It must not become the long-term
source that a high-throughput matcher queries and mutates on every fill.

Recommended table:

```sql
CREATE TABLE exchange_active_orders (
  order_id varchar(64) PRIMARY KEY,
  user_id varchar(128) NOT NULL,
  market varchar(64) NOT NULL,
  base_asset varchar(32) NOT NULL,
  quote_asset varchar(32) NOT NULL,
  side varchar(16) NOT NULL,
  type varchar(32) NOT NULL,
  time_in_force varchar(16) NOT NULL,
  price numeric(78,18) NOT NULL,
  original_quantity numeric(78,18) NOT NULL,
  filled_quantity numeric(78,18) NOT NULL DEFAULT 0,
  remaining_quantity numeric(78,18) NOT NULL,
  sequence_id bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
```

Critical indexes:

```sql
CREATE INDEX CONCURRENTLY idx_active_orders_bids
ON exchange_active_orders (market, price DESC, sequence_id)
WHERE side = 'buy' AND remaining_quantity > 0;

CREATE INDEX CONCURRENTLY idx_active_orders_asks
ON exchange_active_orders (market, price ASC, sequence_id)
WHERE side = 'sell' AND remaining_quantity > 0;

CREATE INDEX CONCURRENTLY idx_active_orders_user_market
ON exchange_active_orders (user_id, market, created_at DESC);
```

This table should stay small compared to history because only open and
partially filled limit/stop-limit orders live here.

Target write model:

- `OrderAccepted` / `OrderAddedToBook` event inserts the active row.
- `MakerPartiallyFilled` event updates remaining quantity.
- `OrderFilled`, `OrderCanceled`, and `OrderExpired` events delete the active
  row.
- Projection workers batch these writes and store their consumed event offset.
- Reconciliation can rebuild this table from the durable event log.

Temporary bridge rule:

During migration, the current Postgres matcher may dual-write this table for
safety. That bridge must be explicitly temporary and should not delay the event
log and in-memory matcher phases.

### 7. Order History Table

Keep `exchange_orders` as the canonical order history table.

It stores all orders:

- accepted
- open
- partially filled
- filled
- canceled
- expired
- rejected

But the matching hot path should read active state from memory or
`exchange_active_orders`, not scan `exchange_orders`.

### 8. Partially Filled Behavior

Partially filled limit order example:

```text
Limit buy: 100 units at 10 USD
Fill: 40 units

remaining_quantity = 60
filled_quantity = 40
status = partially_filled
```

Expected state:

- Remains in `exchange_active_orders`.
- Remains in memory price-level FIFO queue.
- Keeps original `sequence_id`.
- Keeps original FIFO priority.
- `order_book_levels.quantity` decreases by 40.
- User locked balance remains only for the unfilled 60 units.

Full fill:

- `remaining_quantity = 0`
- history order status becomes `filled`
- row is removed from `exchange_active_orders`
- memory order is removed
- price level quantity decreases
- price level is removed if quantity becomes zero

Cancel:

- history order status becomes `canceled`
- row is removed from `exchange_active_orders`
- memory order is removed
- price level quantity decreases by remaining quantity
- locked balance is released

Market order:

- inserted into history
- never inserted into `exchange_active_orders`
- never added to memory book
- consumes existing liquidity only
- leftover becomes expired/canceled according to product rules

### 9. Orderbook Price-Level Projection

Orderbook should not be calculated by aggregating active orders on each REST
request.

Keep a dedicated materialized projection:

```sql
CREATE TABLE exchange_order_book_levels (
  market varchar(64) NOT NULL,
  side varchar(16) NOT NULL,
  price numeric(78,18) NOT NULL,
  quantity numeric(78,18) NOT NULL,
  order_count bigint NOT NULL,
  first_sequence_id bigint NOT NULL,
  version bigint NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (market, side, price)
);
```

Indexes:

```sql
CREATE INDEX CONCURRENTLY idx_order_book_levels_bids
ON exchange_order_book_levels (market, price DESC)
WHERE side = 'buy' AND quantity > 0;

CREATE INDEX CONCURRENTLY idx_order_book_levels_asks
ON exchange_order_book_levels (market, price ASC)
WHERE side = 'sell' AND quantity > 0;
```

Update rules:

- New limit order:
  - consume `OrderAddedToBook`
  - increment level quantity
  - increment order count
  - publish orderbook delta

- Partial fill:
  - consume `MakerPartiallyFilled`
  - decrease maker remaining
  - decrease level quantity by fill quantity
  - keep order count unchanged
  - publish orderbook delta

- Full fill:
  - consume `MakerFilled`
  - remove active order projection
  - decrease level quantity
  - decrement order count
  - delete level if quantity is zero
  - publish orderbook delta

- Cancel:
  - consume `OrderCanceled`
  - remove active order projection
  - decrease level quantity by remaining quantity
  - decrement order count
  - delete level if quantity is zero
  - publish orderbook delta

REST orderbook reads should use this table or a Redis snapshot. Websocket should
stream deltas.

The price-level projection must be updated from ordered market events, not by
many concurrent API transactions racing to update the same hot price rows.
Projection workers can batch and coalesce deltas for the same market/price
level.

### 10. Redis / Cache Layer for Orderbook

For high read traffic:

```text
Matcher -> orderbook delta event -> Redis sorted snapshot -> websocket clients
```

Redis can store:

- top N bid levels
- top N ask levels
- sequence/version
- last snapshot timestamp

REST snapshot path:

1. Try Redis snapshot.
2. Fallback to `exchange_order_book_levels`.
3. Never aggregate `exchange_orders` on user request.

### 11. Websocket Delta Model

Orderbook websocket events must be deterministic and versioned:

```json
{
  "type": "orderbook_delta",
  "market": "USDC/USD",
  "version": 123456,
  "bids": [["0.9901", "1500"]],
  "asks": [["0.9910", "250"]],
  "removed_bids": ["0.9890"],
  "removed_asks": []
}
```

Client flow:

1. Load REST snapshot with version.
2. Subscribe to deltas.
3. Apply only deltas with `version > snapshot.version`.
4. If version gap occurs, reload snapshot.

### 12. Trades and Candles with TimescaleDB

TimescaleDB should be used for time-series/history workloads, not as the primary
matching engine.

Target hypertables:

- `exchange_trades`
- `exchange_candles`
- `exchange_order_events`
- `exchange_balance_events`

Example:

```sql
CREATE EXTENSION IF NOT EXISTS timescaledb;

SELECT create_hypertable(
  'exchange_trades',
  'created_at',
  partitioning_column => 'market',
  number_partitions => 16,
  if_not_exists => TRUE
);

SELECT create_hypertable(
  'exchange_candles',
  'open_time',
  partitioning_column => 'market',
  number_partitions => 16,
  if_not_exists => TRUE
);
```

Candles should not be updated synchronously inside the matching transaction.

Better options:

1. Async candle worker consumes trade events and writes candles.
2. Timescale continuous aggregates generate OHLCV.
3. Keep 1m candles as base and roll up higher intervals async.

Matching must not synchronously update candles. Candle/chart data can be
eventually consistent and rebuilt from durable trade events.

### 13. Persistence, Atomicity and Projection Model

The hot path must distinguish between confirmation durability and query-model
persistence.

Hot-path requirement:

```text
match result is confirmed only after durable replicated log append succeeds
```

Cold-path requirement:

```text
Postgres, TimescaleDB, Redis, and websocket fanout are projections/consumers
of the durable event log
```

The matcher should not wait for synchronous Postgres commits per match batch.
Instead:

```text
matcher computes fills in memory
  -> append match events to durable log
  -> acknowledge user/websocket after log append
  -> database workers consume event log
  -> workers batch-write Postgres/Timescale/Redis projections
```

Events that must be appended durably:

- order accepted/rejected
- order added to active book
- maker/taker fill
- order partially filled
- order filled
- order canceled
- order expired
- reservation consumed
- reservation released
- trade created
- fee charged
- orderbook level delta

Database projection workers should batch writes:

```text
read N events
begin transaction
  update order history projection
  update active order projection
  insert trades
  update orderbook level projection
  update wallet ledger projection if owned locally
  mark projection offsets
commit
```

This converts many small commits into fewer batch commits and prevents Postgres
from throttling matcher throughput.

Projection lag policy:

- Order/trade websocket confirmations come from durable matcher events.
- REST history can lag within an agreed SLA.
- Admin/audit screens read Postgres projections.
- Reconciliation compares event log, active projections, balances, and trades.

Postgres remains important, but it is not the synchronous hot-path authority for
fill confirmation.

### 14. Recovery, Snapshots and Replay

Matcher startup for a market:

1. Acquire market ownership lease.
2. Load latest durable matcher snapshot for that market.
3. Restore in-memory bids, asks, levels, active orders, processed command ids,
   and last applied sequence from snapshot.
4. Replay durable command/match events after `snapshot_sequence_id`.
5. Rebuild derived indexes in memory.
6. Start consuming new commands.

Database-only recovery from `exchange_active_orders` is a fallback, not the
primary recovery path for hot markets.

Snapshot requirements:

- Snapshot after every N commands or every M seconds.
- Include market, sequence id, book levels, FIFO queues, active orders,
  processed command id window, and checksum.
- Include recent terminal orders and cancel outcomes so duplicate cancel
  commands remain idempotent after restart.
- Store snapshot in a durable location:
  - local NVMe plus replicated object store
  - replicated key-value store
  - Redis only if persistence and replication are explicitly configured
- Snapshot capture must not pass mutable matcher data structures to a background
  writer.
- Snapshot write must not block matching for long periods.
- Snapshot must be validated by checksum before use.

Safe snapshot capture model:

```text
matcher actor reaches sequence N
  -> creates immutable/frozen snapshot view or copy-on-write root
  -> immediately continues processing new commands on a new mutable state/root
  -> background writer serializes the frozen view
  -> writer stores snapshot with sequence, checksum, and schema version
```

Unsafe snapshot model:

```text
background writer receives pointers to mutable bids/asks/FIFO queues
matcher continues mutating those same structures
writer serializes inconsistent state
```

Implementation options:

- Copy-on-write state roots.
- Immutable/persistent tree structures.
- Short synchronous copy into a compact snapshot struct, followed by async
  serialization.
- Process-level fork style snapshotting only if the runtime/operating model
  supports it safely.

The synchronous capture step must be bounded and observable. The async writer
must never read data that the live matcher can still mutate.

Event-log retention invariant:

- Event log retention must always cover replay from the latest valid snapshot.
- Operationally, the retention window should be at least 3x the maximum allowed
  age of the latest successful snapshot for hot markets.
- The oldest retained event sequence must be less than or equal to
  `latest_valid_snapshot_sequence + 1`.
- If snapshot creation fails, the system must alert immediately.
- If latest snapshot age approaches the retention safety window, the market must
  enter degraded/circuit-breaker mode before recovery becomes impossible.
- Projection rebuild retention can require a longer stream retention window than
  matcher recovery. That requirement must be sized separately.

Target RTO:

- Hot markets should recover from snapshot plus replay in under 1 second when
  practical.
- Full DB rebuild is acceptable only as disaster recovery or offline repair.

If memory state and DB projection diverge:

- Durable event log and latest valid snapshot are source of recovery.
- Rebuild memory from snapshot plus replay.
- Rebuild `exchange_active_orders` and `exchange_order_book_levels` projections
  from event log if needed.
- Emit snapshot invalidation.

### 15. Partitioning / Sharding Strategy

Avoid one physical table per pair unless there is a very specific operational
reason. It becomes hard to migrate and operate.

Preferred:

- Single logical table.
- Partition by market hash or market group.
- Later shard by market group across databases.

Hot markets can be isolated:

```text
shard-1: BTC/USD, ETH/USD
shard-2: USDC/USD, SOL/USD
shard-3: long-tail markets
```

Each market still has one logical matching owner.

## Roadmap

Execution rule:

Apply the phases in order. Phase 0 is only a bridge to keep the current system
safe while Phase 1-3 are built. Do not turn temporary Postgres optimizations
into the target architecture.

### Phase 0 - Immediate Safety Fixes

Goal: keep the current Postgres model safe while the event-sourced
architecture is being built.

Tasks:

- Add proper partial indexes for active order candidate queries.
- Replace old `idx_orders_book` if it still uses `created_at`.
- Ensure matching query uses `(market, side, price, sequence_id)`.
- Ensure open-order queries exclude market orders and zero remaining quantity.
- Remove the 1000-row limit from price-level rebuild or implement cursor-based
  full rebuild.
- Add EXPLAIN-based regression checks for matching candidate queries.
- Remove candle updates from the synchronous matching transaction if possible.
- Add explicit DB metrics for matching transaction duration and candidate query
  latency.

Definition of done:

- Candidate query uses index scan on realistic seeded data.
- Market buy/sell and limit matching tests pass.
- Rebuild price levels handles more than 1000 active orders per side.
- Current system has observability for DB bottlenecks.

### Phase 1 - Durable Event Log and Command Idempotency

Goal: introduce the durable append-only log before adding new hot projections,
so active-order and orderbook tables are projections from day one.

Tasks:

- Choose event log technology:
  - Kafka/Redpanda
  - NATS JetStream
  - Pulsar
  - custom replicated WAL if latency requirements justify it
- Define event schemas for commands, fills, book deltas, reservations, and
  order terminal states.
- Add per-market ordered streams.
- Add `command_id`, `client_order_id`, payload fingerprint, and per-market
  sequence semantics.
- Add idempotent outcomes for new, cancel, replace, expire, and admin cancel
  commands.
- Add atomic cancel-replace command outcomes and payload fingerprints.
- Add command validation before matcher state mutation.
- Add retry-count limiter, command quarantine, and DLQ path for poison-pill
  commands.
- Add durable `command_quarantined` events and reservation release behavior for
  safely rejected poison commands.
- Store recent processed command ids and terminal order outcomes in snapshot
  state.
- Add command outcome lookup by `client_order_id` and cancel command id.
- Add duplicate-command, duplicate-cancel, duplicate-replace, and poison-pill
  tests.

Definition of done:

- Accepted commands are durable before processing is acknowledged.
- Duplicate new-order commands cannot create duplicate fills.
- Duplicate cancel commands return the original terminal outcome.
- Cancel after already-canceled/filled/expired order is idempotent and never
  releases funds twice.
- Replace after already-filled/canceled/expired order is rejected idempotently
  and never opens a new order.
- A poison command cannot crash-loop a market indefinitely.
- Command replay is deterministic.

### Phase 2 - Wallet/Risk Account Actors

Goal: prevent double spend without moving the hot-path bottleneck from matcher
Postgres writes to wallet Postgres locks.

Tasks:

- Implement account actor routing by `user_id + asset` or another shard-safe
  account key.
- Keep actor memory state for available/locked/pending balances and recent
  reservation ids.
- Append reservation events to a durable wallet stream before acknowledging
  reservation success.
- Ensure buy orders reserve quote asset and sell orders reserve base asset
  before entering the market command stream.
- Add reservation consume/release events driven by match/cancel/expire events.
- Add SQL balance and ledger projections as async read models.
- Add generated ledger invariant tests for available/locked/pending balances.

Definition of done:

- Matcher never processes an unfunded order command.
- Wallet reservation does not require one synchronous SQL transaction per order
  in the target path.
- Reservation release/consume events reconcile with order terminal state.
- Balance projections can be rebuilt from wallet and match events.

### Phase 3 - In-Memory Market Matcher

Goal: move matching hot path out of Postgres candidate queries.

Tasks:

- Implement market actor process.
- Consume reserved order commands from the durable command stream.
- Maintain in-memory bid/ask trees and per-price FIFO queues.
- Produce deterministic fills and terminal order outcomes.
- Apply cancel-replace atomically inside the market actor.
- Reject replace if the target order is terminal before command application.
- Apply replace only to remaining quantity when the target order is partially
  filled.
- Append match results to durable event log.
- Confirm users/websocket only after durable append succeeds.
- Keep Postgres writes out of the matcher hot path.
- Add market ownership lease.
- Add generated matching, replay, duplicate-command, poison-pill, and
  cancel-replace race tests.

Definition of done:

- Candidate matching does not query Postgres per fill.
- Market actor can process commands from the durable stream.
- Matcher does not wait for Postgres commit per match batch.
- Cancel-replace cannot create a new order after the original has filled.
- Poison-pill commands are isolated by quarantine/DLQ or circuit breaker.
- All balances remain non-negative under generated workloads.

### Phase 4 - Projection Workers, Active Orders and Orderbook Levels

Goal: create SQL/Redis read models from events without making them the matcher
authority.

Tasks:

- Create `exchange_active_orders`.
- Rename or replace `exchange_price_levels` with
  `exchange_order_book_levels`.
- Add `version` and projection offset tracking.
- Add projection workers that batch-write:
  - order history
  - active orders
  - trades
  - orderbook levels
  - wallet ledger projections if local
  - projection offsets
- Backfill/rebuild projections from the durable event log.
- Add Redis snapshot writer.
- Add orderbook delta websocket event.
- Add reconciliation command:
  - compare event log vs active table
  - compare active table vs orderbook levels
  - compare balances vs wallet ledger

Definition of done:

- DB projections can be rebuilt from event log.
- Projection workers can lag without blocking matcher command intake.
- Duplicate events do not duplicate trades or balance ledger rows.
- REST orderbook never aggregates all historical orders.
- Websocket clients can use snapshot + delta.
- Version gaps trigger snapshot reload.

### Phase 5 - Async Market Data

Goal: remove chart work from the matching transaction.

Tasks:

- Stop updating all candle intervals inside trade transaction.
- Treat trade events from the durable log as the source for candle generation.
- Create candle worker that consumes trade events.
- Generate 1m candles first.
- Roll up 5m, 15m, 1h, 4h, 1d async.
- Add TimescaleDB hypertables for trades/candles/events.

Definition of done:

- Matching latency is not affected by candle updates.
- Chart reads use indexed/hypertable candle data.
- Backfill candle worker can rebuild from trades.

### Phase 6 - Copy-on-Write Snapshot and Replay Recovery

Goal: reduce hot-market matcher restart time from DB rebuild duration to
snapshot plus replay duration.

Tasks:

- Define matcher snapshot format.
- Include active orders, price levels, FIFO queues, processed command ids,
  terminal order outcomes, last sequence id, and checksums.
- Implement bounded snapshot capture using copy-on-write, immutable state roots,
  or a short synchronous copy into a frozen snapshot struct.
- Ensure background snapshot writer never reads live mutable matcher pointers.
- Store snapshots in durable storage.
- Optionally cache latest snapshot in Redis with persistence enabled.
- Snapshot every fixed command count and/or time interval.
- Enforce event-log retention invariant:
  `oldest_retained_sequence <= latest_valid_snapshot_sequence + 1`.
- Alert if latest successful snapshot age exceeds the configured threshold.
- Alert and degrade/halt the market before retention makes replay impossible.
- Replay durable event log after snapshot sequence.
- Add disaster recovery path that can rebuild from event log and DB if snapshot
  is unavailable.

Definition of done:

- 500k active-order book can restore from snapshot plus replay within target
  RTO.
- Snapshot capture does not create unacceptable matcher latency spikes.
- Corrupt snapshot is detected and skipped.
- Snapshot freshness and event retention are monitored together.
- Snapshot failure pages operations before replay coverage is at risk.
- Replay is deterministic and idempotent.

### Phase 7 - Scale and Operations

Goal: production readiness.

Tasks:

- Partition/hypertable history tables.
- Shard markets by market group if needed.
- Add metrics:
  - matcher latency
  - queue lag
  - trade persist latency
  - orderbook delta lag
  - websocket fanout lag
  - active book size
  - DB transaction duration
  - snapshot age
  - event retention headroom
  - command quarantine count
  - DLQ depth
- Add replay simulation tests.
- Add chaos tests:
  - matcher crash before event append
  - matcher crash after event append before websocket publish
  - projection worker failure
  - duplicate command
  - duplicate cancel
  - duplicate replace
  - poison-pill command
  - cancel-replace racing with fill
  - stale websocket client
  - corrupt snapshot
  - snapshot writer failure
  - event retention gap after stale snapshot
  - event replay from older snapshot
- Add load tests:
  - 100k active orders
  - 1m active orders
  - burst market orders
  - top-of-book churn
  - high-cancel workloads

Definition of done:

- System survives matcher restart without losing or duplicating fills.
- Orderbook snapshot and active orders reconcile.
- Trades and balances reconcile.
- P99 matching latency and orderbook publish latency are within target.
- Projection lag is observable and bounded by SLA.

## Recommended Priority

Do not start with TimescaleDB first if matching correctness, idempotency,
event durability, reservation, and projection ownership are still unresolved.

Recommended order:

1. Apply immediate safety fixes to the current Postgres path only as a bridge.
2. Introduce durable command/match event logs, idempotent command outcomes,
   poison-pill quarantine, and DLQ handling.
3. Implement wallet/risk account actors with durable reservation events.
4. Implement market-owned in-memory matcher using durable log as commit
   boundary, including atomic cancel-replace.
5. Add projection workers for order history, `exchange_active_orders`,
   `exchange_order_book_levels`, trades, balances, and offsets.
6. Move candle generation async from trade events.
7. Add TimescaleDB for trades/candles/events.
8. Add copy-on-write matcher snapshots, replay, and event retention guardrails.
9. Add Redis snapshots, websocket delta sequencing, sharding, and operational
   tooling.

## Questions for Architecture Review

Use these questions when asking another AI or engineer to review the plan:

1. Should the durable event log be introduced before active-order/orderbook SQL
   projections are added?
2. Should active-order and orderbook tables be treated only as projections,
   never as matcher authority?
3. What should be the hot-path commit boundary: Kafka/Redpanda, NATS JetStream,
   Pulsar, or a custom replicated WAL?
4. What acknowledgment level is required before telling the user a fill is
   confirmed?
5. Should commands and match events be separate streams or one per-market
   stream?
6. How should durable idempotency be restored after matcher restart?
7. How should cancel command idempotency behave for already-canceled,
   already-filled, and expired orders?
8. How should cancel-replace behave when the target order fills before the
   replace command is applied?
9. What retry threshold should quarantine a poison-pill command, and when
   should the market circuit breaker halt instead of skipping?
10. How many processed command ids and terminal order outcomes must be retained
   in the matcher snapshot?
11. What is the target RTO for a hot market with 500k active orders?
12. Where should matcher snapshots be stored: local NVMe, object store,
   replicated KV, Redis with persistence, or a combination?
13. Which copy-on-write or immutable-state strategy should be used so snapshot
   writers never read live mutable matcher state?
14. What event retention window is required for matcher recovery and projection
   rebuilds?
15. What alert threshold should fire when latest valid snapshot age approaches
   retention risk?
16. Should wallet reservation be implemented as account actors with durable
   wallet streams, or another low-latency ledger design?
17. What is the target p99 reservation latency under burst load?
18. Should candles be generated by Timescale continuous aggregates or a custom
   worker?
19. Which queue should own order commands: Kafka, NATS, Redis Streams, or
   Postgres jobs?
20. What exact guarantees do we need for order command idempotency?
21. How should market ownership leases be implemented?
22. Should wallet reservation be its own service/database/ledger shard?
23. What is the target p99 matching latency and websocket delta latency?
24. What is the expected active order count per hot market?
25. What is the expected trade volume per second?
26. How much Postgres projection lag is acceptable for REST history endpoints?

## Non-Negotiable Rules

- Do not use float for price or quantity.
- Market orders must never rest in active orderbook.
- Filled orders must not remain in active table.
- Partially filled limit orders must remain active with original FIFO priority.
- Active-order and orderbook SQL tables are projections, not the long-term
  matcher authority.
- Orderbook must not be computed by grouping all orders per request.
- Chart/candle aggregation must not block matching.
- Websocket deltas must be versioned.
- User fill confirmation must not be sent before durable event log append
  succeeds.
- Matcher hot path must not wait for synchronous Postgres commit per match
  batch.
- Wallet reservation target path must not wait for one synchronous SQL
  transaction per order.
- Postgres projections must be rebuildable from durable event log.
- Recovery must rebuild memory book from durable snapshot plus event replay.
- Snapshot writers must never serialize live mutable matcher structures.
- Redis-only snapshot is not enough unless persistence and replication are
  explicitly guaranteed.
- Every command must be idempotent across retries and matcher restarts.
- Cancel commands must be idempotent for active and terminal orders.
- Replace/amend commands must be atomic and must not create a new order if the
  target order is already filled, canceled, or expired.
- Poison-pill commands must not create infinite matcher crash loops.
- Quarantined commands must be durable, auditable, and tied to reservation
  release or circuit-breaker handling.
- Snapshot freshness and event-log retention must be monitored as one recovery
  invariant.
- Latest valid snapshot must never become older than the safe replay coverage
  window without a critical alert.
- Every order reaching the matcher must reference a valid wallet reservation.
- Every fill must preserve balance invariants.
