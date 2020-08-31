# Miner setup

This page will guide you through all you need to know to sucessfully run a **Lotus Miner**. Before proceeding, remember that you should be running the Lotus daemon on a fully synced chain.

## Performance tweaks

This is a list of performance tweaks to consider before starting the miner:

### Building

As [explained already](en+install-linux#native-filecoin-ffi-10) should have exported the following variables before building the Lotus applications:

```sh
export RUSTFLAGS="-C target-cpu=native -g"
export FFI_BUILD_FROM_SOURCE=1
```

### Environment

For high performance mining, we recommend setting the following variables in your environment so that they are available when running any of the Lotus applications:

```sh
# See https://github.com/filecoin-project/bellman
export BELLMAN_CPU_UTILIZATION=0.875

# See https://github.com/filecoin-project/rust-fil-proofs/
export FIL_PROOFS_MAXIMIZE_CACHING=1 # More speed at RAM cost (1x sector-size of RAM - 32 GB).
export FIL_PROOFS_USE_GPU_COLUMN_BUILDER=1 # precommit2 GPU acceleration
export FIL_PROOFS_USE_GPU_TREE_BUILDER=1
```

IF YOU ARE RUNNING FROM CHINA:

```sh
export IPFS_GATEWAY="https://proof-parameters.s3.cn-south-1.jdcloud-oss.com/ipfs/"
```

IF YOUR MINER RUNS IN A DIFFERENT MACHINE AS THE LOTUS DAEMON:

```sh
export FULLNODE_API_INFO=<api_token>:/ip4/<lotus_daemon_ip>/tcp/<lotus_daemon_port>/http
```

If you will be using systemd service files to run the Lotus daemon and miner, make sure you include these variables manually in the service files.

### Adding swap

If you have only 128GiB of RAM, you will need to make sure your system provides at least an extra 256GiB of fast swap (preferably NVMe SSD):

```sh
sudo fallocate -l 256G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
# show current swap spaces and take note of the current highest priority
swapon --show
# append the following line to /etc/fstab (ensure highest priority) and then reboot
# /swapfile swap swap pri=50 0 0
sudo reboot
# check a 256GB swap file is automatically mounted and has the highest priority
swapon --show
```

## Creating a new BLS wallet

You will need a BLS wallet (`t3...`) for mining. To create it, if you don't have one already, run:

```sh
lotus wallet new bls
```

Next make sure to [send some funds](en+wallet) to this address so that the miner setup can be completed.

## Initializing the miner

> SPACE RACE:
> To participate in the Space race, please register your miner:
>
> - Visit the [faucet](http://spacerace.faucet.glif.io/)
> - Paste the address you created under REQUEST.
> - Press the Request button.

Now that you have a miner address you can initialize the Lotus Miner:

```sh
lotus-miner init --owner=<bls address>  --no-local-storage
```

* The `--no-local-storage` flag is used so that we configure specific locations for storage later below.
* The init process will download over 100GiB of initialization parameters to /var/tmp/filecoin-proof-parameters. Make sure there is space or set `FIL_PROOFS_PARAMETER_CACHE` to somewhere else.
* The Lotus Miner configuration folder is created at `~/.lotusminer/` or `$LOTUS_MINER_PATH` if set.

## Reachability

Before you start your miner, it is __very important__ to configure it so that it is reachable from any peer in the Filecoin network. For this you will need a stable public IP and edit your `~/.lotusminer/config.toml` as follows:

```toml
...
[Libp2p]
  ListenAddresses = ["/ip4/0.0.0.0/tcp/24001"] # choose a fixed port
  AnnounceAddresses = ["/ip4/<YOUR_PUBLIC_IP_ADDRESS>/tcp/24001"] # important!
...
```

Once you start your miner, make sure you can connect to its public IP/port (you can use `telnet`, `nc` for the task...). If you have an active firewall or some sort, you may need to additionally open ports in it.


## Starting the miner

You are now ready to start your Lotus miner:

```sh
lotus-miner run
```

or if you are using the systemd service file:

```sh
systemctl start lotus-miner
```

> __Do not proceed__ from here until you have verified that your miner not only is running, but also __reachable on its public IP address__.

## Publishing the miner addresses

Once the miner is up and running, publish your miner address (which you configured above) on the chain (please ensure it is dialable):

```sh
lotus-miner actor set-addrs /ip4/<YOUR_PUBLIC_IP_ADDRESS>/tcp/24001
```

## Setting locations for sealing and long-term storage

If you used the `--no-local-storage` flag during initialization, you can now specify the disk locations for sealing (SSD recommended) and long-term storage (otherwise you can skip this):

```
lotus-miner storage attach --init --seal <PATH_FOR_SEALING_STORAGE>
lotus-miner storage attach --init --store <PATH_FOR_LONG_TERM_STORAGE>
lotus-miner storage list
```

## Pledging sectors

If you would like to compete for block rewards by increasing your power in the network as soon as possible, you can optionally pledge one or several sectors, depending on your storage. It can also be used to test that the sealing process works correctly. Pledging is equivalent to storing random data instead of real data obtained through storage deals.

> Note that pledging sectors to the mainnet network makes most sense when trying to obtain a reasonable amount of total power in the network, thus obtaining real chances to mine new blocks. Otherwise it is only useful for testing purposes.

If you decide to go ahead, then do:

```sh
lotus-miner sectors pledge
```

This will write data to `$TMPDIR` so make sure that there is enough space available.

You shoud check that your sealing job has started with:

```sh
lotus-miner sealing jobs
```

This will be accommpanied by a file in `<PATH_FOR_SEALING_STORAGE>/unsealed`.

After some minutes, you can check the sealing progress with:

```sh
lotus-miner sectors list
# and
lotus-miner sealing workers
```

When sealing for the new is complete, `pSet: NO` will become `pSet: YES`.

Once the sealing is finished, you will want to configure how long it took your miner to seal this sector and configure the miner accordingly. To find out how long it took use:

```
lotus-miner sectors status --log 0
```

Once you know, you can edit the Miner's `~/.lotusminer/config.toml` accordingly:

```
...
[Dealmaking]
...
  ExpectedSealDuration = "12h0m0s" # The time it took your miner
```

You can also take the chance to edit other values, such as `WaitForDealsDelay` which specifies the delay between accepting the first deal and sealing, allowing to place multiple deals in the same sector.

Once you are done editing the configuration, [restart your miner](en+update).

If you wish to be able to re-use a pledged sector for real storage deals before the pledged period of 6 months ends, you will need to mark them for upgrade:

```sh
lotus-miner sectors mark-for-upgrade <sector number>
```

The sector should become inactive within 24 hours. From that point, the pledged storage can be re-used to store real data associated with real storage deals.

## Separate address for windowPoSt messages

WindowPoSt is the mechanism through which storage is verified in Filecoin. It requires miners to submit proofs for all sectors every 24h, which require sending messages to the chain.

Because many other mining related actions require sending messages to the chain, and not all of those are "high value", it may be desirable to use a separate account to send PoSt messages from. This allows for setting lower GasFeeCaps on the lower value messages without creating head-of-line blocking problems for the PoSt messages in congested chain conditions

To set this up, first create a new account, and send it some funds for gas fees:

```sh
lotus wallet new bls
t3defg...

lotus send t3defg... 100
```

Next add the control address:

```sh
lotus-miner actor control set --really-do-it t3defg...
Add t3defg...
Message CID: bafy2..
```

Wait for the message to land on chain:

```sh
lotus state wait-msg bafy2..
...
Exit Code: 0
...
```

Finally, check the miner control address list to make sure the address was correctly setup:

```sh
lotus-miner actor control list
name       ID      key           use    balance
owner      t01111  t3abcd...  other  300 FIL
worker     t01111  t3abcd...  other  300 FIL
control-0  t02222  t3defg...  post   100 FIL
```
