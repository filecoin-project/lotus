# lotus-worker
```
NAME:
   lotus-worker - Remote miner worker

USAGE:
   lotus-worker [global options] command [command options] [arguments...]

VERSION:
   1.21.0-dev

COMMANDS:
   run        Start lotus worker
   stop       Stop a running lotus worker
   info       Print worker info
   storage    manage sector storage
   resources  Manage resource table overrides
   tasks      Manage task processing
   help, h    Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --enable-gpu-proving                     enable use of GPU for mining operations (default: true) [$LOTUS_WORKER_ENABLE_GPU_PROVING]
   --help, -h                               show help (default: false)
   --miner-repo value, --storagerepo value  Specify miner repo path. flag storagerepo and env LOTUS_STORAGE_PATH are DEPRECATION, will REMOVE SOON (default: "~/.lotusminer") [$LOTUS_MINER_PATH, $LOTUS_STORAGE_PATH]
   --version, -v                            print the version (default: false)
   --worker-repo value, --workerrepo value  Specify worker repo path. flag workerrepo and env WORKER_PATH are DEPRECATION, will REMOVE SOON (default: "~/.lotusworker") [$LOTUS_WORKER_PATH, $WORKER_PATH]
   
```

## lotus-worker run
```
NAME:
   lotus-worker run - Start lotus worker

USAGE:
   lotus-worker run [command options] [arguments...]

OPTIONS:
   --addpiece                    enable addpiece (default: true) [$LOTUS_WORKER_ADDPIECE]
   --commit                      enable commit (default: true) [$LOTUS_WORKER_COMMIT]
   --http-server-timeout value   (default: "30s")
   --listen value                host address and port the worker api will listen on (default: "0.0.0.0:3456") [$LOTUS_WORKER_LISTEN]
   --name value                  custom worker name (default: hostname) [$LOTUS_WORKER_NAME]
   --no-default                  disable all default compute tasks, use the worker for storage/fetching only (default: false) [$LOTUS_WORKER_NO_DEFAULT]
   --no-local-storage            don't use storageminer repo for sector storage (default: false) [$LOTUS_WORKER_NO_LOCAL_STORAGE]
   --no-swap                     don't use swap (default: false) [$LOTUS_WORKER_NO_SWAP]
   --parallel-fetch-limit value  maximum fetch operations to run in parallel (default: 5) [$LOTUS_WORKER_PARALLEL_FETCH_LIMIT]
   --post-parallel-reads value   maximum number of parallel challenge reads (0 = no limit) (default: 32) [$LOTUS_WORKER_POST_PARALLEL_READS]
   --post-read-timeout value     time limit for reading PoSt challenges (0 = no limit) (default: 0s) [$LOTUS_WORKER_POST_READ_TIMEOUT]
   --precommit1                  enable precommit1 (default: true) [$LOTUS_WORKER_PRECOMMIT1]
   --precommit2                  enable precommit2 (default: true) [$LOTUS_WORKER_PRECOMMIT2]
   --prove-replica-update2       enable prove replica update 2 (default: true) [$LOTUS_WORKER_PROVE_REPLICA_UPDATE2]
   --regen-sector-key            enable regen sector key (default: true) [$LOTUS_WORKER_REGEN_SECTOR_KEY]
   --replica-update              enable replica update (default: true) [$LOTUS_WORKER_REPLICA_UPDATE]
   --sector-download             enable external sector data download (default: false) [$LOTUS_WORKER_SECTOR_DOWNLOAD]
   --timeout value               used when 'listen' is unspecified. must be a valid duration recognized by golang's time.ParseDuration function (default: "30m") [$LOTUS_WORKER_TIMEOUT]
   --unseal                      enable unsealing (default: true) [$LOTUS_WORKER_UNSEAL]
   --windowpost                  enable window post (default: false) [$LOTUS_WORKER_WINDOWPOST]
   --winningpost                 enable winning post (default: false) [$LOTUS_WORKER_WINNINGPOST]
   
```

## lotus-worker stop
```
NAME:
   lotus-worker stop - Stop a running lotus worker

USAGE:
   lotus-worker stop [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-worker info
```
NAME:
   lotus-worker info - Print worker info

USAGE:
   lotus-worker info [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-worker storage
```
NAME:
   lotus-worker storage - manage sector storage

USAGE:
   lotus-worker storage command [command options] [arguments...]

COMMANDS:
     attach     attach local storage path
     detach     detach local storage path
     redeclare  redeclare sectors in a local storage path
     help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-worker storage attach
```
NAME:
   lotus-worker storage attach - attach local storage path

USAGE:
   lotus-worker storage attach [command options] [arguments...]

OPTIONS:
   --allow-to value [ --allow-to value ]  path groups allowed to pull data from this path (allow all if not specified)
   --groups value [ --groups value ]      path group names
   --init                                 initialize the path first (default: false)
   --max-storage value                    (for init) limit storage space for sectors (expensive for very large paths!)
   --seal                                 (for init) use path for sealing (default: false)
   --store                                (for init) use path for long-term storage (default: false)
   --weight value                         (for init) path weight (default: 10)
   
```

### lotus-worker storage detach
```
NAME:
   lotus-worker storage detach - detach local storage path

USAGE:
   lotus-worker storage detach [command options] [path]

OPTIONS:
   --really-do-it  (default: false)
   
```

### lotus-worker storage redeclare
```
NAME:
   lotus-worker storage redeclare - redeclare sectors in a local storage path

USAGE:
   lotus-worker storage redeclare [command options] [arguments...]

OPTIONS:
   --all           redeclare all storage paths (default: false)
   --drop-missing  Drop index entries with missing files (default: false)
   --id value      storage path ID
   
```

## lotus-worker resources
```
NAME:
   lotus-worker resources - Manage resource table overrides

USAGE:
   lotus-worker resources [command options] [arguments...]

OPTIONS:
   --all      print all resource envvars (default: false)
   --default  print default resource envvars (default: false)
   
```

## lotus-worker tasks
```
NAME:
   lotus-worker tasks - Manage task processing

USAGE:
   lotus-worker tasks command [command options] [arguments...]

COMMANDS:
     enable   Enable a task type
     disable  Disable a task type
     help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-worker tasks enable
```
NAME:
   lotus-worker tasks enable - Enable a task type

USAGE:
   lotus-worker tasks enable [command options] --all | [UNS|C2|PC2|PC1|PR2|RU|AP|DC|GSK]

OPTIONS:
   --all  Enable all task types (default: false)
   
```

### lotus-worker tasks disable
```
NAME:
   lotus-worker tasks disable - Disable a task type

USAGE:
   lotus-worker tasks disable [command options] --all | [UNS|C2|PC2|PC1|PR2|RU|AP|DC|GSK]

OPTIONS:
   --all  Disable all task types (default: false)
   
```
