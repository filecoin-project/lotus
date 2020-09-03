# API endpoints and methods

The API can be accessed on:

- `http://[api:port]/rpc/v0` - HTTP RPC-API endpoint
- `ws://[api:port]/rpc/v0` - Websocket RPC-API endpoint
- `PUT http://[api:port]/rest/v0/import` - REST endpoint for file import (multipart upload). It requires write permissions.

The RPC methods can be found in the [Reference](en+api-methods) and directly in the source code:

- [Both Lotus node + miner APIs](https://github.com/filecoin-project/lotus/blob/master/api/api_common.go)
- [Lotus node API](https://github.com/filecoin-project/lotus/blob/master/api/api_full.go)
- [Lotus miner API](https://github.com/filecoin-project/lotus/blob/master/api/api_storage.go)


## JSON-RPC client

Lotus uses its own Go library implementation of [JSON-RPC](https://github.com/filecoin-project/go-jsonrpc).

## cURL example

To demonstrate making an API request, we will take the method `ChainHead` from [api/api_full.go](https://github.com/filecoin-project/lotus/blob/master/api/api_full.go).

```go
ChainHead(context.Context) (*types.TipSet, error)
```

And create a CURL command. In this command, `ChainHead` is included as `{ "method": "Filecoin.ChainHead" }`:

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer $(cat ~/.lotusminer/token)" \
     --data '{ "jsonrpc": "2.0", "method": "Filecoin.ChainHead", "params": [], "id": 3 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

(See [this section](en+remote-api) to learn how to generate authorization tokens).
