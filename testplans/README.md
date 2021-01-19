# Testground testplans for Lotus

This directory consists of [testplans](https://docs.testground.ai/concepts-and-architecture/test-structure) built to be run on [Testground](https://github.com/testground/testground) that exercise Lotus on [TaaS](https://ci.testground.ipfs.team).

## Table of Contents

- [Testing topics](#testing-topics)
- [Running the test cases](#running-the-test-cases)

## Testing topics

* **storage and retrieval deals:**
    * end-to-end flows where clients store and retrieve pieces from miners, including stress testing the system.
* **payment channels:**
    * stress testing payment channels via excessive lane creation, excessive payment voucher atomisation, and redemption.

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

6. Run a composition for the baseline deals end-to-end test case

```
testground run composition -f ./lotus-soup/_compositions/baseline-docker-5-1.toml
```
