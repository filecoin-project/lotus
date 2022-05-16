# Builtin Actor Bundles

With NV16, builtin actor bundles must be loaded into lotus for the FVM to operate.

The bundles are specified in build/bundles.toml using the following syntax:
```toml
[[bundles]]
version = X   # actors version
release = tag # release gag
```

This will add a bundle for version `X`, using the github release `tag`
to fetch the bundles at first startup.

If you don't want to fetch the bundle from github, you can specify an explicit path to the bundle (which must be appropriate for your network, typically mainnet):
```toml
[[bundles]]
version = X   # actors version
release = tag # release tag
path = /path/to/builtin-actors.car
```

For development bundles, you can also specify `development = true` so that the bundle is not
recorded in the datastore and reloaded every time the daemon starts up:
```toml
[[bundles]]
version = X   # actors version
release = tag # release gag
path = /path/to/builtin-actors.car
development = true
```

## Local Storage

Bundles downloaded from github will be stored in
`$LOTUS_PATH/builtin-actors/vXXX/YYY/builtin-actors-ZZZ.car``, where
`XXX` is the actors version, `YYY` is the release tag, and `ZZZ` is
the network bundle name.

The sha256 sum of the bundle will be stored next to it, in
`$LOTUS_PATH/builtin-actors/vXXX/YYY/builtin-actors-ZZZ.sha256`

On startup, if a bundle is recorded as loaded the manifest CID will be
checked for presence in the blockstore.  If the manifest is missing,
then the bundle will be reloaded from the local file (if it exists) or
refetched from github.  The sha256 sum is always checked before
loading the bundle.
