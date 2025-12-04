# lotus-worker

```
NAME:
   lotus-worker - Remote miner worker

USAGE:
   lotus-worker [global options] command [command options]

VERSION:
   1.34.4-dev

COMMANDS:
   run        Start lotus worker
   stop       Stop a running lotus worker
   info       Print worker info
   storage    manage sector storage
   resources  Manage resource table overrides
   tasks      Manage task processing
   help, h    Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --worker-repo value, --workerrepo value  Specify worker repo path. flag workerrepo and env WORKER_PATH are DEPRECATION, will REMOVE SOON (default: "~/.lotusworker") [$LOTUS_WORKER_PATH, $WORKER_PATH]
   --miner-repo value, --storagerepo value  Specify miner repo path. flag storagerepo and env LOTUS_STORAGE_PATH are DEPRECATION, will REMOVE SOON (default: "~/.lotusminer") [$LOTUS_MINER_PATH, $LOTUS_STORAGE_PATH]
   --enable-gpu-proving                     enable use of GPU for mining operations (default: true) [$LOTUS_WORKER_ENABLE_GPU_PROVING]
   --help, -h                               show help
   --version, -v                            print the version
```

## lotus-worker run

```
NAME:
   lotus-worker run - Start lotus worker

USAGE:
   lotus-worker run [command options]

DESCRIPTION:
   Run lotus-worker.

   --external-pc2 can be used to compute the PreCommit2 inputs externally.
   The flag behaves similarly to the related lotus-worker flag, using it in
   lotus-bench may be useful for testing if the external PreCommit2 command is
   invoked correctly.

   The command will be called with a number of environment variables set:
   * EXTSEAL_PC2_SECTOR_NUM: the sector number
   * EXTSEAL_PC2_SECTOR_MINER: the miner id
   * EXTSEAL_PC2_PROOF_TYPE: the proof type
   * EXTSEAL_PC2_SECTOR_SIZE: the sector size in bytes
   * EXTSEAL_PC2_CACHE: the path to the cache directory
   * EXTSEAL_PC2_SEALED: the path to the sealed sector file (initialized with unsealed data by the caller)
   * EXTSEAL_PC2_PC1OUT: output from rust-fil-proofs precommit1 phase (base64 encoded json)

   The command is expected to:
   * Create cache sc-02-data-tree-r* files
   * Create cache sc-02-data-tree-c* files
   * Create cache p_aux / t_aux files
   * Transform the sealed file in place

   Example invocation of lotus-bench as external executor:
   './lotus-bench simple precommit2 --sector-size $EXTSEAL_PC2_SECTOR_SIZE $EXTSEAL_PC2_SEALED $EXTSEAL_PC2_CACHE $EXTSEAL_PC2_PC1OUT'


OPTIONS:
   --listen value                host address and port the worker api will listen on (default: "0.0.0.0:3456") [$LOTUS_WORKER_LISTEN]
   --no-local-storage            don't use storageminer repo for sector storage (default: false) [$LOTUS_WORKER_NO_LOCAL_STORAGE]
   --no-swap                     don't use swap (default: false) [$LOTUS_WORKER_NO_SWAP]
   --name value                  custom worker name (default: hostname) [$LOTUS_WORKER_NAME]
   --addpiece                    enable addpiece (default: true) [$LOTUS_WORKER_ADDPIECE]
   --precommit1                  enable precommit1 (default: true) [$LOTUS_WORKER_PRECOMMIT1]
   --unseal                      enable unsealing (default: true) [$LOTUS_WORKER_UNSEAL]
   --precommit2                  enable precommit2 (default: true) [$LOTUS_WORKER_PRECOMMIT2]
   --commit                      enable commit (default: true) [$LOTUS_WORKER_COMMIT]
   --replica-update              enable replica update (default: true) [$LOTUS_WORKER_REPLICA_UPDATE]
   --prove-replica-update2       enable prove replica update 2 (default: true) [$LOTUS_WORKER_PROVE_REPLICA_UPDATE2]
   --regen-sector-key            enable regen sector key (default: true) [$LOTUS_WORKER_REGEN_SECTOR_KEY]
   --sector-download             enable external sector data download (default: false) [$LOTUS_WORKER_SECTOR_DOWNLOAD]
   --windowpost                  enable window post (default: false) [$LOTUS_WORKER_WINDOWPOST]
   --winningpost                 enable winning post (default: false) [$LOTUS_WORKER_WINNINGPOST]
   --no-default                  disable all default compute tasks, use the worker for storage/fetching only (default: false) [$LOTUS_WORKER_NO_DEFAULT]
   --parallel-fetch-limit value  maximum fetch operations to run in parallel (default: 5) [$LOTUS_WORKER_PARALLEL_FETCH_LIMIT]
   --post-parallel-reads value   maximum number of parallel challenge reads (0 = no limit) (default: 32) [$LOTUS_WORKER_POST_PARALLEL_READS]
   --post-read-timeout value     time limit for reading PoSt challenges (0 = no limit) (default: 0s) [$LOTUS_WORKER_POST_READ_TIMEOUT]
   --timeout value               used when 'listen' is unspecified. must be a valid duration recognized by golang's time.ParseDuration function (default: "30m") [$LOTUS_WORKER_TIMEOUT]
   --http-server-timeout value   (default: "30s")
   --data-cid                    Run the data-cid task. true|false (default: inherits --addpiece)
   --external-pc2 value          command for computing PC2 externally
   --help, -h                    show help
```

## lotus-worker stop

```
NAME:
   lotus-worker stop - Stop a running lotus worker

USAGE:
   lotus-worker stop [command options]

OPTIONS:
   --help, -h  show help
```

## lotus-worker info

```
NAME:
   lotus-worker info - Print worker info

USAGE:
   lotus-worker info [command options]

OPTIONS:
   --help, -h  show help
```

## lotus-worker storage

```
NAME:
   lotus-worker storage - manage sector storage

USAGE:
   lotus-worker storage [command options]

COMMANDS:
   attach     attach local storage path
   detach     detach local storage path
   redeclare  redeclare sectors in a local storage path
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-worker storage attach

```
NAME:
   lotus-worker storage attach - attach local storage path

USAGE:
   lotus-worker storage attach [command options]

OPTIONS:
   --init                                 initialize the path first (default: false)
   --weight value                         (for init) path weight (default: 10)
   --seal                                 (for init) use path for sealing (default: false)
   --store                                (for init) use path for long-term storage (default: false)
   --max-storage value                    (for init) limit storage space for sectors (expensive for very large paths!)
   --groups value [ --groups value ]      path group names
   --allow-to value [ --allow-to value ]  path groups allowed to pull data from this path (allow all if not specified)
   --help, -h                             show help
```

### lotus-worker storage detach

```
NAME:
   lotus-worker storage detach - detach local storage path

USAGE:
   lotus-worker storage detach [command options] [path]

OPTIONS:
   --really-do-it  (default: false)
   --help, -h      show help
```

### lotus-worker storage redeclare

```
NAME:
   lotus-worker storage redeclare - redeclare sectors in a local storage path

USAGE:
   lotus-worker storage redeclare [command options]

OPTIONS:
   --id value      storage path ID
   --all           redeclare all storage paths (default: false)
   --drop-missing  Drop index entries with missing files (default: true)
   --help, -h      show help
```

## lotus-worker resources

```
NAME:
   lotus-worker resources - Manage resource table overrides

USAGE:
   lotus-worker resources [command options]

OPTIONS:
   --all       print all resource envvars (default: false)
   --default   print default resource envvars (default: false)
   --help, -h  show help
```

## lotus-worker tasks

```
NAME:
   lotus-worker tasks - Manage task processing

USAGE:
   lotus-worker tasks [command options]

COMMANDS:
   enable   Enable a task type
   disable  Disable a task type
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-worker tasks enable

```
NAME:
   lotus-worker tasks enable - Enable a task type

USAGE:
   lotus-worker tasks enable [command options] --all | [UNS|C2|PC2|PC1|PR2|RU|AP|DC|GSK]

OPTIONS:
   --all       Enable all task types (default: false)
   --help, -h  show help
```

### lotus-worker tasks disable

```
NAME:
   lotus-worker tasks disable - Disable a task type

USAGE:
   lotus-worker tasks disable [command options] --all | [UNS|C2|PC2|PC1|PR2|RU|AP|DC|GSK]

OPTIONS:
   --all       Disable all task types (default: false)
   --help, -h  show help
```
