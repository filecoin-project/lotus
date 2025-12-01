package ethtypes

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
)

type TestCase struct {
	Input  interface{}
	Output interface{}
}

func TestEthIntMarshalJSON(t *testing.T) {
	// https://ethereum.org/en/developers/docs/apis/json-rpc/#quantities-encoding
	testcases := []TestCase{
		{EthUint64(0), []byte("\"0x0\"")},
		{EthUint64(65), []byte("\"0x41\"")},
		{EthUint64(1024), []byte("\"0x400\"")},
	}

	for _, tc := range testcases {
		j, err := tc.Input.(EthUint64).MarshalJSON()
		require.Nil(t, err)
		require.Equal(t, j, tc.Output)
	}
}

func TestEthIntUnmarshalJSON(t *testing.T) {
	testcases := []TestCase{
		{[]byte("\"0x0\""), EthUint64(0)},
		{[]byte("\"0x41\""), EthUint64(65)},
		{[]byte("\"0x400\""), EthUint64(1024)},
		{[]byte("\"0\""), EthUint64(0)},
		{[]byte("\"41\""), EthUint64(41)},
		{[]byte("\"400\""), EthUint64(400)},
		{[]byte("0"), EthUint64(0)},
		{[]byte("100"), EthUint64(100)},
		{[]byte("1024"), EthUint64(1024)},
	}

	for _, tc := range testcases {
		var i EthUint64
		err := i.UnmarshalJSON(tc.Input.([]byte))
		require.Nil(t, err)
		require.Equal(t, tc.Output, i)

		i, err = EthUint64FromString(strings.Replace(string(tc.Input.([]byte)), `"`, "", -1))
		require.Nil(t, err)
		require.Equal(t, tc.Output, i)
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

		jm, err := json.Marshal(h)
		require.NoError(t, err)
		require.Equal(t, hash, string(jm))
	}

	for _, c := range []cid.Cid{
		cid.Undef,
		cid.MustParse("bafy2bzacaa"),
		cid.MustParse("bafkqaaa"),
		cid.MustParse("bafy2bzacidqenpags5upxuhzpynnbhaojm5cy6dfoiojibz4gk2u4degs4qiclksacibt65ogzfjqionu7o25nl3y4ayzsizwbc32crqgnrqntqv"),
		cid.MustParse("bafyrgqhai26anf3i7pips7q22coa4sz2fr4gk4q4sqdtymvvjyginfzaqewveaeqdh524nsktaq43j65v22xxrybrtertmcfxufdam3da3hbk"),
	} {
		_, err := EthHashFromCid(c)
		require.ErrorContains(t, err, "CID does not have the expected prefix")
	}
}

func TestEthFilterID(t *testing.T) {
	testcases := []string{
		`"0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184"`,
		`"0xab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738"`,
	}

	for _, hash := range testcases {
		var h EthFilterID
		err := h.UnmarshalJSON([]byte(hash))

		require.Nil(t, err)
		require.Equal(t, h.String(), strings.Replace(hash, `"`, "", -1))

		jm, err := json.Marshal(h)
		require.NoError(t, err)
		require.Equal(t, hash, string(jm))
	}
}

func TestEthSubscriptionID(t *testing.T) {
	testcases := []string{
		`"0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184"`,
		`"0xab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738"`,
	}

	for _, hash := range testcases {
		var h EthSubscriptionID
		err := h.UnmarshalJSON([]byte(hash))

		require.Nil(t, err)
		require.Equal(t, h.String(), strings.Replace(hash, `"`, "", -1))

		jm, err := json.Marshal(h)
		require.NoError(t, err)
		require.Equal(t, hash, string(jm))
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

		eaddr, err := EthAddressFromFilecoinAddress(addr)
		require.Nil(t, err)

		faddr, err := eaddr.ToFilecoinAddress()
		require.Nil(t, err)

		require.Equal(t, addr, faddr)
	}
}

func TestMaskedIDInF4(t *testing.T) {
	addr, err := address.NewIDAddress(100)
	require.NoError(t, err)

	eaddr, err := EthAddressFromFilecoinAddress(addr)
	require.NoError(t, err)

	badaddr, err := address.NewDelegatedAddress(builtin.EthereumAddressManagerActorID, eaddr[:])
	require.NoError(t, err)

	_, err = EthAddressFromFilecoinAddress(badaddr)
	require.Error(t, err)
}

func TestUnmarshalEthCall(t *testing.T) {
	data := `{"from":"0x4D6D86b31a112a05A473c4aE84afaF873f632325","to":"0xFe01CC39f5Ae8553D6914DBb9dC27D219fa22D7f","gas":"0x5","gasPrice":"0x6","value":"0x123","data":"0xFF"}`

	var c EthCall
	err := c.UnmarshalJSON([]byte(data))
	require.Nil(t, err)
	require.EqualValues(t, []byte{0xff}, c.Data)
}

func TestUnmarshalEthCallInput(t *testing.T) {
	data := `{"from":"0x4D6D86b31a112a05A473c4aE84afaF873f632325","to":"0xFe01CC39f5Ae8553D6914DBb9dC27D219fa22D7f","gas":"0x5","gasPrice":"0x6","value":"0x123","input":"0xFF"}`

	var c EthCall
	err := c.UnmarshalJSON([]byte(data))
	require.Nil(t, err)
	require.EqualValues(t, []byte{0xff}, c.Data)
}

func TestUnmarshalEthCallInputAndData(t *testing.T) {
	data := `{"from":"0x4D6D86b31a112a05A473c4aE84afaF873f632325","to":"0xFe01CC39f5Ae8553D6914DBb9dC27D219fa22D7f","gas":"0x5","gasPrice":"0x6","value":"0x123","data":"0xFE","input":"0xFF"}`

	var c EthCall
	err := c.UnmarshalJSON([]byte(data))
	require.Nil(t, err)
	require.EqualValues(t, []byte{0xff}, c.Data)
}

func TestUnmarshalEthCallInputAndDataEmpty(t *testing.T) {
	// Even if the input is empty, it should be used when specified.
	data := `{"from":"0x4D6D86b31a112a05A473c4aE84afaF873f632325","to":"0xFe01CC39f5Ae8553D6914DBb9dC27D219fa22D7f","gas":"0x5","gasPrice":"0x6","value":"0x123","data":"0xFE","input":""}`

	var c EthCall
	err := c.UnmarshalJSON([]byte(data))
	require.Nil(t, err)
	require.EqualValues(t, []byte{}, c.Data)
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

func TestEthFilterResultMarshalJSON(t *testing.T) {
	hash1, err := ParseEthHash("013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184")
	require.NoError(t, err, "eth hash")

	hash2, err := ParseEthHash("ab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738")
	require.NoError(t, err, "eth hash")

	addr, err := ParseEthAddress("d4c5fb16488Aa48081296299d54b0c648C9333dA")
	require.NoError(t, err, "eth address")

	log := EthLog{
		Removed:          true,
		LogIndex:         5,
		TransactionIndex: 45,
		TransactionHash:  hash1,
		BlockHash:        hash2,
		BlockNumber:      53,
		Topics:           []EthHash{hash1},
		Data:             EthBytes(hash1[:]),
		Address:          addr,
	}
	logjson, err := json.Marshal(log)
	require.NoError(t, err, "log json")

	testcases := []struct {
		res  EthFilterResult
		want string
	}{
		{
			res:  EthFilterResult{},
			want: "[]",
		},

		{
			res: EthFilterResult{
				Results: []any{hash1, hash2},
			},
			want: `["0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184","0xab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738"]`,
		},

		{
			res: EthFilterResult{
				Results: []any{hash1, hash2},
			},
			want: `["0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184","0xab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738"]`,
		},

		{
			res: EthFilterResult{
				Results: []any{log},
			},
			want: `[` + string(logjson) + `]`,
		},
	}

	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			data, err := json.Marshal(tc.res)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(data))
		})
	}
}

func TestEthFilterSpecUnmarshalJSON(t *testing.T) {
	hash1, err := ParseEthHash("013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184")
	require.NoError(t, err, "eth hash")

	hash2, err := ParseEthHash("ab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738")
	require.NoError(t, err, "eth hash")

	addr, err := ParseEthAddress("d4c5fb16488Aa48081296299d54b0c648C9333dA")
	require.NoError(t, err, "eth address")

	pstring := func(s string) *string { return &s }
	phash := func(h EthHash) *EthHash { return &h }

	testcases := []struct {
		input string
		want  EthFilterSpec
	}{
		{
			input: `{"fromBlock":"latest"}`,
			want:  EthFilterSpec{FromBlock: pstring("latest")},
		},
		{
			input: `{"toBlock":"pending"}`,
			want:  EthFilterSpec{ToBlock: pstring("pending")},
		},
		{
			input: `{"address":["0xd4c5fb16488Aa48081296299d54b0c648C9333dA"]}`,
			want:  EthFilterSpec{Address: EthAddressList{addr}},
		},
		{
			input: `{"address":"0xd4c5fb16488Aa48081296299d54b0c648C9333dA"}`,
			want:  EthFilterSpec{Address: EthAddressList{addr}},
		},
		{
			input: `{"blockHash":"0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184"}`,
			want:  EthFilterSpec{BlockHash: phash(hash1)},
		},
		{
			input: `{"topics":["0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184"]}`,
			want: EthFilterSpec{
				Topics: EthTopicSpec{
					{hash1},
				},
			},
		},
		{
			input: `{"topics":["0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184","0xab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738"]}`,
			want: EthFilterSpec{
				Topics: EthTopicSpec{
					{hash1},
					{hash2},
				},
			},
		},
		{
			input: `{"topics":[null, ["0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184","0xab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738"]]}`,
			want: EthFilterSpec{
				Topics: EthTopicSpec{
					nil,
					{hash1, hash2},
				},
			},
		},
		{
			input: `{"topics":[null, "0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184"]}`,
			want: EthFilterSpec{
				Topics: EthTopicSpec{
					nil,
					{hash1},
				},
			},
		},
	}

	for _, tc := range testcases {
		var got EthFilterSpec
		err := json.Unmarshal([]byte(tc.input), &got)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}

func TestEthAddressListUnmarshalJSON(t *testing.T) {
	addr1, err := ParseEthAddress("d4c5fb16488Aa48081296299d54b0c648C9333dA")
	require.NoError(t, err, "eth address")

	addr2, err := ParseEthAddress("abbbfb16488Aa48081296299d54b0c648C9333dA")
	require.NoError(t, err, "eth address")

	testcases := []struct {
		input string
		want  EthAddressList
	}{
		{
			input: `["0xd4c5fb16488Aa48081296299d54b0c648C9333dA"]`,
			want:  EthAddressList{addr1},
		},
		{
			input: `["0xd4c5fb16488Aa48081296299d54b0c648C9333dA","abbbfb16488Aa48081296299d54b0c648C9333dA"]`,
			want:  EthAddressList{addr1, addr2},
		},
		{
			input: `"0xd4c5fb16488Aa48081296299d54b0c648C9333dA"`,
			want:  EthAddressList{addr1},
		},
		{
			input: `[]`,
			want:  EthAddressList{},
		},
		{
			input: `null`,
			want:  EthAddressList(nil),
		},
	}
	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			var got EthAddressList
			err := json.Unmarshal([]byte(tc.input), &got)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestEthHashListUnmarshalJSON(t *testing.T) {
	hash1, err := ParseEthHash("013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184")
	require.NoError(t, err, "eth hash")

	hash2, err := ParseEthHash("ab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738")
	require.NoError(t, err, "eth hash")

	testcases := []struct {
		input string
		want  *EthHashList
	}{
		{
			input: `["0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184"]`,
			want:  &EthHashList{hash1},
		},
		{
			input: `["0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184","0xab8653edf9f51785664a643b47605a7ba3d917b5339a0724e7642c114d0e4738"]`,
			want:  &EthHashList{hash1, hash2},
		},
		{
			input: `"0x013dbb9442ca9667baccc6230fcd5c1c4b2d4d2870f4bd20681d4d47cfd15184"`,
			want:  &EthHashList{hash1},
		},
		{
			input: `null`,
			want:  nil,
		},
	}
	for _, tc := range testcases {
		var got *EthHashList
		err := json.Unmarshal([]byte(tc.input), &got)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}

func TestFilecoinAddressToEthAddressParams(t *testing.T) {
	testAddr, err := address.NewIDAddress(1000)
	require.NoError(t, err)

	testCases := []struct {
		name                string
		input               string
		expectedErrContains string
		expected            FilecoinAddressToEthAddressParams
	}{
		{
			name:                "One param",
			input:               `["t01000"]`,
			expectedErrContains: "",
			expected:            FilecoinAddressToEthAddressParams{FilecoinAddress: testAddr},
		},
		{
			name:                "Two params",
			input:               `["t01000", "latest"]`,
			expectedErrContains: "",
			expected:            FilecoinAddressToEthAddressParams{FilecoinAddress: testAddr, BlkParam: stringPtr("latest")},
		},
		{
			name:                "Three params (should fail)",
			input:               `["t01000", "latest", "extra"]`,
			expectedErrContains: "expected 1 or 2 params",
			expected:            FilecoinAddressToEthAddressParams{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var params FilecoinAddressToEthAddressParams
			err := json.Unmarshal([]byte(tc.input), &params)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedErrContains)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected, params)
		})
	}
}

func TestEthBlockNumberOrHashUnmarshalJSON(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected func() EthBlockNumberOrHash
		wantErr  bool
	}{
		{
			name:  "Valid block number",
			input: `{"blockNumber": "0x1234"}`,
			expected: func() EthBlockNumberOrHash {
				v := uint64(0x1234)
				return EthBlockNumberOrHash{BlockNumber: (*EthUint64)(&v)}
			},
		},
		{
			name:  "Valid block hash",
			input: `{"blockHash": "0x1234567890123456789012345678901234567890123456789012345678901234"}`,
			expected: func() EthBlockNumberOrHash {
				h, _ := ParseEthHash("0x1234567890123456789012345678901234567890123456789012345678901234")
				return EthBlockNumberOrHash{BlockHash: &h}
			},
		},
		{
			name:  "Valid block number as string",
			input: `"0x1234"`,
			expected: func() EthBlockNumberOrHash {
				v := uint64(0x1234)
				return EthBlockNumberOrHash{BlockNumber: (*EthUint64)(&v)}
			},
		},
		{
			name:  "Valid block hash as string",
			input: `"0x1234567890123456789012345678901234567890123456789012345678901234"`,
			expected: func() EthBlockNumberOrHash {
				h, _ := ParseEthHash("0x1234567890123456789012345678901234567890123456789012345678901234")
				return EthBlockNumberOrHash{BlockHash: &h}
			},
		},
		{
			name:  "Valid 'latest' string",
			input: `"latest"`,
			expected: func() EthBlockNumberOrHash {
				return EthBlockNumberOrHash{PredefinedBlock: stringPtr("latest")}
			},
		},
		{
			name:  "Valid 'earliest' string",
			input: `"earliest"`,
			expected: func() EthBlockNumberOrHash {
				return EthBlockNumberOrHash{PredefinedBlock: stringPtr("earliest")}
			},
		},
		{
			name:  "Valid 'pending' string",
			input: `"pending"`,
			expected: func() EthBlockNumberOrHash {
				return EthBlockNumberOrHash{PredefinedBlock: stringPtr("pending")}
			},
		},
		{
			name:    "Invalid: both block number and hash",
			input:   `{"blockNumber": "0x1234", "blockHash": "0x1234567890123456789012345678901234567890123456789012345678901234"}`,
			wantErr: true,
		},
		{
			name:    "Invalid block number",
			input:   `{"blockNumber": "invalid"}`,
			wantErr: true,
		},
		{
			name:    "Invalid block hash",
			input:   `{"blockHash": "invalid"}`,
			wantErr: true,
		},
		{
			name:    "Invalid string",
			input:   `"invalid"`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var got EthBlockNumberOrHash
			err := got.UnmarshalJSON([]byte(tc.input))

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err, fmt.Sprintf("did not expect error but got %s", err))
				require.Equal(t, tc.expected(), got)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func BenchmarkEthHashFromCid(b *testing.B) {
	c := cid.MustParse("bafy2bzacedwviarjtjraqakob5pslltmuo5n3xev3nt5zylezofkbbv5jclyu")

	for b.Loop() {
		_, err := EthHashFromCid(c)
		if err != nil {
			b.Fatalf("Error in EthHashFromCid: %v", err)
		}
	}
}
