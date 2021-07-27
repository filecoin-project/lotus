### Environment variables

Ensure that workers have access to the following environment variables when they run.

```
export TMPDIR=/fast/disk/folder3                    # used when sealing
export MINER_API_INFO:<TOKEN>:/ip4/<miner_api_address>/tcp/<port>/http`
export BELLMAN_CPU_UTILIZATION=0.875      # optimal value depends on exact hardware
export FIL_PROOFS_MAXIMIZE_CACHING=1
export FIL_PROOFS_USE_GPU_COLUMN_BUILDER=1 # when GPU is available
export FIL_PROOFS_USE_GPU_TREE_BUILDER=1   # when GPU is available
export FIL_PROOFS_PARAMETER_CACHE=/fast/disk/folder # > 100GiB!
export FIL_PROOFS_PARENT_CACHE=/fast/disk/folder2   # > 50GiB!

# The following increases speed of PreCommit1 at the cost of using a full
# CPU core-complex rather than a single core.
# See https://github.com/filecoin-project/rust-fil-proofs/ and the
# "Worker co-location" section below.
export FIL_PROOFS_USE_MULTICORE_SDR=1
```

### Run the worker

```
lotus-worker run <flags>
```

These are old flags:

```
--addpiece             enable addpiece (default: true)
--precommit1           enable precommit1 (32G sectors: 1 core, 128GiB RAM) (default: true)
--unseal               enable unsealing (32G sectors: 1 core, 128GiB RAM) (default: true)
--precommit2           enable precommit2 (32G sectors: multiple cores, 96GiB RAM) (default: true)
--commit               enable commit (32G sectors: multiple cores or GPUs, 128GiB RAM + 64GiB swap) (default: true)
```

We added two new flags:

```
--windowpost           enable windowpost (default: false)
--winnningpost         enable winningpost (default: false)
```

These post flags have priority over old flags. If you want this worker to be a window post machine, you can just set the windowpost flag to be `true`. Similar to winning post machine. If you set both of them to be `true`, it will be a window post machine.

