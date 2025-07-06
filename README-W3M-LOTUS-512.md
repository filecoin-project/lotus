# W3M Lotus 512 MiB Test Network Configuration

This configuration sets up a custom Lotus build optimized for testing with 512 MiB sectors and a specific bootstrap node.

## Overview

The `w3m-lotus-512` build configuration provides:
- **512 MiB minimum consensus power** (suitable for testing)
- **10-second block times** (3x faster than mainnet)
- **Custom bootstrap node** support
- **Development network** settings with debugging enabled
- **Shorter PreCommit delays** for faster testing

## Quick Start

### 1. Configure Your Bootstrap Node

Edit the bootstrap configuration file with your node's multiaddress:

```bash
vim build/bootstrap/w3m-lotus-512.pi
```

Replace the placeholder with your actual bootstrap node:
```
/ip4/YOUR_IP_HERE/tcp/YOUR_PORT_HERE/p2p/YOUR_PEER_ID_HERE
```

Example:
```
/ip4/192.168.1.100/tcp/1234/p2p/12D3KooWBF8cpp65hp2u9LK5mh19x67ftAam84z9LsfaquTDSBpt
```

### 2. Build the Binaries

```bash
# Clean any existing builds
make clean

# Build all w3m-lotus-512 binaries
make w3mlotus512

# Or build individual components:
make w3mlotus512-lotus        # Lotus daemon
make w3mlotus512-lotus-miner  # Lotus miner
make w3mlotus512-lotus-worker # Lotus worker
```

### 3. Initialize and Run

#### Start the Lotus Daemon
```bash
# Initialize the daemon (first time only)
./lotus daemon --genesis-file=/path/to/genesis.car

# The daemon will automatically connect to your configured bootstrap node
```

#### Initialize the Miner with 512 MiB Sectors
```bash
# Create a new miner with 512 MiB sectors
./lotus-miner init --sector-size=512MiB

# Start the miner
./lotus-miner run
```

## Configuration Details

### Network Parameters
- **Build Tag**: `w3mlotus512`
- **Network Bundle**: `devnet`
- **Actor Debugging**: Enabled
- **Block Delay**: 10 seconds
- **Minimum Consensus Power**: 512 MiB
- **PreCommit Challenge Delay**: 10 epochs
- **Bootstrap Peers Threshold**: 4

### File Locations
- **Bootstrap Configuration**: `build/bootstrap/w3m-lotus-512.pi`
- **Build Parameters**: `build/buildconstants/params_w3m-lotus-512.go`
- **Network Configuration**: Uses devnet actor bundles

### Build System
The Makefile has been extended with:
- `W3MLOTUS512_FLAGS=-tags=w3mlotus512`
- Pattern rule: `w3mlotus512-%` for building network-specific binaries
- Main target: `make w3mlotus512` to build all components

## Alternative: Environment Variable

If you prefer not to modify the code, you can use the environment variable to override bootstrap peers:

```bash
export LOTUS_P2P_BOOTSTRAPPERS="/ip4/YOUR_IP/tcp/YOUR_PORT/p2p/YOUR_PEER_ID"
./lotus daemon
```

## Important Notes

1. **Not for Production**: This configuration is designed for testing and development only. The low consensus power requirement (512 MiB) makes it unsuitable for production use.

2. **Sector Size**: While the minimum power is 512 MiB, you can still seal larger sectors (32 GiB, 64 GiB) if needed. The 512 MiB is just the minimum to gain power on the network.

3. **Genesis File**: You'll need a genesis file to start your network. This can be created using `lotus-seed` or obtained from your network administrator.

4. **Network Isolation**: This configuration creates an isolated network that won't connect to mainnet or other testnets due to the custom bootstrap node.

## Troubleshooting

### Bootstrap Connection Issues
- Ensure your bootstrap node is running and accessible
- Check firewall settings allow connections on the specified port
- Verify the multiaddress format is correct

### Build Issues
- Ensure you have the correct Go version installed (check GO_VERSION_MIN)
- Run `make deps` if you encounter dependency issues
- Check that the FFI submodule is properly initialized

### Sector Size Issues
- If miner init fails, ensure you have enough disk space for 512 MiB sectors
- Check that proof parameters for 512 MiB are downloaded (lotus will auto-download if needed)

## For Developers

To modify the configuration, edit:
- `build/buildconstants/params_w3m-lotus-512.go` for network parameters
- `build/bootstrap/w3m-lotus-512.pi` for bootstrap peers
- Add more bootstrap nodes (one per line) if running multiple bootstrap nodes