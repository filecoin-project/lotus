## Lotus API

This package contains all lotus API definitions. Interfaces defined here are
exposed as JsonRPC 2.0 endpoints by lotus programs.

### Versions

| File             | Alias File        | Interface      | Exposed by         | Version | HTTP Endpoint | Status                       | Docs
|------------------|-------------------|----------------|--------------------|---------|---------------|------------------------------|------
| `api_common.go`  | `v0api/latest.go` | `Common`       | lotus; lotus-miner | v0      | `/rpc/v0`     | Latest, Stable               | [Methods](../documentation/en/api-v0-methods.md)
| `api_full.go`    | `v1api/latest.go` | `FullNode`     | lotus              | v1      | `/rpc/v1`     | Latest, **Work in progress** | [Methods](../documentation/en/api-v1-unstable-methods.md)
| `api_storage.go` | `v0api/latest.go` | `StorageMiner` | lotus-miner        | v0      | `/rpc/v0`     | Latest, Stable               | [Methods](../documentation/en/api-v0-methods-miner.md)
| `api_worker.go`  | `v0api/latest.go` | `Worker`       | lotus-worker       | v0      | `/rpc/v0`     | Latest, Stable               | [Methods](../documentation/en/api-v0-methods-worker.md)
| `v0api/full.go`  |                   | `FullNode`     | lotus              | v0      | `/rpc/v0`     | Stable                       | [Methods](../documentation/en/api-v0-methods.md)
