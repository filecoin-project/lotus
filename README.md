# Project Oni 👹

Our mandate is:

> To verify the successful end-to-end outcome of the filecoin protocol and filecoin implementations, under a variety of real-world and simulated scenarios. 

➡️  Find out more about our goals, requirements, execution plan, and team culture, in our [Project Description](https://docs.google.com/document/d/16jYL--EWYpJhxT9bakYq7ZBGLQ9SB940Wd1lTDOAbNE).

## Table of Contents

- [Testing topics](#testing-topics)
- [Repository contents](#repository-contents)
- [Running the test cases](#running-the-test-cases)
- [Catalog](#catalog)
- [Debugging](#debugging)
- [Dependencies](#dependencies)
- [Docker images changelog](#docker-images-changelog)
- [Team](#team)

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

1. Compile and Install Testground from source code.
    * See the [Getting Started](https://github.com/testground/testground#getting-started) section of the README for instructions.

2. Run a Testground daemon

```
testground daemon
```

3. Download required Docker images for the `lotus-soup` test plan

```
make pull-images
```

Alternatively you can build them locally with

```
make build-images
```

4. Import the `lotus-soup` test plan into your Testground home directory

```
testground plan import --from ./lotus-soup
```

5. Init the `filecoin-ffi` Git submodule in the `extra` folder.

```
git submodule update --init --recursive
```

6. Compile the `filecoin-ffi` version locally (necessary if you use `local:exec`)

```
cd extra/filecoin-ffi
make
```

7. Run a composition for the baseline deals end-to-end test case

```
testground run composition -f ./lotus-soup/_compositions/baseline-docker-5-1.toml
```

## Batch-running randomised test cases

The Oni testkit supports [range parameters](https://github.com/filecoin-project/oni/blob/master/lotus-soup/testkit/testenv_ranges.go),
which test cases can use to generate random values, either at the instance level
(each instance computes a random value within range), or at the run level (one
instance computes the values, and propagates them to all other instances via the
sync service).

For example:

```toml
latency_range   = '["20ms", "500ms"]'
loss_range      = '[0, 0.2]'
```

Could pick a random latency between 20ms and 500ms, and a packet loss
probability between 0 and 0.2. We could apply those values through the
`netclient.ConfigureNetwork` Testground SDK API.

Randomized range-based parameters are specially interesting when combined with
batch runs, as it enables Monte Carlo approaches to testing.

The Oni codebase includes a batch test run driver in package `lotus-soup/runner`.
You can point it at a composition file that uses range parameters and tell it to
run N iterations of the test:

```shell script
$ go run ./runner -runs 5 _compositions/net-chaos/latency.toml
```

This will run the test as many times as instructed, and will place all outputs
in a temporary directory. You can pass a concrete output directory with
the `-output` flag. 

## Catalog

### Test cases part of `lotus-soup`

* `deals-e2e` - Deals end-to-end test case. Clients pick a miner at random, start a deal, wait for it to be sealed, and try to retrieve from another random miner who offers back the data.
* `drand-halting` - Test case that instructs Drand with a sequence of halt/resume/wait events, while running deals between clients and miners at the same time.
* `deals-stress` - Deals stress test case. Clients pick a miner and send multiple deals (concurrently or serially) in order to test how many deals miners can handle.
* `paych-stress` - A test case exercising various payment channel stress tests.

### Compositions part of `lotus-soup`

* `baseline-docker-5-1.toml` - Runs a `baseline` test (deals e2e test) with a network of 5 clients and 1 miner targeting `local:docker`
* `baseline-k8s-10-3.toml` - Runs a `baseline` test (deals e2e test) with a network of 10 clients and 3 miner targeting `cluster:k8s`
* `baseline-k8s-3-1.toml` - Runs a `baseline` test (deals e2e test) with a network of 3 clients and 1 miner targeting `cluster:k8s`
* `baseline-k8s-3-2.toml` - Runs a `baseline` test (deals e2e test) with a network of 3 clients and 2 miner targeting `cluster:k8s`
* `baseline.toml` - Runs a `baseline` test (deals e2e test) with a network of 3 clients and 2 miner targeting `local:exec`. You have to manually download the proof parameters and place them in `/var/tmp`.
* `deals-stress-concurrent-natural-k8s.toml`
* `deals-stress-concurrent-natural.toml`
* `deals-stress-concurrent.toml`
* `deals-stress-serial-natural.toml`
* `deals-stress-serial.toml`
* `drand-halt.toml`
* `local-drand.toml`
* `natural.toml`
* `paych-stress.toml`
* `pubsub-tracer.toml`


## Debugging

Find commands and how-to guides on debugging test plans at [DELVING.md](https://github.com/filecoin-project/oni/blob/master/DELVING.md)

1. Querying the Lotus RPC API

2. Useful commands / checks

* Making sure miners are on the same chain

* Checking deals

* Sector queries

* Sector sealing errors

## Dependencies

Our current test plan `lotus-soup` is building programatically the Lotus filecoin implementation and therefore requires all it's dependencies. The build process is slightly more complicated than a normal Go project, because we are binding a bit of Rust code. Lotus codebase is in Go, however its `proofs` and `crypto` libraries are in Rust (BLS signatures, SNARK verification, etc.).

Depending on the runner you want to use to run the test plan, these dependencies are included in the build process in a different way, which you should be aware of should you require to use the test plan with a newer version of Lotus:

### Filecoin FFI libraries

* `local:docker`

The Rust libraries are included in the Filecoin FFI Git submodule, which is part of the `iptestground/oni-buildbase` image. If the FFI changes on Lotus, we have to rebuild this image with the `make build-images` command, where X is the next version (see [Docker images changelog](#docker-images-changelog)
below).

* `local:exec`

The Rust libraries are included via the `extra` directory. Make sure that the test plan reference to Lotus in `go.mod` and the `extra` directory are pointing to the same commit of the FFI git submodule. You also need to compile the `extra/filecoin-ffi` libraries with `make`.

* `cluster:k8s`

The same process as for `local:docker`, however you need to make sure that the respective `iptestground/oni-buildbase` image is available as a public Docker image, so that the Kubernetes cluster can download it.

### proof parameters

Additional to the Filecoin FFI Git submodules, we are also bundling `proof parameters` in the `iptestground/oni-runtime` image. If these change, you will need to rebuild that image with `make build-images` command, where X is the next version.

## Docker images changelog

### oni-buildbase

* `v1` => initial image locking in Filecoin FFI commit ca281af0b6c00314382a75ae869e5cb22c83655b.
* `v2` => no changes; released only for aligning both images to aesthetically please @nonsense :D
* `v3` => locking in Filecoin FFI commit 5342c7c97d1a1df4650629d14f2823d52889edd9.
* `v4` => locking in Filecoin FFI commit 6a143e06f923f3a4f544c7a652e8b4df420a3d28.
* `v5` => locking in Filecoin FFI commit cddc56607e1d851ea6d09d49404bd7db70cb3c2e.
* `v6` => locking in Filecoin FFI commit 40569104603407c999d6c9e4c3f1228cbd4d0e5c.
* `v7` => add Filecoin-BLST repo to buildbase.
* `v8` => locking in Filecoin FFI commit f640612a1a1f7a2d.
* `v9` => locking in Filecoin FFI commit 57e38efe4943f09d3127dcf6f0edd614e6acf68e and Filecoin-BLST commit 8609119cf4595d1741139c24378fcd8bc4f1c475.


### oni-runtime

* `v1` => initial image with 2048 parameters.
* `v2` => adds auxiliary tools: `net-tools netcat traceroute iputils-ping wget vim curl telnet iproute2 dnsutils`.
* `v3` => bump proof parameters from v27 to v28

### oni-runtime-debug

* `v1` => initial image
* `v2` => locking in Lotus commit e21ea53
* `v3` => locking in Lotus commit d557c40
* `v4` => bump proof parameters from v27 to v28
* `v5` => locking in Lotus commit 1a170e18a


## Team

* [@raulk](https://github.com/raulk) (Captain + TL)
* [@nonsense](https://github.com/nonsense) (Testground TG + engineer)
* [@yusefnapora](https://github.com/yusefnapora) (engineer and technical writer)
* [@vyzo](https://github.com/vyzo) (engineer)
* [@schomatis](https://github.com/schomatis) (advisor)
* [@willscott](https://github.com/willscott) (engineer)
* [@alanshaw](https://github.com/alanshaw) (engineer)

