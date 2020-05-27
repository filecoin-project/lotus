# API

Here is an early overview of how to make API calls.

Implementation details for the **JSON-RPC** package are [here](https://github.com/filecoin-project/lotus/tree/master/lib/jsonrpc).

## Overview: How do you modify the config.toml to change the API endpoint?

API requests are made against `127.0.0.1:1234` unless you modify `.lotus/config.toml`.

Options:

- `http://[api:port]/rpc/v0` - HTTP endpoint
- `ws://[api:port]/rpc/v0` - Websocket endpoint
- `PUT http://[api:port]/rest/v0/import` - File import, it requires write permissions.

## What methods can I use?

For now, you can look into different files to find methods available to you based on your needs:

- [Both Lotus node + storage miner APIs](https://github.com/filecoin-project/lotus/blob/master/api/api_common.go)
- [Lotus node API](https://github.com/filecoin-project/lotus/blob/master/api/api_full.go)
- [Storage miner API](https://github.com/filecoin-project/lotus/blob/master/api/api_storage.go)

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

## CURL authorization

To authorize your request, you will need to include the **JWT** in a HTTP header, for example:

```sh
-H "Authorization: Bearer $(cat ~/.lotusstorage/token)"
```

Admin token is stored in `~/.lotus/token` for the **Lotus Node** or `~/.lotusstorage/token` for the **Lotus Storage Miner**.

## How do I generate a token?

To generate a JWT with custom permissions, use this command:

```sh
# Lotus Node
lotus auth create-token --perm admin

# Lotus Storage Miner
lotus-storage-miner auth create-token --perm admin
```

## What authorization level should I use?

When viewing [api/apistruct/struct.go](https://github.com/filecoin-project/lotus/blob/master/api/apistruct/struct.go), you will encounter these types:

- `read` - Read node state, no private data.
- `write` - Write to local store / chain, and `read` permissions.
- `sign` - Use private keys stored in wallet for signing, `read` and `write` permissions.
- `admin` - Manage permissions, `read`, `write`, and `sign` permissions.
