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
