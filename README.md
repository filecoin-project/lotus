<p align="center">
  <a href="https://docs.filecoin.io/" title="Filecoin Docs">
    <img src="documentation/images/lotus_logo_h.png" alt="Project Lotus Logo" width="244" />
  </a>
</p>

<h1 align="center">Project Lotus - 莲</h1>

<p align="center">
  <a href="https://circleci.com/gh/filecoin-project/lotus"><img src="https://circleci.com/gh/filecoin-project/lotus.svg?style=svg"></a>
  <a href="https://codecov.io/gh/filecoin-project/lotus"><img src="https://codecov.io/gh/filecoin-project/lotus/branch/master/graph/badge.svg"></a>
  <a href="https://goreportcard.com/report/github.com/filecoin-project/lotus"><img src="https://goreportcard.com/badge/github.com/filecoin-project/lotus" /></a>  
  <a href=""><img src="https://img.shields.io/badge/golang-%3E%3D1.14.7-blue.svg" /></a>
  <br>
</p>

Lotus is an implementation of the Filecoin Distributed Storage Network. For more details about Filecoin, check out the [Filecoin Spec](https://spec.filecoin.io).

## Building & Documentation

For instructions on how to build, install and setup lotus, please visit [https://docs.filecoin.io/get-started/lotus](https://docs.filecoin.io/get-started/lotus/).

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
- [go-fil-markets](https://github.com/filecoin-project/go-fil-markets) which has its own [kanban work tracker available here](https://app.zenhub.com/workspaces/markets-shared-components-5daa144a7046a60001c6e253/board)
- [spec-actors](https://github.com/filecoin-project/specs-actors) which has its own [kanban work tracker available here](https://app.zenhub.com/workspaces/actors-5ee6f3aa87591f0016c05685/board)

## License

Dual-licensed under [MIT](https://github.com/filecoin-project/lotus/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/filecoin-project/lotus/blob/master/LICENSE-APACHE)
