# Storage Mining

This section of the documentation explains how to do storage mining with Lotus. Please note that not everyone can do storage mining, and that you should not attempt it on on networks where sector sizes are 32GB+ unless you meet the [hardware requirements](en+install#hardware-requirements-1).

From this point we assume that you have setup and are running the [Lotus Node](en+setup), that it has fully synced the Filecoin chain and that you are familiar with how to interact with it using the `lotus` command-line interface.

In order to perform storage mining, apart from the Lotus daemon, you will be additionally interacting with the `lotus-miner` and potentially the `lotus-worker` applications (which you should have [installed](en+install-linux) along the `lotus` application already).

