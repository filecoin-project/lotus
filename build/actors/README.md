# Bundles

This directory includes the actors bundles for each release. Each actor bundle is a zstd compressed
tarfile containing one bundle per network type. These tarfiles are subsequently embedded in the
lotus binary.

## Updating

To update, run the `./pack.sh` script. For example, the following will pack the [builtin actors release](https://github.com/filecoin-project/builtin-actors/releases) `dev/20220602` into the `v8` tarfile.

```bash
./pack.sh v8 dev/20220602
```

This will:

1. Download the actors bundles and pack them into the appropriate tarfile (`$VERSION.tar.zst`).
2. Run `make bundle-gen` in the top-level directory to regenerate the bundle metadata file for _all_ network versions (all `*.tar.zst` files in this directory).

## Overriding

To build a bundle, but specify a different release/tag for a specific network, append `$network=$alternative_release` on the command line. For example:

```bash
./pack.sh v8 dev/20220602 mainnet=v8.0.0 calibrationnet=v8.0.0-rc.1
```

Alternatively, if using a set of locally compiled builtin-actors bundles (`make all-bundles` in builtin-actors), you can specify the path to the directory containing the bundles. For example:

```bash
./pack v15 local/20240930 /path/to/builtin-actors/build/actors
```
