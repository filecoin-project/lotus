# lotus  
    NAME:  
       lotus - Filecoin decentralized storage network client  
      
    USAGE:  
       lotus [global options] command [command options] [arguments...]  
      
    VERSION:  
       0.3.0  
      
    COMMANDS:  
         daemon   Start a lotus daemon process  
         version  Print version  
         help, h  Shows a list of commands or help for one command  
       basic:  
         send    Send funds between accounts  
         wallet  Manage wallet  
         client  Make deals, store data, retrieve data  
         msig    Interact with a multisig wallet  
         paych   Manage payment channels  
       developer:  
         auth          Manage RPC permissions  
         mpool         Manage message pool  
         state         Interact with and query filecoin chain state  
         chain         Interact with filecoin blockchain  
         log           Manage logging  
         wait-api      Wait for lotus api to come online  
         fetch-params  Fetch proving parameters  
       network:  
         net   Manage P2P Network  
         sync  Inspect or interact with the chain syncer  
      
    GLOBAL OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  

  



## lotus daemon  
    NAME:  
       lotus daemon - Start a lotus daemon process  
      
    USAGE:  
       lotus daemon [command options] [arguments...]  
      
    OPTIONS:  
       --api value           (default: "1234")  
       --genesis value       genesis file to use for first node run  
       --bootstrap           (default: true)  
       --import-chain value  on first run, load chain from given file  
       --halt-after-import   halt the process after importing chain from file (default: false)  
       --pprof value         specify name of file for writing cpu profile to  
       --profile value       specify type of node  
       --help, -h            show help (default: false)  


​      

## lotus version  
    NAME:  
       lotus version - Print version  
      
    USAGE:  
       lotus version [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

## lotus send  
    NAME:  
       lotus send - Send funds between accounts  
      
    USAGE:  
       lotus send [command options] [targetAddress] [amount]  
      
    CATEGORY:  
       basic  
      
    OPTIONS:  
       --source value     optionally specify the account to send funds from  
       --gas-price value  specify gas price to use in AttoFIL (default: "0")  
       --nonce value      specify the nonce to use (default: -1)  
       --help, -h         show help (default: false)  


​      

## lotus wallet  
    NAME:  
       lotus wallet - Manage wallet  
      
    USAGE:  
       lotus wallet command [command options] [arguments...]  
      
    COMMANDS:  
         new          Generate a new key of the given type  
         list         List wallet address  
         balance      Get account balance  
         export       export keys  
         import       import keys  
         default      Get default wallet address  
         set-default  Set default wallet address  
         sign         sign a message  
         verify       verify the signature of a message  
         help, h      Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus wallet new  
    NAME:  
       lotus wallet new - Generate a new key of the given type  
      
    USAGE:  
       lotus wallet new [command options] [bls|secp256k1 (default secp256k1)]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus wallet list  
    NAME:  
       lotus wallet list - List wallet address  
      
    USAGE:  
       lotus wallet list [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus wallet balance  
    NAME:  
       lotus wallet balance - Get account balance  
      
    USAGE:  
       lotus wallet balance [command options] [address]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus wallet export  
    NAME:  
       lotus wallet export - export keys  
      
    USAGE:  
       lotus wallet export [command options] [address]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus wallet import  
    NAME:  
       lotus wallet import - import keys  
      
    USAGE:  
       lotus wallet import [command options] [<path> (optional, will read from stdin if omitted)]  
      
    OPTIONS:  
       --format value  specify input format for key (default: "hex-lotus")  
       --help, -h      show help (default: false)  


​      

### lotus wallet default  
    NAME:  
       lotus wallet default - Get default wallet address  
      
    USAGE:  
       lotus wallet default [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus wallet set-default  
    NAME:  
       lotus wallet set-default - Set default wallet address  
      
    USAGE:  
       lotus wallet set-default [command options] [address]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus wallet sign  
    NAME:  
       lotus wallet sign - sign a message  
      
    USAGE:  
       lotus wallet sign [command options] <signing address> <hexMessage>  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus wallet verify  
    NAME:  
       lotus wallet verify - verify the signature of a message  
      
    USAGE:  
       lotus wallet verify [command options] <signing address> <hexMessage> <signature>  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

## lotus client  
    NAME:  
       lotus client - Make deals, store data, retrieve data  
      
    USAGE:  
       lotus client command [command options] [arguments...]  
      
    COMMANDS:  
         import        Import data  
         commP         calculate the piece-cid (commP) of a CAR file  
         local         List locally imported data  
         deal          Initialize storage deal with a miner  
         find          find data in the network  
         retrieve      retrieve data from network  
         query-ask     find a miners ask  
         list-deals    List storage market deals  
         generate-car  generate a car file from input  
         help, h       Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus client import  
    NAME:  
       lotus client import - Import data  
      
    USAGE:  
       lotus client import [command options] [inputPath]  
      
    OPTIONS:  
       --car       import from a car file instead of a regular file (default: false)  
       --help, -h  show help (default: false)  


​      

### lotus client commP  
    NAME:  
       lotus client commP - calculate the piece-cid (commP) of a CAR file  
      
    USAGE:  
       lotus client commP [command options] [inputFile minerAddress]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus client local  
    NAME:  
       lotus client local - List locally imported data  
      
    USAGE:  
       lotus client local [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus client deal  
    NAME:  
       lotus client deal - Initialize storage deal with a miner  
      
    USAGE:  
       lotus client deal [command options] [dataCid miner price duration]  
      
    OPTIONS:  
       --manual-piece-cid value   manually specify piece commitment for data (dataCid must be to a car file)  
       --manual-piece-size value  if manually specifying piece cid, used to specify size (dataCid must be to a car file) (default: 0)  
       --from value               specify address to fund the deal with  
       --start-epoch value        specify the epoch that the deal should start at (default: -1)  
       --help, -h                 show help (default: false)  


​      

### lotus client find  
    NAME:  
       lotus client find - find data in the network  
      
    USAGE:  
       lotus client find [command options] [dataCid]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus client retrieve  
    NAME:  
       lotus client retrieve - retrieve data from network  
      
    USAGE:  
       lotus client retrieve [command options] [dataCid outputPath]  
      
    OPTIONS:  
       --address value  address to use for transactions  
       --car            export to a car file instead of a regular file (default: false)  
       --help, -h       show help (default: false)  


​      

### lotus client query-ask  
    NAME:  
       lotus client query-ask - find a miners ask  
      
    USAGE:  
       lotus client query-ask [command options] [minerAddress]  
      
    OPTIONS:  
       --peerid value    specify peer ID of node to make query against  
       --size value      data size in bytes (default: 0)  
       --duration value  deal duration (default: 0)  
       --help, -h        show help (default: false)  


​      

### lotus client list-deals  
    NAME:  
       lotus client list-deals - List storage market deals  
      
    USAGE:  
       lotus client list-deals [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus client generate-car  
    NAME:  
       lotus client generate-car - generate a car file from input  
      
    USAGE:  
       lotus client generate-car [command options] [inputPath outputPath]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

## lotus msig  
    NAME:  
       lotus msig - Interact with a multisig wallet  
      
    USAGE:  
       lotus msig command [command options] [arguments...]  
      
    COMMANDS:  
         create   Create a new multisig wallet  
         inspect  Inspect a multisig wallet  
         propose  Propose a multisig transaction  
         approve  Approve a multisig message  
         help, h  Shows a list of commands or help for one command  
      
    OPTIONS:  
       --source value           specify the account to send propose from  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus msig create  
    NAME:  
       lotus msig create - Create a new multisig wallet  
      
    USAGE:  
       lotus msig create [command options] [address1 address2 ...]  
      
    OPTIONS:  
       --required value  (default: 0)  
       --value value     initial funds to give to multisig (default: "0")  
       --sender value    account to send the create message from  
       --help, -h        show help (default: false)  


​      

### lotus msig inspect  
    NAME:  
       lotus msig inspect - Inspect a multisig wallet  
      
    USAGE:  
       lotus msig inspect [command options] [address]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus msig propose  
    NAME:  
       lotus msig propose - Propose a multisig transaction  
      
    USAGE:  
       lotus msig propose [command options] [multisigAddress destinationAddress value <methodId methodParams> (optional)]  
      
    OPTIONS:  
       --source value  account to send the propose message from  
       --help, -h      show help (default: false)  


​      

### lotus msig approve  
    NAME:  
       lotus msig approve - Approve a multisig message  
      
    USAGE:  
       lotus msig approve [command options] [multisigAddress messageId proposerAddress destination value <methodId methodParams> (optional)]  
      
    OPTIONS:  
       --source value  account to send the approve message from  
       --help, -h      show help (default: false)  


​      

## lotus paych  
    NAME:  
       lotus paych - Manage payment channels  
      
    USAGE:  
       lotus paych command [command options] [arguments...]  
      
    COMMANDS:  
         get      Create a new payment channel or get existing one  
         list     List all locally registered payment channels  
         voucher  Interact with payment channel vouchers  
         help, h  Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus paych get  
    NAME:  
       lotus paych get - Create a new payment channel or get existing one  
      
    USAGE:  
       lotus paych get [command options] [fromAddress toAddress amount]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus paych list  
    NAME:  
       lotus paych list - List all locally registered payment channels  
      
    USAGE:  
       lotus paych list [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus paych voucher  
    NAME:  
       lotus paych voucher - Interact with payment channel vouchers  
      
    USAGE:  
       lotus paych voucher command [command options] [arguments...]  
      
    COMMANDS:  
         create          Create a signed payment channel voucher  
         check           Check validity of payment channel voucher  
         add             Add payment channel voucher to local datastore  
         list            List stored vouchers for a given payment channel  
         best-spendable  Print voucher with highest value that is currently spendable  
         submit          Submit voucher to chain to update payment channel state  
         help, h         Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

#### lotus paych voucher create  
    NAME:  
       lotus paych voucher create - Create a signed payment channel voucher  
      
    USAGE:  
       lotus paych voucher create [command options] [channelAddress amount]  
      
    OPTIONS:  
       --lane value  specify payment channel lane to use (default: 0)  
       --help, -h    show help (default: false)  


​      

#### lotus paych voucher check  
    NAME:  
       lotus paych voucher check - Check validity of payment channel voucher  
      
    USAGE:  
       lotus paych voucher check [command options] [channelAddress voucher]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

#### lotus paych voucher add  
    NAME:  
       lotus paych voucher add - Add payment channel voucher to local datastore  
      
    USAGE:  
       lotus paych voucher add [command options] [channelAddress voucher]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

#### lotus paych voucher list  
    NAME:  
       lotus paych voucher list - List stored vouchers for a given payment channel  
      
    USAGE:  
       lotus paych voucher list [command options] [channelAddress]  
      
    OPTIONS:  
       --export    Print export strings (default: false)  
       --help, -h  show help (default: false)  


​      

#### lotus paych voucher best-spendable  
    NAME:  
       lotus paych voucher best-spendable - Print voucher with highest value that is currently spendable  
      
    USAGE:  
       lotus paych voucher best-spendable [command options] [channelAddress]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

#### lotus paych voucher submit  
    NAME:  
       lotus paych voucher submit - Submit voucher to chain to update payment channel state  
      
    USAGE:  
       lotus paych voucher submit [command options] [channelAddress voucher]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

## lotus auth  
    NAME:  
       lotus auth - Manage RPC permissions  
      
    USAGE:  
       lotus auth command [command options] [arguments...]  
      
    COMMANDS:  
         create-token  Create token  
         api-info      Get token with API info required to connect to this node  
         help, h       Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus auth create-token  
    NAME:  
       lotus auth create-token - Create token  
      
    USAGE:  
       lotus auth create-token [command options] [arguments...]  
      
    OPTIONS:  
       --perm value  permission to assign to the token, one of: read, write, sign, admin  
       --help, -h    show help (default: false)  


​      

### lotus auth api-info  
    NAME:  
       lotus auth api-info - Get token with API info required to connect to this node  
      
    USAGE:  
       lotus auth api-info [command options] [arguments...]  
      
    OPTIONS:  
       --perm value  permission to assign to the token, one of: read, write, sign, admin  
       --help, -h    show help (default: false)  


​      

## lotus mpool  
    NAME:  
       lotus mpool - Manage message pool  
      
    USAGE:  
       lotus mpool command [command options] [arguments...]  
      
    COMMANDS:  
         pending  Get pending messages  
         sub      Subscibe to mpool changes  
         stat     print mempool stats  
         help, h  Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus mpool pending  
    NAME:  
       lotus mpool pending - Get pending messages  
      
    USAGE:  
       lotus mpool pending [command options] [arguments...]  
      
    OPTIONS:  
       --local     print pending messages for addresses in local wallet only (default: false)  
       --help, -h  show help (default: false)  


​      

### lotus mpool sub  
    NAME:  
       lotus mpool sub - Subscibe to mpool changes  
      
    USAGE:  
       lotus mpool sub [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus mpool stat  
    NAME:  
       lotus mpool stat - print mempool stats  
      
    USAGE:  
       lotus mpool stat [command options] [arguments...]  
      
    OPTIONS:  
       --local     print stats for addresses in local wallet only (default: false)  
       --help, -h  show help (default: false)  


​      

## lotus state  
    NAME:  
       lotus state - Interact with and query filecoin chain state  
      
    USAGE:  
       lotus state command [command options] [arguments...]  
      
    COMMANDS:  
         power              Query network or miner power  
         sectors            Query the sector set of a miner  
         proving            Query the proving set of a miner  
         pledge-collateral  Get minimum miner pledge collateral  
         list-actors        list all actors in the network  
         list-miners        list all miners in the network  
         get-actor          Print actor information  
         lookup             Find corresponding ID address  
         replay             Replay a particular message within a tipset  
         sector-size        Look up miners sector size  
         read-state         View a json representation of an actors state  
         list-messages      list messages on chain matching given criteria  
         compute-state      Perform state computations  
         call               Invoke a method on an actor locally  
         get-deal           View on-chain deal info  
         wait-msg           Wait for a message to appear on chain  
         search-msg         Search to see whether a message has appeared on chain  
         miner-info         Retrieve miner information  
         help, h            Shows a list of commands or help for one command  
      
    OPTIONS:  
       --tipset value           specify tipset to call method on (pass comma separated array of cids)  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus state power  
    NAME:  
       lotus state power - Query network or miner power  
      
    USAGE:  
       lotus state power [command options] [<minerAddress> (optional)]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state sectors  
    NAME:  
       lotus state sectors - Query the sector set of a miner  
      
    USAGE:  
       lotus state sectors [command options] [minerAddress]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state proving  
    NAME:  
       lotus state proving - Query the proving set of a miner  
      
    USAGE:  
       lotus state proving [command options] [minerAddress]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state pledge-collateral  
    NAME:  
       lotus state pledge-collateral - Get minimum miner pledge collateral  
      
    USAGE:  
       lotus state pledge-collateral [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state list-actors  
    NAME:  
       lotus state list-actors - list all actors in the network  
      
    USAGE:  
       lotus state list-actors [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state list-miners  
    NAME:  
       lotus state list-miners - list all miners in the network  
      
    USAGE:  
       lotus state list-miners [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state get-actor  
    NAME:  
       lotus state get-actor - Print actor information  
      
    USAGE:  
       lotus state get-actor [command options] [actorrAddress]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state lookup  
    NAME:  
       lotus state lookup - Find corresponding ID address  
      
    USAGE:  
       lotus state lookup [command options] [address]  
      
    OPTIONS:  
       --reverse, -r  Perform reverse lookup (default: false)  
       --help, -h     show help (default: false)  


​      

### lotus state replay  
    NAME:  
       lotus state replay - Replay a particular message within a tipset  
      
    USAGE:  
       lotus state replay [command options] [tipsetKey messageCid]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state sector-size  
    NAME:  
       lotus state sector-size - Look up miners sector size  
      
    USAGE:  
       lotus state sector-size [command options] [minerAddress]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state read-state  
    NAME:  
       lotus state read-state - View a json representation of an actors state  
      
    USAGE:  
       lotus state read-state [command options] [actorAddress]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state list-messages  
    NAME:  
       lotus state list-messages - list messages on chain matching given criteria  
      
    USAGE:  
       lotus state list-messages [command options] [arguments...]  
      
    OPTIONS:  
       --to value        return messages to a given address  
       --from value      return messages from a given address  
       --toheight value  don't look before given block height (default: 0)  
       --cids            print message CIDs instead of messages (default: false)  
       --help, -h        show help (default: false)  


​      

### lotus state compute-state  
    NAME:  
       lotus state compute-state - Perform state computations  
      
    USAGE:  
       lotus state compute-state [command options] [arguments...]  
      
    OPTIONS:  
       --height value          set the height to compute state at (default: 0)  
       --apply-mpool-messages  apply messages from the mempool to the computed state (default: false)  
       --show-trace            print out full execution trace for given tipset (default: false)  
       --html                  generate html report (default: false)  
       --help, -h              show help (default: false)  


​      

### lotus state call  
    NAME:  
       lotus state call - Invoke a method on an actor locally  
      
    USAGE:  
       lotus state call [command options] [toAddress methodId <param1 param2 ...> (optional)]  
      
    OPTIONS:  
       --from value   (default: "t00")  
       --value value  specify value field for invocation (default: "0")  
       --ret value    specify how to parse output (auto, raw, addr, big) (default: "auto")  
       --help, -h     show help (default: false)  


​      

### lotus state get-deal  
    NAME:  
       lotus state get-deal - View on-chain deal info  
      
    USAGE:  
       lotus state get-deal [command options] [dealId]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state wait-msg  
    NAME:  
       lotus state wait-msg - Wait for a message to appear on chain  
      
    USAGE:  
       lotus state wait-msg [command options] [messageCid]  
      
    OPTIONS:  
       --timeout value  (default: "10m")  
       --help, -h       show help (default: false)  


​      

### lotus state search-msg  
    NAME:  
       lotus state search-msg - Search to see whether a message has appeared on chain  
      
    USAGE:  
       lotus state search-msg [command options] [messageCid]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus state miner-info  
    NAME:  
       lotus state miner-info - Retrieve miner information  
      
    USAGE:  
       lotus state miner-info [command options] [minerAddress]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

## lotus chain  
    NAME:  
       lotus chain - Interact with filecoin blockchain  
      
    USAGE:  
       lotus chain command [command options] [arguments...]  
      
    COMMANDS:  
         head             Print chain head  
         getblock         Get a block and print its details  
         read-obj         Read the raw bytes of an object  
         stat-obj         Collect size and ipld link counts for objs  
         getmessage       Get and print a message by its cid  
         sethead          manually set the local nodes head tipset (Caution: normally only used for recovery)  
         list             View a segment of the chain  
         get              Get chain DAG node by path  
         bisect           bisect chain for an event  
         export           export chain to a car file  
         slash-consensus  Report consensus fault  
         help, h          Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus chain head  
    NAME:  
       lotus chain head - Print chain head  
      
    USAGE:  
       lotus chain head [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus chain getblock  
    NAME:  
       lotus chain getblock - Get a block and print its details  
      
    USAGE:  
       lotus chain getblock [command options] [blockCid]  
      
    OPTIONS:  
       --raw       print just the raw block header (default: false)  
       --help, -h  show help (default: false)  


​      

### lotus chain read-obj  
    NAME:  
       lotus chain read-obj - Read the raw bytes of an object  
      
    USAGE:  
       lotus chain read-obj [command options] [objectCid]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus chain stat-obj  
    NAME:  
       lotus chain stat-obj - Collect size and ipld link counts for objs  
      
    USAGE:  
       lotus chain stat-obj [command options] [cid]  
      
    DESCRIPTION:  
       Collect object size and ipld link count for an object.  
      
       When a base is provided it will be walked first, and all links visisted  
       will be ignored when the passed in object is walked.  
       
    OPTIONS:  
       --base value  ignore links found in this obj  
       --help, -h    show help (default: false)  


​      

### lotus chain getmessage  
    NAME:  
       lotus chain getmessage - Get and print a message by its cid  
      
    USAGE:  
       lotus chain getmessage [command options] [messageCid]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus chain sethead  
    NAME:  
       lotus chain sethead - manually set the local nodes head tipset (Caution: normally only used for recovery)  
      
    USAGE:  
       lotus chain sethead [command options] [tipsetkey]  
      
    OPTIONS:  
       --genesis      reset head to genesis (default: false)  
       --epoch value  reset head to given epoch (default: 0)  
       --help, -h     show help (default: false)  


​      

### lotus chain list  
    NAME:  
       lotus chain list - View a segment of the chain  
      
    USAGE:  
       lotus chain list [command options] [arguments...]  
      
    OPTIONS:  
       --height value  (default: 0)  
       --count value   (default: 30)  
       --format value  specify the format to print out tipsets (default: "<height>: (<time>) <blocks>")  
       --help, -h      show help (default: false)  


​      

### lotus chain get  
    NAME:  
       lotus chain get - Get chain DAG node by path  
      
    USAGE:  
       lotus chain get [command options] [path]  
      
    DESCRIPTION:  
       Get ipld node under a specified path:  
      
       lotus chain get /ipfs/[cid]/some/path  
      
       Path prefixes:  
       - /ipfs/[cid], /ipld/[cid] - traverse IPLD path  
       - /pstate - traverse from head.ParentStateRoot  
      
       Note:  
       You can use special path elements to traverse through some data structures:  
       - /ipfs/[cid]/@H:elem - get 'elem' from hamt  
       - /ipfs/[cid]/@Hi:123 - get varint elem 123 from hamt  
       - /ipfs/[cid]/@Hu:123 - get uvarint elem 123 from hamt  
       - /ipfs/[cid]/@Ha:t01 - get element under Addr(t01).Bytes  
       - /ipfs/[cid]/@A:10 - get 10th amt element  
      
       List of --as-type types:  
       - raw  
       - block  
       - message  
       - smessage, signedmessage  
       - actor  
       - amt  
       - hamt-epoch  
       - hamt-address  
       - cronevent  
       - account-state  
       
    OPTIONS:  
       --as-type value  specify type to interpret output as  
       --verbose        (default: false)  
       --tipset value   specify tipset for /pstate (pass comma separated array of cids)  
       --help, -h       show help (default: false)  



​      

### lotus chain bisect  
    NAME:  
       lotus chain bisect - bisect chain for an event  
      
    USAGE:  
       lotus chain bisect [command options] [minHeight maxHeight path shellCommand <shellCommandArgs (if any)>]  
      
    DESCRIPTION:  
       Bisect the chain state tree:  
      
       lotus chain bisect [min height] [max height] '1/2/3/state/path' 'shell command' 'args'  
      
       Returns the first tipset in which condition is true  
                      v  
       [start] FFFFFFFTTT [end]  
      
       Example: find height at which deal ID 100 000 appeared  
        - lotus chain bisect 1 32000 '@Ha:t03/1' jq -e '.[2] > 100000'  
      
       For special path elements see 'chain get' help  
       
    OPTIONS:  
       --help, -h  show help (default: false)      


​      

### lotus chain export  
    NAME:  
       lotus chain export - export chain to a car file  
      
    USAGE:  
       lotus chain export [command options] [outputPath]  
      
    OPTIONS:  
       --tipset value    
       --help, -h      show help (default: false)  


​      

### lotus chain slash-consensus  
    NAME:  
       lotus chain slash-consensus - Report consensus fault  
      
    USAGE:  
       lotus chain slash-consensus [command options] [blockCid1 blockCid2]  
      
    OPTIONS:  
       --miner value  Miner address  
       --help, -h     show help (default: false)  


​      

## lotus log  
    NAME:  
       lotus log - Manage logging  
      
    USAGE:  
       lotus log command [command options] [arguments...]  
      
    COMMANDS:  
         list       List log systems  
         set-level  Set log level  
         help, h    Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus log list  
    NAME:  
       lotus log list - List log systems  
      
    USAGE:  
       lotus log list [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus log set-level  
    NAME:  
       lotus log set-level - Set log level  
      
    USAGE:  
       lotus log set-level [command options] [level]  
      
    DESCRIPTION:  
       Set the log level for logging systems:  
      
       The system flag can be specified multiple times.  
      
       eg) log set-level --system chain --system blocksync debug  
      
       Available Levels:  
       debug  
       info  
       warn  
       error  
      
       Environment Variables:  
       GOLOG_LOG_LEVEL - Default log level for all log systems  
       GOLOG_LOG_FMT   - Change output log format (json, nocolor)  
       GOLOG_FILE      - Write logs to file in addition to stderr   
       
    OPTIONS:  
       --system value  limit to log system  
       --help, -h      show help (default: false)  


​      

## lotus wait-api  
    NAME:  
       lotus wait-api - Wait for lotus api to come online  
      
    USAGE:  
       lotus wait-api [command options] [arguments...]  
      
    CATEGORY:  
       developer  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

## lotus fetch-params  
    NAME:  
       lotus fetch-params - Fetch proving parameters  
      
    USAGE:  
       lotus fetch-params [command options] [arguments...]  
      
    CATEGORY:  
       developer  
      
    OPTIONS:  
       --proving-params value  download params used creating proofs for given size, i.e. 32GiB  
       --help, -h              show help (default: false)  


​      

## lotus net  
    NAME:  
       lotus net - Manage P2P Network  
      
    USAGE:  
       lotus net command [command options] [arguments...]  
      
    COMMANDS:  
         peers     Print peers  
         connect   Connect to a peer  
         listen    List listen addresses  
         id        Get node identity  
         findpeer  Find the addresses of a given peerID  
         help, h   Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus net peers  
    NAME:  
       lotus net peers - Print peers  
      
    USAGE:  
       lotus net peers [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus net connect  
    NAME:  
       lotus net connect - Connect to a peer  
      
    USAGE:  
       lotus net connect [command options] [peerMultiaddr]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus net listen  
    NAME:  
       lotus net listen - List listen addresses  
      
    USAGE:  
       lotus net listen [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus net id  
    NAME:  
       lotus net id - Get node identity  
      
    USAGE:  
       lotus net id [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus net findpeer  
    NAME:  
       lotus net findpeer - Find the addresses of a given peerID  
      
    USAGE:  
       lotus net findpeer [command options] [peerId]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

## lotus sync  
    NAME:  
       lotus sync - Inspect or interact with the chain syncer  
      
    USAGE:  
       lotus sync command [command options] [arguments...]  
      
    COMMANDS:  
         status     check sync status  
         wait       Wait for sync to be complete  
         mark-bad   Mark the given block as bad, will prevent syncing to a chain that contains it  
         check-bad  check if the given block was marked bad, and for what reason  
         help, h    Shows a list of commands or help for one command  
      
    OPTIONS:  
       --help, -h               show help (default: false)  
       --init-completion value  generate completion code. Value must be 'bash' or 'zsh'  
       --version, -v            print the version (default: false)  


​      

### lotus sync status  
    NAME:  
       lotus sync status - check sync status  
      
    USAGE:  
       lotus sync status [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus sync wait  
    NAME:  
       lotus sync wait - Wait for sync to be complete  
      
    USAGE:  
       lotus sync wait [command options] [arguments...]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus sync mark-bad  
    NAME:  
       lotus sync mark-bad - Mark the given block as bad, will prevent syncing to a chain that contains it  
      
    USAGE:  
       lotus sync mark-bad [command options] [blockCid]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​      

### lotus sync check-bad  
    NAME:  
       lotus sync check-bad - check if the given block was marked bad, and for what reason  
      
    USAGE:  
       lotus sync check-bad [command options] [blockCid]  
      
    OPTIONS:  
       --help, -h  show help (default: false)  


​     