# Benchmarking additional GPUs

If you want to test a GPU that is not explicitly supported, set the following *environment variable*:

```sh
BELLMAN_CUSTOM_GPU="<NAME>:<NUMBER_OF_CORES>"
```

Here is an example of trying a GeForce GTX 1660 Ti with 1536 cores.

```sh
BELLMAN_CUSTOM_GPU="GeForce GTX 1660 Ti:1536"
```

To get the number of cores for your GPU, you will need to check your card’s specifications.

To perform the benchmark you can use Lotus' [benchmarking tool](https://github.com/filecoin-project/lotus/tree/master/cmd/lotus-bench). Results and discussion are tracked in a [GitHub issue thread](https://github.com/filecoin-project/lotus/issues/694).
