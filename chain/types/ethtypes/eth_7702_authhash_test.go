package ethtypes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEthAuthorization_DomainHash_UsesTupleFields(t *testing.T) {
	var addr EthAddress
	for i := range addr {
		addr[i] = 0x33
	}
	auth := EthAuthorization{ChainID: EthUint64(123), Address: addr, Nonce: EthUint64(9)}
	h1, err := auth.DomainHash()
	require.NoError(t, err)

	// Mutate nonce -> hash must change
	auth2 := auth
	auth2.Nonce = EthUint64(10)
	h2, err := auth2.DomainHash()
	require.NoError(t, err)
	require.NotEqual(t, h1, h2)

	// Same tuple computed via raw helper
	h3, err := AuthorizationKeccak(uint64(auth.ChainID), auth.Address, uint64(auth.Nonce))
	require.NoError(t, err)
	require.Equal(t, h1, h3)
}

func TestAuthorizationKeccak_BoundaryValues(t *testing.T) {
	var addr EthAddress
	for i := range addr {
		addr[i] = 0xAB
	}
	// chainId=0, nonce=0
	h0, err := AuthorizationKeccak(0, addr, 0)
	require.NoError(t, err)
	// chainId=max, nonce=max
	hmax, err := AuthorizationKeccak(^uint64(0), addr, ^uint64(0))
	require.NoError(t, err)
	// Distinct hashes expected
	require.NotEqual(t, h0, hmax)
}

func TestAuthorizationKeccak_AddressEdgeCases(t *testing.T) {
	// All-zero address
	var zero EthAddress
	// All-0xff address
	var ff EthAddress
	for i := range ff {
		ff[i] = 0xFF
	}
	// One-bit toggle
	var one EthAddress
	one[19] = 0x01

	hZero, err := AuthorizationKeccak(1, zero, 2)
	require.NoError(t, err)
	hFF, err := AuthorizationKeccak(1, ff, 2)
	require.NoError(t, err)
	hOne, err := AuthorizationKeccak(1, one, 2)
	require.NoError(t, err)

	// Pairwise distinct
	require.NotEqual(t, hZero, hFF)
	require.NotEqual(t, hZero, hOne)
	require.NotEqual(t, hFF, hOne)

	// Cross-check against EthAuthorization.DomainHash for zero address
	a := EthAuthorization{ChainID: EthUint64(1), Address: zero, Nonce: EthUint64(2)}
	hAuth, err := a.DomainHash()
	require.NoError(t, err)
	require.Equal(t, hZero, hAuth)
}
