# sptool
```
NAME:
   sptool - Manage Filecoin Miner Actor

USAGE:
   sptool [global options] command [command options] [arguments...]

VERSION:
   1.27.0-dev

COMMANDS:
   actor    Manage Filecoin Miner Actor Metadata
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --log-level value  (default: "info")
   --actor value      miner actor to manage
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
   withdraw  withdraw available balance to beneficiary
   help, h   Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
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
