# lotus-miner
```
NAME:
   lotus-miner - Filecoin decentralized storage network miner

USAGE:
   lotus-miner [global options] command [command options] [arguments...]

VERSION:
   1.14.2

COMMANDS:
   init     Initialize a lotus miner repo
   run      Start a lotus miner process
   stop     Stop a running lotus miner
   config   Manage node config
   backup   Create node metadata backup
   version  Print version
   help, h  Shows a list of commands or help for one command
   CHAIN:
     actor  manipulate the miner actor
     info   Print miner info
   DEVELOPER:
     auth          Manage RPC permissions
     log           Manage logging
     wait-api      Wait for lotus api to come online
     fetch-params  Fetch proving parameters
   MARKET:
     storage-deals    Manage storage deals and related configuration
     retrieval-deals  Manage retrieval deals and related configuration
     data-transfers   Manage data transfers
     dagstore         Manage the dagstore on the markets subsystem
   NETWORK:
     net  Manage P2P Network
   RETRIEVAL:
     pieces  interact with the piecestore
   STORAGE:
     sectors  interact with sector store
     proving  View proving information
     storage  manage sector storage
     sealing  interact with sealing pipeline

GLOBAL OPTIONS:
   --actor value, -a value                  specify other actor to query / manipulate
   --color                                  use color in display output (default: depends on output being a TTY)
   --miner-repo value, --storagerepo value  Specify miner repo path. flag(storagerepo) and env(LOTUS_STORAGE_PATH) are DEPRECATION, will REMOVE SOON (default: "~/.lotusminer") [$LOTUS_MINER_PATH, $LOTUS_STORAGE_PATH]
   --markets-repo value                     Markets repo path [$LOTUS_MARKETS_PATH]
   --call-on-markets                        (experimental; may be removed) call this command against a markets node; use only with common commands like net, auth, pprof, etc. whose target may be ambiguous (default: false)
   --vv                                     enables very verbose mode, useful for debugging the CLI (default: false)
   --help, -h                               show help (default: false)
   --version, -v                            print the version (default: false)
```

## lotus-miner init
```
NAME:
   lotus-miner init - Initialize a lotus miner repo

USAGE:
   lotus-miner init command [command options] [arguments...]

COMMANDS:
   restore  Initialize a lotus miner repo from a backup
   service  Initialize a lotus miner sub-service
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --actor value                specify the address of an already created miner actor
   --create-worker-key          create separate worker key (default: false)
   --worker value, -w value     worker key to use (overrides --create-worker-key)
   --owner value, -o value      owner key to use
   --sector-size value          specify sector size to use (default: "32GiB")
   --pre-sealed-sectors value   specify set of presealed sectors for starting as a genesis miner
   --pre-sealed-metadata value  specify the metadata file for the presealed sectors
   --nosync                     don't check full-node sync status (default: false)
   --symlink-imported-sectors   attempt to symlink to presealed sectors instead of copying them into place (default: false)
   --no-local-storage           don't use storageminer repo for sector storage (default: false)
   --gas-premium value          set gas premium for initialization messages in AttoFIL (default: "0")
   --from value                 select which address to send actor creation message from
   --help, -h                   show help (default: false)
   --version, -v                print the version (default: false)
   
```

### lotus-miner init restore
```
NAME:
   lotus-miner init restore - Initialize a lotus miner repo from a backup

USAGE:
   lotus-miner init restore [command options] [backupFile]

OPTIONS:
   --nosync                don't check full-node sync status (default: false)
   --config value          config file (config.toml)
   --storage-config value  storage paths config (storage.json)
   --help, -h              show help (default: false)
   
```

### lotus-miner init service
```
NAME:
   lotus-miner init service - Initialize a lotus miner sub-service

USAGE:
   lotus-miner init service [command options] [backupFile]

OPTIONS:
   --config value            config file (config.toml)
   --nosync                  don't check full-node sync status (default: false)
   --type value              type of service to be enabled
   --api-sealer value        sealer API info (lotus-miner auth api-info --perm=admin)
   --api-sector-index value  sector Index API info (lotus-miner auth api-info --perm=admin)
   --help, -h                show help (default: false)
   
```

## lotus-miner run
```
NAME:
   lotus-miner run - Start a lotus miner process

USAGE:
   lotus-miner run [command options] [arguments...]

OPTIONS:
   --miner-api value     2345
   --enable-gpu-proving  enable use of GPU for mining operations (default: true)
   --nosync              don't check full-node sync status (default: false)
   --manage-fdlimit      manage open file limit (default: true)
   --help, -h            show help (default: false)
   
```

## lotus-miner stop
```
NAME:
   lotus-miner stop - Stop a running lotus miner

USAGE:
   lotus-miner stop [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner config
```
NAME:
   lotus-miner config - Manage node config

USAGE:
   lotus-miner config command [command options] [arguments...]

COMMANDS:
   default  Print default node config
   updated  Print updated node config
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner config default
```
NAME:
   lotus-miner config default - Print default node config

USAGE:
   lotus-miner config default [command options] [arguments...]

OPTIONS:
   --no-comment  don't comment default values (default: false)
   --help, -h    show help (default: false)
   
```

### lotus-miner config updated
```
NAME:
   lotus-miner config updated - Print updated node config

USAGE:
   lotus-miner config updated [command options] [arguments...]

OPTIONS:
   --no-comment  don't comment default values (default: false)
   --help, -h    show help (default: false)
   
```

## lotus-miner backup
```
NAME:
   lotus-miner backup - Create node metadata backup

USAGE:
   lotus-miner backup [command options] [backup file path]

DESCRIPTION:
   The backup command writes a copy of node metadata under the specified path

Online backups:
For security reasons, the daemon must be have LOTUS_BACKUP_BASE_PATH env var set
to a path where backup files are supposed to be saved, and the path specified in
this command must be within this base path

OPTIONS:
   --offline   create backup without the node running (default: false)
   --help, -h  show help (default: false)
   
```

## lotus-miner version
```
NAME:
   lotus-miner version - Print version

USAGE:
   lotus-miner version [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner actor
```
NAME:
   lotus-miner actor - manipulate the miner actor

USAGE:
   lotus-miner actor command [command options] [arguments...]

COMMANDS:
   set-addrs              set addresses that your miner can be publicly dialed on
   withdraw               withdraw available balance
   repay-debt             pay down a miner's debt
   set-peer-id            set the peer id of your miner
   set-owner              Set owner address (this command should be invoked twice, first with the old owner as the senderAddress, and then with the new owner)
   control                Manage control addresses
   propose-change-worker  Propose a worker address change
   confirm-change-worker  Confirm a worker address change
   compact-allocated      compact allocated sectors bitfield
   help, h                Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner actor set-addrs
```
NAME:
   lotus-miner actor set-addrs - set addresses that your miner can be publicly dialed on

USAGE:
   lotus-miner actor set-addrs [command options] [arguments...]

OPTIONS:
   --gas-limit value  set gas limit (default: 0)
   --unset            unset address (default: false)
   --help, -h         show help (default: false)
   
```

### lotus-miner actor withdraw
```
NAME:
   lotus-miner actor withdraw - withdraw available balance

USAGE:
   lotus-miner actor withdraw [command options] [amount (FIL)]

OPTIONS:
   --confidence value  number of block confirmations to wait for (default: 5)
   --help, -h          show help (default: false)
   
```

### lotus-miner actor repay-debt
```
NAME:
   lotus-miner actor repay-debt - pay down a miner's debt

USAGE:
   lotus-miner actor repay-debt [command options] [amount (FIL)]

OPTIONS:
   --from value  optionally specify the account to send funds from
   --help, -h    show help (default: false)
   
```

### lotus-miner actor set-peer-id
```
NAME:
   lotus-miner actor set-peer-id - set the peer id of your miner

USAGE:
   lotus-miner actor set-peer-id [command options] [arguments...]

OPTIONS:
   --gas-limit value  set gas limit (default: 0)
   --help, -h         show help (default: false)
   
```

### lotus-miner actor set-owner
```
NAME:
   lotus-miner actor set-owner - Set owner address (this command should be invoked twice, first with the old owner as the senderAddress, and then with the new owner)

USAGE:
   lotus-miner actor set-owner [command options] [newOwnerAddress senderAddress]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner actor control
```
NAME:
   lotus-miner actor control - Manage control addresses

USAGE:
   lotus-miner actor control command [command options] [arguments...]

COMMANDS:
   list     Get currently set control addresses
   set      Set control address(-es)
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

#### lotus-miner actor control list
```
NAME:
   lotus-miner actor control list - Get currently set control addresses

USAGE:
   lotus-miner actor control list [command options] [arguments...]

OPTIONS:
   --verbose   (default: false)
   --color     use color in display output (default: depends on output being a TTY)
   --help, -h  show help (default: false)
   
```

#### lotus-miner actor control set
```
NAME:
   lotus-miner actor control set - Set control address(-es)

USAGE:
   lotus-miner actor control set [command options] [...address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner actor propose-change-worker
```
NAME:
   lotus-miner actor propose-change-worker - Propose a worker address change

USAGE:
   lotus-miner actor propose-change-worker [command options] [address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner actor confirm-change-worker
```
NAME:
   lotus-miner actor confirm-change-worker - Confirm a worker address change

USAGE:
   lotus-miner actor confirm-change-worker [command options] [address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner actor compact-allocated
```
NAME:
   lotus-miner actor compact-allocated - compact allocated sectors bitfield

USAGE:
   lotus-miner actor compact-allocated [command options] [arguments...]

OPTIONS:
   --mask-last-offset value  Mask sector IDs from 0 to 'higest_allocated - offset' (default: 0)
   --mask-upto-n value       Mask sector IDs from 0 to 'n' (default: 0)
   --really-do-it            Actually send transaction performing the action (default: false)
   --help, -h                show help (default: false)
   
```

## lotus-miner info
```
NAME:
   lotus-miner info - Print miner info

USAGE:
   lotus-miner info command [command options] [arguments...]

COMMANDS:
   all      dump all related miner info
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --hide-sectors-info  hide sectors info (default: false)
   --blocks value       Log of produced <blocks> newest blocks and rewards(Miner Fee excluded) (default: 0)
   --help, -h           show help (default: false)
   --version, -v        print the version (default: false)
   
```

### lotus-miner info all
```
NAME:
   lotus-miner info all - dump all related miner info

USAGE:
   lotus-miner info all [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner auth
```
NAME:
   lotus-miner auth - Manage RPC permissions

USAGE:
   lotus-miner auth command [command options] [arguments...]

COMMANDS:
   create-token  Create token
   api-info      Get token with API info required to connect to this node
   help, h       Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner auth create-token
```
NAME:
   lotus-miner auth create-token - Create token

USAGE:
   lotus-miner auth create-token [command options] [arguments...]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   --help, -h    show help (default: false)
   
```

### lotus-miner auth api-info
```
NAME:
   lotus-miner auth api-info - Get token with API info required to connect to this node

USAGE:
   lotus-miner auth api-info [command options] [arguments...]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   --help, -h    show help (default: false)
   
```

## lotus-miner log
```
NAME:
   lotus-miner log - Manage logging

USAGE:
   lotus-miner log command [command options] [arguments...]

COMMANDS:
   list       List log systems
   set-level  Set log level
   alerts     Get alert states
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner log list
```
NAME:
   lotus-miner log list - List log systems

USAGE:
   lotus-miner log list [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner log set-level
```
NAME:
   lotus-miner log set-level - Set log level

USAGE:
   lotus-miner log set-level [command options] [level]

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
   --system value  limit to log system
   --help, -h      show help (default: false)
   
```

### lotus-miner log alerts
```
NAME:
   lotus-miner log alerts - Get alert states

USAGE:
   lotus-miner log alerts [command options] [arguments...]

OPTIONS:
   --all       get all (active and inactive) alerts (default: false)
   --help, -h  show help (default: false)
   
```

## lotus-miner wait-api
```
NAME:
   lotus-miner wait-api - Wait for lotus api to come online

USAGE:
   lotus-miner wait-api [command options] [arguments...]

CATEGORY:
   DEVELOPER

OPTIONS:
   --timeout value  duration to wait till fail (default: 30s)
   --help, -h       show help (default: false)
   
```

## lotus-miner fetch-params
```
NAME:
   lotus-miner fetch-params - Fetch proving parameters

USAGE:
   lotus-miner fetch-params [command options] [sectorSize]

CATEGORY:
   DEVELOPER

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner storage-deals
```
NAME:
   lotus-miner storage-deals - Manage storage deals and related configuration

USAGE:
   lotus-miner storage-deals command [command options] [arguments...]

COMMANDS:
   import-data        Manually import data for a deal
   list               List all deals for this miner
   selection          Configure acceptance criteria for storage deal proposals
   set-ask            Configure the miner's ask
   get-ask            Print the miner's ask
   set-blocklist      Set the miner's list of blocklisted piece CIDs
   get-blocklist      List the contents of the miner's piece CID blocklist
   reset-blocklist    Remove all entries from the miner's piece CID blocklist
   set-seal-duration  Set the expected time, in minutes, that you expect sealing sectors to take. Deals that start before this duration will be rejected.
   pending-publish    list deals waiting in publish queue
   retry-publish      retry publishing a deal
   help, h            Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner storage-deals import-data
```
NAME:
   lotus-miner storage-deals import-data - Manually import data for a deal

USAGE:
   lotus-miner storage-deals import-data [command options] <proposal CID> <file>

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner storage-deals list
```
NAME:
   lotus-miner storage-deals list - List all deals for this miner

USAGE:
   lotus-miner storage-deals list [command options] [arguments...]

OPTIONS:
   --format value  output format of data, supported: table, json (default: "table")
   --verbose, -v   (default: false)
   --watch         watch deal updates in real-time, rather than a one time list (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner storage-deals selection
```
NAME:
   lotus-miner storage-deals selection - Configure acceptance criteria for storage deal proposals

USAGE:
   lotus-miner storage-deals selection command [command options] [arguments...]

COMMANDS:
   list     List storage deal proposal selection criteria
   reset    Reset storage deal proposal selection criteria to default values
   reject   Configure criteria which necessitate automatic rejection
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

#### lotus-miner storage-deals selection list
```
NAME:
   lotus-miner storage-deals selection list - List storage deal proposal selection criteria

USAGE:
   lotus-miner storage-deals selection list [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner storage-deals selection reset
```
NAME:
   lotus-miner storage-deals selection reset - Reset storage deal proposal selection criteria to default values

USAGE:
   lotus-miner storage-deals selection reset [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner storage-deals selection reject
```
NAME:
   lotus-miner storage-deals selection reject - Configure criteria which necessitate automatic rejection

USAGE:
   lotus-miner storage-deals selection reject [command options] [arguments...]

OPTIONS:
   --online      (default: false)
   --offline     (default: false)
   --verified    (default: false)
   --unverified  (default: false)
   --help, -h    show help (default: false)
   
```

### lotus-miner storage-deals set-ask
```
NAME:
   lotus-miner storage-deals set-ask - Configure the miner's ask

USAGE:
   lotus-miner storage-deals set-ask [command options] [arguments...]

OPTIONS:
   --price PRICE           Set the price of the ask for unverified deals (specified as FIL / GiB / Epoch) to PRICE.
   --verified-price PRICE  Set the price of the ask for verified deals (specified as FIL / GiB / Epoch) to PRICE
   --min-piece-size SIZE   Set minimum piece size (w/bit-padding, in bytes) in ask to SIZE (default: 256B)
   --max-piece-size SIZE   Set maximum piece size (w/bit-padding, in bytes) in ask to SIZE (default: miner sector size)
   --help, -h              show help (default: false)
   
```

### lotus-miner storage-deals get-ask
```
NAME:
   lotus-miner storage-deals get-ask - Print the miner's ask

USAGE:
   lotus-miner storage-deals get-ask [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner storage-deals set-blocklist
```
NAME:
   lotus-miner storage-deals set-blocklist - Set the miner's list of blocklisted piece CIDs

USAGE:
   lotus-miner storage-deals set-blocklist [command options] [<path-of-file-containing-newline-delimited-piece-CIDs> (optional, will read from stdin if omitted)]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner storage-deals get-blocklist
```
NAME:
   lotus-miner storage-deals get-blocklist - List the contents of the miner's piece CID blocklist

USAGE:
   lotus-miner storage-deals get-blocklist [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner storage-deals reset-blocklist
```
NAME:
   lotus-miner storage-deals reset-blocklist - Remove all entries from the miner's piece CID blocklist

USAGE:
   lotus-miner storage-deals reset-blocklist [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner storage-deals set-seal-duration
```
NAME:
   lotus-miner storage-deals set-seal-duration - Set the expected time, in minutes, that you expect sealing sectors to take. Deals that start before this duration will be rejected.

USAGE:
   lotus-miner storage-deals set-seal-duration [command options] <minutes>

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner storage-deals pending-publish
```
NAME:
   lotus-miner storage-deals pending-publish - list deals waiting in publish queue

USAGE:
   lotus-miner storage-deals pending-publish [command options] [arguments...]

OPTIONS:
   --publish-now  send a publish message now (default: false)
   --help, -h     show help (default: false)
   
```

### lotus-miner storage-deals retry-publish
```
NAME:
   lotus-miner storage-deals retry-publish - retry publishing a deal

USAGE:
   lotus-miner storage-deals retry-publish [command options] <proposal CID>

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner retrieval-deals
```
NAME:
   lotus-miner retrieval-deals - Manage retrieval deals and related configuration

USAGE:
   lotus-miner retrieval-deals command [command options] [arguments...]

COMMANDS:
   selection  Configure acceptance criteria for retrieval deal proposals
   list       List all active retrieval deals for this miner
   set-ask    Configure the provider's retrieval ask
   get-ask    Get the provider's current retrieval ask configured by the provider in the ask-store using the set-ask CLI command
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner retrieval-deals selection
```
NAME:
   lotus-miner retrieval-deals selection - Configure acceptance criteria for retrieval deal proposals

USAGE:
   lotus-miner retrieval-deals selection command [command options] [arguments...]

COMMANDS:
   list     List retrieval deal proposal selection criteria
   reset    Reset retrieval deal proposal selection criteria to default values
   reject   Configure criteria which necessitate automatic rejection
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

#### lotus-miner retrieval-deals selection list
```
NAME:
   lotus-miner retrieval-deals selection list - List retrieval deal proposal selection criteria

USAGE:
   lotus-miner retrieval-deals selection list [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner retrieval-deals selection reset
```
NAME:
   lotus-miner retrieval-deals selection reset - Reset retrieval deal proposal selection criteria to default values

USAGE:
   lotus-miner retrieval-deals selection reset [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner retrieval-deals selection reject
```
NAME:
   lotus-miner retrieval-deals selection reject - Configure criteria which necessitate automatic rejection

USAGE:
   lotus-miner retrieval-deals selection reject [command options] [arguments...]

OPTIONS:
   --online    (default: false)
   --offline   (default: false)
   --help, -h  show help (default: false)
   
```

### lotus-miner retrieval-deals list
```
NAME:
   lotus-miner retrieval-deals list - List all active retrieval deals for this miner

USAGE:
   lotus-miner retrieval-deals list [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner retrieval-deals set-ask
```
NAME:
   lotus-miner retrieval-deals set-ask - Configure the provider's retrieval ask

USAGE:
   lotus-miner retrieval-deals set-ask [command options] [arguments...]

OPTIONS:
   --price value                      Set the price of the ask for retrievals (FIL/GiB)
   --unseal-price value               Set the price to unseal
   --payment-interval value           Set the payment interval (in bytes) for retrieval (default: 1MiB)
   --payment-interval-increase value  Set the payment interval increase (in bytes) for retrieval (default: 1MiB)
   --help, -h                         show help (default: false)
   
```

### lotus-miner retrieval-deals get-ask
```
NAME:
   lotus-miner retrieval-deals get-ask - Get the provider's current retrieval ask configured by the provider in the ask-store using the set-ask CLI command

USAGE:
   lotus-miner retrieval-deals get-ask [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner data-transfers
```
NAME:
   lotus-miner data-transfers - Manage data transfers

USAGE:
   lotus-miner data-transfers command [command options] [arguments...]

COMMANDS:
   list     List ongoing data transfers for this miner
   restart  Force restart a stalled data transfer
   cancel   Force cancel a data transfer
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner data-transfers list
```
NAME:
   lotus-miner data-transfers list - List ongoing data transfers for this miner

USAGE:
   lotus-miner data-transfers list [command options] [arguments...]

OPTIONS:
   --verbose, -v  print verbose transfer details (default: false)
   --color        use color in display output (default: depends on output being a TTY)
   --completed    show completed data transfers (default: false)
   --watch        watch deal updates in real-time, rather than a one time list (default: false)
   --show-failed  show failed/cancelled transfers (default: false)
   --help, -h     show help (default: false)
   
```

### lotus-miner data-transfers restart
```
NAME:
   lotus-miner data-transfers restart - Force restart a stalled data transfer

USAGE:
   lotus-miner data-transfers restart [command options] [arguments...]

OPTIONS:
   --peerid value  narrow to transfer with specific peer
   --initiator     specify only transfers where peer is/is not initiator (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner data-transfers cancel
```
NAME:
   lotus-miner data-transfers cancel - Force cancel a data transfer

USAGE:
   lotus-miner data-transfers cancel [command options] [arguments...]

OPTIONS:
   --peerid value          narrow to transfer with specific peer
   --initiator             specify only transfers where peer is/is not initiator (default: false)
   --cancel-timeout value  time to wait for cancel to be sent to client (default: 5s)
   --help, -h              show help (default: false)
   
```

## lotus-miner dagstore
```
NAME:
   lotus-miner dagstore - Manage the dagstore on the markets subsystem

USAGE:
   lotus-miner dagstore command [command options] [arguments...]

COMMANDS:
   list-shards       List all shards known to the dagstore, with their current status
   initialize-shard  Initialize the specified shard
   recover-shard     Attempt to recover a shard in errored state
   initialize-all    Initialize all uninitialized shards, streaming results as they're produced; only shards for unsealed pieces are initialized by default
   gc                Garbage collect the dagstore
   help, h           Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner dagstore list-shards
```
NAME:
   lotus-miner dagstore list-shards - List all shards known to the dagstore, with their current status

USAGE:
   lotus-miner dagstore list-shards [command options] [arguments...]

OPTIONS:
   --color     use color in display output (default: depends on output being a TTY)
   --help, -h  show help (default: false)
   
```

### lotus-miner dagstore initialize-shard
```
NAME:
   lotus-miner dagstore initialize-shard - Initialize the specified shard

USAGE:
   lotus-miner dagstore initialize-shard [command options] [key]

OPTIONS:
   --color     use color in display output (default: depends on output being a TTY)
   --help, -h  show help (default: false)
   
```

### lotus-miner dagstore recover-shard
```
NAME:
   lotus-miner dagstore recover-shard - Attempt to recover a shard in errored state

USAGE:
   lotus-miner dagstore recover-shard [command options] [key]

OPTIONS:
   --color     use color in display output (default: depends on output being a TTY)
   --help, -h  show help (default: false)
   
```

### lotus-miner dagstore initialize-all
```
NAME:
   lotus-miner dagstore initialize-all - Initialize all uninitialized shards, streaming results as they're produced; only shards for unsealed pieces are initialized by default

USAGE:
   lotus-miner dagstore initialize-all [command options] [arguments...]

OPTIONS:
   --concurrency value  maximum shards to initialize concurrently at a time; use 0 for unlimited (default: 0)
   --include-sealed     initialize sealed pieces as well (default: false)
   --color              use color in display output (default: depends on output being a TTY)
   --help, -h           show help (default: false)
   
```

### lotus-miner dagstore gc
```
NAME:
   lotus-miner dagstore gc - Garbage collect the dagstore

USAGE:
   lotus-miner dagstore gc [command options] [arguments...]

OPTIONS:
   --color     use color in display output (default: depends on output being a TTY)
   --help, -h  show help (default: false)
   
```

## lotus-miner net
```
NAME:
   lotus-miner net - Manage P2P Network

USAGE:
   lotus-miner net command [command options] [arguments...]

COMMANDS:
   peers         Print peers
   connect       Connect to a peer
   listen        List listen addresses
   id            Get node identity
   findpeer      Find the addresses of a given peerID
   scores        Print peers' pubsub scores
   reachability  Print information about reachability from the internet
   bandwidth     Print bandwidth usage information
   block         Manage network connection gating rules
   help, h       Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner net peers
```
NAME:
   lotus-miner net peers - Print peers

USAGE:
   lotus-miner net peers [command options] [arguments...]

OPTIONS:
   --agent, -a     Print agent name (default: false)
   --extended, -x  Print extended peer information in json (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner net connect
```
NAME:
   lotus-miner net connect - Connect to a peer

USAGE:
   lotus-miner net connect [command options] [peerMultiaddr|minerActorAddress]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner net listen
```
NAME:
   lotus-miner net listen - List listen addresses

USAGE:
   lotus-miner net listen [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner net id
```
NAME:
   lotus-miner net id - Get node identity

USAGE:
   lotus-miner net id [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner net findpeer
```
NAME:
   lotus-miner net findpeer - Find the addresses of a given peerID

USAGE:
   lotus-miner net findpeer [command options] [peerId]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner net scores
```
NAME:
   lotus-miner net scores - Print peers' pubsub scores

USAGE:
   lotus-miner net scores [command options] [arguments...]

OPTIONS:
   --extended, -x  print extended peer scores in json (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner net reachability
```
NAME:
   lotus-miner net reachability - Print information about reachability from the internet

USAGE:
   lotus-miner net reachability [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner net bandwidth
```
NAME:
   lotus-miner net bandwidth - Print bandwidth usage information

USAGE:
   lotus-miner net bandwidth [command options] [arguments...]

OPTIONS:
   --by-peer      list bandwidth usage by peer (default: false)
   --by-protocol  list bandwidth usage by protocol (default: false)
   --help, -h     show help (default: false)
   
```

### lotus-miner net block
```
NAME:
   lotus-miner net block - Manage network connection gating rules

USAGE:
   lotus-miner net block command [command options] [arguments...]

COMMANDS:
   add      Add connection gating rules
   remove   Remove connection gating rules
   list     list connection gating rules
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

#### lotus-miner net block add
```
NAME:
   lotus-miner net block add - Add connection gating rules

USAGE:
   lotus-miner net block add command [command options] [arguments...]

COMMANDS:
   peer     Block a peer
   ip       Block an IP address
   subnet   Block an IP subnet
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

##### lotus-miner net block add peer
```
NAME:
   lotus-miner net block add peer - Block a peer

USAGE:
   lotus-miner net block add peer [command options] <Peer> ...

OPTIONS:
   --help, -h  show help (default: false)
   
```

##### lotus-miner net block add ip
```
NAME:
   lotus-miner net block add ip - Block an IP address

USAGE:
   lotus-miner net block add ip [command options] <IP> ...

OPTIONS:
   --help, -h  show help (default: false)
   
```

##### lotus-miner net block add subnet
```
NAME:
   lotus-miner net block add subnet - Block an IP subnet

USAGE:
   lotus-miner net block add subnet [command options] <CIDR> ...

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner net block remove
```
NAME:
   lotus-miner net block remove - Remove connection gating rules

USAGE:
   lotus-miner net block remove command [command options] [arguments...]

COMMANDS:
   peer     Unblock a peer
   ip       Unblock an IP address
   subnet   Unblock an IP subnet
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

##### lotus-miner net block remove peer
```
NAME:
   lotus-miner net block remove peer - Unblock a peer

USAGE:
   lotus-miner net block remove peer [command options] <Peer> ...

OPTIONS:
   --help, -h  show help (default: false)
   
```

##### lotus-miner net block remove ip
```
NAME:
   lotus-miner net block remove ip - Unblock an IP address

USAGE:
   lotus-miner net block remove ip [command options] <IP> ...

OPTIONS:
   --help, -h  show help (default: false)
   
```

##### lotus-miner net block remove subnet
```
NAME:
   lotus-miner net block remove subnet - Unblock an IP subnet

USAGE:
   lotus-miner net block remove subnet [command options] <CIDR> ...

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner net block list
```
NAME:
   lotus-miner net block list - list connection gating rules

USAGE:
   lotus-miner net block list [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner pieces
```
NAME:
   lotus-miner pieces - interact with the piecestore

USAGE:
   lotus-miner pieces command [command options] [arguments...]

DESCRIPTION:
   The piecestore is a database that tracks and manages data that is made available to the retrieval market

COMMANDS:
   list-pieces  list registered pieces
   list-cids    list registered payload CIDs
   piece-info   get registered information for a given piece CID
   cid-info     get registered information for a given payload CID
   help, h      Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner pieces list-pieces
```
NAME:
   lotus-miner pieces list-pieces - list registered pieces

USAGE:
   lotus-miner pieces list-pieces [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner pieces list-cids
```
NAME:
   lotus-miner pieces list-cids - list registered payload CIDs

USAGE:
   lotus-miner pieces list-cids [command options] [arguments...]

OPTIONS:
   --verbose, -v  (default: false)
   --help, -h     show help (default: false)
   
```

### lotus-miner pieces piece-info
```
NAME:
   lotus-miner pieces piece-info - get registered information for a given piece CID

USAGE:
   lotus-miner pieces piece-info [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner pieces cid-info
```
NAME:
   lotus-miner pieces cid-info - get registered information for a given payload CID

USAGE:
   lotus-miner pieces cid-info [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner sectors
```
NAME:
   lotus-miner sectors - interact with sector store

USAGE:
   lotus-miner sectors command [command options] [arguments...]

COMMANDS:
   status                Get the seal status of a sector by its number
   list                  List sectors
   refs                  List References to sectors
   update-state          ADVANCED: manually update the state of a sector, this may aid in error recovery
   pledge                store random data in a sector
   check-expire          Inspect expiring sectors
   expired               Get or cleanup expired sectors
   renew                 Renew expiring sectors while not exceeding each sector's max life
   extend                Extend sector expiration
   terminate             Terminate sector on-chain then remove (WARNING: This means losing power and collateral for the removed sector)
   remove                Forcefully remove a sector (WARNING: This means losing power and collateral for the removed sector (use 'terminate' for lower penalty))
   snap-up               Mark a committed capacity sector to be filled with deals
   abort-upgrade         Abort the attempted (SnapDeals) upgrade of a CC sector, reverting it to as before
   mark-for-upgrade      Mark a committed capacity sector for replacement by a sector with deals
   seal                  Manually start sealing a sector (filling any unused space with junk)
   set-seal-delay        Set the time, in minutes, that a new sector waits for deals before sealing starts
   get-cc-collateral     Get the collateral required to pledge a committed capacity sector
   batching              manage batch sector operations
   match-pending-pieces  force a refreshed match of pending pieces to open sectors without manually waiting for more deals
   help, h               Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner sectors status
```
NAME:
   lotus-miner sectors status - Get the seal status of a sector by its number

USAGE:
   lotus-miner sectors status [command options] <sectorNum>

OPTIONS:
   --log, -l             display event log (default: false)
   --on-chain-info, -c   show sector on chain info (default: false)
   --partition-info, -p  show partition related info (default: false)
   --proof               print snark proof bytes as hex (default: false)
   --help, -h            show help (default: false)
   
```

### lotus-miner sectors list
```
NAME:
   lotus-miner sectors list - List sectors

USAGE:
   lotus-miner sectors list [command options] [arguments...]

OPTIONS:
   --show-removed, -r  show removed sectors (default: false)
   --color, -c         use color in display output (default: depends on output being a TTY)
   --fast, -f          don't show on-chain info for better performance (default: false)
   --events, -e        display number of events the sector has received (default: false)
   --seal-time         display how long it took for the sector to be sealed (default: false)
   --states value      filter sectors by a comma-separated list of states
   --unproven, -u      only show sectors which aren't in the 'Proving' state (default: false)
   --help, -h          show help (default: false)
   
```

### lotus-miner sectors refs
```
NAME:
   lotus-miner sectors refs - List References to sectors

USAGE:
   lotus-miner sectors refs [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sectors update-state
```
NAME:
   lotus-miner sectors update-state - ADVANCED: manually update the state of a sector, this may aid in error recovery

USAGE:
   lotus-miner sectors update-state [command options] <sectorNum> <newState>

OPTIONS:
   --really-do-it  pass this flag if you know what you are doing (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner sectors pledge
```
NAME:
   lotus-miner sectors pledge - store random data in a sector

USAGE:
   lotus-miner sectors pledge [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sectors check-expire
```
NAME:
   lotus-miner sectors check-expire - Inspect expiring sectors

USAGE:
   lotus-miner sectors check-expire [command options] [arguments...]

OPTIONS:
   --cutoff value  skip sectors whose current expiration is more than <cutoff> epochs from now, defaults to 60 days (default: 172800)
   --help, -h      show help (default: false)
   
```

### lotus-miner sectors expired
```
NAME:
   lotus-miner sectors expired - Get or cleanup expired sectors

USAGE:
   lotus-miner sectors expired [command options] [arguments...]

OPTIONS:
   --show-removed         show removed sectors (default: false)
   --remove-expired       remove expired sectors (default: false)
   --expired-epoch value  epoch at which to check sector expirations (default: WinningPoSt lookback epoch)
   --help, -h             show help (default: false)
   
```

### lotus-miner sectors renew
```
NAME:
   lotus-miner sectors renew - Renew expiring sectors while not exceeding each sector's max life

USAGE:
   lotus-miner sectors renew [command options] [arguments...]

OPTIONS:
   --from value            only consider sectors whose current expiration epoch is in the range of [from, to], <from> defaults to: now + 120 (1 hour) (default: 0)
   --to value              only consider sectors whose current expiration epoch is in the range of [from, to], <to> defaults to: now + 92160 (32 days) (default: 0)
   --sector-file value     provide a file containing one sector number in each line, ignoring above selecting criteria
   --exclude value         optionally provide a file containing excluding sectors
   --extension value       try to extend selected sectors by this number of epochs, defaults to 540 days (default: 1555200)
   --new-expiration value  try to extend selected sectors to this epoch, ignoring extension (default: 0)
   --tolerance value       don't try to extend sectors by fewer than this number of epochs, defaults to 7 days (default: 20160)
   --max-fee value         use up to this amount of FIL for one message. pass this flag to avoid message congestion. (default: "0")
   --really-do-it          pass this flag to really renew sectors, otherwise will only print out json representation of parameters (default: false)
   --help, -h              show help (default: false)
   
```

### lotus-miner sectors extend
```
NAME:
   lotus-miner sectors extend - Extend sector expiration

USAGE:
   lotus-miner sectors extend [command options] <sectorNumbers...>

OPTIONS:
   --new-expiration value     new expiration epoch (default: 0)
   --v1-sectors               renews all v1 sectors up to the maximum possible lifetime (default: false)
   --tolerance value          when extending v1 sectors, don't try to extend sectors by fewer than this number of epochs (default: 20160)
   --expiration-ignore value  when extending v1 sectors, skip sectors whose current expiration is less than <ignore> epochs from now (default: 120)
   --expiration-cutoff value  when extending v1 sectors, skip sectors whose current expiration is more than <cutoff> epochs from now (infinity if unspecified) (default: 0)
                              
   --help, -h                 show help (default: false)
   
```

### lotus-miner sectors terminate
```
NAME:
   lotus-miner sectors terminate - Terminate sector on-chain then remove (WARNING: This means losing power and collateral for the removed sector)

USAGE:
   lotus-miner sectors terminate command [command options] <sectorNum>

COMMANDS:
   flush    Send a terminate message if there are sectors queued for termination
   pending  List sector numbers of sectors pending termination
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --really-do-it  pass this flag if you know what you are doing (default: false)
   --help, -h      show help (default: false)
   --version, -v   print the version (default: false)
   
```

#### lotus-miner sectors terminate flush
```
NAME:
   lotus-miner sectors terminate flush - Send a terminate message if there are sectors queued for termination

USAGE:
   lotus-miner sectors terminate flush [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner sectors terminate pending
```
NAME:
   lotus-miner sectors terminate pending - List sector numbers of sectors pending termination

USAGE:
   lotus-miner sectors terminate pending [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sectors remove
```
NAME:
   lotus-miner sectors remove - Forcefully remove a sector (WARNING: This means losing power and collateral for the removed sector (use 'terminate' for lower penalty))

USAGE:
   lotus-miner sectors remove [command options] <sectorNum>

OPTIONS:
   --really-do-it  pass this flag if you know what you are doing (default: false)
   --help, -h      show help (default: false)
   
```

### lotus-miner sectors snap-up
```
NAME:
   lotus-miner sectors snap-up - Mark a committed capacity sector to be filled with deals

USAGE:
   lotus-miner sectors snap-up [command options] <sectorNum>

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sectors abort-upgrade
```
NAME:
   lotus-miner sectors abort-upgrade - Abort the attempted (SnapDeals) upgrade of a CC sector, reverting it to as before

USAGE:
   lotus-miner sectors abort-upgrade [command options] <sectorNum>

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sectors mark-for-upgrade
```
NAME:
   lotus-miner sectors mark-for-upgrade - Mark a committed capacity sector for replacement by a sector with deals

USAGE:
   lotus-miner sectors mark-for-upgrade [command options] <sectorNum>

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sectors seal
```
NAME:
   lotus-miner sectors seal - Manually start sealing a sector (filling any unused space with junk)

USAGE:
   lotus-miner sectors seal [command options] <sectorNum>

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sectors set-seal-delay
```
NAME:
   lotus-miner sectors set-seal-delay - Set the time, in minutes, that a new sector waits for deals before sealing starts

USAGE:
   lotus-miner sectors set-seal-delay [command options] <minutes>

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sectors get-cc-collateral
```
NAME:
   lotus-miner sectors get-cc-collateral - Get the collateral required to pledge a committed capacity sector

USAGE:
   lotus-miner sectors get-cc-collateral [command options] [arguments...]

OPTIONS:
   --expiration value  the epoch when the sector will expire (default: 0)
   --help, -h          show help (default: false)
   
```

### lotus-miner sectors batching
```
NAME:
   lotus-miner sectors batching - manage batch sector operations

USAGE:
   lotus-miner sectors batching command [command options] [arguments...]

COMMANDS:
   commit     list sectors waiting in commit batch queue
   precommit  list sectors waiting in precommit batch queue
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

#### lotus-miner sectors batching commit
```
NAME:
   lotus-miner sectors batching commit - list sectors waiting in commit batch queue

USAGE:
   lotus-miner sectors batching commit [command options] [arguments...]

OPTIONS:
   --publish-now  send a batch now (default: false)
   --help, -h     show help (default: false)
   
```

#### lotus-miner sectors batching precommit
```
NAME:
   lotus-miner sectors batching precommit - list sectors waiting in precommit batch queue

USAGE:
   lotus-miner sectors batching precommit [command options] [arguments...]

OPTIONS:
   --publish-now  send a batch now (default: false)
   --help, -h     show help (default: false)
   
```

### lotus-miner sectors match-pending-pieces
```
NAME:
   lotus-miner sectors match-pending-pieces - force a refreshed match of pending pieces to open sectors without manually waiting for more deals

USAGE:
   lotus-miner sectors match-pending-pieces [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner proving
```
NAME:
   lotus-miner proving - View proving information

USAGE:
   lotus-miner proving command [command options] [arguments...]

COMMANDS:
   info       View current state information
   deadlines  View the current proving period deadlines information
   deadline   View the current proving period deadline information by its index 
   faults     View the currently known proving faulty sectors information
   check      Check sectors provable
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner proving info
```
NAME:
   lotus-miner proving info - View current state information

USAGE:
   lotus-miner proving info [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner proving deadlines
```
NAME:
   lotus-miner proving deadlines - View the current proving period deadlines information

USAGE:
   lotus-miner proving deadlines [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner proving deadline
```
NAME:
   lotus-miner proving deadline - View the current proving period deadline information by its index 

USAGE:
   lotus-miner proving deadline [command options] <deadlineIdx>

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner proving faults
```
NAME:
   lotus-miner proving faults - View the currently known proving faulty sectors information

USAGE:
   lotus-miner proving faults [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner proving check
```
NAME:
   lotus-miner proving check - Check sectors provable

USAGE:
   lotus-miner proving check [command options] <deadlineIdx>

OPTIONS:
   --only-bad          print only bad sectors (default: false)
   --slow              run slower checks (default: false)
   --storage-id value  filter sectors by storage path (path id)
   --help, -h          show help (default: false)
   
```

## lotus-miner storage
```
NAME:
   lotus-miner storage - manage sector storage

USAGE:
   lotus-miner storage command [command options] [arguments...]

DESCRIPTION:
   Sectors can be stored across many filesystem paths. These
commands provide ways to manage the storage the miner will used to store sectors
long term for proving (references as 'store') as well as how sectors will be
stored while moving through the sealing pipeline (references as 'seal').

COMMANDS:
   attach   attach local storage path
   list     list local storage paths
   find     find sector in the storage system
   cleanup  trigger cleanup actions
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner storage attach
```
NAME:
   lotus-miner storage attach - attach local storage path

USAGE:
   lotus-miner storage attach [command options] [arguments...]

DESCRIPTION:
   Storage can be attached to the miner using this command. The storage volume
list is stored local to the miner in $LOTUS_MINER_PATH/storage.json. We do not
recommend manually modifying this value without further understanding of the
storage system.

Each storage volume contains a configuration file which describes the
capabilities of the volume. When the '--init' flag is provided, this file will
be created using the additional flags.

Weight
A high weight value means data will be more likely to be stored in this path

Seal
Data for the sealing process will be stored here

Store
Finalized sectors that will be moved here for long term storage and be proven
over time
   

OPTIONS:
   --init               initialize the path first (default: false)
   --weight value       (for init) path weight (default: 10)
   --seal               (for init) use path for sealing (default: false)
   --store              (for init) use path for long-term storage (default: false)
   --max-storage value  (for init) limit storage space for sectors (expensive for very large paths!)
   --groups value       path group names
   --allow-to value     path groups allowed to pull data from this path (allow all if not specified)
   --help, -h           show help (default: false)
   
```

### lotus-miner storage list
```
NAME:
   lotus-miner storage list - list local storage paths

USAGE:
   lotus-miner storage list command [command options] [arguments...]

COMMANDS:
   sectors  get list of all sector files
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --color        use color in display output (default: depends on output being a TTY)
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

#### lotus-miner storage list sectors
```
NAME:
   lotus-miner storage list sectors - get list of all sector files

USAGE:
   lotus-miner storage list sectors [command options] [arguments...]

OPTIONS:
   --color     use color in display output (default: depends on output being a TTY)
   --help, -h  show help (default: false)
   
```

### lotus-miner storage find
```
NAME:
   lotus-miner storage find - find sector in the storage system

USAGE:
   lotus-miner storage find [command options] [sector number]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner storage cleanup
```
NAME:
   lotus-miner storage cleanup - trigger cleanup actions

USAGE:
   lotus-miner storage cleanup [command options] [arguments...]

OPTIONS:
   --removed   cleanup remaining files from removed sectors (default: true)
   --help, -h  show help (default: false)
   
```

## lotus-miner sealing
```
NAME:
   lotus-miner sealing - interact with sealing pipeline

USAGE:
   lotus-miner sealing command [command options] [arguments...]

COMMANDS:
   jobs        list running jobs
   workers     list workers
   sched-diag  Dump internal scheduler state
   abort       Abort a running job
   help, h     Shows a list of commands or help for one command

OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
   
```

### lotus-miner sealing jobs
```
NAME:
   lotus-miner sealing jobs - list running jobs

USAGE:
   lotus-miner sealing jobs [command options] [arguments...]

OPTIONS:
   --color          use color in display output (default: depends on output being a TTY)
   --show-ret-done  show returned but not consumed calls (default: false)
   --help, -h       show help (default: false)
   
```

### lotus-miner sealing workers
```
NAME:
   lotus-miner sealing workers - list workers

USAGE:
   lotus-miner sealing workers [command options] [arguments...]

OPTIONS:
   --color     use color in display output (default: depends on output being a TTY)
   --help, -h  show help (default: false)
   
```

### lotus-miner sealing sched-diag
```
NAME:
   lotus-miner sealing sched-diag - Dump internal scheduler state

USAGE:
   lotus-miner sealing sched-diag [command options] [arguments...]

OPTIONS:
   --force-sched  (default: false)
   --help, -h     show help (default: false)
   
```

### lotus-miner sealing abort
```
NAME:
   lotus-miner sealing abort - Abort a running job

USAGE:
   lotus-miner sealing abort [command options] [callid]

OPTIONS:
   --help, -h  show help (default: false)
   
```
