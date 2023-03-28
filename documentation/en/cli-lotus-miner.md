# lotus-miner
```
NAME:
   lotus-miner - Filecoin decentralized storage network miner

USAGE:
   lotus-miner [global options] command [command options] [arguments...]

VERSION:
   1.21.0-dev

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
     index            Manage the index provider on the markets subsystem
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
   --call-on-markets                        (experimental; may be removed) call this command against a markets node; use only with common commands like net, auth, pprof, etc. whose target may be ambiguous (default: false)
   --color                                  use color in display output (default: depends on output being a TTY)
   --help, -h                               show help (default: false)
   --markets-repo value                     Markets repo path [$LOTUS_MARKETS_PATH]
   --miner-repo value, --storagerepo value  Specify miner repo path. flag(storagerepo) and env(LOTUS_STORAGE_PATH) are DEPRECATION, will REMOVE SOON (default: "~/.lotusminer") [$LOTUS_MINER_PATH, $LOTUS_STORAGE_PATH]
   --version, -v                            print the version (default: false)
   --vv                                     enables very verbose mode, useful for debugging the CLI (default: false)
   
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
   --actor value                                              specify the address of an already created miner actor
   --create-worker-key                                        create separate worker key (default: false)
   --worker value, -w value                                   worker key to use (overrides --create-worker-key)
   --owner value, -o value                                    owner key to use
   --sector-size value                                        specify sector size to use
   --pre-sealed-sectors value [ --pre-sealed-sectors value ]  specify set of presealed sectors for starting as a genesis miner
   --pre-sealed-metadata value                                specify the metadata file for the presealed sectors
   --nosync                                                   don't check full-node sync status (default: false)
   --symlink-imported-sectors                                 attempt to symlink to presealed sectors instead of copying them into place (default: false)
   --no-local-storage                                         don't use storageminer repo for sector storage (default: false)
   --gas-premium value                                        set gas premium for initialization messages in AttoFIL (default: "0")
   --from value                                               select which address to send actor creation message from
   --help, -h                                                 show help (default: false)
   
```

### lotus-miner init restore
```
NAME:
   lotus-miner init restore - Initialize a lotus miner repo from a backup

USAGE:
   lotus-miner init restore [command options] [backupFile]

OPTIONS:
   --config value          config file (config.toml)
   --nosync                don't check full-node sync status (default: false)
   --storage-config value  storage paths config (storage.json)
   
```

### lotus-miner init service
```
NAME:
   lotus-miner init service - Initialize a lotus miner sub-service

USAGE:
   lotus-miner init service [command options] [backupFile]

OPTIONS:
   --api-sealer value             sealer API info (lotus-miner auth api-info --perm=admin)
   --api-sector-index value       sector Index API info (lotus-miner auth api-info --perm=admin)
   --config value                 config file (config.toml)
   --nosync                       don't check full-node sync status (default: false)
   --type value [ --type value ]  type of service to be enabled
   
```

## lotus-miner run
```
NAME:
   lotus-miner run - Start a lotus miner process

USAGE:
   lotus-miner run [command options] [arguments...]

OPTIONS:
   --enable-gpu-proving  enable use of GPU for mining operations (default: true)
   --manage-fdlimit      manage open file limit (default: true)
   --miner-api value     2345
   --nosync              don't check full-node sync status (default: false)
   
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
   --help, -h  show help (default: false)
   
```

### lotus-miner config default
```
NAME:
   lotus-miner config default - Print default node config

USAGE:
   lotus-miner config default [command options] [arguments...]

OPTIONS:
   --no-comment  don't comment default values (default: false)
   
```

### lotus-miner config updated
```
NAME:
   lotus-miner config updated - Print updated node config

USAGE:
   lotus-miner config updated [command options] [arguments...]

OPTIONS:
   --no-comment  don't comment default values (default: false)
   
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
   --offline  create backup without the node running (default: false)
   
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
     set-addresses, set-addrs    set addresses that your miner can be publicly dialed on
     withdraw                    withdraw available balance to beneficiary
     repay-debt                  pay down a miner's debt
     set-peer-id                 set the peer id of your miner
     set-owner                   Set owner address (this command should be invoked twice, first with the old owner as the senderAddress, and then with the new owner)
     control                     Manage control addresses
     propose-change-worker       Propose a worker address change
     confirm-change-worker       Confirm a worker address change
     compact-allocated           compact allocated sectors bitfield
     propose-change-beneficiary  Propose a beneficiary address change
     confirm-change-beneficiary  Confirm a beneficiary address change
     help, h                     Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner actor set-addresses, set-addrs
```
```

### lotus-miner actor withdraw
```
NAME:
   lotus-miner actor withdraw - withdraw available balance to beneficiary

USAGE:
   lotus-miner actor withdraw [command options] [amount (FIL)]

OPTIONS:
   --beneficiary       send withdraw message from the beneficiary address (default: false)
   --confidence value  number of block confirmations to wait for (default: 5)
   
```

### lotus-miner actor repay-debt
```
NAME:
   lotus-miner actor repay-debt - pay down a miner's debt

USAGE:
   lotus-miner actor repay-debt [command options] [amount (FIL)]

OPTIONS:
   --from value  optionally specify the account to send funds from
   
```

### lotus-miner actor set-peer-id
```
NAME:
   lotus-miner actor set-peer-id - set the peer id of your miner

USAGE:
   lotus-miner actor set-peer-id [command options] <peer id>

OPTIONS:
   --gas-limit value  set gas limit (default: 0)
   
```

### lotus-miner actor set-owner
```
NAME:
   lotus-miner actor set-owner - Set owner address (this command should be invoked twice, first with the old owner as the senderAddress, and then with the new owner)

USAGE:
   lotus-miner actor set-owner [command options] [newOwnerAddress senderAddress]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   
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
   --help, -h  show help (default: false)
   
```

#### lotus-miner actor control list
```
NAME:
   lotus-miner actor control list - Get currently set control addresses

USAGE:
   lotus-miner actor control list [command options] [arguments...]

OPTIONS:
   --verbose  (default: false)
   
```

#### lotus-miner actor control set
```
NAME:
   lotus-miner actor control set - Set control address(-es)

USAGE:
   lotus-miner actor control set [command options] [...address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   
```

### lotus-miner actor propose-change-worker
```
NAME:
   lotus-miner actor propose-change-worker - Propose a worker address change

USAGE:
   lotus-miner actor propose-change-worker [command options] [address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   
```

### lotus-miner actor confirm-change-worker
```
NAME:
   lotus-miner actor confirm-change-worker - Confirm a worker address change

USAGE:
   lotus-miner actor confirm-change-worker [command options] [address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   
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
   
```

### lotus-miner actor propose-change-beneficiary
```
NAME:
   lotus-miner actor propose-change-beneficiary - Propose a beneficiary address change

USAGE:
   lotus-miner actor propose-change-beneficiary [command options] [beneficiaryAddress quota expiration]

OPTIONS:
   --actor value               specify the address of miner actor
   --overwrite-pending-change  Overwrite the current beneficiary change proposal (default: false)
   --really-do-it              Actually send transaction performing the action (default: false)
   
```

### lotus-miner actor confirm-change-beneficiary
```
NAME:
   lotus-miner actor confirm-change-beneficiary - Confirm a beneficiary address change

USAGE:
   lotus-miner actor confirm-change-beneficiary [command options] [minerAddress]

OPTIONS:
   --existing-beneficiary  send confirmation from the existing beneficiary address (default: false)
   --new-beneficiary       send confirmation from the new beneficiary address (default: false)
   --really-do-it          Actually send transaction performing the action (default: false)
   
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
   --help, -h  show help (default: false)
   
```

### lotus-miner auth create-token
```
NAME:
   lotus-miner auth create-token - Create token

USAGE:
   lotus-miner auth create-token [command options] [arguments...]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   
```

### lotus-miner auth api-info
```
NAME:
   lotus-miner auth api-info - Get token with API info required to connect to this node

USAGE:
   lotus-miner auth api-info [command options] [arguments...]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   
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
   --help, -h  show help (default: false)
   
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
   --system value [ --system value ]  limit to log system
   
```

### lotus-miner log alerts
```
NAME:
   lotus-miner log alerts - Get alert states

USAGE:
   lotus-miner log alerts [command options] [arguments...]

OPTIONS:
   --all  get all (active and inactive) alerts (default: false)
   
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
   --help, -h  show help (default: false)
   
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
   --help, -h  show help (default: false)
   
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
   --offline     (default: false)
   --online      (default: false)
   --unverified  (default: false)
   --verified    (default: false)
   
```

### lotus-miner storage-deals set-ask
```
NAME:
   lotus-miner storage-deals set-ask - Configure the miner's ask

USAGE:
   lotus-miner storage-deals set-ask [command options] [arguments...]

OPTIONS:
   --max-piece-size SIZE   Set maximum piece size (w/bit-padding, in bytes) in ask to SIZE (default: miner sector size)
   --min-piece-size SIZE   Set minimum piece size (w/bit-padding, in bytes) in ask to SIZE (default: 256B)
   --price PRICE           Set the price of the ask for unverified deals (specified as FIL / GiB / Epoch) to PRICE.
   --verified-price PRICE  Set the price of the ask for verified deals (specified as FIL / GiB / Epoch) to PRICE
   
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
     set-ask    Configure the provider's retrieval ask
     get-ask    Get the provider's current retrieval ask configured by the provider in the ask-store using the set-ask CLI command
     help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
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
   --help, -h  show help (default: false)
   
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
   --offline  (default: false)
   --online   (default: false)
   
```

### lotus-miner retrieval-deals set-ask
```
NAME:
   lotus-miner retrieval-deals set-ask - Configure the provider's retrieval ask

USAGE:
   lotus-miner retrieval-deals set-ask [command options] [arguments...]

OPTIONS:
   --payment-interval value           Set the payment interval (in bytes) for retrieval (default: 1MiB)
   --payment-interval-increase value  Set the payment interval increase (in bytes) for retrieval (default: 1MiB)
   --price value                      Set the price of the ask for retrievals (FIL/GiB)
   --unseal-price value               Set the price to unseal
   
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
     list         List ongoing data transfers for this miner
     restart      Force restart a stalled data transfer
     cancel       Force cancel a data transfer
     diagnostics  Get detailed diagnostics on active transfers with a specific peer
     help, h      Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner data-transfers list
```
NAME:
   lotus-miner data-transfers list - List ongoing data transfers for this miner

USAGE:
   lotus-miner data-transfers list [command options] [arguments...]

OPTIONS:
   --completed    show completed data transfers (default: false)
   --show-failed  show failed/cancelled transfers (default: false)
   --verbose, -v  print verbose transfer details (default: false)
   --watch        watch deal updates in real-time, rather than a one time list (default: false)
   
```

### lotus-miner data-transfers restart
```
NAME:
   lotus-miner data-transfers restart - Force restart a stalled data transfer

USAGE:
   lotus-miner data-transfers restart [command options] [arguments...]

OPTIONS:
   --initiator     specify only transfers where peer is/is not initiator (default: false)
   --peerid value  narrow to transfer with specific peer
   
```

### lotus-miner data-transfers cancel
```
NAME:
   lotus-miner data-transfers cancel - Force cancel a data transfer

USAGE:
   lotus-miner data-transfers cancel [command options] [arguments...]

OPTIONS:
   --cancel-timeout value  time to wait for cancel to be sent to client (default: 5s)
   --initiator             specify only transfers where peer is/is not initiator (default: false)
   --peerid value          narrow to transfer with specific peer
   
```

### lotus-miner data-transfers diagnostics
```
NAME:
   lotus-miner data-transfers diagnostics - Get detailed diagnostics on active transfers with a specific peer

USAGE:
   lotus-miner data-transfers diagnostics [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner dagstore
```
NAME:
   lotus-miner dagstore - Manage the dagstore on the markets subsystem

USAGE:
   lotus-miner dagstore command [command options] [arguments...]

COMMANDS:
     list-shards       List all shards known to the dagstore, with their current status
     register-shard    Register a shard
     initialize-shard  Initialize the specified shard
     recover-shard     Attempt to recover a shard in errored state
     initialize-all    Initialize all uninitialized shards, streaming results as they're produced; only shards for unsealed pieces are initialized by default
     gc                Garbage collect the dagstore
     lookup-pieces     Lookup pieces that a given CID belongs to
     help, h           Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner dagstore list-shards
```
NAME:
   lotus-miner dagstore list-shards - List all shards known to the dagstore, with their current status

USAGE:
   lotus-miner dagstore list-shards [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner dagstore register-shard
```
NAME:
   lotus-miner dagstore register-shard - Register a shard

USAGE:
   lotus-miner dagstore register-shard [command options] [key]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner dagstore initialize-shard
```
NAME:
   lotus-miner dagstore initialize-shard - Initialize the specified shard

USAGE:
   lotus-miner dagstore initialize-shard [command options] [key]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner dagstore recover-shard
```
NAME:
   lotus-miner dagstore recover-shard - Attempt to recover a shard in errored state

USAGE:
   lotus-miner dagstore recover-shard [command options] [key]

OPTIONS:
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
   
```

### lotus-miner dagstore gc
```
NAME:
   lotus-miner dagstore gc - Garbage collect the dagstore

USAGE:
   lotus-miner dagstore gc [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner dagstore lookup-pieces
```
NAME:
   lotus-miner dagstore lookup-pieces - Lookup pieces that a given CID belongs to

USAGE:
   lotus-miner dagstore lookup-pieces [command options] <cid>

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner index
```
NAME:
   lotus-miner index - Manage the index provider on the markets subsystem

USAGE:
   lotus-miner index command [command options] [arguments...]

COMMANDS:
     announce      Announce a deal to indexers so they can download its index
     announce-all  Announce all active deals to indexers so they can download the indices
     help, h       Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner index announce
```
NAME:
   lotus-miner index announce - Announce a deal to indexers so they can download its index

USAGE:
   lotus-miner index announce [command options] <deal proposal cid>

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner index announce-all
```
NAME:
   lotus-miner index announce-all - Announce all active deals to indexers so they can download the indices

USAGE:
   lotus-miner index announce-all [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

## lotus-miner net
```
NAME:
   lotus-miner net - Manage P2P Network

USAGE:
   lotus-miner net command [command options] [arguments...]

COMMANDS:
     peers                Print peers
     ping                 Ping peers
     connect              Connect to a peer
     disconnect           Disconnect from a peer
     listen               List listen addresses
     id                   Get node identity
     find-peer, findpeer  Find the addresses of a given peerID
     scores               Print peers' pubsub scores
     reachability         Print information about reachability from the internet
     bandwidth            Print bandwidth usage information
     block                Manage network connection gating rules
     stat                 Report resource usage for a scope
     limit                Get or set resource limits for a scope
     protect              Add one or more peer IDs to the list of protected peer connections
     unprotect            Remove one or more peer IDs from the list of protected peer connections.
     list-protected       List the peer IDs with protected connection.
     help, h              Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
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
   
```

### lotus-miner net ping
```
NAME:
   lotus-miner net ping - Ping peers

USAGE:
   lotus-miner net ping [command options] [peerMultiaddr]

OPTIONS:
   --count value, -c value     specify the number of times it should ping (default: 10)
   --interval value, -i value  minimum time between pings (default: 1s)
   
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

### lotus-miner net disconnect
```
NAME:
   lotus-miner net disconnect - Disconnect from a peer

USAGE:
   lotus-miner net disconnect [command options] [peerID]

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

#### lotus-miner net find-peer, findpeer
```
```

### lotus-miner net scores
```
NAME:
   lotus-miner net scores - Print peers' pubsub scores

USAGE:
   lotus-miner net scores [command options] [arguments...]

OPTIONS:
   --extended, -x  print extended peer scores in json (default: false)
   
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
   --help, -h  show help (default: false)
   
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
   --help, -h  show help (default: false)
   
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
   --help, -h  show help (default: false)
   
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

### lotus-miner net stat
```
NAME:
   lotus-miner net stat - Report resource usage for a scope

USAGE:
   lotus-miner net stat [command options] scope

DESCRIPTION:
   Report resource usage for a scope.
   
     The scope can be one of the following:
     - system        -- reports the system aggregate resource usage.
     - transient     -- reports the transient resource usage.
     - svc:<service> -- reports the resource usage of a specific service.
     - proto:<proto> -- reports the resource usage of a specific protocol.
     - peer:<peer>   -- reports the resource usage of a specific peer.
     - all           -- reports the resource usage for all currently active scopes.
   

OPTIONS:
   --json  (default: false)
   
```

### lotus-miner net limit
```
NAME:
   lotus-miner net limit - Get or set resource limits for a scope

USAGE:
   lotus-miner net limit [command options] scope [limit]

DESCRIPTION:
   Get or set resource limits for a scope.
   
     The scope can be one of the following:
     - system        -- reports the system aggregate resource usage.
     - transient     -- reports the transient resource usage.
     - svc:<service> -- reports the resource usage of a specific service.
     - proto:<proto> -- reports the resource usage of a specific protocol.
     - peer:<peer>   -- reports the resource usage of a specific peer.
   
    The limit is json-formatted, with the same structure as the limits file.
   

OPTIONS:
   --set  set the limit for a scope (default: false)
   
```

### lotus-miner net protect
```
NAME:
   lotus-miner net protect - Add one or more peer IDs to the list of protected peer connections

USAGE:
   lotus-miner net protect [command options] <peer-id> [<peer-id>...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner net unprotect
```
NAME:
   lotus-miner net unprotect - Remove one or more peer IDs from the list of protected peer connections.

USAGE:
   lotus-miner net unprotect [command options] <peer-id> [<peer-id>...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner net list-protected
```
NAME:
   lotus-miner net list-protected - List the peer IDs with protected connection.

USAGE:
   lotus-miner net list-protected [command options] [arguments...]

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
   --help, -h  show help (default: false)
   
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
     numbers               manage sector number assignments
     precommits            Print on-chain precommit info
     check-expire          Inspect expiring sectors
     expired               Get or cleanup expired sectors
     extend                Extend expiring sectors while not exceeding each sector's max life
     terminate             Terminate sector on-chain then remove (WARNING: This means losing power and collateral for the removed sector)
     remove                Forcefully remove a sector (WARNING: This means losing power and collateral for the removed sector (use 'terminate' for lower penalty))
     snap-up               Mark a committed capacity sector to be filled with deals
     abort-upgrade         Abort the attempted (SnapDeals) upgrade of a CC sector, reverting it to as before
     seal                  Manually start sealing a sector (filling any unused space with junk)
     set-seal-delay        Set the time (in minutes) that a new sector waits for deals before sealing starts
     get-cc-collateral     Get the collateral required to pledge a committed capacity sector
     batching              manage batch sector operations
     match-pending-pieces  force a refreshed match of pending pieces to open sectors without manually waiting for more deals
     compact-partitions    removes dead sectors from partitions and reduces the number of partitions used if possible
     help, h               Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
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
   
```

### lotus-miner sectors list
```
NAME:
   lotus-miner sectors list - List sectors

USAGE:
   lotus-miner sectors list [command options] [arguments...]

OPTIONS:
   --check-parallelism value  number of parallel requests to make for checking sector states (default: 300)
   --events, -e               display number of events the sector has received (default: false)
   --fast, -f                 don't show on-chain info for better performance (default: false)
   --initial-pledge, -p       display initial pledge (default: false)
   --seal-time, -t            display how long it took for the sector to be sealed (default: false)
   --show-removed, -r         show removed sectors (default: false)
   --states value             filter sectors by a comma-separated list of states
   --unproven, -u             only show sectors which aren't in the 'Proving' state (default: false)
   
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

### lotus-miner sectors numbers
```
NAME:
   lotus-miner sectors numbers - manage sector number assignments

USAGE:
   lotus-miner sectors numbers command [command options] [arguments...]

COMMANDS:
     info          view sector assigner state
     reservations  list sector number reservations
     reserve       create sector number reservations
     free          remove sector number reservations
     help, h       Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner sectors numbers info
```
NAME:
   lotus-miner sectors numbers info - view sector assigner state

USAGE:
   lotus-miner sectors numbers info [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner sectors numbers reservations
```
NAME:
   lotus-miner sectors numbers reservations - list sector number reservations

USAGE:
   lotus-miner sectors numbers reservations [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

#### lotus-miner sectors numbers reserve
```
NAME:
   lotus-miner sectors numbers reserve - create sector number reservations

USAGE:
   lotus-miner sectors numbers reserve [command options] [reservation name] [reserved ranges]

OPTIONS:
   --force  skip duplicate reservation checks (note: can lead to damaging other reservations on free) (default: false)
   
```

#### lotus-miner sectors numbers free
```
NAME:
   lotus-miner sectors numbers free - remove sector number reservations

USAGE:
   lotus-miner sectors numbers free [command options] [reservation name]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sectors precommits
```
NAME:
   lotus-miner sectors precommits - Print on-chain precommit info

USAGE:
   lotus-miner sectors precommits [command options] [arguments...]

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
   
```

### lotus-miner sectors expired
```
NAME:
   lotus-miner sectors expired - Get or cleanup expired sectors

USAGE:
   lotus-miner sectors expired [command options] [arguments...]

OPTIONS:
   --expired-epoch value  epoch at which to check sector expirations (default: WinningPoSt lookback epoch)
   --remove-expired       remove expired sectors (default: false)
   --show-removed         show removed sectors (default: false)
   
```

### lotus-miner sectors extend
```
NAME:
   lotus-miner sectors extend - Extend expiring sectors while not exceeding each sector's max life

USAGE:
   lotus-miner sectors extend [command options] <sectorNumbers...(optional)>

OPTIONS:
   --drop-claims           drop claims for sectors that can be extended, but only by dropping some of their verified power claims (default: false)
   --exclude value         optionally provide a file containing excluding sectors
   --extension value       try to extend selected sectors by this number of epochs, defaults to 540 days (default: 1555200)
   --from value            only consider sectors whose current expiration epoch is in the range of [from, to], <from> defaults to: now + 120 (1 hour) (default: 0)
   --max-fee value         use up to this amount of FIL for one message. pass this flag to avoid message congestion. (default: "0")
   --max-sectors value     the maximum number of sectors contained in each message (default: 0)
   --new-expiration value  try to extend selected sectors to this epoch, ignoring extension (default: 0)
   --only-cc               only extend CC sectors (useful for making sector ready for snap upgrade) (default: false)
   --really-do-it          pass this flag to really extend sectors, otherwise will only print out json representation of parameters (default: false)
   --sector-file value     provide a file containing one sector number in each line, ignoring above selecting criteria
   --to value              only consider sectors whose current expiration epoch is in the range of [from, to], <to> defaults to: now + 92160 (32 days) (default: 0)
   --tolerance value       don't try to extend sectors by fewer than this number of epochs, defaults to 7 days (default: 20160)
   
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
   --really-do-it  pass this flag if you know what you are doing (default: false)
   
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
   lotus-miner sectors set-seal-delay - Set the time (in minutes) that a new sector waits for deals before sealing starts

USAGE:
   lotus-miner sectors set-seal-delay [command options] <time>

OPTIONS:
   --seconds  Specifies that the time argument should be in seconds (default: false)
   
```

### lotus-miner sectors get-cc-collateral
```
NAME:
   lotus-miner sectors get-cc-collateral - Get the collateral required to pledge a committed capacity sector

USAGE:
   lotus-miner sectors get-cc-collateral [command options] [arguments...]

OPTIONS:
   --expiration value  the epoch when the sector will expire (default: 0)
   
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
   --help, -h  show help (default: false)
   
```

#### lotus-miner sectors batching commit
```
NAME:
   lotus-miner sectors batching commit - list sectors waiting in commit batch queue

USAGE:
   lotus-miner sectors batching commit [command options] [arguments...]

OPTIONS:
   --publish-now  send a batch now (default: false)
   
```

#### lotus-miner sectors batching precommit
```
NAME:
   lotus-miner sectors batching precommit - list sectors waiting in precommit batch queue

USAGE:
   lotus-miner sectors batching precommit [command options] [arguments...]

OPTIONS:
   --publish-now  send a batch now (default: false)
   
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

### lotus-miner sectors compact-partitions
```
NAME:
   lotus-miner sectors compact-partitions - removes dead sectors from partitions and reduces the number of partitions used if possible

USAGE:
   lotus-miner sectors compact-partitions [command options] [arguments...]

OPTIONS:
   --actor value                              Specify the address of the miner to run this command
   --deadline value                           the deadline to compact the partitions in (default: 0)
   --partitions value [ --partitions value ]  list of partitions to compact sectors in
   --really-do-it                             Actually send transaction performing the action (default: false)
   
```

## lotus-miner proving
```
NAME:
   lotus-miner proving - View proving information

USAGE:
   lotus-miner proving command [command options] [arguments...]

COMMANDS:
     info            View current state information
     deadlines       View the current proving period deadlines information
     deadline        View the current proving period deadline information by its index
     faults          View the currently known proving faulty sectors information
     check           Check sectors provable
     workers         list workers
     compute         Compute simulated proving tasks
     recover-faults  Manually recovers faulty sectors on chain
     help, h         Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
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
   --all, -a  Count all sectors (only live sectors are counted by default) (default: false)
   
```

### lotus-miner proving deadline
```
NAME:
   lotus-miner proving deadline - View the current proving period deadline information by its index

USAGE:
   lotus-miner proving deadline [command options] <deadlineIdx>

OPTIONS:
   --bitfield, -b     Print partition bitfield stats (default: false)
   --sector-nums, -n  Print sector/fault numbers belonging to this deadline (default: false)
   
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
   --faulty            only check faulty sectors (default: false)
   --only-bad          print only bad sectors (default: false)
   --slow              run slower checks (default: false)
   --storage-id value  filter sectors by storage path (path id)
   
```

### lotus-miner proving workers
```
NAME:
   lotus-miner proving workers - list workers

USAGE:
   lotus-miner proving workers [command options] [arguments...]

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner proving compute
```
NAME:
   lotus-miner proving compute - Compute simulated proving tasks

USAGE:
   lotus-miner proving compute command [command options] [arguments...]

COMMANDS:
     windowed-post, window-post  Compute WindowPoSt for a specific deadline
     help, h                     Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

##### lotus-miner proving compute windowed-post, window-post
```
```

### lotus-miner proving recover-faults
```
NAME:
   lotus-miner proving recover-faults - Manually recovers faulty sectors on chain

USAGE:
   lotus-miner proving recover-faults [command options] <faulty sectors>

OPTIONS:
   --confidence value  number of block confirmations to wait for (default: 5)
   
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
     attach     attach local storage path
     detach     detach local storage path
     redeclare  redeclare sectors in a local storage path
     list       list local storage paths
     find       find sector in the storage system
     cleanup    trigger cleanup actions
     locks      show active sector locks
     help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner storage attach
```
NAME:
   lotus-miner storage attach - attach local storage path

USAGE:
   lotus-miner storage attach [command options] [path]

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
   --allow-to value [ --allow-to value ]  path groups allowed to pull data from this path (allow all if not specified)
   --groups value [ --groups value ]      path group names
   --init                                 initialize the path first (default: false)
   --max-storage value                    (for init) limit storage space for sectors (expensive for very large paths!)
   --seal                                 (for init) use path for sealing (default: false)
   --store                                (for init) use path for long-term storage (default: false)
   --weight value                         (for init) path weight (default: 10)
   
```

### lotus-miner storage detach
```
NAME:
   lotus-miner storage detach - detach local storage path

USAGE:
   lotus-miner storage detach [command options] [path]

OPTIONS:
   --really-do-it  (default: false)
   
```

### lotus-miner storage redeclare
```
NAME:
   lotus-miner storage redeclare - redeclare sectors in a local storage path

USAGE:
   lotus-miner storage redeclare [command options] [path]

OPTIONS:
   --all           redeclare all storage paths (default: false)
   --drop-missing  Drop index entries with missing files (default: false)
   --id value      storage path ID
   
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
   --help, -h  show help (default: false)
   
```

#### lotus-miner storage list sectors
```
NAME:
   lotus-miner storage list sectors - get list of all sector files

USAGE:
   lotus-miner storage list sectors [command options] [arguments...]

OPTIONS:
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
   --removed  cleanup remaining files from removed sectors (default: true)
   
```

### lotus-miner storage locks
```
NAME:
   lotus-miner storage locks - show active sector locks

USAGE:
   lotus-miner storage locks [command options] [arguments...]

OPTIONS:
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
     data-cid    Compute data CID using workers
     help, h     Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
   
```

### lotus-miner sealing jobs
```
NAME:
   lotus-miner sealing jobs - list running jobs

USAGE:
   lotus-miner sealing jobs [command options] [arguments...]

OPTIONS:
   --show-ret-done  show returned but not consumed calls (default: false)
   
```

### lotus-miner sealing workers
```
NAME:
   lotus-miner sealing workers - list workers

USAGE:
   lotus-miner sealing workers [command options] [arguments...]

OPTIONS:
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
   
```

### lotus-miner sealing abort
```
NAME:
   lotus-miner sealing abort - Abort a running job

USAGE:
   lotus-miner sealing abort [command options] [callid]

OPTIONS:
   --sched  Specifies that the argument is UUID of the request to be removed from scheduler (default: false)
   
```

### lotus-miner sealing data-cid
```
NAME:
   lotus-miner sealing data-cid - Compute data CID using workers

USAGE:
   lotus-miner sealing data-cid [command options] [file/url] <padded piece size>

OPTIONS:
   --file-size value  real file size (default: 0)
   
```
