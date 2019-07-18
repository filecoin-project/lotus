TODO: make this into a nicer doc

### Endpoints

By default `127.0.0.1:1234` - daemon stores the api endpoint multiaddr in `~/.lotus/api`

* `http://[api:port]/rpc/v0` - jsonrpc http endpoint
* `ws://[api:port]/rpc/v0` - jsonrpc websocket endpoint

### Auth:

JWT in the `Authorization: Bearer <token>` http header

Permissions:
* `read` - read node state, no private data
* `write` - basically root access, for now

Payload:
```json
{
  "Allow": ["read", "write", ...]
}
```
