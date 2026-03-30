package ethtypes

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
)

func mustDecodeHex(s string) []byte {
	d, err := hex.DecodeString(strings.Replace(s, "0x", "", -1))
	if err != nil {
		panic(fmt.Errorf("err must be nil: %w", err))
	}
	return d
}

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
		require.NoError(t, err)

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
		require.NoError(t, err)

		output, err := hex.DecodeString(strings.Replace(tc.Output.(string), "0x", "", -1))
		require.NoError(t, err)

		result, err := DecodeRLP(input)
		require.NoError(t, err)
		require.Equal(t, output, result.([]byte))
	}
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
		require.NoError(t, err)

		result, err := DecodeRLP(input)
		require.NoError(t, err)

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
		require.NoError(t, err)

		encoded, err := EncodeRLP(decoded)
		require.NoError(t, err)
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

// TestDecodeLimits verifies the decoder's input validation bounds: empty input,
// max input size, list nesting depth, and per-list element count.
func TestDecodeLimits(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		_, err := DecodeRLP([]byte{})
		require.ErrorContains(t, err, "data is empty")
	})

	t.Run("input too large", func(t *testing.T) {
		// Just over the limit; content doesn't matter, the check is pre-parse.
		data := make([]byte, maxInputLen+1)
		data[0] = 0x80
		_, err := DecodeRLP(data)
		require.ErrorContains(t, err, "exceeds limit")
	})

	t.Run("nesting exceeds limit", func(t *testing.T) {
		payload := bytes.Repeat([]byte{0xc1}, maxListDepth+2)
		_, err := DecodeRLP(payload)
		require.ErrorContains(t, err, "nesting depth exceeds limit")
	})

	t.Run("nesting at limit with leaf value", func(t *testing.T) {
		// Build a validly-encoded nested structure: maxListDepth layers of
		// short lists wrapping a leaf byte. Each prefix encodes the correct
		// content length:
		//   innermost: 0xc1 0x42             (list with 1-byte content)
		//   next out:  0xc2 0xc1 0x42        (list with 2-byte content)
		//   next out:  0xc3 0xc2 0xc1 0x42   (list with 3-byte content)
		depth := maxListDepth
		data := make([]byte, depth+1)
		for i := 0; i < depth; i++ {
			data[i] = byte(0xc0 + depth - i)
		}
		data[depth] = 0x42
		result, err := DecodeRLP(data)
		require.NoError(t, err)

		// Unwrap the nested lists down to the leaf.
		var cur interface{} = result
		for i := 0; i < depth; i++ {
			lst, ok := cur.([]interface{})
			require.True(t, ok, "expected list at depth %d", i)
			require.Len(t, lst, 1)
			cur = lst[0]
		}
		require.Equal(t, []byte{0x42}, cur)
	})

	t.Run("list element count exceeds limit", func(t *testing.T) {
		// Encode a list with maxListElements+1 single-byte elements.
		elems := make([]byte, maxListElements+1)
		for i := range elems {
			elems[i] = byte(i)
		}
		// Short list prefix: 0xc0 + content length.
		data := append([]byte{byte(0xc0 + len(elems))}, elems...)
		_, err := DecodeRLP(data)
		require.ErrorContains(t, err, "incorrect list length")
	})
}

// TestDecodeCanonical verifies that the decoder rejects non-canonical RLP
// encodings. Test vectors are derived from go-ethereum's rlp test suite
// (ErrCanonSize cases in decode_test.go and raw_test.go).
func TestDecodeCanonical(t *testing.T) {
	// Single byte values [0x00..0x7f] must not use a string prefix.
	t.Run("single byte with unnecessary prefix", func(t *testing.T) {
		for _, tc := range []string{
			"8100", // 0x00 should be encoded as 0x00
			"8101", // 0x01 should be encoded as 0x01
			"8105", // 0x05 should be encoded as 0x05
			"817f", // 0x7f should be encoded as 0x7f
		} {
			_, err := DecodeRLP(mustDecodeHex("0x" + tc))
			require.ErrorContains(t, err, "non-canonical single byte", "input: %s", tc)
		}
		// 0x80 is the first value that legitimately needs the prefix.
		_, err := DecodeRLP(mustDecodeHex("0x8180"))
		require.NoError(t, err)
	})

	// Long string prefix (0xb8+) must not be used for lengths <= 55.
	t.Run("long string prefix for short content", func(t *testing.T) {
		for _, tc := range []string{
			"b800",                              // 0-byte string: should use 0x80
			"b80100",                            // 1-byte string: should use 0x81
			"b83700" + strings.Repeat("aa", 54), // 55-byte string: should use 0xb7
		} {
			_, err := DecodeRLP(mustDecodeHex("0x" + tc))
			require.ErrorContains(t, err, "non-canonical", "input prefix: %s", tc[:4])
		}
	})

	// Long list prefix (0xf8+) must not be used for lengths <= 55.
	t.Run("long list prefix for short content", func(t *testing.T) {
		for _, tc := range []string{
			"f800",   // 0-byte list: should use 0xc0
			"f80100", // 1-byte list: should use 0xc1
		} {
			_, err := DecodeRLP(mustDecodeHex("0x" + tc))
			require.ErrorContains(t, err, "non-canonical", "input prefix: %s", tc[:4])
		}
	})

	// Length encoding must not have leading zero bytes.
	t.Run("leading zeros in length", func(t *testing.T) {
		for _, tc := range []string{
			"b90055" + strings.Repeat("aa", 0x55), // 2-byte len but 0x0055 has leading zero
			"ba0002ffff",                          // 3-byte len but 0x0002ff has leading zero
		} {
			_, err := DecodeRLP(mustDecodeHex("0x" + tc))
			require.ErrorContains(t, err, "non-canonical", "input: %s...", tc[:6])
		}
	})
}

// TestDecodeLengthMismatch checks that the decoder rejects payloads where the
// declared length does not match the available data (truncated payloads or
// extra trailing bytes).
func TestDecodeLengthMismatch(t *testing.T) {
	testcases := []struct {
		name  string
		input string
	}{
		{"short string truncated", "8201"},                           // claims 2 bytes, only 1
		{"long string truncated", "b838" + strings.Repeat("ff", 10)}, // claims 56, only 10
		{"short list trailing data", "c3000102ff"},                   // list claims 3 bytes content, but 4 follow (extra byte)
		{"long list truncated", "f838" + strings.Repeat("00", 10)},   // claims 56 bytes content, only 10
		{"string with trailing data", "850505050505ff"},              // claims 5 bytes, has 5 + trailing
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeRLP(mustDecodeHex("0x" + tc.input))
			require.Error(t, err, "input: %s", tc.input)
		})
	}
}

// TestDecodeEncodeRoundtrip verifies encode/decode symmetry across a range
// of value types including edge cases at encoding boundaries.
func TestDecodeEncodeRoundtrip(t *testing.T) {
	testcases := []struct {
		name string
		val  interface{}
	}{
		{"empty string", []byte{}},
		{"single byte 0x00", []byte{0x00}},
		{"single byte 0x7f", []byte{0x7f}},
		{"single byte 0x80", []byte{0x80}},
		{"55 byte string", bytes.Repeat([]byte{0xab}, 55)},
		{"56 byte string", bytes.Repeat([]byte{0xab}, 56)},
		{"256 byte string", bytes.Repeat([]byte{0xab}, 256)},
		{"empty list", []interface{}{}},
		{"list with empty string", []interface{}{[]byte{}}},
		{"nested empty lists", []interface{}{[]interface{}{}}},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := EncodeRLP(tc.val)
			require.NoError(t, err)

			decoded, err := DecodeRLP(encoded)
			require.NoError(t, err)

			reencoded, err := EncodeRLP(decoded)
			require.NoError(t, err)
			require.Equal(t, encoded, reencoded)
		})
	}
}

// TestDecodeEncodeTxRoundtrip is the original transaction roundtrip test,
// exercising real transaction payloads including contract-deploy sizes.
func TestDecodeEncodeTxRoundtrip(t *testing.T) {
	testcases := [][]byte{
		mustDecodeHex("0xdc82013a0185012a05f2008504a817c8008080872386f26fc1000000c0"),
		mustDecodeHex("0xf85f82013a0185012a05f2008504a817c8008080872386f26fc1000000c001a027fa36fb9623e4d71fcdd7f7dce71eb814c9560dcf3908c1719386e2efd122fba05fb4e4227174eeb0ba84747a4fb883c8d4e0fdb129c4b1f42e90282c41480234"),
		mustDecodeHex("0xf9061c82013a0185012a05f2008504a817c8008080872386f26fc10000b905bb608060405234801561001057600080fd5b506127106000803273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550610556806100656000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80637bd703e81461004657806390b98a1114610076578063f8b2cb4f146100a6575b600080fd5b610060600480360381019061005b919061030a565b6100d6565b60405161006d9190610350565b60405180910390f35b610090600480360381019061008b9190610397565b6100f4565b60405161009d91906103f2565b60405180910390f35b6100c060048036038101906100bb919061030a565b61025f565b6040516100cd9190610350565b60405180910390f35b600060026100e38361025f565b6100ed919061043c565b9050919050565b6000816000803373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410156101455760009050610259565b816000803373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546101939190610496565b92505081905550816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546101e891906104ca565b925050819055508273ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef8460405161024c9190610350565b60405180910390a3600190505b92915050565b60008060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b600080fd5b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b60006102d7826102ac565b9050919050565b6102e7816102cc565b81146102f257600080fd5b50565b600081359050610304816102de565b92915050565b6000602082840312156103205761031f6102a7565b5b600061032e848285016102f5565b91505092915050565b6000819050919050565b61034a81610337565b82525050565b60006020820190506103656000830184610341565b92915050565b61037481610337565b811461037f57600080fd5b50565b6000813590506103918161036b565b92915050565b600080604083850312156103ae576103ad6102a7565b5b60006103bc858286016102f5565b92505060206103cd85828601610382565b9150509250929050565b60008115159050919050565b6103ec816103d7565b82525050565b600060208201905061040760008301846103e3565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061044782610337565b915061045283610337565b9250817fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff048311821515161561048b5761048a61040d565b5b828202905092915050565b60006104a182610337565b91506104ac83610337565b9250828210156104bf576104be61040d565b5b828203905092915050565b60006104d582610337565b91506104e083610337565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff038211156105155761051461040d565b5b82820190509291505056fea26469706673582212208e5b4b874c839967f88008ed2fa42d6c2d9c9b0ae05d1d2c61faa7d229c134e664736f6c634300080d0033c080a0c4e9477f57c6848b2f1ea73a14809c1f44529d20763c947f3ac8ffd3d1629d93a011485a215457579bb13ac7b53bb9d6804763ae6fe5ce8ddd41642cea55c9a09a"),
		mustDecodeHex("0xf9063082013a0185012a05f2008504a817c8008094025b594a4f1c4888cafcfaf2bb24ed95507749e0872386f26fc10000b905bb608060405234801561001057600080fd5b506127106000803273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550610556806100656000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80637bd703e81461004657806390b98a1114610076578063f8b2cb4f146100a6575b600080fd5b610060600480360381019061005b919061030a565b6100d6565b60405161006d9190610350565b60405180910390f35b610090600480360381019061008b9190610397565b6100f4565b60405161009d91906103f2565b60405180910390f35b6100c060048036038101906100bb919061030a565b61025f565b6040516100cd9190610350565b60405180910390f35b600060026100e38361025f565b6100ed919061043c565b9050919050565b6000816000803373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410156101455760009050610259565b816000803373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546101939190610496565b92505081905550816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546101e891906104ca565b925050819055508273ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef8460405161024c9190610350565b60405180910390a3600190505b92915050565b60008060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b600080fd5b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b60006102d7826102ac565b9050919050565b6102e7816102cc565b81146102f257600080fd5b50565b600081359050610304816102de565b92915050565b6000602082840312156103205761031f6102a7565b5b600061032e848285016102f5565b91505092915050565b6000819050919050565b61034a81610337565b82525050565b60006020820190506103656000830184610341565b92915050565b61037481610337565b811461037f57600080fd5b50565b6000813590506103918161036b565b92915050565b600080604083850312156103ae576103ad6102a7565b5b60006103bc858286016102f5565b92505060206103cd85828601610382565b9150509250929050565b60008115159050919050565b6103ec816103d7565b82525050565b600060208201905061040760008301846103e3565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061044782610337565b915061045283610337565b9250817fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff048311821515161561048b5761048a61040d565b5b828202905092915050565b60006104a182610337565b91506104ac83610337565b9250828210156104bf576104be61040d565b5b828203905092915050565b60006104d582610337565b91506104e083610337565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff038211156105155761051461040d565b5b82820190509291505056fea26469706673582212208e5b4b874c839967f88008ed2fa42d6c2d9c9b0ae05d1d2c61faa7d229c134e664736f6c634300080d0033c080a0fe38720928596f9e9dfbf891d00311638efce3713f03cdd67b212ecbbcf18f29a05993e656c0b35b8a580da6aff7c89b3d3e8b1c6f83a7ce09473c0699a8500b9c"),
	}

	for _, tc := range testcases {
		decoded, err := DecodeRLP(tc)
		require.NoError(t, err)

		encoded, err := EncodeRLP(decoded)
		require.NoError(t, err)
		require.Equal(t, tc, encoded)
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
