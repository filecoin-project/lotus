package api

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeString(t *testing.T) {
	testcases := []TestCase{
		{"0x00", "0x00"},
		{"0x0f", "0x0f"},
		{"0x81aa", "0xaa"},
		{"0x820400", "0x0400"},
		{"0xb83cabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd",
			"0xabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"},
	}

	for _, tc := range testcases {
		input, err := hex.DecodeString(strings.Replace(tc.Input.(string), "0x", "", -1))
		require.Nil(t, err)

		output, err := hex.DecodeString(strings.Replace(tc.Output.(string), "0x", "", -1))
		require.Nil(t, err)

		result, err := DecodeRLP(input)
		require.Nil(t, err)
		require.Equal(t, output, result.([]byte))
	}
}

func mustDecodeHex(s string) []byte {
	d, err := hex.DecodeString(strings.Replace(s, "0x", "", -1))
	if err != nil {
		panic(fmt.Errorf("err must be nil: %w", err))
	}
	return d
}

func TestDecodeList(t *testing.T) {
	testcases := []TestCase{
		{"0xc0", []interface{}{}},
		{"0xc100", []interface{}{[]byte{0}}},
		{"0xc3000102", []interface{}{[]byte{0}, []byte{1}, []byte{2}}},
		{"0xc4000181aa", []interface{}{[]byte{0}, []byte{1}, []byte{0xaa}}},
		{"0xc6000181aa81ff", []interface{}{[]byte{0}, []byte{1}, []byte{0xaa}, []byte{0xff}}},
		{"0xf8428aabcdabcdabcdabcdabcd8aabcdabcdabcdabcdabcd8aabcdabcdabcdabcdabcd8aabcdabcdabcdabcdabcd8aabcdabcdabcdabcdabcd8aabcdabcdabcdabcdabcd",
			[]interface{}{
				mustDecodeHex("0xabcdabcdabcdabcdabcd"),
				mustDecodeHex("0xabcdabcdabcdabcdabcd"),
				mustDecodeHex("0xabcdabcdabcdabcdabcd"),
				mustDecodeHex("0xabcdabcdabcdabcdabcd"),
				mustDecodeHex("0xabcdabcdabcdabcdabcd"),
				mustDecodeHex("0xabcdabcdabcdabcdabcd"),
			},
		},
		// EIP-1559 tx: rlp([chain_id, nonce, max_priority_fee_per_gas, max_fee_per_gas, gas_limit, destination, amount, data, access_list
		{"0xf1030185012a05f2008504a817c800825208942b87d1cb599bc2a606db9a0169fcec96af04ad3a880de0b6b3a764000080c0",
			[]interface{}{
				[]byte{3},
				[]byte{1},
				mustDecodeHex("0x012a05f200"),
				mustDecodeHex("0x04a817c800"),
				mustDecodeHex("0x5208"),
				mustDecodeHex("0x2b87d1CB599Bc2a606Db9A0169fcEc96Af04ad3a"),
				mustDecodeHex("0x0de0b6b3a7640000"),
				[]byte{},
				[]interface{}{},
			}},
	}

	for _, tc := range testcases {
		input, err := hex.DecodeString(strings.Replace(tc.Input.(string), "0x", "", -1))
		require.Nil(t, err)

		result, err := DecodeRLP(input)
		require.Nil(t, err)

		fmt.Println(result)
		r := result.([]interface{})
		require.Equal(t, len(tc.Output.([]interface{})), len(r))

		for i, v := range r {
			require.Equal(t, tc.Output.([]interface{})[i], v)
		}
	}
}
