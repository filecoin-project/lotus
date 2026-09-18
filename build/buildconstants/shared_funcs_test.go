// External test package: ethtypes depends on buildconstants, so only a test outside the package
// can reach it.
package buildconstants_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// MustParseFilOrEthAddress carries its own copy of the EVM conversion, so hold it to the same
// output as ethtypes, and to the same output for either spelling of one address.
func TestMustParseFilOrEthAddress(t *testing.T) {
	for _, eth := range []string{
		"0x5fbdb2315678afecb367f032d93f642f64180aa3",
		"0x0000000000000000000000000000000000000000",
		"0xff00000000000000000000000000000000000063", // masked ID, f099
		"0xff00000000000000000000000000000000000000", // masked ID, f00
	} {
		t.Run(eth, func(t *testing.T) {
			parsed, err := ethtypes.ParseEthAddress(eth)
			require.NoError(t, err)
			expected, err := parsed.ToFilecoinAddress()
			require.NoError(t, err)

			require.Equal(t, expected, buildconstants.MustParseFilOrEthAddress(eth))
			require.Equal(t, expected, buildconstants.MustParseFilOrEthAddress(expected.String()))
		})
	}

	// Addresses with no EVM spelling take the plain Filecoin branch.
	secp, err := address.NewSecp256k1Address(bytes.Repeat([]byte{0x02}, 65))
	require.NoError(t, err)
	bls, err := address.NewBLSAddress(bytes.Repeat([]byte{0x03}, 48))
	require.NoError(t, err)
	actor, err := address.NewActorAddress([]byte("actor"))
	require.NoError(t, err)

	for _, fil := range []address.Address{
		builtin.SystemActorAddr,
		builtin.BurntFundsActorAddr,
		secp,
		bls,
		actor,
	} {
		t.Run(fil.String(), func(t *testing.T) {
			require.Equal(t, fil, buildconstants.MustParseFilOrEthAddress(fil.String()))
		})
	}

	require.Panics(t, func() { buildconstants.MustParseFilOrEthAddress("0xdeadbeef") })
	require.Panics(t, func() { buildconstants.MustParseFilOrEthAddress("0xzz5fbdb2315678afecb367f032d93f642f64180a") })
	require.Panics(t, func() { buildconstants.MustParseFilOrEthAddress("not an address") })
}
