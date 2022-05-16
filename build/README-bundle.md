# Builtin Actor Bundles

With NV16, builtin actor bundles must be loaded into lotus for the FVM to operate.

The bundles are specified in build/bundles.toml using the foloowing syntax:
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
release = tag # release gag
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
