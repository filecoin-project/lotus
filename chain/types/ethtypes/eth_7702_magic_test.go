package ethtypes

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

func TestAuthorizationPreimage_Shape(t *testing.T) {
	var addr EthAddress
	for i := range addr {
		addr[i] = 0x11
	}

	pre, err := AuthorizationPreimage(1, addr, 0)
	require.NoError(t, err)
	require.Greater(t, len(pre), 1)
	// First byte must be the MAGIC 0x05
	require.EqualValues(t, SetCodeAuthorizationMagic, pre[0])

	// Decode the RLP tail and check tuple contents
	dec, err := DecodeRLP(pre[1:])
	require.NoError(t, err)
	lst, ok := dec.([]interface{})
	require.True(t, ok)
	require.Len(t, lst, 3)

	ciU, err := parseUint64(lst[0])
	require.NoError(t, err)
	require.Equal(t, uint64(1), ciU)

	gotAddr, err := parseEthAddr(lst[1])
	require.NoError(t, err)
	require.NotNil(t, gotAddr)
	require.Equal(t, addr, *gotAddr)

	nnU, err := parseUint64(lst[2])
	require.NoError(t, err)
	require.Equal(t, uint64(0), nnU)
}

func TestAuthorizationKeccak_DifferentFromWrongDomain(t *testing.T) {
	var addr EthAddress
	for i := range addr {
		addr[i] = 0x22
	}

	good, err := AuthorizationKeccak(314, addr, 7)
	require.NoError(t, err)

	// Compute a bad preimage with wrong domain prefix (0x00) and ensure hashes differ.
	// This guards against accidentally omitting the MAGIC byte.
	ci, _ := formatUint64(314)
	ni, _ := formatUint64(7)
	rl, _ := EncodeRLP([]interface{}{ci, addr[:], ni})
	badPre := append([]byte{0x00}, rl...)
	hh := sha3.NewLegacyKeccak256()
	_, _ = hh.Write(badPre)
	var bad [32]byte
	copy(bad[:], hh.Sum(nil))

	require.NotEqual(t, good, bad)
}
