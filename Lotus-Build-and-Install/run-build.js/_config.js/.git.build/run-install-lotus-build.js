Build and install Lotus
Once all the dependencies are installed, you can build and install the Lotus suite (lotus, lotus-miner, and lotus-worker).

Clone the repository:

git clone https://github.com/filecoin-project/lotus.git
cd lotus/
Note: The default branch master is the dev branch where the latest new features, bug fixes and improvement are in. However, if you want to run lotus on Filecoin mainnet and want to run a production-ready lotus, get the latest release here.

To join mainnet, checkout the latest release.

If you are changing networks from a previous Lotus installation or there has been a network reset, read the Switch networks guide before proceeding.

For networks other than mainnet, look up the current branch or tag/commit for the network you want to join in the Filecoin networks dashboard, then build Lotus for your specific network below.

git checkout <tag_or_branch>
# For example:
git checkout <vX.X.X> # tag for a release
Currently, the latest code on the master branch corresponds to mainnet.

If you are in China, see "Lotus: tips when running in China".

This build instruction uses the prebuilt proofs binaries. If you want to build the proof binaries from source check the complete instructions. Note, if you are building the proof binaries from source, installing rustup is also needed.

Build and install Lotus:

make clean all #mainnet

# Or to join a testnet or devnet:
make clean calibnet # Calibration with min 32GiB sectors
make clean nerpanet # Nerpa with min 512MiB sectors

sudo make install
This will put lotus, lotus-miner and lotus-worker in /usr/local/bin.

lotus will use the $HOME/.lotus folder by default for storage (configuration, chain data, wallets, etc). See advanced options for information on how to customize the Lotus folder.

You should now have Lotus installed. You can now start the Lotus daemon and sync the chain.
