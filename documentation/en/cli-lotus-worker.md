# lotus-worker
```
NAME:
   lotus-worker - Remote miner worker

USAGE:
   lotus-worker [global options] command [command options] [arguments...]

VERSION:
   1.13.2-dev

COMMANDS:
   run         Start lotus worker
   info        Print worker info
   storage     manage sector storage
   set         Manage worker settings
   wait-quiet  Block until all running tasks exit
   tasks       Manage task processing
   help, h     Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --worker-repo value, --workerrepo value  Specify worker repo path. flag workerrepo and env WORKER_PATH are DEPRECATION, will REMOVE SOON (default: "~/.lotusworker") [$LOTUS_WORKER_PATH, $WORKER_PATH]
   --miner-repo value, --storagerepo value  Specify miner repo path. flag storagerepo and env LOTUS_STORAGE_PATH are DEPRECATION, will REMOVE SOON (default: "~/.lotusminer") [$LOTUS_MINER_PATH, $LOTUS_STORAGE_PATH]
   --enable-gpu-proving                     enable use of GPU for mining operations (default: true)
   --help, -h                               show help (default: false)
   --version, -v                            print the version (default: false)
```

## lotus-worker run
```
NAME:
   lotus-worker run - Start lotus worker

USAGE:
   lotus-worker run [command options] [arguments...]

OPTIONS:
   --listen value                host address and port the worker api will listen on (default: "0.0.0.0:3456")
   --no-local-storage            don't use storageminer repo for sector storage (default: false)
   --no-swap                     don't use swap (default: false)
   --addpiece                    enable addpiece (default: true)
   --precommit1                  enable precommit1 (32G sectors: 1 core, 128GiB Memory) (default: true)
   --unseal                      enable unsealing (32G sectors: 1 core, 128GiB Memory) (default: true)
   --precommit2                  enable precommit2 (32G sectors: all cores, 96GiB Memory) (default: true)
   --commit                      enable commit (32G sectors: all cores or GPUs, 128GiB Memory + 64GiB swap) (default: true)
   --parallel-fetch-limit value  maximum fetch operations to run in parallel (default: 5)
   --timeout value               used when 'listen' is unspecified. must be a valid duration recognized by golang's time.ParseDuration function (default: "30m")
   --help, -h                    show help (default: false)
   
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
   attach   attach local storage path
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-worker storage attach
```
NAME:
   lotus-worker storage attach - attach local storage path

USAGE:
   lotus-worker storage attach [command options] [arguments...]

OPTIONS:
   --init               initialize the path first (default: false)
   --weight value       (for init) path weight (default: 10)
   --seal               (for init) use path for sealing (default: false)
   --store              (for init) use path for long-term storage (default: false)
   --max-storage value  (for init) limit storage space for sectors (expensive for very large paths!)
   --help, -h           show help (default: false)
   
```

## lotus-worker set
```
NAME:
   lotus-worker set - Manage worker settings

USAGE:
   lotus-worker set [command options] [arguments...]

OPTIONS:
   --enabled   enable/disable new task processing (default: true)
   --help, -h  show help (default: false)
   
```

## lotus-worker wait-quiet
```
NAME:
   lotus-worker wait-quiet - Block until all running tasks exit

USAGE:
   lotus-worker wait-quiet [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
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
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-worker tasks enable
```
NAME:
   lotus-worker tasks enable - Enable a task type

USAGE:
   lotus-worker tasks enable [command options] [UNS|C2|PC2|PC1|AP]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-worker tasks disable
```
NAME:
   lotus-worker tasks disable - Disable a task type

USAGE:
   lotus-worker tasks disable [command options] [UNS|C2|PC2|PC1|AP]

OPTIONS:
   --help, -h  show help (default: false)
   
```
