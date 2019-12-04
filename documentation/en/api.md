# API

> This document is a work in progress.

The systems API is defined in here. The **JSON RPC** maps directly to the API defined here using the [JSON RPC package](https://github.com/filecoin-project/lotus/tree/master/lib/jsonrpc).

## Overview

By default `127.0.0.1:1234` - **daemon** stores the API endpoint multiaddr in `~/.lotus/api`

- `http://[api:port]/rpc/v0` - **JSON RPC** HTTP endpoint
- `ws://[api:port]/rpc/v0` - **JSON RPC** websocket endpoint
- `PUT http://[api:port]/rest/v0/import` - import file to the node repo, it requires write permission.

For **JSON RPC** interface definition see [api/api.go](https://github.com/filecoin-project/lotus/blob/master/api/api_full.go). Required permissions are
defined in [api/struct.go](https://github.com/filecoin-project/lotus/blob/master/api/struct.go)

## Auth

**JWT** in the `Authorization: Bearer <token>` http header

Permissions

- `read` - Read node state, no private data
- `write` - Write to local store / chain, read private data
- `sign` - Use private keys stored in wallet for signing
- `admin` - Manage permissions

Payload

```json
{
  "Allow": ["read", "write", ...]
}
```

Admin token is stored in `~/.lotus/token`
