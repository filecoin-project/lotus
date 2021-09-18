# build bench
```shell
cd lotus
make lotus-bench
```

## Instructions For Use

### bench
```shell
# sealing
./lotus-bench sealing --sector-size=2KiB --storage-dir=$LOTUS_BENCH_PATH --save-commit2-input c2in.json
# prove
./lotus-bench prove c2in.json 
```

### section-bench
```shell
# addpiece
./lotus-bench addpiece --sector-size=2KiB --storage-dir=$LOTUS_BENCH_PATH
# precommit1
./lotus-bench precommit1 --storage-dir=$LOTUS_BENCH_PATH
# precommit2
./lotus-bench precommit2 --storage-dir=$LOTUS_BENCH_PATH
# commit1
./lotus-bench commit1 --storage-dir=$LOTUS_BENCH_PATH
# commit2
./lotus-bench commit2 c2in.json
```

### sector-recovery
```shell
# recovery
./lotus-bench recovery --sector-size=2KiB --miner-id=t01000 --sector-id=$SectorID --ticket=<Ticket> --storage-dir=$LOTUS_BENCH_PATH
```
