## Gas Balancing

The gas balancing process targets to set gas costs of syscalls to be in line with
10 gas per nanosecond on reference hardware.
The process can be either performed for all syscalls based on existing messages and chains or targeted
at a single syscall.

#### Reference hardware

The reference hardware is TR3970x with 128GB of RAM. This is what was available at the time and
may be subject to change.

### Complete gas balancing

Complete gas balancing is performed using a `lotus-bench` the process is based on importing a chain export
and collecting gas traces which are later aggregated.

Before building `lotus-bench` make sure `EnableDetailedTracing` in `chain/vm/runtime.go` is set to `true`.

The process can be started using `./lotus-bench import` with `--car` flag set to the location of
CAR chain export. `--start-epoch` and `--end-epoch` can be used to limit the range of epochs to run
the benchmark. Note that the state tree of `start-epoch` needs to be in the CAR file or has to be previously computed
to work.

The output will be a `bench.json` file containing information about every syscall invoked
and the time taken by these invocations. This file can grow to be quite big in size so make sure you have
spare space.

After the bench run is complete the `bench.json` file can be analyzed with `./lotus-bench import analyze bench.json`.

It will compute means, standard deviations and co-variances (when applicable) of syscall runtimes. 
The output is in nanoseconds, so the gas values for syscalls should be 10x that. In cases where the co-variance of
execution time to some parameter is evaluated, the strength of the correlation should be taken into account.

#### Special cases

OnImplPut compute gas is based on the flush time to disk of objects created,
during block execution (when gas traces are formed) objects are only written to memory. Use `vm/flush_copy_ms` and  `vm/flush_copy_count` to estimate OnIpldPut compute cost.


### Targeted gas balancing

In some cases complete gas balancing is infeasible, either a new syscall gets introduced or
complete balancing is too time consuming.

In these cases, the recommended way to estimate gas for a given syscall is to perform an `in-vivo` benchmark.
In the past `in-vitro` as in standalone benchmarks were found to be highly inaccurate when compared to results
of real execution.

An in-vivo benchmark can be performed by running an example of such a syscall during block execution.
The best place to hook-in such a benchmark is the message execution loop in
`chain/stmgr/stmgr.go` in `ApplyBlocks()`. Depending on the time required to complete the syscall it might be
advisable to run the execution only once every few messages.

