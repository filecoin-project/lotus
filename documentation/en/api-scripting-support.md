# API Scripting Support

You may want to delegate the work **Lotus Storage Miner** or **Lotus Node** perform to other machines. Here is how to setup the necessary authorization and environment variables.

## Generate a JWT

To generate a JWT for your environment variables, use this command:

```sh
lotus auth create-token --perm admin
lotus-storage-miner auth create-token --perm admin
```

## Environment variables

Environmental variables are variables that are defined for the current shell and are inherited by any child shells or processes. Environmental variables are used to pass information into processes that are spawned from the shell.

Using the JWT you generated, you can assign it and the **multiaddr** to the appropriate environment variable.

```sh
# Lotus Node
FULLNODE_API_INFO="JWT_TOKEN:/ip4/127.0.0.1/tcp/1234/http"

# Lotus Storage Miner
STORAGE_API_INFO="JWT_TOKEN:/ip4/127.0.0.1/tcp/2345/http"
```

- The **Lotus Node**'s `mutliaddr` is in `~/.lotus/api`.
- The default token is in `~/.lotus/token`.
- The **Lotus Storage Miner**'s `multiaddr` is in `~/.lotusstorage/config`.
- The default token is in `~/.lotusstorage/token`.
