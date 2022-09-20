//stm: #unit
package api

import (
	mathbig "math/big"
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
func TestEthIntUnmarshalJSON(t *testing.T) {
	testcases := []TestCase{
		{[]byte("\"0x0\""), EthInt(0)},
		{[]byte("\"0x41\""), EthInt(65)},
		{[]byte("\"0x400\""), EthInt(1024)},
	}

	for _, tc := range testcases {
		var i EthInt
		err := i.UnmarshalJSON(tc.Input.([]byte))
		require.Nil(t, err)
		require.Equal(t, i, tc.Output)
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

func TestEthBigIntUnmarshalJSON(t *testing.T) {
	testcases := []TestCase{
		{[]byte("\"0x0\""), EthBigInt(big.MustFromString("0"))},
		{[]byte("\"0x41\""), EthBigInt(big.MustFromString("65"))},
		{[]byte("\"0x400\""), EthBigInt(big.MustFromString("1024"))},
		{[]byte("\"0xff1000000000000000000000000\""), EthBigInt(big.MustFromString("323330131220712761719252861321216"))},
	}

	for _, tc := range testcases {
		var i EthBigInt
		err := i.UnmarshalJSON(tc.Input.([]byte))
		require.Nil(t, err)
		require.Equal(t, i, tc.Output)
	}
}

func TestEthHash(t *testing.T) {
	testcases := []string{
		`"0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184"`,
		`"0xab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738"`,
	}

	for _, hash := range testcases {
		var h EthHash
		err := h.UnmarshalJSON([]byte(hash))

		require.Nil(t, err)
		require.Equal(t, h.String(), strings.Replace(hash, `"`, "", -1))

		c := h.ToCid()
		h1, err := EthHashFromCid(c)
		require.Nil(t, err)
		require.Equal(t, h, h1)
	}
}

func TestEthAddr(t *testing.T) {
	testcases := []string{
		strings.ToLower(`"0xd4c5fb16488Aa48081296299d54b0c648C9333dA"`),
		strings.ToLower(`"0x2C2EC67e3e1FeA8e4A39601cB3A3Cd44f5fa830d"`),
		strings.ToLower(`"0x01184F793982104363F9a8a5845743f452dE0586"`),
	}

	for _, addr := range testcases {
		var a EthAddress
		err := a.UnmarshalJSON([]byte(addr))

		require.Nil(t, err)
		require.Equal(t, a.String(), strings.Replace(addr, `"`, "", -1))
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

func TestUnmarshalEthCall(t *testing.T) {
	data := `{"from":"0x4D6D86b31a112a05A473c4aE84afaF873f632325","to":"0xFe01CC39f5Ae8553D6914DBb9dC27D219fa22D7f","gas":"0x5","gasPrice":"0x6","value":"0x123","data":""}`

	var c EthCall
	err := c.UnmarshalJSON([]byte(data))
	require.Nil(t, err)
}

func TestUnmarshalEthBytes(t *testing.T) {
	testcases := []string{
		`"0x00"`,
		strings.ToLower(`"0xd4c5fb16488Aa48081296299d54b0c648C9333dA"`),
		strings.ToLower(`"0x2C2EC67e3e1FeA8e4A39601cB3A3Cd44f5fa830d"`),
		strings.ToLower(`"0x01184F793982104363F9a8a5845743f452dE0586"`),
	}

	for _, tc := range testcases {
		var s EthBytes
		err := s.UnmarshalJSON([]byte(tc))
		require.Nil(t, err)

		data, err := s.MarshalJSON()
		require.Nil(t, err)
		require.Equal(t, string(data), tc)
	}
}

func TestParseEthTx(t *testing.T) {
	mustParseEthAddr := func(s string) *EthAddress {
		a, err := EthAddressFromHex(s)
		if err != nil {
			panic(err)
		}
		return &a
	}
	takePointer := func(v EthInt) *EthInt {
		return &v
	}

	testcases := []TestCase{
		{`"0x02f1030185012a05f2008504a817c800825208942b87d1cb599bc2a606db9a0169fcec96af04ad3a880de0b6b3a764000080c0"`,
			EthTx{
				ChainID:              EthInt(3),
				Nonce:                EthInt(1),
				Type:                 2,
				To:                   mustParseEthAddr("0x2b87d1CB599Bc2a606Db9A0169fcEc96Af04ad3a"),
				MaxPriorityFeePerGas: EthBigInt{Int: mathbig.NewInt(5000000000)},
				MaxFeePerGas:         EthBigInt{Int: mathbig.NewInt(20000000000)},
				GasLimit:             takePointer(EthInt(21000)),
			},
		},
	}

	for _, tc := range testcases {
		var b EthBytes
		err := b.UnmarshalJSON([]byte(tc.Input.(string)))
		require.Nil(t, err)

		tx, err := ParseEthTx(b)
		require.Nil(t, err)

		expected := tc.Output.(EthTx)
		require.Equal(t, expected.ChainID, tx.ChainID)
		require.Equal(t, expected.Nonce, tx.Nonce)
		require.Equal(t, expected.Type, tx.Type)
		require.Equal(t, expected.To, tx.To)
		require.Equal(t, expected.MaxFeePerGas, tx.MaxFeePerGas)
		require.Equal(t, expected.MaxPriorityFeePerGas, tx.MaxPriorityFeePerGas)
		require.Equal(t, expected.GasLimit, tx.GasLimit)
	}

}
