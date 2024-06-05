package ethtypes

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
)

func TestEncode(t *testing.T) {
	testcases := []TestCase{
		{[]byte(""), mustDecodeHex("0x80")},
		{mustDecodeHex("0x01"), mustDecodeHex("0x01")},
		{mustDecodeHex("0xaa"), mustDecodeHex("0x81aa")},
		{mustDecodeHex("0x0402"), mustDecodeHex("0x820402")},
		{
			[]interface{}{},
			mustDecodeHex("0xc0"),
		},
		{
			mustDecodeHex("0xabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"),
			mustDecodeHex("0xb83cabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"),
		},
		{
			mustDecodeHex("0xabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"),
			mustDecodeHex("0xb8aaabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"),
		},
		{
			[]interface{}{
				mustDecodeHex("0xaaaa"),
				mustDecodeHex("0xbbbb"),
				mustDecodeHex("0xcccc"),
				mustDecodeHex("0xdddd"),
			},
			mustDecodeHex("0xcc82aaaa82bbbb82cccc82dddd"),
		},
		{
			[]interface{}{
				mustDecodeHex("0xaaaaaaaaaaaaaaaaaaaa"),
				mustDecodeHex("0xbbbbbbbbbbbbbbbbbbbb"),
				[]interface{}{
					mustDecodeHex("0xc1c1c1c1c1c1c1c1c1c1"),
					mustDecodeHex("0xc2c2c2c2c2c2c2c2c2c2"),
					mustDecodeHex("0xc3c3c3c3c3c3c3c3c3c3"),
				},
				mustDecodeHex("0xdddddddddddddddddddd"),
				mustDecodeHex("0xeeeeeeeeeeeeeeeeeeee"),
				mustDecodeHex("0xffffffffffffffffffff"),
			},
			mustDecodeHex("0xf8598aaaaaaaaaaaaaaaaaaaaa8abbbbbbbbbbbbbbbbbbbbe18ac1c1c1c1c1c1c1c1c1c18ac2c2c2c2c2c2c2c2c2c28ac3c3c3c3c3c3c3c3c3c38adddddddddddddddddddd8aeeeeeeeeeeeeeeeeeeee8affffffffffffffffffff"),
		},
	}

	for _, tc := range testcases {
		result, err := EncodeRLP(tc.Input)
		require.Nil(t, err)

		require.Equal(t, tc.Output.([]byte), result)
	}
}

func TestDecodeString(t *testing.T) {
	testcases := []TestCase{
		{"0x00", "0x00"},
		{"0x80", "0x"},
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

func TestDecodeNegativeLength(t *testing.T) {
	testcases := [][]byte{
		mustDecodeHex("0xbfffffffffffffff0041424344"),
		mustDecodeHex("0xc1bFFF1111111111111111"),
		mustDecodeHex("0xbFFF11111111111111"),
		mustDecodeHex("0xbf7fffffffffffffff41424344"),
	}

	for _, tc := range testcases {
		_, err := DecodeRLP(tc)
		require.ErrorContains(t, err, "invalid rlp data")
	}
}

func TestDecodeEncodeTx(t *testing.T) {
	testcases := [][]byte{
		mustDecodeHex("0xdc82013a0185012a05f2008504a817c8008080872386f26fc1000000c0"),
		mustDecodeHex("0xf85f82013a0185012a05f2008504a817c8008080872386f26fc1000000c001a027fa36fb9623e4d71fcdd7f7dce71eb814c9560dcf3908c1719386e2efd122fba05fb4e4227174eeb0ba84747a4fb883c8d4e0fdb129c4b1f42e90282c41480234"),
		mustDecodeHex("0xf9061c82013a0185012a05f2008504a817c8008080872386f26fc10000b905bb608060405234801561001057600080fd5b506127106000803273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550610556806100656000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80637bd703e81461004657806390b98a1114610076578063f8b2cb4f146100a6575b600080fd5b610060600480360381019061005b919061030a565b6100d6565b60405161006d9190610350565b60405180910390f35b610090600480360381019061008b9190610397565b6100f4565b60405161009d91906103f2565b60405180910390f35b6100c060048036038101906100bb919061030a565b61025f565b6040516100cd9190610350565b60405180910390f35b600060026100e38361025f565b6100ed919061043c565b9050919050565b6000816000803373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410156101455760009050610259565b816000803373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546101939190610496565b92505081905550816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546101e891906104ca565b925050819055508273ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef8460405161024c9190610350565b60405180910390a3600190505b92915050565b60008060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b600080fd5b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b60006102d7826102ac565b9050919050565b6102e7816102cc565b81146102f257600080fd5b50565b600081359050610304816102de565b92915050565b6000602082840312156103205761031f6102a7565b5b600061032e848285016102f5565b91505092915050565b6000819050919050565b61034a81610337565b82525050565b60006020820190506103656000830184610341565b92915050565b61037481610337565b811461037f57600080fd5b50565b6000813590506103918161036b565b92915050565b600080604083850312156103ae576103ad6102a7565b5b60006103bc858286016102f5565b92505060206103cd85828601610382565b9150509250929050565b60008115159050919050565b6103ec816103d7565b82525050565b600060208201905061040760008301846103e3565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061044782610337565b915061045283610337565b9250817fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff048311821515161561048b5761048a61040d565b5b828202905092915050565b60006104a182610337565b91506104ac83610337565b9250828210156104bf576104be61040d565b5b828203905092915050565b60006104d582610337565b91506104e083610337565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff038211156105155761051461040d565b5b82820190509291505056fea26469706673582212208e5b4b874c839967f88008ed2fa42d6c2d9c9b0ae05d1d2c61faa7d229c134e664736f6c634300080d0033c080a0c4e9477f57c6848b2f1ea73a14809c1f44529d20763c947f3ac8ffd3d1629d93a011485a215457579bb13ac7b53bb9d6804763ae6fe5ce8ddd41642cea55c9a09a"),
		mustDecodeHex("0xf9063082013a0185012a05f2008504a817c8008094025b594a4f1c4888cafcfaf2bb24ed95507749e0872386f26fc10000b905bb608060405234801561001057600080fd5b506127106000803273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550610556806100656000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80637bd703e81461004657806390b98a1114610076578063f8b2cb4f146100a6575b600080fd5b610060600480360381019061005b919061030a565b6100d6565b60405161006d9190610350565b60405180910390f35b610090600480360381019061008b9190610397565b6100f4565b60405161009d91906103f2565b60405180910390f35b6100c060048036038101906100bb919061030a565b61025f565b6040516100cd9190610350565b60405180910390f35b600060026100e38361025f565b6100ed919061043c565b9050919050565b6000816000803373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410156101455760009050610259565b816000803373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546101939190610496565b92505081905550816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546101e891906104ca565b925050819055508273ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef8460405161024c9190610350565b60405180910390a3600190505b92915050565b60008060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b600080fd5b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b60006102d7826102ac565b9050919050565b6102e7816102cc565b81146102f257600080fd5b50565b600081359050610304816102de565b92915050565b6000602082840312156103205761031f6102a7565b5b600061032e848285016102f5565b91505092915050565b6000819050919050565b61034a81610337565b82525050565b60006020820190506103656000830184610341565b92915050565b61037481610337565b811461037f57600080fd5b50565b6000813590506103918161036b565b92915050565b600080604083850312156103ae576103ad6102a7565b5b60006103bc858286016102f5565b92505060206103cd85828601610382565b9150509250929050565b60008115159050919050565b6103ec816103d7565b82525050565b600060208201905061040760008301846103e3565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061044782610337565b915061045283610337565b9250817fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff048311821515161561048b5761048a61040d565b5b828202905092915050565b60006104a182610337565b91506104ac83610337565b9250828210156104bf576104be61040d565b5b828203905092915050565b60006104d582610337565b91506104e083610337565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff038211156105155761051461040d565b5b82820190509291505056fea26469706673582212208e5b4b874c839967f88008ed2fa42d6c2d9c9b0ae05d1d2c61faa7d229c134e664736f6c634300080d0033c080a0fe38720928596f9e9dfbf891d00311638efce3713f03cdd67b212ecbbcf18f29a05993e656c0b35b8a580da6aff7c89b3d3e8b1c6f83a7ce09473c0699a8500b9c"),
	}

	for _, tc := range testcases {
		decoded, err := DecodeRLP(tc)
		require.Nil(t, err)

		encoded, err := EncodeRLP(decoded)
		require.Nil(t, err)
		require.Equal(t, tc, encoded)
	}
}

func TestDecodeError(t *testing.T) {
	testcases := [][]byte{
		mustDecodeHex("0xdc82013a0185012a05f2008504a817c8008080872386f26fc1000000"),
		mustDecodeHex("0xdc013a01012a05f2008504a817c8008080872386f26fc1000000"),
		mustDecodeHex("0xdc82013a0185012a05f28504a817c08080872386f26fc1000000"),
		mustDecodeHex("0xdc82013a0185012a05f504a817c080872386ffc1000000"),
		mustDecodeHex("0x013a018505f2008504a817c8008080872386f26fc1000000"),
	}

	for _, tc := range testcases {
		_, err := DecodeRLP(tc)
		require.NotNil(t, err, hex.EncodeToString(tc))
	}
}

func TestDecode1(t *testing.T) {
	b := mustDecodeHex("0x02f8758401df5e7680832c8411832c8411830767f89452963ef50e27e06d72d59fcb4f3c2a687be3cfef880de0b6b3a764000080c080a094b11866f453ad85a980e0e8a2fc98cbaeb4409618c7734a7e12ae2f66fd405da042dbfb1b37af102023830ceeee0e703ffba0b8b3afeb8fe59f405eca9ed61072")
	decoded, err := parseEip1559Tx(b)
	require.NoError(t, err)

	sender, err := decoded.Sender()
	require.NoError(t, err)

	addr, err := address.NewFromString("f410fkkld55ioe7qg24wvt7fu6pbknb56ht7pt4zamxa")
	require.NoError(t, err)
	require.Equal(t, sender, addr)
}
