# Builtin Actor Bundles

With NV16, builtin actor bundles must be loaded into lotus for the FVM to operate.

The bundles are specified in build/bundles.toml using the following syntax:
```toml
[[bundles]]
version = X   # actors version
release = tag # release tag
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

## Additional Options for Bundles

- You can also specify a URL, together with a sha256 checksum to avoid downloading from
  github.
- You can also specify an environment variable (`LOTUS_BUILTIN_ACTORS_VX_BUNDLE`), to provide the path dynamically at runtime.

The precedence for bundle fetching/loading is as folllows:
- Check the environment variable `LOTUS_BUILTIN_ACTORS_VX_BUNDLE` for version X bundle; use it if set.
- Check the Path; use the bundle specified by it.
- Check the URL; use the bundle specified by it, and verify the checksum which must be present.
- Otherwise, use the release tag and download from github.

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
