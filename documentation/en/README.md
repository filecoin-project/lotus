# Lotus documentation

This folder contains some Lotus documentation mostly intended for Lotus developers.

User documentation (including documentation for miners) has been moved to specific Lotus sections in https://docs.filecoin.io:

- https://docs.filecoin.io/get-started/lotus
- https://docs.filecoin.io/store/lotus
- https://docs.filecoin.io/mine/lotus
- https://docs.filecoin.io/build/lotus

## The Lotu.sh site

The https://lotu.sh and https://docs.lotu.sh sites are generated from this folder based on the index provided by [.library.json](.library.json). This is done at the [lotus-docs repository](https://github.com/filecoin-project/lotus-docs), which contains Lotus as a git submodule.

To update the site, the lotus-docs repository should be updated with the desired version for the lotus git submodule. Once pushed to master, it will be auto-deployed.
