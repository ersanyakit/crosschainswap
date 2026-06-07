# Microservice Runtime

This repository stays a Go monorepo, but runtime entrypoints are now split so each service can be deployed independently.

## Services

| Service | Command | Responsibility |
| --- | --- | --- |
| API | `go run ./cmd/api` | REST API, auth, websocket hub, swap quotes, order intake |
| Scanner | `go run ./cmd/scanner` | DEX pool scanning and price update notifications |
| Matcher | `go run ./cmd/matcher` | Async order matching from `exchange_match_jobs` |
| Worker | `go run ./cmd/worker` | Durable outbox dispatch to delivery channels |
| Migration job | `go run ./cmd/migrate` | GORM schema sync and backfills |
| Executor | `go run ./cmd/executor` | Local all-in-one runtime for development |

`cmd/indexer`, `cmd/scheduler`, and `cmd/settler` run as isolated placeholder services until their domain workflows are implemented.

## Production Startup

Run migrations once before starting service replicas:

```bash
go run ./cmd/migrate
```

Then start services with automatic migrations disabled:

```bash
AUTO_MIGRATE=false go run ./cmd/api
AUTO_MIGRATE=false SCANNER_INTERVAL=1s go run ./cmd/scanner
AUTO_MIGRATE=false MATCHING_MODE=async go run ./cmd/matcher
AUTO_MIGRATE=false go run ./cmd/worker
```

Select the event backend with `EVENT_BACKEND`. The default is `postgres`.

Set `MATCHING_MODE=async` on the API service as well when orders should be accepted by API and matched by `cmd/matcher`:

```bash
AUTO_MIGRATE=false MATCHING_MODE=async go run ./cmd/api
```

Without `MATCHING_MODE=async`, the API keeps the previous synchronous matching behavior.

## Coordination

Scanner instances coordinate with DB-backed leases. The lease key is `scanner:<chain>:<venue>`, so multiple scanner replicas can run without scanning the same venue concurrently.

Useful scanner settings:

```bash
SCANNER_LEASES=true
SCANNER_LEASE_TTL=5m
SCANNER_INTERVAL=1s
```

Async matcher instances claim jobs with `FOR UPDATE SKIP LOCKED`, so multiple matcher replicas can run in parallel. The order row locks and market order sequence locks remain in Postgres.

Useful matcher settings:

```bash
MATCHER_BATCH_SIZE=50
MATCHER_POLL_INTERVAL=500ms
MATCHER_RETRY_DELAY=2s
MATCHER_MAX_ATTEMPTS=5
MATCHER_JOB_LOCK_TTL=5m
```

Matcher and scanner events are written to `exchange_outbox_events`. Worker instances claim outbox rows with `FOR UPDATE SKIP LOCKED`, publish them to the configured delivery backend, and mark them published. API instances subscribe to the same backend and forward payloads to websocket clients.

Useful outbox settings:

```bash
OUTBOX_BATCH_SIZE=100
OUTBOX_POLL_INTERVAL=500ms
OUTBOX_RETRY_DELAY=2s
OUTBOX_MAX_ATTEMPTS=10
OUTBOX_LOCK_TTL=5m
```

API instances listen to `price_updates` and `exchange_updates` and forward payloads to websocket clients.

## Event Backends

Postgres is the default backend and uses `LISTEN/NOTIFY`:

```bash
EVENT_BACKEND=postgres
```

Redis uses Pub/Sub channels named by the outbox topic:

```bash
EVENT_BACKEND=redis
REDIS_URL=redis://localhost:6379/0
# or:
REDIS_ADDR=localhost:6379
REDIS_USERNAME=
REDIS_PASSWORD=
REDIS_DB=0
```

NATS uses core NATS subjects named by the outbox topic:

```bash
EVENT_BACKEND=nats
NATS_URL=nats://localhost:4222
NATS_CLIENT_NAME=exchange-eventstream
```

Kafka uses topics named by the outbox topic. API instances default to an instance-unique consumer group so every API replica receives websocket events; set `KAFKA_CONSUMER_GROUP` only when you intentionally want competing consumers.

```bash
EVENT_BACKEND=kafka
KAFKA_BROKERS=localhost:9092
KAFKA_CONSUMER_GROUP=
```

See `docs/service-ownership.md` for the logical table ownership contract.
