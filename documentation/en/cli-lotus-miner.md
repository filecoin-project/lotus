# lotus-miner

```
NAME:
   lotus-miner - Filecoin decentralized storage network miner

USAGE:
   lotus-miner [global options] command [command options]

VERSION:
   1.34.4-dev

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
   STORAGE:
     sectors  interact with sector store
     proving  View proving information
     storage  manage sector storage
     sealing  interact with sealing pipeline

GLOBAL OPTIONS:
   --actor value, -a value                  specify other actor to query / manipulate
   --color                                  use color in display output (default: depends on output being a TTY)
   --miner-repo value, --storagerepo value  Specify miner repo path. flag(storagerepo) and env(LOTUS_STORAGE_PATH) are DEPRECATION, will REMOVE SOON (default: "~/.lotusminer") [$LOTUS_MINER_PATH, $LOTUS_STORAGE_PATH]
   --vv                                     enables very verbose mode, useful for debugging the CLI (default: false)
   --help, -h                               show help
   --version, -v                            print the version
```

## lotus-miner init

```
NAME:
   lotus-miner init - Initialize a lotus miner repo

USAGE:
   lotus-miner init [command options]

COMMANDS:
   restore  Initialize a lotus miner repo from a backup
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
   --confidence value                                         number of block confirmations to wait for (default: 5)
   --deposit-margin-factor value                              Multiplier (>=1.0) to scale the suggested deposit for on-chain variance (e.g. 1.01 adds 1%) (default: 1.01)
   --help, -h                                                 show help
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
   --help, -h              show help
```

## lotus-miner run

```
NAME:
   lotus-miner run - Start a lotus miner process

USAGE:
   lotus-miner run [command options]

OPTIONS:
   --miner-api value     2345
   --enable-gpu-proving  enable use of GPU for mining operations (default: true)
   --nosync              don't check full-node sync status (default: false)
   --manage-fdlimit      manage open file limit (default: true)
   --help, -h            show help
```

## lotus-miner stop

```
NAME:
   lotus-miner stop - Stop a running lotus miner

USAGE:
   lotus-miner stop [command options]

OPTIONS:
   --help, -h  show help
```

## lotus-miner config

```
NAME:
   lotus-miner config - Manage node config

USAGE:
   lotus-miner config [command options]

COMMANDS:
   default  Print default node config
   updated  Print updated node config
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-miner config default

```
NAME:
   lotus-miner config default - Print default node config

USAGE:
   lotus-miner config default [command options]

OPTIONS:
   --no-comment  don't comment default values (default: false)
   --help, -h    show help
```

### lotus-miner config updated

```
NAME:
   lotus-miner config updated - Print updated node config

USAGE:
   lotus-miner config updated [command options]

OPTIONS:
   --no-comment  don't comment default values (default: false)
   --help, -h    show help
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
   For security reasons, the daemon must have LOTUS_BACKUP_BASE_PATH env var set
   to a path where backup files are supposed to be saved, and the path specified in
   this command must be within this base path

OPTIONS:
   --offline   create backup without the node running (default: false)
   --help, -h  show help
```

## lotus-miner version

```
NAME:
   lotus-miner version - Print version

USAGE:
   lotus-miner version [command options]

OPTIONS:
   --help, -h  show help
```

## lotus-miner actor

```
NAME:
   lotus-miner actor - manipulate the miner actor

USAGE:
   lotus-miner actor [command options]

CATEGORY:
   CHAIN

COMMANDS:
   set-addresses, set-addrs    set addresses that your miner can be publicly dialed on
   settle-deal                 Settle deals manually, if dealIds are not provided all deals will be settled. Deal IDs can be specified as individual numbers or ranges (e.g., '123 124 125-200 220')
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
   --help, -h  show help
```

### lotus-miner actor set-addresses

```
NAME:
   lotus-miner actor set-addresses - set addresses that your miner can be publicly dialed on

USAGE:
   lotus-miner actor set-addresses [command options] <multiaddrs>

OPTIONS:
   --from value       optionally specify the account to send the message from
   --gas-limit value  set gas limit (default: 0)
   --unset            unset address (default: false)
   --help, -h         show help
```

### lotus-miner actor settle-deal

```
NAME:
   lotus-miner actor settle-deal - Settle deals manually, if dealIds are not provided all deals will be settled. Deal IDs can be specified as individual numbers or ranges (e.g., '123 124 125-200 220')

USAGE:
   lotus-miner actor settle-deal [command options] [...dealIds]

OPTIONS:
   --confidence value  number of block confirmations to wait for (default: 5)
   --from value        specify where to send the message from (any address)
   --max-deals value   the maximum number of deals contained in each message (default: 50)
   --skip-wait-msg     skip to check the message status (default: false)
   --all-deals         settle all deals. only expired deals are calculated by default (default: false)
   --really-do-it      Actually send transaction performing the action (default: false)
   --help, -h          show help
```

### lotus-miner actor withdraw

```
NAME:
   lotus-miner actor withdraw - withdraw available balance to beneficiary

USAGE:
   lotus-miner actor withdraw [command options] [amount (FIL)]

OPTIONS:
   --confidence value  number of block confirmations to wait for (default: 5)
   --beneficiary       send withdraw message from the beneficiary address (default: false)
   --help, -h          show help
```

### lotus-miner actor repay-debt

```
NAME:
   lotus-miner actor repay-debt - pay down a miner's debt

USAGE:
   lotus-miner actor repay-debt [command options] [amount (FIL)]

OPTIONS:
   --from value  optionally specify the account to send funds from
   --help, -h    show help
```

### lotus-miner actor set-peer-id

```
NAME:
   lotus-miner actor set-peer-id - set the peer id of your miner

USAGE:
   lotus-miner actor set-peer-id [command options] <peer id>

OPTIONS:
   --gas-limit value  set gas limit (default: 0)
   --help, -h         show help
```

### lotus-miner actor set-owner

```
NAME:
   lotus-miner actor set-owner - Set owner address (this command should be invoked twice, first with the old owner as the senderAddress, and then with the new owner)

USAGE:
   lotus-miner actor set-owner [command options] [newOwnerAddress senderAddress]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help
```

### lotus-miner actor control

```
NAME:
   lotus-miner actor control - Manage control addresses

USAGE:
   lotus-miner actor control [command options]

COMMANDS:
   list     Get currently set control addresses
   set      Set control address(-es)
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

#### lotus-miner actor control list

```
NAME:
   lotus-miner actor control list - Get currently set control addresses

USAGE:
   lotus-miner actor control list [command options]

OPTIONS:
   --verbose   (default: false)
   --help, -h  show help
```

#### lotus-miner actor control set

```
NAME:
   lotus-miner actor control set - Set control address(-es)

USAGE:
   lotus-miner actor control set [command options] [...address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help
```

### lotus-miner actor propose-change-worker

```
NAME:
   lotus-miner actor propose-change-worker - Propose a worker address change

USAGE:
   lotus-miner actor propose-change-worker [command options] [address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help
```

### lotus-miner actor confirm-change-worker

```
NAME:
   lotus-miner actor confirm-change-worker - Confirm a worker address change

USAGE:
   lotus-miner actor confirm-change-worker [command options] [address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help
```

### lotus-miner actor compact-allocated

```
NAME:
   lotus-miner actor compact-allocated - compact allocated sectors bitfield

USAGE:
   lotus-miner actor compact-allocated [command options]

OPTIONS:
   --mask-last-offset value  Mask sector IDs from 0 to 'highest_allocated - offset' (default: 0)
   --mask-upto-n value       Mask sector IDs from 0 to 'n' (default: 0)
   --really-do-it            Actually send transaction performing the action (default: false)
   --help, -h                show help
```

### lotus-miner actor propose-change-beneficiary

```
NAME:
   lotus-miner actor propose-change-beneficiary - Propose a beneficiary address change

USAGE:
   lotus-miner actor propose-change-beneficiary [command options] [beneficiaryAddress quota expiration]

OPTIONS:
   --really-do-it              Actually send transaction performing the action (default: false)
   --overwrite-pending-change  Overwrite the current beneficiary change proposal (default: false)
   --actor value               specify the address of miner actor
   --help, -h                  show help
```

### lotus-miner actor confirm-change-beneficiary

```
NAME:
   lotus-miner actor confirm-change-beneficiary - Confirm a beneficiary address change

USAGE:
   lotus-miner actor confirm-change-beneficiary [command options] [minerID]

OPTIONS:
   --really-do-it          Actually send transaction performing the action (default: false)
   --existing-beneficiary  send confirmation from the existing beneficiary address (default: false)
   --new-beneficiary       send confirmation from the new beneficiary address (default: false)
   --help, -h              show help
```

## lotus-miner info

```
NAME:
   lotus-miner info - Print miner info

USAGE:
   lotus-miner info [command options]

CATEGORY:
   CHAIN

COMMANDS:
   all      dump all related miner info
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --hide-sectors-info  hide sectors info (default: false)
   --blocks value       Log of produced <blocks> newest blocks and rewards(Miner Fee excluded) (default: 0)
   --help, -h           show help
```

### lotus-miner info all

```
NAME:
   lotus-miner info all - dump all related miner info

USAGE:
   lotus-miner info all [command options]

OPTIONS:
   --help, -h  show help
```

## lotus-miner sectors

```
NAME:
   lotus-miner sectors - interact with sector store

USAGE:
   lotus-miner sectors [command options]

CATEGORY:
   STORAGE

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
   unseal                unseal a sector
   help, h               Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
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
   --help, -h            show help
```

### lotus-miner sectors list

```
NAME:
   lotus-miner sectors list - List sectors

USAGE:
   lotus-miner sectors list [command options]

COMMANDS:
   upgrade-bounds  Output upgrade bounds for available sectors
   help, h         Shows a list of commands or help for one command

OPTIONS:
   --show-removed, -r         show removed sectors (default: false)
   --fast, -f                 don't show on-chain info for better performance (default: false)
   --events, -e               display number of events the sector has received (default: false)
   --initial-pledge, -p       display initial pledge (default: false)
   --seal-time, -t            display how long it took for the sector to be sealed (default: false)
   --states value             filter sectors by a comma-separated list of states
   --unproven, -u             only show sectors which aren't in the 'Proving' state (default: false)
   --check-parallelism value  number of parallel requests to make for checking sector states (default: 300)
   --help, -h                 show help
```

#### lotus-miner sectors list upgrade-bounds

```
NAME:
   lotus-miner sectors list upgrade-bounds - Output upgrade bounds for available sectors

USAGE:
   lotus-miner sectors list upgrade-bounds [command options]

OPTIONS:
   --buckets value  (default: 25)
   --csv            output machine-readable values (default: false)
   --deal-terms     bucket by how many deal-sectors can start at a given expiration (default: false)
   --help, -h       show help
```

### lotus-miner sectors refs

```
NAME:
   lotus-miner sectors refs - List References to sectors

USAGE:
   lotus-miner sectors refs [command options]

OPTIONS:
   --help, -h  show help
```

### lotus-miner sectors update-state

```
NAME:
   lotus-miner sectors update-state - ADVANCED: manually update the state of a sector, this may aid in error recovery

USAGE:
   lotus-miner sectors update-state [command options] <sectorNum> <newState>

OPTIONS:
   --really-do-it  pass this flag if you know what you are doing (default: false)
   --help, -h      show help
```

### lotus-miner sectors pledge

```
NAME:
   lotus-miner sectors pledge - store random data in a sector

USAGE:
   lotus-miner sectors pledge [command options]

OPTIONS:
   --help, -h  show help
```

### lotus-miner sectors numbers

```
NAME:
   lotus-miner sectors numbers - manage sector number assignments

USAGE:
   lotus-miner sectors numbers [command options]

COMMANDS:
   info          view sector assigner state
   reservations  list sector number reservations
   reserve       create sector number reservations
   free          remove sector number reservations
   help, h       Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

#### lotus-miner sectors numbers info

```
NAME:
   lotus-miner sectors numbers info - view sector assigner state

USAGE:
   lotus-miner sectors numbers info [command options]

OPTIONS:
   --help, -h  show help
```

#### lotus-miner sectors numbers reservations

```
NAME:
   lotus-miner sectors numbers reservations - list sector number reservations

USAGE:
   lotus-miner sectors numbers reservations [command options]

OPTIONS:
   --help, -h  show help
```

#### lotus-miner sectors numbers reserve

```
NAME:
   lotus-miner sectors numbers reserve - create sector number reservations

USAGE:
   lotus-miner sectors numbers reserve [command options] [reservation name] [reserved ranges]

OPTIONS:
   --force     skip duplicate reservation checks (note: can lead to damaging other reservations on free) (default: false)
   --help, -h  show help
```

#### lotus-miner sectors numbers free

```
NAME:
   lotus-miner sectors numbers free - remove sector number reservations

USAGE:
   lotus-miner sectors numbers free [command options] [reservation name]

OPTIONS:
   --help, -h  show help
```

### lotus-miner sectors precommits

```
NAME:
   lotus-miner sectors precommits - Print on-chain precommit info

USAGE:
   lotus-miner sectors precommits [command options]

OPTIONS:
   --help, -h  show help
```

### lotus-miner sectors check-expire

```
NAME:
   lotus-miner sectors check-expire - Inspect expiring sectors

USAGE:
   lotus-miner sectors check-expire [command options]

OPTIONS:
   --cutoff value  skip sectors whose current expiration is more than <cutoff> epochs from now, defaults to 60 days (default: 172800)
   --help, -h      show help
```

### lotus-miner sectors expired

```
NAME:
   lotus-miner sectors expired - Get or cleanup expired sectors

USAGE:
   lotus-miner sectors expired [command options]

OPTIONS:
   --show-removed         show removed sectors (default: false)
   --remove-expired       remove expired sectors (default: false)
   --expired-epoch value  epoch at which to check sector expirations (default: WinningPoSt lookback epoch)
   --help, -h             show help
```

### lotus-miner sectors extend

```
NAME:
   lotus-miner sectors extend - Extend expiring sectors while not exceeding each sector's max life

USAGE:
   lotus-miner sectors extend [command options] <sectorNumbers...(optional)>

OPTIONS:
   --from value            only consider sectors whose current expiration epoch is in the range of [from, to], <from> defaults to: now + 120 (1 hour) (default: 0)
   --to value              only consider sectors whose current expiration epoch is in the range of [from, to], <to> defaults to: now + 92160 (32 days) (default: 0)
   --sector-file value     provide a file containing one sector number in each line, ignoring above selecting criteria
   --exclude value         optionally provide a file containing excluding sectors
   --extension value       try to extend selected sectors by this number of epochs, defaults to 540 days (default: 1555200)
   --new-expiration value  try to extend selected sectors to this epoch, ignoring extension (default: 0)
   --drop-claims           drop claims for sectors that can be extended, but only by dropping some of their verified power claims (default: false)
   --tolerance value       don't try to extend sectors by fewer than this number of epochs, defaults to 7 days (default: 20160)
   --max-fee value         use up to this amount of FIL for one message. pass this flag to avoid message congestion. (default: "0")
   --max-sectors value     the maximum number of sectors contained in each message (default: 500)
   --really-do-it          pass this flag to really extend sectors, otherwise will only print out json representation of parameters (default: false)
   --help, -h              show help
```

### lotus-miner sectors terminate

```
NAME:
   lotus-miner sectors terminate - Terminate sector on-chain then remove (WARNING: This means losing power and collateral for the removed sector)

USAGE:
   lotus-miner sectors terminate [command options] <sectorNum>

COMMANDS:
   flush    Send a terminate message if there are sectors queued for termination
   pending  List sector numbers of sectors pending termination
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --really-do-it  pass this flag if you know what you are doing (default: false)
   --help, -h      show help
```

#### lotus-miner sectors terminate flush

```
NAME:
   lotus-miner sectors terminate flush - Send a terminate message if there are sectors queued for termination

USAGE:
   lotus-miner sectors terminate flush [command options]

OPTIONS:
   --help, -h  show help
```

#### lotus-miner sectors terminate pending

```
NAME:
   lotus-miner sectors terminate pending - List sector numbers of sectors pending termination

USAGE:
   lotus-miner sectors terminate pending [command options]

OPTIONS:
   --help, -h  show help
```

### lotus-miner sectors remove

```
NAME:
   lotus-miner sectors remove - Forcefully remove a sector (WARNING: This means losing power and collateral for the removed sector (use 'terminate' for lower penalty))

USAGE:
   lotus-miner sectors remove [command options] <sectorNum>

OPTIONS:
   --really-do-it  pass this flag if you know what you are doing (default: false)
   --help, -h      show help
```

### lotus-miner sectors snap-up

```
NAME:
   lotus-miner sectors snap-up - Mark a committed capacity sector to be filled with deals

USAGE:
   lotus-miner sectors snap-up [command options] <sectorNum>

OPTIONS:
   --help, -h  show help
```

### lotus-miner sectors abort-upgrade

```
NAME:
   lotus-miner sectors abort-upgrade - Abort the attempted (SnapDeals) upgrade of a CC sector, reverting it to as before

USAGE:
   lotus-miner sectors abort-upgrade [command options] <sectorNum>

OPTIONS:
   --really-do-it  pass this flag if you know what you are doing (default: false)
   --help, -h      show help
```

### lotus-miner sectors seal

```
NAME:
   lotus-miner sectors seal - Manually start sealing a sector (filling any unused space with junk)

USAGE:
   lotus-miner sectors seal [command options] <sectorNum>

OPTIONS:
   --help, -h  show help
```

### lotus-miner sectors set-seal-delay

```
NAME:
   lotus-miner sectors set-seal-delay - Set the time (in minutes) that a new sector waits for deals before sealing starts

USAGE:
   lotus-miner sectors set-seal-delay [command options] <time>

OPTIONS:
   --seconds   Specifies that the time argument should be in seconds (default: false)
   --help, -h  show help
```

### lotus-miner sectors get-cc-collateral

```
NAME:
   lotus-miner sectors get-cc-collateral - Get the collateral required to pledge a committed capacity sector

USAGE:
   lotus-miner sectors get-cc-collateral [command options]

OPTIONS:
   --expiration value  the epoch when the sector will expire (default: 0)
   --help, -h          show help
```

### lotus-miner sectors batching

```
NAME:
   lotus-miner sectors batching - manage batch sector operations

USAGE:
   lotus-miner sectors batching [command options]

COMMANDS:
   commit     list sectors waiting in commit batch queue
   precommit  list sectors waiting in precommit batch queue
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

#### lotus-miner sectors batching commit

```
NAME:
   lotus-miner sectors batching commit - list sectors waiting in commit batch queue

USAGE:
   lotus-miner sectors batching commit [command options]

OPTIONS:
   --publish-now  send a batch now (default: false)
   --help, -h     show help
```

#### lotus-miner sectors batching precommit

```
NAME:
   lotus-miner sectors batching precommit - list sectors waiting in precommit batch queue

USAGE:
   lotus-miner sectors batching precommit [command options]

OPTIONS:
   --publish-now  send a batch now (default: false)
   --help, -h     show help
```

### lotus-miner sectors match-pending-pieces

```
NAME:
   lotus-miner sectors match-pending-pieces - force a refreshed match of pending pieces to open sectors without manually waiting for more deals

USAGE:
   lotus-miner sectors match-pending-pieces [command options]

OPTIONS:
   --help, -h  show help
```

### lotus-miner sectors compact-partitions

```
NAME:
   lotus-miner sectors compact-partitions - removes dead sectors from partitions and reduces the number of partitions used if possible

USAGE:
   lotus-miner sectors compact-partitions [command options]

OPTIONS:
   --deadline value                           the deadline to compact the partitions in (default: 0)
   --partitions value [ --partitions value ]  list of partitions to compact sectors in
   --really-do-it                             Actually send transaction performing the action (default: false)
   --help, -h                                 show help
```

### lotus-miner sectors unseal

```
NAME:
   lotus-miner sectors unseal - unseal a sector

USAGE:
   lotus-miner sectors unseal [command options] [sector number]

OPTIONS:
   --help, -h  show help
```

## lotus-miner proving

```
NAME:
   lotus-miner proving - View proving information

USAGE:
   lotus-miner proving [command options]

CATEGORY:
   STORAGE

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
   --help, -h  show help
```

### lotus-miner proving info

```
NAME:
   lotus-miner proving info - View current state information

USAGE:
   lotus-miner proving info [command options]

OPTIONS:
   --help, -h  show help
```

### lotus-miner proving deadlines

```
NAME:
   lotus-miner proving deadlines - View the current proving period deadlines information

USAGE:
   lotus-miner proving deadlines [command options]

OPTIONS:
   --all, -a   Count all sectors (only live sectors are counted by default) (default: false)
   --help, -h  show help
```

### lotus-miner proving deadline

```
NAME:
   lotus-miner proving deadline - View the current proving period deadline information by its index

USAGE:
   lotus-miner proving deadline [command options] <deadlineIdx>

OPTIONS:
   --sector-nums, -n  Print sector/fault numbers belonging to this deadline (default: false)
   --bitfield, -b     Print partition bitfield stats (default: false)
   --help, -h         show help
```

### lotus-miner proving faults

```
NAME:
   lotus-miner proving faults - View the currently known proving faulty sectors information

USAGE:
   lotus-miner proving faults [command options]

OPTIONS:
   --help, -h  show help
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
   --faulty            only check faulty sectors (default: false)
   --help, -h          show help
```

### lotus-miner proving workers

```
NAME:
   lotus-miner proving workers - list workers

USAGE:
   lotus-miner proving workers [command options]

OPTIONS:
   --help, -h  show help
```

### lotus-miner proving compute

```
NAME:
   lotus-miner proving compute - Compute simulated proving tasks

USAGE:
   lotus-miner proving compute [command options]

COMMANDS:
   windowed-post, window-post  Compute WindowPoSt for a specific deadline
   help, h                     Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

#### lotus-miner proving compute windowed-post

```
NAME:
   lotus-miner proving compute windowed-post - Compute WindowPoSt for a specific deadline

USAGE:
   lotus-miner proving compute windowed-post [command options] [deadline index]

DESCRIPTION:
   Note: This command is intended to be used to verify PoSt compute performance.
   It will not send any messages to the chain.

OPTIONS:
   --help, -h  show help
```

### lotus-miner proving recover-faults

```
NAME:
   lotus-miner proving recover-faults - Manually recovers faulty sectors on chain

USAGE:
   lotus-miner proving recover-faults [command options] <faulty sectors>

OPTIONS:
   --confidence value  number of block confirmations to wait for (default: 5)
   --help, -h          show help
```

## lotus-miner storage

```
NAME:
   lotus-miner storage - manage sector storage

USAGE:
   lotus-miner storage [command options]

CATEGORY:
   STORAGE

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
   --help, -h  show help
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
   --init                                 initialize the path first (default: false)
   --weight value                         (for init) path weight (default: 10)
   --seal                                 (for init) use path for sealing (default: false)
   --store                                (for init) use path for long-term storage (default: false)
   --max-storage value                    (for init) limit storage space for sectors (expensive for very large paths!)
   --groups value [ --groups value ]      path group names
   --allow-to value [ --allow-to value ]  path groups allowed to pull data from this path (allow all if not specified)
   --help, -h                             show help
```

### lotus-miner storage detach

```
NAME:
   lotus-miner storage detach - detach local storage path

USAGE:
   lotus-miner storage detach [command options] [path]

OPTIONS:
   --really-do-it  (default: false)
   --help, -h      show help
```

### lotus-miner storage redeclare

```
NAME:
   lotus-miner storage redeclare - redeclare sectors in a local storage path

USAGE:
   lotus-miner storage redeclare [command options] [path]

OPTIONS:
   --id value      storage path ID
   --all           redeclare all storage paths (default: false)
   --drop-missing  Drop index entries with missing files (default: true)
   --help, -h      show help
```

### lotus-miner storage list

```
NAME:
   lotus-miner storage list - list local storage paths

USAGE:
   lotus-miner storage list [command options]

COMMANDS:
   sectors  get list of all sector files
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

#### lotus-miner storage list sectors

```
NAME:
   lotus-miner storage list sectors - get list of all sector files

USAGE:
   lotus-miner storage list sectors [command options]

OPTIONS:
   --help, -h  show help
```

### lotus-miner storage find

```
NAME:
   lotus-miner storage find - find sector in the storage system

USAGE:
   lotus-miner storage find [command options] [sector number]

OPTIONS:
   --help, -h  show help
```

### lotus-miner storage cleanup

```
NAME:
   lotus-miner storage cleanup - trigger cleanup actions

USAGE:
   lotus-miner storage cleanup [command options]

OPTIONS:
   --removed   cleanup remaining files from removed sectors (default: true)
   --help, -h  show help
```

### lotus-miner storage locks

```
NAME:
   lotus-miner storage locks - show active sector locks

USAGE:
   lotus-miner storage locks [command options]

OPTIONS:
   --help, -h  show help
```

## lotus-miner sealing

```
NAME:
   lotus-miner sealing - interact with sealing pipeline

USAGE:
   lotus-miner sealing [command options]

CATEGORY:
   STORAGE

COMMANDS:
   jobs        list running jobs
   workers     list workers
   sched-diag  Dump internal scheduler state
   abort       Abort a running job
   data-cid    Compute data CID using workers
   help, h     Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-miner sealing jobs

```
NAME:
   lotus-miner sealing jobs - list running jobs

USAGE:
   lotus-miner sealing jobs [command options]

OPTIONS:
   --show-ret-done  show returned but not consumed calls (default: false)
   --help, -h       show help
```

### lotus-miner sealing workers

```
NAME:
   lotus-miner sealing workers - list workers

USAGE:
   lotus-miner sealing workers [command options]

OPTIONS:
   --help, -h  show help
```

### lotus-miner sealing sched-diag

```
NAME:
   lotus-miner sealing sched-diag - Dump internal scheduler state

USAGE:
   lotus-miner sealing sched-diag [command options]

OPTIONS:
   --force-sched  (default: false)
   --help, -h     show help
```

### lotus-miner sealing abort

```
NAME:
   lotus-miner sealing abort - Abort a running job

USAGE:
   lotus-miner sealing abort [command options] [callid]

OPTIONS:
   --sched     Specifies that the argument is UUID of the request to be removed from scheduler (default: false)
   --help, -h  show help
```

### lotus-miner sealing data-cid

```
NAME:
   lotus-miner sealing data-cid - Compute data CID using workers

USAGE:
   lotus-miner sealing data-cid [command options] [file/url] <padded piece size>

OPTIONS:
   --file-size value  real file size (default: 0)
   --help, -h         show help
```

## lotus-miner auth

```
NAME:
   lotus-miner auth - Manage RPC permissions

USAGE:
   lotus-miner auth [command options]

CATEGORY:
   DEVELOPER

COMMANDS:
   create-token  Create token
   api-info      Get token with API info required to connect to this node
   help, h       Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-miner auth create-token

```
NAME:
   lotus-miner auth create-token - Create token

USAGE:
   lotus-miner auth create-token [command options]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   --help, -h    show help
```

### lotus-miner auth api-info

```
NAME:
   lotus-miner auth api-info - Get token with API info required to connect to this node

USAGE:
   lotus-miner auth api-info [command options]

OPTIONS:
   --perm value  permission to assign to the token, one of: read, write, sign, admin
   --help, -h    show help
```

## lotus-miner log

```
NAME:
   lotus-miner log - Manage logging

USAGE:
   lotus-miner log [command options]

CATEGORY:
   DEVELOPER

COMMANDS:
   list       List log systems
   set-level  Set log level
   alerts     Get alert states
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### lotus-miner log list

```
NAME:
   lotus-miner log list - List log systems

USAGE:
   lotus-miner log list [command options]

OPTIONS:
   --help, -h  show help
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
   --help, -h                         show help
```

### lotus-miner log alerts

```
NAME:
   lotus-miner log alerts - Get alert states

USAGE:
   lotus-miner log alerts [command options]

OPTIONS:
   --all       get all (active and inactive) alerts (default: false)
   --help, -h  show help
```

## lotus-miner wait-api

```
NAME:
   lotus-miner wait-api - Wait for lotus api to come online

USAGE:
   lotus-miner wait-api [command options]

CATEGORY:
   DEVELOPER

OPTIONS:
   --timeout value  duration to wait till fail (default: 30s)
   --help, -h       show help
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
   --help, -h  show help
```
