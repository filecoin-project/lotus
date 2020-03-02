# Lotus Command Line Interface

The Command Line Interface (CLI) is a convenient way to interact with
a Lotus node. You can use the CLI to operate your node,
get information about the blockchain,
manage your accounts and transfer funds,
create storage deals, and much more! 

The CLI is intended to be self-documenting, so when in doubt, simply add `--help`
to whatever command you're trying to run! This will also display all of the 
input parameters that can be provided to a command.

We highlight some of the commonly
used features of the CLI below.
All CLI commands should be run from the home directory of the Lotus project.

## Operating a Lotus node

### Starting up a node

```sh
lotus daemon
```
This command will start up your Lotus node, with its API port open at 1234. 
You can pass `--api=<number>` to use a different port.

### Checking your sync progress

```sh
lotus sync status
```
This command will print your current tipset height under `Height`, and the target tipset height
under `Taregt`. 

You can also run `lotus sync wait` to get constant updates on your sync progress.

### Getting the head tipset

```sh
lotus chain head
```

### Control the logging level

```sh
lotus log set-level
```
This command can be used to toggle the logging levels of the different
systems of a Lotus node. In decreasing order
of logging detail, the levels are `debug`, `info`, `warn`, and `error`. 

As an example,
to set the `chain` and `blocksync` to log at the `debug` level, run 
`lotus log set-level --system chain --system blocksync debug`. 

To see the various logging system, run `lotus log list`.

### Find out what version of Lotus you're running

```sh
lotus version
```

## Managing your accounts

### Listing accounts in your wallet

```sh
lo*tus wallet list
``` 

### Creating a new account

```sh
lotus wallet new bls
```
This command will create a new BLS account in your wallet; these
addresses start with the prefix `t3`. Running `lotus wallet new secp256k1` 
(or just `lotus wallet new`) will create
a new Secp256k1 account, which begins with the prefix `t1`.

### Getting an account's balance

```sh
lotus wallet balance <address>
``` 

### Transferring funds 

```sh
lotus send --source=<source address> <destination address> <amount>
``` 
This command will transfer `amount` (in attoFIL) from `source address` to `destination address`.

### Importing an account into your wallet

```sh
lotus wallet import <path to private key>
``` 
This command will import an account whose private key is saved at the specified file.

### Exporting an account from your wallet

```sh
lotus wallet export <address>
``` 
This command will print out the private key of the specified address
if it is in your wallet. Always be careful with your private key!
