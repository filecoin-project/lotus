//go:build butterflynet

package buildconstants

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-keccak"
)

func Test_NetworkName(t *testing.T) {
	require.Equal(t, address.CurrentNetwork, address.Testnet)
}

// The deployer key published above UpgradeSolsticeRewardBootstrapParams.
const solsticeDeployer = "0x48C7DC38e74C9fA9eA6484Ad6Ad0520349dC9B40"

// The SRA and SWA are proxies the deployer creates at nonces 1 and 3, so the baked addresses
// follow from the deployer address alone.
func TestSolsticeContractAddresses(t *testing.T) {
	require.Equal(t, MustParseFilOrEthAddress(solsticeDeployer),
		UpgradeSolsticeRewardBootstrapParams.InitialOrchestrator)
	require.Equal(t, createAddress(t, solsticeDeployer, 1),
		UpgradeSolsticeRewardBootstrapParams.SRAActor)
	require.Equal(t, createAddress(t, solsticeDeployer, 3),
		UpgradeSolsticeRewardBootstrapParams.SWAActor)
}

// createAddress derives an EVM CREATE address, keccak256(rlp([deployer, nonce]))[12:], for a
// 20-byte deployer and a nonce that RLP encodes as a single byte.
func createAddress(t *testing.T, deployer string, nonce byte) address.Address {
	t.Helper()

	payload, err := hex.DecodeString(strings.TrimPrefix(deployer, "0x"))
	require.NoError(t, err)
	require.Len(t, payload, ethAddressLength)
	require.Less(t, nonce, byte(0x80))

	rlp := append([]byte{0xc0 + 1 + byte(ethAddressLength) + 1, 0x80 + byte(ethAddressLength)}, payload...)
	if nonce == 0 {
		rlp = append(rlp, 0x80)
	} else {
		rlp = append(rlp, nonce)
	}

	hasher := keccak.NewLegacyKeccak256()
	hasher.Write(rlp)
	return MustParseFilOrEthAddress("0x" + hex.EncodeToString(hasher.Sum(nil)[12:]))
}
