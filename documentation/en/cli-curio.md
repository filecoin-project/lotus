# curio
```
NAME:
   curio - Filecoin decentralized storage network provider

USAGE:
   curio [global options] command [command options] [arguments...]

VERSION:
   1.27.0-dev

COMMANDS:
   cli           Execute cli commands
   run           Start a Curio process
   stop          Stop a running Curio process
   config        Manage node config by layers. The layer 'base' will always be applied at Curio start-up.
   test          Utility functions for testing
   web           Start Curio web interface
   guided-setup  Run the guided setup for migrating from lotus-miner to Curio or Creating a new Curio miner
   from-miner    Express a database config (for curio) from an existing miner.
   seal          Manage the sealing pipeline
   auth          Manage RPC permissions
   log           Manage logging
   wait-api      Wait for lotus api to come online
   fetch-params  Fetch proving parameters
   version       Print version
   help, h       Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --color              use color in display output (default: depends on output being a TTY)
   --db-host value      Command separated list of hostnames for yugabyte cluster (default: "yugabyte") [$CURIO_DB_HOST, $CURIO_HARMONYDB_HOSTS]
   --db-name value      (default: "yugabyte") [$CURIO_DB_NAME, $CURIO_HARMONYDB_NAME]
   --db-user value      (default: "yugabyte") [$CURIO_DB_USER, $CURIO_HARMONYDB_USERNAME]
   --db-password value  (default: "yugabyte") [$CURIO_DB_PASSWORD, $CURIO_HARMONYDB_PASSWORD]
   --repo-path value    (default: "~/.curio") [$CURIO_REPO_PATH]
   --vv                 enables very verbose mode, useful for debugging the CLI (default: false)
   --help, -h           show help
   --version, -v        print the version
```

## curio cli
```
NAME:
   curio cli - Execute cli commands

USAGE:
   curio cli command [command options] [arguments...]

COMMANDS:
   storage  manage sector storage
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --machine value  machine host:port (lotus-provider run --listen address)
   --help, -h       show help
```

### curio cli storage
```
```

## curio run
```
NAME:
   curio run - Start a Curio process

USAGE:
   curio run [command options] [arguments...]

OPTIONS:
   --listen value                     host address and port the worker api will listen on (default: "0.0.0.0:12300") [$LOTUS_WORKER_LISTEN]
   --nosync                           don't check full-node sync status (default: false)
   --manage-fdlimit                   manage open file limit (default: true)
   --storage-json value               path to json file containing storage config (default: "~/.curio/storage.json")
   --journal value                    path to journal files (default: "~/.curio/")
   --layers value [ --layers value ]  list of layers to be interpreted (atop defaults). Default: base
   --help, -h                         show help
```

## curio stop
```
NAME:
   curio stop - Stop a running Curio process

USAGE:
   curio stop [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

## curio config
```
NAME:
   curio config - Manage node config by layers. The layer 'base' will always be applied at Curio start-up.

USAGE:
   curio config command [command options] [arguments...]

COMMANDS:
   default, defaults                Print default node config
   set, add, update, create         Set a config layer or the base by providing a filename or stdin.
   get, cat, show                   Get a config layer by name. You may want to pipe the output to a file, or use 'less'
   list, ls                         List config layers present in the DB.
   interpret, view, stacked, stack  Interpret stacked config layers by this version of curio, with system-generated comments.
   remove, rm, del, delete          Remove a named config layer.
   edit                             edit a config layer
   from-miner                       Express a database config (for curio) from an existing miner.
   new-cluster                      Create new configuration for a new cluster
   help, h                          Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### curio config default
```
NAME:
   curio config default - Print default node config

USAGE:
   curio config default [command options] [arguments...]

OPTIONS:
   --no-comment  don't comment default values (default: false)
   --help, -h    show help
```

### curio config set
```
NAME:
   curio config set - Set a config layer or the base by providing a filename or stdin.

USAGE:
   curio config set [command options] a layer's file name

OPTIONS:
   --title value  title of the config layer (req'd for stdin)
   --help, -h     show help
```

### curio config get
```
NAME:
   curio config get - Get a config layer by name. You may want to pipe the output to a file, or use 'less'

USAGE:
   curio config get [command options] layer name

OPTIONS:
   --help, -h  show help
```

### curio config list
```
NAME:
   curio config list - List config layers present in the DB.

USAGE:
   curio config list [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

### curio config interpret
```
NAME:
   curio config interpret - Interpret stacked config layers by this version of curio, with system-generated comments.

USAGE:
   curio config interpret [command options] a list of layers to be interpreted as the final config

OPTIONS:
   --layers value [ --layers value ]  comma or space separated list of layers to be interpreted (base is always applied)
   --help, -h                         show help
```

### curio config remove
```
NAME:
   curio config remove - Remove a named config layer.

USAGE:
   curio config remove [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

### curio config edit
```
NAME:
   curio config edit - edit a config layer

USAGE:
   curio config edit [command options] [layer name]

OPTIONS:
   --editor value         editor to use (default: "vim") [$EDITOR]
   --source value         source config layer (default: <edited layer>)
   --allow-overwrite      allow overwrite of existing layer if source is a different layer (default: false)
   --no-source-diff       save the whole config into the layer, not just the diff (default: false)
   --no-interpret-source  do not interpret source layer (default: true if --source is set)
   --help, -h             show help
```

### curio config from-miner
```
NAME:
   curio from-miner - Express a database config (for curio) from an existing miner.

USAGE:
   curio from-miner [command options] [arguments...]

DESCRIPTION:
   Express a database config (for curio) from an existing miner.

OPTIONS:
   --miner-repo value, --storagerepo value  Specify miner repo path. flag(storagerepo) and env(LOTUS_STORAGE_PATH) are DEPRECATION, will REMOVE SOON (default: "~/.lotusminer") [$LOTUS_MINER_PATH, $LOTUS_STORAGE_PATH]
   --to-layer value, -t value               The layer name for this data push. 'base' is recommended for single-miner setup.
   --overwrite, -o                          Use this with --to-layer to replace an existing layer (default: false)
   --help, -h                               show help
```

### curio config new-cluster
```
NAME:
   curio config new-cluster - Create new configuration for a new cluster

USAGE:
   curio config new-cluster [command options] [SP actor address...]

OPTIONS:
   --help, -h  show help
```

## curio test
```
NAME:
   curio test - Utility functions for testing

USAGE:
   curio test command [command options] [arguments...]

COMMANDS:
   window-post, wd, windowpost, wdpost  Compute a proof-of-spacetime for a sector (requires the sector to be pre-sealed). These will not send to the chain.
   help, h                              Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### curio test window-post
```
NAME:
   curio test window-post - Compute a proof-of-spacetime for a sector (requires the sector to be pre-sealed). These will not send to the chain.

USAGE:
   curio test window-post command [command options] [arguments...]

COMMANDS:
   here, cli                                       Compute WindowPoSt for performance and configuration testing.
   task, scheduled, schedule, async, asynchronous  Test the windowpost scheduler by running it on the next available curio. 
   help, h                                         Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

#### curio test window-post here
```
NAME:
   curio test window-post here - Compute WindowPoSt for performance and configuration testing.

USAGE:
   curio test window-post here [command options] [deadline index]

DESCRIPTION:
   Note: This command is intended to be used to verify PoSt compute performance.
   It will not send any messages to the chain. Since it can compute any deadline, output may be incorrectly timed for the chain.

OPTIONS:
   --deadline value                   deadline to compute WindowPoSt for  (default: 0)
   --layers value [ --layers value ]  list of layers to be interpreted (atop defaults). Default: base
   --storage-json value               path to json file containing storage config (default: "~/.curio/storage.json")
   --partition value                  partition to compute WindowPoSt for (default: 0)
   --help, -h                         show help
```

#### curio test window-post task
```
NAME:
   curio test window-post task - Test the windowpost scheduler by running it on the next available curio. 

USAGE:
   curio test window-post task [command options] [arguments...]

OPTIONS:
   --deadline value                   deadline to compute WindowPoSt for  (default: 0)
   --layers value [ --layers value ]  list of layers to be interpreted (atop defaults). Default: base
   --help, -h                         show help
```

## curio web
```
NAME:
   curio web - Start Curio web interface

USAGE:
   curio web [command options] [arguments...]

DESCRIPTION:
   Start an instance of Curio web interface. 
     This creates the 'web' layer if it does not exist, then calls run with that layer.

OPTIONS:
   --listen value                     Address to listen on (default: "127.0.0.1:4701")
   --nosync                           don't check full-node sync status (default: false)
   --layers value [ --layers value ]  list of layers to be interpreted (atop defaults). Default: base
   --help, -h                         show help
```

## curio guided-setup
```
NAME:
   curio guided-setup - Run the guided setup for migrating from lotus-miner to Curio or Creating a new Curio miner

USAGE:
   curio guided-setup [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

## curio from-miner
```
NAME:
   curio from-miner - Express a database config (for curio) from an existing miner.

USAGE:
   curio from-miner [command options] [arguments...]

DESCRIPTION:
   Express a database config (for curio) from an existing miner.

OPTIONS:
   --miner-repo value, --storagerepo value  Specify miner repo path. flag(storagerepo) and env(LOTUS_STORAGE_PATH) are DEPRECATION, will REMOVE SOON (default: "~/.lotusminer") [$LOTUS_MINER_PATH, $LOTUS_STORAGE_PATH]
   --to-layer value, -t value               The layer name for this data push. 'base' is recommended for single-miner setup.
   --overwrite, -o                          Use this with --to-layer to replace an existing layer (default: false)
   --help, -h                               show help
```

## curio seal
```
NAME:
   curio seal - Manage the sealing pipeline

USAGE:
   curio seal command [command options] [arguments...]

COMMANDS:
   start    Start new sealing operations manually
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### curio seal start
```
NAME:
   curio seal start - Start new sealing operations manually

USAGE:
   curio seal start [command options] [arguments...]

OPTIONS:
   --actor value                      Specify actor address to start sealing sectors for
   --now                              Start sealing sectors for all actors now (not on schedule) (default: false)
   --cc                               Start sealing new CC sectors (default: false)
   --count value                      Number of sectors to start (default: 1)
   --synthetic                        Use synthetic PoRep (default: false)
   --layers value [ --layers value ]  list of layers to be interpreted (atop defaults). Default: base
   --help, -h                         show help
```

## curio auth
```
NAME:
   curio auth - Manage RPC permissions

USAGE:
   curio auth command [command options] [arguments...]

COMMANDS:
   create-token  Create token
   api-info      Get token with API info required to connect to this node
   help, h       Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### curio auth create-token
```
NAME:
   curio auth create-token - Create token

USAGE:
   curio auth create-token [command options] [arguments...]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   --help, -h    show help
```

### curio auth api-info
```
NAME:
   curio auth api-info - Get token with API info required to connect to this node

USAGE:
   curio auth api-info [command options] [arguments...]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   --help, -h    show help
```

## curio log
```
NAME:
   curio log - Manage logging

USAGE:
   curio log command [command options] [arguments...]

COMMANDS:
   list       List log systems
   set-level  Set log level
   alerts     Get alert states
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### curio log list
```
NAME:
   curio log list - List log systems

USAGE:
   curio log list [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

### curio log set-level
```
NAME:
   curio log set-level - Set log level

USAGE:
   curio log set-level [command options] [level]

DESCRIPTION:
   Set the log level for logging systems:

      The system flag can be specified multiple times.

      eg) log set-level --system chain --system chainxchg debug

      Available Levels:
      debug
      info
      warn
      error

      Environment Variables:
      GOLOG_LOG_LEVEL - Default log level for all log systems
      GOLOG_LOG_FMT   - Change output log format (json, nocolor)
      GOLOG_FILE      - Write logs to file
      GOLOG_OUTPUT    - Specify whether to output to file, stderr, stdout or a combination, i.e. file+stderr


OPTIONS:
   --system value [ --system value ]  limit to log system
   --help, -h                         show help
```

### curio log alerts
```
NAME:
   curio log alerts - Get alert states

USAGE:
   curio log alerts [command options] [arguments...]

OPTIONS:
   --all       get all (active and inactive) alerts (default: false)
   --help, -h  show help
```

## curio wait-api
```
NAME:
   curio wait-api - Wait for lotus api to come online

USAGE:
   curio wait-api [command options] [arguments...]

OPTIONS:
   --timeout value  duration to wait till fail (default: 30s)
   --help, -h       show help
```

## curio fetch-params
```
NAME:
   curio fetch-params - Fetch proving parameters

USAGE:
   curio fetch-params [command options] [sectorSize]

OPTIONS:
   --help, -h  show help
```

## curio version
```
NAME:
   curio version - Print version

USAGE:
   curio version [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```
