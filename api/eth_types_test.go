//stm: #unit
package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
)

type TestCase struct {
	Input  interface{}
	Output interface{}
}

func TestEthIntMarshalJSON(t *testing.T) {
	// https://ethereum.org/en/developers/docs/apis/json-rpc/#quantities-encoding
	testcases := []TestCase{
		{EthInt(0), []byte("\"0x0\"")},
		{EthInt(65), []byte("\"0x41\"")},
		{EthInt(1024), []byte("\"0x400\"")},
	}

	for _, tc := range testcases {
		j, err := tc.Input.(EthInt).MarshalJSON()
		require.Nil(t, err)
		require.Equal(t, j, tc.Output)
	}
}

func TestEthBigIntMarshalJSON(t *testing.T) {
	testcases := []TestCase{
		{EthBigInt(big.NewInt(0)), []byte("\"0x0\"")},
		{EthBigInt(big.NewInt(65)), []byte("\"0x41\"")},
		{EthBigInt(big.NewInt(1024)), []byte("\"0x400\"")},
		{EthBigInt(big.Int{}), []byte("\"0x0\"")},
	}
	for _, tc := range testcases {
		j, err := tc.Input.(EthBigInt).MarshalJSON()
		require.Nil(t, err)
		require.Equal(t, j, tc.Output)
	}
}

func TestEthHash(t *testing.T) {
	testcases := []string{
		"0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184",
		"0xab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738",
	}

	for _, hash := range testcases {
		h, err := EthHashFromHex(hash)
		require.Nil(t, err)
		require.Equal(t, h.String(), hash)

		c := h.ToCid()
		h1, err := EthHashFromCid(c)
		require.Nil(t, err)
		require.Equal(t, h, h1)
	}
}

func TestEthAddr(t *testing.T) {
	testcases := []string{
		strings.ToLower("0xd4c5fb16488Aa48081296299d54b0c648C9333dA"),
		strings.ToLower("0x2C2EC67e3e1FeA8e4A39601cB3A3Cd44f5fa830d"),
		strings.ToLower("0x01184F793982104363F9a8a5845743f452dE0586"),
	}

	for _, addr := range testcases {
		a, err := EthAddressFromHex(addr)
		require.Nil(t, err)
		require.Equal(t, a.String(), addr)
	}
}

func TestParseEthAddr(t *testing.T) {
	testcases := []uint64{
		1, 2, 3, 100, 101,
	}
	for _, id := range testcases {
		addr, err := address.NewIDAddress(id)
		require.Nil(t, err)

		eaddr, err := EthAddressFromFilecoinIDAddress(addr)
		require.Nil(t, err)

		faddr, err := eaddr.ToFilecoinAddress()
		require.Nil(t, err)

		require.Equal(t, addr, faddr)
	}
}
