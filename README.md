# Exchange

Hybrid exchange backend skeleton for Chiliz, Solana, Base, Avalanche and Unichain.

## Runtime model

All command entrypoints boot the same single-process supervisor:

```bash
go run ./cmd/api
```

The supervisor starts these runtime services in one process as goroutines:

- api
- indexer
- matcher
- executor
- settler
- scheduler
- worker

This keeps local development simple while preserving separate `cmd/*` entrypoints for future deployment splits.

## Registries included in runtime

The runtime registers these chains in code and mirrors them in `configs/chains.yaml`:

- Chiliz Chain
- Solana Mainnet
- Base
- Avalanche C-Chain
- Unichain

`PEPPER` is registered as a multi-chain asset across Chiliz, Base, Solana and Unichain. Replace the `TODO_*` addresses/mints in `configs/assets.yaml` and `internal/config/defaults.go` when you have verified contract addresses.
