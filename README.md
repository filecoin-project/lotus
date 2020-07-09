# Project Oni 👹

Our mandate is:

> To verify the successful end-to-end outcome of the filecoin protocol and filecoin implementations, under a variety of real-world and simulated scenarios. 

➡️  Find out more about our goals, requirements, execution plan, and team culture, in our [Project Description](https://docs.google.com/document/d/16jYL--EWYpJhxT9bakYq7ZBGLQ9SB940Wd1lTDOAbNE).

## Testing topics

These are the topics we are currently centering our testing efforts on. Our testing efforts include fault induction, stress tests, and end-to-end testing.

* **slashing:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fslashing)
    * We are recreating the scenarios that lead to slashing, as they are not readily seen in mono-client testnets.
    * Context: slashing is the negative economic consequence of penalising a miner that has breached protocol by deducing FIL and/or removing their power from the network.
* **windowed PoSt/sector proving faults:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fsector-proving)
    * We are recreating the proving fault scenarios and triggering them in an accelerated fasion (by modifying the system configuration), so that we're able to verify that the sector state transitions properly through the different milestones (temporary faults, termination, etc.), and under chain fork conditions.
    * Context: every 24 hours there are 36 windows where miners need to submit their proofs of sector liveness, correctness, and validity. Failure to do so will mark a sector as faulted, and will eventually terminate the sector, triggering slashing consequences for the miner.
* **syncing/fork selection:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fsync-forks)
    * Newly bootstrapped clients, and paused-then-resumed clients, are able to latch on to the correct chain even in the presence of a large number of forks in the network, either in the present, or throughout history.
* **present-time mining/tipset assembly:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fmining-present)
    * Induce forks in the network, create network partitions, simulate chain halts, long-range forks, etc. Stage many kinds of convoluted chain shapes, and network partitions, and ensure that miners are always able to arrive to consensus when disruptions subside.
* **catch-up/rush mining:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fmining-rush)
    * Induce network-wide, or partition-wide arrests, and investigate what the resulting chain is after the system is allowed to recover.
    * Context: catch-up/rush mining is a dedicated pathway in the mining logic that brings the chain up to speed with present time, in order to recover from network halts. Basically it entails producing backdated blocks in a hot loop. Imagine all miners recover in unison from a network-wide disruption; miners will produce blocks for their winning rounds, and will label losing rounds as _null rounds_. In the current implementation, there is no time for block propagation, so miners will produce solo-chains, and the assumption is that when all these chains hit the network, the _fork choice rule_ will pick the heaviest one. Unfortunately this process is brittle and unbalanced, as it favours the miner that held the highest power before the disruption commenced.
* **storage and retrieval deals:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fdeals)
    * end-to-end flows where clients store and retrieve pieces from miners, including stress testing the system.
* **payment channels:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fpaych)
    * stress testing payment channels via excessive lane creation, excessive payment voucher atomisation, and redemption.
* **drand incidents and impact on the filecoin network/protocol/chain:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fdrand)
    * drand total unavailabilities, drand catch-ups, drand slowness, etc.
* **mempool message selection:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fmempool)
    * soundness of message selection logic; potentially targeted attacks against miners by flooding their message pools with different kinds of messages.
* **presealing:** [_(view test scenarios)_](https://github.com/filecoin-project/oni/issues?q=is%3Aissue+sort%3Aupdated-desc+label%3Atopic%2Fpresealing)
    * TBD, anything related to this worth testing?

## Repository contents

This repository consists of [test plans](https://docs.testground.ai/concepts-and-architecture/test-structure) built to be run on [Testground](https://github.com/testground/testground).

The source code for the various test cases can be found in the [`lotus-soup` directory](https://github.com/filecoin-project/oni/tree/master/lotus-soup).

## Running the test cases

If you are unfamiliar with Testground, we strongly suggest you read the Testground [Getting Started guide](https://docs.testground.ai/getting-started) in order to learn how to install Testground and how to use it.

You can find various [composition files](https://docs.testground.ai/running-test-plans#composition-runs) describing various test scenarios built as part of Project Oni at [`lotus-soup/_compositions` directory](https://github.com/filecoin-project/oni/tree/master/lotus-soup/_compositions).

We've designed the test cases so that you can run them via the `local:exec`, `local:docker` and the `cluster:k8s` runners. Note that Lotus miners are quite resource intensive, requiring gigabytes of memory. Hence you would have to run these test cases on a beafy machine (when using `local:docker` and `local:exec`), or on a Kubernetes cluster (when using `cluster:k8s`).

Here are the basics of how to run the baseline deals end-to-end test case:

### Running the baseline deals end-to-end test case

1. Compile and Install Testground from source code from the [`oni`](https://github.com/testground/testground/pull/1083) branch.
    * See the [Getting Started](https://github.com/testground/testground#getting-started) section of the README for instructions.

2. Run a Testground daemon

```
testground daemon
```

3. Run a composition for the baseline deals end-to-end test case

```
testground run composition -f _compositions/composition.toml
```

## Team composition

* [@raulk](https://github.com/raulk) (Captain + TL)
* [@nonsense](https://github.com/nonsense) (Testground TG + engineer)
* [@yusefnapora](https://github.com/yusefnapora) (engineer and technical writer)
* [@vyzo](https://github.com/vyzo) (engineer)
* [@schomatis](https://github.com/schomatis) (advisor)
