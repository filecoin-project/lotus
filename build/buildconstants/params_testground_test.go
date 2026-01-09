//go:build testground

package buildconstants

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
)

func Test_NetworkName(t *testing.T) {
	require.Equal(t, address.CurrentNetwork, address.Testnet)
}
