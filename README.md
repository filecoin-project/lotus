<p align="center">
  <a href="https://docs.lotu.sh/" title="Lotus Docs">
    <img src="documentation/images/lotus_logo_h.png" alt="Project Lotus Logo" width="244" />
  </a>
</p>

<h1 align="center">Project Lotus - 莲</h1>

Lotus is an implementation of the Filecoin Distributed Storage Network. For more details about Filecoin, check out the [Filecoin Spec](https://spec.filecoin.io).

## Building & Documentation

For instructions on how to build lotus from source, please visit [https://docs.lotu.sh](https://docs.lotu.sh) or read the source [here](https://github.com/filecoin-project/lotus/tree/master/documentation).

## Reporting a Vulnerability

Please send an email to security@filecoin.org. See our [security policy](SECURITY.md) for more details.

## Development

The main branches under development at the moment are:
* [`master`](https://github.com/filecoin-project/lotus): current testnet.
* [`next`](https://github.com/filecoin-project/lotus/tree/next): working branch with chain-breaking changes.
* [`ntwk-calibration`](https://github.com/filecoin-project/lotus/tree/ntwk-calibration): devnet running one of `next` commits.

### Tracker

All work is tracked via issues. An attempt at keeping an up-to-date view on remaining work towards Mainnet launch can be seen at the [lotus github project board](https://github.com/orgs/filecoin-project/projects/8). The issues labeled with `incentives` are there to identify the issues needed for Space Race launch.

### Packages

The lotus Filecoin implementation unfolds into the following packages:

- [This repo](https://github.com/filecoin-project/lotus)
- [storage-fsm](https://github.com/filecoin-project/storage-fsm)
- [sector-storage](https://github.com/filecoin-project/sector-storage)
- [specs-storage](https://github.com/filecoin-project/specs-storage)
- [go-fil-markets](https://github.com/filecoin-project/go-fil-markets) which has its own [kanban work tracker available here](https://app.zenhub.com/workspaces/markets-shared-components-5daa144a7046a60001c6e253/board)
- [spec-actors](https://github.com/filecoin-project/specs-actors) which has its own [kanban work tracker available here](https://app.zenhub.com/workspaces/actors-5ee6f3aa87591f0016c05685/board)

## License

Dual-licensed under [MIT](https://github.com/filecoin-project/lotus/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/filecoin-project/lotus/blob/master/LICENSE-APACHE)
