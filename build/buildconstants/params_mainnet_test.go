//go:build !debug && !2k && !testground && !calibnet && !butterflynet && !interopnet
// +build !debug,!2k,!testground,!calibnet,!butterflynet,!interopnet

package buildconstants

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
)

func Test_NetworkName(t *testing.T) {
	require.Equal(t, address.CurrentNetwork, address.Mainnet)
}
