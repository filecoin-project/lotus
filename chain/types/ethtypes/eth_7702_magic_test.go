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

	ci, err := parseInt(lst[0])
	require.NoError(t, err)
	require.Equal(t, 1, ci)

	gotAddr, err := parseEthAddr(lst[1])
	require.NoError(t, err)
	require.NotNil(t, gotAddr)
	require.Equal(t, addr, *gotAddr)

	nn, err := parseInt(lst[2])
	require.NoError(t, err)
	require.Equal(t, 0, nn)
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
	ci, _ := formatInt(314)
	ni, _ := formatInt(7)
	rl, _ := EncodeRLP([]interface{}{ci, addr[:], ni})
	badPre := append([]byte{0x00}, rl...)
	hh := sha3.NewLegacyKeccak256()
	_, _ = hh.Write(badPre)
	var bad [32]byte
	copy(bad[:], hh.Sum(nil))

	require.NotEqual(t, good, bad)
}
