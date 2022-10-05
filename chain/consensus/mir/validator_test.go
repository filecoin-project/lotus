package mir

import (
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/stretchr/testify/require"
)

func TestValidatorsFromString(t *testing.T) {
	sAddr := "t1wpixt5mihkj75lfhrnaa6v56n27epvlgwparujy"
	v, err := ValidatorFromString(sAddr + "@/ip4/127.0.0.1/tcp/10000/p2p/12D3KooWJhKBXvytYgPCAaiRtiNLJNSFG5jreKDu2jiVpJetzvVJ")
	require.NoError(t, err)
	addr, err := address.NewFromString(sAddr)
	require.NoError(t, err)
	require.Equal(t, addr, v.Addr)
}
