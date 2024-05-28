# sptool
```
NAME:
   sptool - Manage Filecoin Miner Actor

USAGE:
   sptool [global options] command [command options] [arguments...]

VERSION:
   1.27.1-dev

COMMANDS:
   actor    Manage Filecoin Miner Actor Metadata
   info     Print miner actor info
   sectors  interact with sector store
   proving  View proving information
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --log-level value  (default: "info")
   --actor value      miner actor to manage [$SP_ADDRESS]
   --help, -h         show help
   --version, -v      print the version
```

## sptool actor
```
NAME:
   sptool actor - Manage Filecoin Miner Actor Metadata

USAGE:
   sptool actor command [command options] [arguments...]

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
   new-miner                   Initializes a new miner actor
   help, h                     Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### sptool actor set-addresses
```
NAME:
   sptool actor set-addresses - set addresses that your miner can be publicly dialed on

USAGE:
   sptool actor set-addresses [command options] <multiaddrs>

OPTIONS:
   --from value       optionally specify the account to send the message from
   --gas-limit value  set gas limit (default: 0)
   --unset            unset address (default: false)
   --help, -h         show help
```

### sptool actor withdraw
```
NAME:
   sptool actor withdraw - withdraw available balance to beneficiary

USAGE:
   sptool actor withdraw [command options] [amount (FIL)]

OPTIONS:
   --confidence value  number of block confirmations to wait for (default: 5)
   --beneficiary       send withdraw message from the beneficiary address (default: false)
   --help, -h          show help
```

### sptool actor repay-debt
```
NAME:
   sptool actor repay-debt - pay down a miner's debt

USAGE:
   sptool actor repay-debt [command options] [amount (FIL)]

OPTIONS:
   --from value  optionally specify the account to send funds from
   --help, -h    show help
```

### sptool actor set-peer-id
```
NAME:
   sptool actor set-peer-id - set the peer id of your miner

USAGE:
   sptool actor set-peer-id [command options] <peer id>

OPTIONS:
   --gas-limit value  set gas limit (default: 0)
   --help, -h         show help
```

### sptool actor set-owner
```
NAME:
   sptool actor set-owner - Set owner address (this command should be invoked twice, first with the old owner as the senderAddress, and then with the new owner)

USAGE:
   sptool actor set-owner [command options] [newOwnerAddress senderAddress]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help
```

### sptool actor control
```
NAME:
   sptool actor control - Manage control addresses

USAGE:
   sptool actor control command [command options] [arguments...]

COMMANDS:
   list     Get currently set control addresses. Note: This excludes most roles as they are not known to the immediate chain state.
   set      Set control address(-es)
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

#### sptool actor control list
```
NAME:
   sptool actor control list - Get currently set control addresses. Note: This excludes most roles as they are not known to the immediate chain state.

USAGE:
   sptool actor control list [command options] [arguments...]

OPTIONS:
   --verbose   (default: false)
   --help, -h  show help
```

#### sptool actor control set
```
NAME:
   sptool actor control set - Set control address(-es)

USAGE:
   sptool actor control set [command options] [...address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help
```

### sptool actor propose-change-worker
```
NAME:
   sptool actor propose-change-worker - Propose a worker address change

USAGE:
   sptool actor propose-change-worker [command options] [address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help
```

### sptool actor confirm-change-worker
```
NAME:
   sptool actor confirm-change-worker - Confirm a worker address change

USAGE:
   sptool actor confirm-change-worker [command options] [address]

OPTIONS:
   --really-do-it  Actually send transaction performing the action (default: false)
   --help, -h      show help
```

### sptool actor compact-allocated
```
NAME:
   sptool actor compact-allocated - compact allocated sectors bitfield

USAGE:
   sptool actor compact-allocated [command options] [arguments...]

OPTIONS:
   --mask-last-offset value  Mask sector IDs from 0 to 'highest_allocated - offset' (default: 0)
   --mask-upto-n value       Mask sector IDs from 0 to 'n' (default: 0)
   --really-do-it            Actually send transaction performing the action (default: false)
   --help, -h                show help
```

### sptool actor propose-change-beneficiary
```
NAME:
   sptool actor propose-change-beneficiary - Propose a beneficiary address change

USAGE:
   sptool actor propose-change-beneficiary [command options] [beneficiaryAddress quota expiration]

OPTIONS:
   --really-do-it              Actually send transaction performing the action (default: false)
   --overwrite-pending-change  Overwrite the current beneficiary change proposal (default: false)
   --actor value               specify the address of miner actor
   --help, -h                  show help
```

### sptool actor confirm-change-beneficiary
```
NAME:
   sptool actor confirm-change-beneficiary - Confirm a beneficiary address change

USAGE:
   sptool actor confirm-change-beneficiary [command options] [minerID]

OPTIONS:
   --really-do-it          Actually send transaction performing the action (default: false)
   --existing-beneficiary  send confirmation from the existing beneficiary address (default: false)
   --new-beneficiary       send confirmation from the new beneficiary address (default: false)
   --help, -h              show help
```

### sptool actor new-miner
```
NAME:
   sptool actor new-miner - Initializes a new miner actor

USAGE:
   sptool actor new-miner [command options] [arguments...]

OPTIONS:
   --worker value, -w value  worker key to use for new miner initialisation
   --owner value, -o value   owner key to use for new miner initialisation
   --from value, -f value    address to send actor(miner) creation message from
   --sector-size value       specify sector size to use for new miner initialisation
   --confidence value        number of block confirmations to wait for (default: 5)
   --help, -h                show help
```

## sptool info
```
NAME:
   sptool info - Print miner actor info

USAGE:
   sptool info [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

## sptool sectors
```
NAME:
   sptool sectors - interact with sector store

USAGE:
   sptool sectors command [command options] [arguments...]

COMMANDS:
   status              Get the seal status of a sector by its number
   list                List sectors
   precommits          Print on-chain precommit info
   check-expire        Inspect expiring sectors
   expired             Get or cleanup expired sectors
   extend              Extend expiring sectors while not exceeding each sector's max life
   terminate           Forcefully terminate a sector (WARNING: This means losing power and pay a one-time termination penalty(including collateral) for the terminated sector)
   compact-partitions  removes dead sectors from partitions and reduces the number of partitions used if possible
   help, h             Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### sptool sectors status
```
NAME:
   sptool sectors status - Get the seal status of a sector by its number

USAGE:
   sptool sectors status [command options] <sectorNum>

OPTIONS:
   --log, -l             display event log (default: false)
   --on-chain-info, -c   show sector on chain info (default: false)
   --partition-info, -p  show partition related info (default: false)
   --proof               print snark proof bytes as hex (default: false)
   --help, -h            show help
```

### sptool sectors list
```
NAME:
   sptool sectors list - List sectors

USAGE:
   sptool sectors list [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

### sptool sectors precommits
```
NAME:
   sptool sectors precommits - Print on-chain precommit info

USAGE:
   sptool sectors precommits [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

### sptool sectors check-expire
```
NAME:
   sptool sectors check-expire - Inspect expiring sectors

USAGE:
   sptool sectors check-expire [command options] [arguments...]

OPTIONS:
   --cutoff value  skip sectors whose current expiration is more than <cutoff> epochs from now, defaults to 60 days (default: 172800)
   --help, -h      show help
```

### sptool sectors expired
```
NAME:
   sptool sectors expired - Get or cleanup expired sectors

USAGE:
   sptool sectors expired [command options] [arguments...]

OPTIONS:
   --expired-epoch value  epoch at which to check sector expirations (default: WinningPoSt lookback epoch)
   --help, -h             show help
```

### sptool sectors extend
```
NAME:
   sptool sectors extend - Extend expiring sectors while not exceeding each sector's max life

USAGE:
   sptool sectors extend [command options] <sectorNumbers...(optional)>

OPTIONS:
   --from value            only consider sectors whose current expiration epoch is in the range of [from, to], <from> defaults to: now + 120 (1 hour) (default: 0)
   --to value              only consider sectors whose current expiration epoch is in the range of [from, to], <to> defaults to: now + 92160 (32 days) (default: 0)
   --sector-file value     provide a file containing one sector number in each line, ignoring above selecting criteria
   --exclude value         optionally provide a file containing excluding sectors
   --extension value       try to extend selected sectors by this number of epochs, defaults to 540 days (default: 1555200)
   --new-expiration value  try to extend selected sectors to this epoch, ignoring extension (default: 0)
   --only-cc               only extend CC sectors (useful for making sector ready for snap upgrade) (default: false)
   --drop-claims           drop claims for sectors that can be extended, but only by dropping some of their verified power claims (default: false)
   --tolerance value       don't try to extend sectors by fewer than this number of epochs, defaults to 7 days (default: 20160)
   --max-fee value         use up to this amount of FIL for one message. pass this flag to avoid message congestion. (default: "0")
   --max-sectors value     the maximum number of sectors contained in each message (default: 0)
   --really-do-it          pass this flag to really extend sectors, otherwise will only print out json representation of parameters (default: false)
   --help, -h              show help
```

### sptool sectors terminate
```
NAME:
   sptool sectors terminate - Forcefully terminate a sector (WARNING: This means losing power and pay a one-time termination penalty(including collateral) for the terminated sector)

USAGE:
   sptool sectors terminate [command options] [sectorNum1 sectorNum2 ...]

OPTIONS:
   --actor value   specify the address of miner actor
   --really-do-it  pass this flag if you know what you are doing (default: false)
   --from value    specify the address to send the terminate message from
   --help, -h      show help
```

### sptool sectors compact-partitions
```
NAME:
   sptool sectors compact-partitions - removes dead sectors from partitions and reduces the number of partitions used if possible

USAGE:
   sptool sectors compact-partitions [command options] [arguments...]

OPTIONS:
   --deadline value                           the deadline to compact the partitions in (default: 0)
   --partitions value [ --partitions value ]  list of partitions to compact sectors in
   --really-do-it                             Actually send transaction performing the action (default: false)
   --help, -h                                 show help
```

## sptool proving
```
NAME:
   sptool proving - View proving information

USAGE:
   sptool proving command [command options] [arguments...]

COMMANDS:
   info       View current state information
   deadlines  View the current proving period deadlines information
   deadline   View the current proving period deadline information by its index
   faults     View the currently known proving faulty sectors information
   help, h    Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### sptool proving info
```
NAME:
   sptool proving info - View current state information

USAGE:
   sptool proving info [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```

### sptool proving deadlines
```
NAME:
   sptool proving deadlines - View the current proving period deadlines information

USAGE:
   sptool proving deadlines [command options] [arguments...]

OPTIONS:
   --all, -a   Count all sectors (only live sectors are counted by default) (default: false)
   --help, -h  show help
```

### sptool proving deadline
```
NAME:
   sptool proving deadline - View the current proving period deadline information by its index

USAGE:
   sptool proving deadline [command options] <deadlineIdx>

OPTIONS:
   --sector-nums, -n  Print sector/fault numbers belonging to this deadline (default: false)
   --bitfield, -b     Print partition bitfield stats (default: false)
   --help, -h         show help
```

### sptool proving faults
```
NAME:
   sptool proving faults - View the currently known proving faulty sectors information

USAGE:
   sptool proving faults [command options] [arguments...]

OPTIONS:
   --help, -h  show help
```
