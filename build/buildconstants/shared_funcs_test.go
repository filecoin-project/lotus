// External test package: ethtypes depends on buildconstants, so only a test outside the package
// can reach it.
package buildconstants_test

import (
	"testing"

	"github.com/stretchr/testify/require"

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

	require.Panics(t, func() { buildconstants.MustParseFilOrEthAddress("0xdeadbeef") })
	require.Panics(t, func() { buildconstants.MustParseFilOrEthAddress("0xzz5fbdb2315678afecb367f032d93f642f64180a") })
	require.Panics(t, func() { buildconstants.MustParseFilOrEthAddress("not an address") })
}
