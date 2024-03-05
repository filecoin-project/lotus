# lotus-provider
```
NAME:
   lotus-provider - Filecoin decentralized storage network provider

USAGE:
   lotus-provider [global options] command [command options] [arguments...]

VERSION:
   1.26.0-rc1

COMMANDS:
   run      Start a lotus provider process
   stop     Stop a running lotus provider
   config   Manage node config by layers. The layer 'base' will always be applied. 
   test     Utility functions for testing
   version  Print version
   help, h  Shows a list of commands or help for one command
   DEVELOPER:
     auth          Manage RPC permissions
     log           Manage logging
     wait-api      Wait for lotus api to come online
     fetch-params  Fetch proving parameters

GLOBAL OPTIONS:
   --color              use color in display output (default: depends on output being a TTY)
   --db-host value      Command separated list of hostnames for yugabyte cluster (default: "yugabyte") [$LOTUS_DB_HOST]
   --db-name value      (default: "yugabyte") [$LOTUS_DB_NAME, $LOTUS_HARMONYDB_HOSTS]
   --db-user value      (default: "yugabyte") [$LOTUS_DB_USER, $LOTUS_HARMONYDB_USERNAME]
   --db-password value  (default: "yugabyte") [$LOTUS_DB_PASSWORD, $LOTUS_HARMONYDB_PASSWORD]
   --layers value       (default: "base") [$LOTUS_LAYERS, $LOTUS_CONFIG_LAYERS]
   --repo-path value    (default: "~/.lotusprovider") [$LOTUS_REPO_PATH]
   --vv                 enables very verbose mode, useful for debugging the CLI (default: false)
   --help, -h           show help
   --version, -v        print the version
```

## lotus-provider run
```
NAME:
   lotus-provider run - Start a lotus provider process

USAGE:
   lotus-provider run [command options] [arguments...]

OPTIONS:
   --listen value                     host address and port the worker api will listen on (default: "0.0.0.0:12300") [$LOTUS_WORKER_LISTEN]
   --nosync                           don't check full-node sync status (default: false)
   --manage-fdlimit                   manage open file limit (default: true)
   --layers value [ --layers value ]  list of layers to be interpreted (atop defaults). Default: base (default: "base")
   --storage-json value               path to json file containing storage config (default: "~/.lotus-provider/storage.json")
   --journal value                    path to journal files (default: "~/.lotus-provider/")
   --help, -h                         show help
```

## lotus-provider stop
```
NAME:
   lotus-provider stop - Stop a running lotus provider

USAGE:
   lotus-provider stop [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

## lotus-provider config
```
NAME:
   lotus-provider config - Manage node config by layers. The layer 'base' will always be applied. 

USAGE:
   lotus-provider config command [command options] [arguments...]

COMMANDS:
   default, defaults                Print default node config
   set, add, update, create         Set a config layer or the base by providing a filename or stdin.
   get, cat, show                   Get a config layer by name. You may want to pipe the output to a file, or use 'less'
   list, ls                         List config layers you can get.
   interpret, view, stacked, stack  Interpret stacked config layers by this version of lotus-provider, with system-generated comments.
   remove, rm, del, delete          Remove a named config layer.
   from-miner                       Express a database config (for lotus-provider) from an existing miner.
   help, h                          Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-provider config default
```
NAME:
   lotus-provider config default - Print default node config

USAGE:
   lotus-provider config default [command options] [arguments...]

OPTIONS:
   --no-comment  don't comment default values (default: false)
   --help, -h    show help
```

### lotus-provider config set
```
NAME:
   lotus-provider config set - Set a config layer or the base by providing a filename or stdin.

USAGE:
   lotus-provider config set [command options] a layer's file name

OPTIONS:
   --title value  title of the config layer (req'd for stdin)
   --help, -h     show help
```

### lotus-provider config get
```
NAME:
   lotus-provider config get - Get a config layer by name. You may want to pipe the output to a file, or use 'less'

USAGE:
   lotus-provider config get [command options] layer name

OPTIONS:
   --help, -h  show help
```

### lotus-provider config list
```
NAME:
   lotus-provider config list - List config layers you can get.

USAGE:
   lotus-provider config list [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

### lotus-provider config interpret
```
NAME:
   lotus-provider config interpret - Interpret stacked config layers by this version of lotus-provider, with system-generated comments.

USAGE:
   lotus-provider config interpret [command options] a list of layers to be interpreted as the final config

OPTIONS:
   --layers value [ --layers value ]  comma or space separated list of layers to be interpreted (default: "base")
   --help, -h                         show help
```

### lotus-provider config remove
```
NAME:
   lotus-provider config remove - Remove a named config layer.

USAGE:
   lotus-provider config remove [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

### lotus-provider config from-miner
```
NAME:
   lotus-provider config from-miner - Express a database config (for lotus-provider) from an existing miner.

USAGE:
   lotus-provider config from-miner [command options] [arguments...]

DESCRIPTION:
   Express a database config (for lotus-provider) from an existing miner.

OPTIONS:
   --miner-repo value, --storagerepo value  Specify miner repo path. flag(storagerepo) and env(LOTUS_STORAGE_PATH) are DEPRECATION, will REMOVE SOON (default: "~/.lotusminer") [$LOTUS_MINER_PATH, $LOTUS_STORAGE_PATH]
   --to-layer value, -t value               The layer name for this data push. 'base' is recommended for single-miner setup.
   --overwrite, -o                          Use this with --to-layer to replace an existing layer (default: false)
   --help, -h                               show help
```

## lotus-provider test
```
NAME:
   lotus-provider test - Utility functions for testing

USAGE:
   lotus-provider test command [command options] [arguments...]

COMMANDS:
   window-post, wd, windowpost, wdpost  Compute a proof-of-spacetime for a sector (requires the sector to be pre-sealed). These will not send to the chain.
   help, h                              Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-provider test window-post
```
NAME:
   lotus-provider test window-post - Compute a proof-of-spacetime for a sector (requires the sector to be pre-sealed). These will not send to the chain.

USAGE:
   lotus-provider test window-post command [command options] [arguments...]

COMMANDS:
   here, cli                                       Compute WindowPoSt for performance and configuration testing.
   task, scheduled, schedule, async, asynchronous  Test the windowpost scheduler by running it on the next available lotus-provider. 
   help, h                                         Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

#### lotus-provider test window-post here
```
NAME:
   lotus-provider test window-post here - Compute WindowPoSt for performance and configuration testing.

USAGE:
   lotus-provider test window-post here [command options] [deadline index]

DESCRIPTION:
   Note: This command is intended to be used to verify PoSt compute performance.
   It will not send any messages to the chain. Since it can compute any deadline, output may be incorrectly timed for the chain.

OPTIONS:
   --deadline value                   deadline to compute WindowPoSt for  (default: 0)
   --layers value [ --layers value ]  list of layers to be interpreted (atop defaults). Default: base (default: "base")
   --storage-json value               path to json file containing storage config (default: "~/.lotus-provider/storage.json")
   --partition value                  partition to compute WindowPoSt for (default: 0)
   --help, -h                         show help
```

#### lotus-provider test window-post task
```
NAME:
   lotus-provider test window-post task - Test the windowpost scheduler by running it on the next available lotus-provider. 

USAGE:
   lotus-provider test window-post task [command options] [arguments...]

OPTIONS:
   --deadline value                   deadline to compute WindowPoSt for  (default: 0)
   --layers value [ --layers value ]  list of layers to be interpreted (atop defaults). Default: base (default: "base")
   --help, -h                         show help
```

## lotus-provider version
```
NAME:
   lotus-provider version - Print version

USAGE:
   lotus-provider version [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

## lotus-provider auth
```
NAME:
   lotus-provider auth - Manage RPC permissions

USAGE:
   lotus-provider auth command [command options] [arguments...]

COMMANDS:
   create-token  Create token
   api-info      Get token with API info required to connect to this node
   help, h       Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-provider auth create-token
```
NAME:
   lotus-provider auth create-token - Create token

USAGE:
   lotus-provider auth create-token [command options] [arguments...]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   --help, -h    show help
```

### lotus-provider auth api-info
```
NAME:
   lotus-provider auth api-info - Get token with API info required to connect to this node

USAGE:
   lotus-provider auth api-info [command options] [arguments...]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   --help, -h    show help
```

## lotus-provider log
```
NAME:
   lotus-provider log - Manage logging

USAGE:
   lotus-provider log command [command options] [arguments...]

COMMANDS:
   list       List log systems
   set-level  Set log level
   alerts     Get alert states
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-provider log list
```
NAME:
   lotus-provider log list - List log systems

USAGE:
   lotus-provider log list [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

### lotus-provider log set-level
```
NAME:
   lotus-provider log set-level - Set log level

USAGE:
   lotus-provider log set-level [command options] [level]

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

### lotus-provider log alerts
```
NAME:
   lotus-provider log alerts - Get alert states

USAGE:
   lotus-provider log alerts [command options] [arguments...]

OPTIONS:
   --all       get all (active and inactive) alerts (default: false)
   --help, -h  show help
```

## lotus-provider wait-api
```
NAME:
   lotus-provider wait-api - Wait for lotus api to come online

USAGE:
   lotus-provider wait-api [command options] [arguments...]

CATEGORY:
   DEVELOPER

OPTIONS:
   --timeout value  duration to wait till fail (default: 30s)
   --help, -h       show help
```

## lotus-provider fetch-params
```
NAME:
   lotus-provider fetch-params - Fetch proving parameters

USAGE:
   lotus-provider fetch-params [command options] [sectorSize]

CATEGORY:
   DEVELOPER

OPTIONS:
   --help, -h  show help
```
