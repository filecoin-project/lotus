# Setting up remote API access

The **Lotus Miner** and the **Lotus Node** applications come with their own local API endpoints setup by default when they are running.

These endpoints are used by `lotus` and `lotus-miner` to interact with the running process. In this section we will explain how to enable remote access to the Lotus APIs.

Note that instructions are the same for `lotus` and `lotus-miner`. For simplicity, we will just show how to do it with `lotus`.

## Setting the listening interface for the API endpoint

By default, the API listens on the local "loopback" interface (`127.0.0.1`). This is configured in the `config.toml` file:

```toml
[API]
#  ListenAddress = "/ip4/127.0.0.1/tcp/1234/http"
#  RemoteListenAddress = ""
#  Timeout = "30s"
```

To access the API remotely, Lotus needs to listen on the right IP/interface. The IP associated to each interface can be usually found with the command `ip a`. Once the right IP is known, it can be set in the configuration:

```toml
[API]
ListenAddress = "/ip4/<EXTERNAL_INTERFACE_IP>/tcp/3453/http" # port is an example

# Only relevant for lotus-miner
# This should be the IP:Port pair where the miner is reachable from anyone trying to dial to it.
# If you have placed a reverse proxy or a NAT'ing device in front of it, this may be different from
# the EXTERNAL_INTERFACE_IP.
RemoteListenAddress = "<EXTERNAL_IP_AS_SEEN_BY_OTHERS:<EXTERNAL_PORT_AS_SEEN_BY_OTHERS>"
```

> `0.0.0.0` can be used too. This is a wildcard that means "all interfaces". Depending on the network setup, this may affect security (listening on the wrong, exposed interface).

After making these changes, please restart the affected process.

## Issuing tokens

Any client wishing to talk to the API endpoints will need a token. Tokens can be generated with:

```sh
lotus auth create-token --perm <read,write,sign,admin>
```

(similarly for the Lotus Miner).

The permissions work as follows:

- `read` - Read node state, no private data.
- `write` - Write to local store / chain, and `read` permissions.
- `sign` - Use private keys stored in wallet for signing, `read` and `write` permissions.
- `admin` - Manage permissions, `read`, `write`, and `sign` permissions.


Tokens can then be used in applications by setting an Authorization header as:

```
Authorization: Bearer <token>
```


## Environment variables

`lotus`, `lotus-miner` and `lotus-worker` can actually interact with their respective applications running on a different node. All is needed to configure them are the following the *environment variables*:

```sh
FULLNODE_API_INFO="TOKEN:/ip4/<IP>/tcp/<PORT>/http"
MINER_API_INFO="TOKEN:/ip4/<IP>/tcp/<PORT>/http"
```
