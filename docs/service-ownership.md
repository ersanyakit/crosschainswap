# Service Ownership

This project currently uses one PostgreSQL database, but tables have explicit service ownership. Services may read other domains through repositories or APIs, but writes should stay with the owning service.

## Owned Tables

| Owner | Tables | Write Responsibility |
| --- | --- | --- |
| Scanner | `pools` | DEX pool discovery, reserves, venue metadata |
| API | `exchange_markets`, user wallet registration endpoints | market sync, request intake, HTTP/websocket surface |
| Matcher | `exchange_orders`, `exchange_order_sequences`, `exchange_trades`, `exchange_candles`, `exchange_price_levels`, `exchange_order_events`, `exchange_match_jobs` | order matching, trade creation, book projection |
| Ledger/Settlement | `exchange_balances`, `exchange_balance_events`, `exchange_withdrawals`, `exchange_wallets` | balances, deposits, withdrawals, settlement effects |
| Platform Worker | `exchange_outbox_events`, `service_leases` | durable event dispatch and distributed coordination |

## Rules

- API may create accepted orders and match jobs, but matching state transitions are owned by Matcher.
- Scanner writes pool state and emits durable price events through outbox.
- Matcher writes order/trade/book state and emits durable exchange events through outbox.
- Worker is the only runtime that publishes outbox events to delivery channels.
- Production services should start with `AUTO_MIGRATE=false`; migrations are owned by `cmd/migrate`.
- Future database separation should split by owner above before changing code structure.

## Event Contracts

Current delivery is selected with `EVENT_BACKEND`. Supported backends are Postgres `LISTEN/NOTIFY`, Redis Pub/Sub, NATS core subjects, and Kafka topics. Producers only write the outbox; the worker is responsible for backend delivery.

| Topic | Producer | Consumers |
| --- | --- | --- |
| `price_updates` | Scanner | API websocket publisher |
| `exchange_updates` | API, Matcher, Ledger/Settlement flows | API websocket publisher, future audit/notification workers |
