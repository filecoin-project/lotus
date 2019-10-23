TODO: make this into a nicer doc

### Endpoints

By default `127.0.0.1:1234` - daemon stores the api endpoint multiaddr in `~/.lotus/api`

* `http://[api:port]/rpc/v0` - JsonRPC http endpoint
* `ws://[api:port]/rpc/v0` - JsonRPC websocket endpoint
* `PUT http://[api:port]/rest/v0/import` - import file to the node repo
  * Requires write permission

For JsonRPC interface definition see `api/api.go`. Required permissions are
defined in `api/struct.go`

### Auth:

JWT in the `Authorization: Bearer <token>` http header

Permissions:
* `read` - Read node state, no private data
* `write` - Write to local store / chain, read private data
* `sign` - Use private keys stored in wallet for signing
* `admin` - Manage permissions

Payload:
```json
{
  "Allow": ["read", "write", ...]
}
```

Admin token is stored in `~/.lotus/token`
