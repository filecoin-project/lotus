# Security Policy

## Reporting a Vulnerability

For *critical* bugs, please send an email to security@filecoin.org.

The bug reporting process differs between bugs that are critical and may crash the network, and others that are unlikely to cause problems if malicious parties know about it. For non-critical bugs, please simply file a GitHub [issue](https://github.com/filecoin-project/lotus/issues/new?template=bug_report.md). 

Please try to provide a clear description of any bugs reported, along with how to reproduce the bug if possible. More detailed bug reports (especially those with a PoC included) will help us move forward much faster. Additionally, please avoid reporting bugs that already have open issues. Take a moment to search the issue list of the related GitHub repositories before writing up a new report.

Here are some examples of bugs we would consider 'critical':

* If you can spend from a `multisig` wallet you do not control the keys for.
* If you can cause a miner to be slashed without them actually misbehaving.
* If you can maintain power without submitting windowed posts regularly.
* If you can craft a message that causes lotus nodes to panic.
* If you can cause your miner to win significantly more blocks than it should.
* If you can craft a message that causes a persistent fork in the network.
* If you can cause the total amount of Filecoin in the network to no longer be 2 billion.

This is not an exhaustive list, but should provide some idea of what we consider 'critical'.

## Supported Versions

* TODO: This should be defined and set up by Mainnet launch.

| Version | Supported          |
| ------- | ------------------ |
| Testnet | :white_check_mark: |
