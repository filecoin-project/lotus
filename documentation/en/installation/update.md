# Updating and restarting Lotus

Updating Lotus is as simple as rebuilding and re-installing the software as explained in the previous sections.

You can verify which version of Lotus you are running with:

```sh
lotus version
```

Make sure that you `git pull` the branch that corresponds to the network that your Lotus daemon is using:

```sh
git pull origin <branch>
make clean
make all
sudo make install # if necessary
```

Finally, restart the Lotus Node and/or Lotus Miner(s).

__CAVEAT__: If you are running miners: check if your miner is safe to shut down and restart: `lotus-miner proving info`. If any deadline shows a block height in the past, do not restart:

In the following example, Deadline Open is 454 which is earlier than Current Epoch of 500. This miner should **not** be shut down or restarted.

```
$ sudo lotus-miner proving info
Miner: t01001
Current Epoch:           500
Proving Period Boundary: 154
Proving Period Start:    154 (2h53m0s ago)
Next Period Start:       3034 (in 21h7m0s)
Faults:      768 (100.00%)
Recovering:  768
Deadline Index:       5
Deadline Sectors:     0
Deadline Open:        454 (23m0s ago)
Deadline Close:       514 (in 7m0s)
Deadline Challenge:   434 (33m0s ago)
Deadline FaultCutoff: 384 (58m0s ago)
```

In this next example, the miner can be safely restarted because no Deadlines are earlier than Current Epoch of 497. You have ~45 minutes before the miner must be back online to declare faults (FaultCutoff). If the miner has no faults, you have about an hour.

```
$ sudo lotus-miner proving info
Miner: t01000
Current Epoch:           497
Proving Period Boundary: 658
Proving Period Start:    658 (in 1h20m30s)
Next Period Start:       3538 (in 25h20m30s)
Faults:      0 (0.00%)
Recovering:  0
Deadline Index:       0
Deadline Sectors:     768
Deadline Open:        658 (in 1h20m30s)
Deadline Close:       718 (in 1h50m30s)
Deadline Challenge:   638 (in 1h10m30s)
Deadline FaultCutoff: 588 (in 45m30s)
```

## Switching networks and network resets

If you wish to switch to a different lotus network or there has been a network reset, you will need to:

* Checkout the appropiate repository branch and rebuild
* Ensure you do not mix Lotus data (`LOTUS_PATH`, usually `~/.lotus`) from a previous or different network. For this, either:
  * Rename the folder to something else or,
  * Set a different `LOTUS_PATH` for the new network.
* Same for `~/.lotusminer` if you are running a miner.

Note that deleting the Lotus data folder will wipe all the chain data, wallets and configuration, so think twice before taking any non-reversible action.
