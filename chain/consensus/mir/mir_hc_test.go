package mir

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
)

func TestHCSpec(t *testing.T) {
	addr1, err := address.NewIDAddress(101)
	require.NoError(t, err)
	addr, err := address.NewHCAddress(address.RootSubnet, addr1)
	require.NoError(t, err)

	parts := strings.Split(addr.PrettyPrint(), address.HCAddrSeparator)
	require.Equal(t, 2, len(parts))
	require.Equal(t, strings.Contains(addr.PrettyPrint(), ":"), true)
	require.Equal(t, strings.Contains(addr.PrettyPrint(), "::"), false)
}
