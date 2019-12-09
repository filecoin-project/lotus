# API

Here is an early overview of how to make API calls.

Implementation details for the **JSON-RPC** package are [here](https://github.com/filecoin-project/lotus/tree/master/lib/jsonrpc).

## Overview

API requests are made against `127.0.0.1:1234` unless you modify `~/.lotus/api`. 

Options:

- `http://[api:port]/rpc/v0` - HTTP endpoint
- `ws://[api:port]/rpc/v0` -  Websocket endpoint
- `PUT http://[api:port]/rest/v0/import` - File import, it requires write permissions.

## What methods can I use?

Every `method` is available in [api/api.go](https://github.com/filecoin-project/lotus/blob/master/api/api_full.go). 

The necessary permissions for each are in [api/struct.go](https://github.com/filecoin-project/lotus/blob/master/api/struct.go).

## How do I make an API request?

To demonstrate making an API request, we will take the method `ChainHead` from [api/api.go](https://github.com/filecoin-project/lotus/blob/master/api/api_full.go).

```go
ChainHead(context.Context) (*types.TipSet, error)
```

And create a CURL command. In this command, `ChainHead` is included as `{ "method": "Filecoin.ChainHead" }`:

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     --data '{ "jsonrpc": "2.0", "method": "Filecoin.ChainHead", "params": [], "id": 3 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

If the request requires authorization, add an authorization header:

```sh
curl -X POST \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer $(cat ~/.lotusstorage/token)" \
     --data '{ "jsonrpc": "2.0", "method": "Filecoin.ChainHead", "params": [], "id": 3 }' \
     'http://127.0.0.1:1234/rpc/v0'
```

> In the future we will add a playground to make it easier to build and experiment with API requests.

## Authorization

To authorize your request, you will need to include the **JWT** in a HTTP header, for example:

```sh
-H "Authorization: Bearer $(cat ~/.lotusstorage/token)"
```

Admin token is stored in `~/.lotus/token` for the **Lotus Node** or `~/.lotusstorage/token` for the **Lotus Storage Miner**.

## Authorization types

When viewing [api/struct.go](https://github.com/filecoin-project/lotus/blob/master/api/struct.go), you will encounter these types:

- `read` - Read node state, no private data.
- `write` - Write to local store / chain, read private data.
- `sign` - Use private keys stored in wallet for signing.
- `admin` - Manage permissions.

Payload

```json
{
  "Allow": [
    "read", 
    "write",
    /* other options */
  ]
}
```
