package address

import (
	"bytes"
	"encoding/base32"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/filecoin-project/go-bls-sigs"
	"github.com/filecoin-project/go-leb128"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-lotus/lib/crypto"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func TestRandomIDAddress(t *testing.T) {
	assert := assert.New(t)

	addr, err := NewIDAddress(uint64(rand.Int()))
	assert.NoError(err)
	assert.Equal(ID, addr.Protocol())

	str, err := encode(Testnet, addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)

}

var allTestAddresses = []string{
	"t00",
	"t01",
	"t010",
	"t0150",
	"t0499",
	"t01024",
	"t01729",
	"t0999999",
	"t15ihq5ibzwki2b4ep2f46avlkrqzhpqgtga7pdrq",
	"t12fiakbhe2gwd5cnmrenekasyn6v5tnaxaqizq6a",
	"t1wbxhu3ypkuo6eyp6hjx6davuelxaxrvwb2kuwva",
	"t1xtwapqc6nh4si2hcwpr3656iotzmlwumogqbuaa",
	"t1xcbgdhkgkwht3hrrnui3jdopeejsoatkzmoltqy",
	"t17uoq6tp427uzv7fztkbsnn64iwotfrristwpryy",
	"t24vg6ut43yw2h2jqydgbg2xq7x6f4kub3bg6as6i",
	"t25nml2cfbljvn4goqtclhifepvfnicv6g7mfmmvq",
	"t2nuqrg7vuysaue2pistjjnt3fadsdzvyuatqtfei",
	"t24dd4ox4c2vpf5vk5wkadgyyn6qtuvgcpxxon64a",
	"t2gfvuyh7v2sx3patm5k23wdzmhyhtmqctasbr23y",
	"t3vvmn62lofvhjd2ugzca6sof2j2ubwok6cj4xxbfzz4yuxfkgobpihhd2thlanmsh3w2ptld2gqkn2jvlss4a",
	"t3wmuu6crofhqmm3v4enos73okk2l366ck6yc4owxwbdtkmpk42ohkqxfitcpa57pjdcftql4tojda2poeruwa",
	"t3s2q2hzhkpiknjgmf4zq3ejab2rh62qbndueslmsdzervrhapxr7dftie4kpnpdiv2n6tvkr743ndhrsw6d3a",
	"t3q22fijmmlckhl56rn5nkyamkph3mcfu5ed6dheq53c244hfmnq2i7efdma3cj5voxenwiummf2ajlsbxc65a",
	"t3u5zgwa4ael3vuocgc5mfgygo4yuqocrntuuhcklf4xzg5tcaqwbyfabxetwtj4tsam3pbhnwghyhijr5mixa",
}

func TestVectorsIDAddress(t *testing.T) {
	testCases := []struct {
		input    uint64
		expected string
	}{
		{uint64(0), "t00"},
		{uint64(1), "t01"},
		{uint64(10), "t010"},
		{uint64(150), "t0150"},
		{uint64(499), "t0499"},
		{uint64(1024), "t01024"},
		{uint64(1729), "t01729"},
		{uint64(999999), "t0999999"},
		{math.MaxUint64, fmt.Sprintf("t0%s", strconv.FormatUint(math.MaxUint64, 10))},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("testing actorID address: %s", tc.expected), func(t *testing.T) {
			assert := assert.New(t)

			// Round trip encoding and decoding from string
			addr, err := NewIDAddress(tc.input)
			assert.NoError(err)
			assert.Equal(tc.expected, addr.String())

			maybeAddr, err := NewFromString(tc.expected)
			assert.NoError(err)
			assert.Equal(ID, maybeAddr.Protocol())
			assert.Equal(tc.input, leb128.ToUInt64(maybeAddr.Payload()))

			// Round trip to and from bytes
			maybeAddrBytes, err := NewFromBytes(maybeAddr.Bytes())
			assert.NoError(err)
			assert.Equal(maybeAddr, maybeAddrBytes)

			// Round trip encoding and decoding json
			b, err := addr.MarshalJSON()
			assert.NoError(err)

			var newAddr Address
			err = newAddr.UnmarshalJSON(b)
			assert.NoError(err)
			assert.Equal(addr, newAddr)
		})
	}

}

func TestSecp256k1Address(t *testing.T) {
	assert := assert.New(t)

	sk, err := crypto.GenerateKey()
	assert.NoError(err)

	addr, err := NewSecp256k1Address(crypto.PublicKey(sk))
	assert.NoError(err)
	assert.Equal(SECP256K1, addr.Protocol())

	str, err := encode(Mainnet, addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)

}

func TestVectorSecp256k1Address(t *testing.T) {
	testCases := []struct {
		input    []byte
		expected string
	}{
		{[]byte{4, 148, 2, 250, 195, 126, 100, 50, 164, 22, 163, 160, 202, 84,
			38, 181, 24, 90, 179, 178, 79, 97, 52, 239, 162, 92, 228, 135, 200,
			45, 46, 78, 19, 191, 69, 37, 17, 224, 210, 36, 84, 33, 248, 97, 59,
			193, 13, 114, 250, 33, 102, 102, 169, 108, 59, 193, 57, 32, 211,
			255, 35, 63, 208, 188, 5},
			"t15ihq5ibzwki2b4ep2f46avlkrqzhpqgtga7pdrq"},

		{[]byte{4, 118, 135, 185, 16, 55, 155, 242, 140, 190, 58, 234, 103, 75,
			18, 0, 12, 107, 125, 186, 70, 255, 192, 95, 108, 148, 254, 42, 34,
			187, 204, 38, 2, 255, 127, 92, 118, 242, 28, 165, 93, 54, 149, 145,
			82, 176, 225, 232, 135, 145, 124, 57, 53, 118, 238, 240, 147, 246,
			30, 189, 58, 208, 111, 127, 218},
			"t12fiakbhe2gwd5cnmrenekasyn6v5tnaxaqizq6a"},
		{[]byte{4, 222, 253, 208, 16, 1, 239, 184, 110, 1, 222, 213, 206, 52,
			248, 71, 167, 58, 20, 129, 158, 230, 65, 188, 182, 11, 185, 41, 147,
			89, 111, 5, 220, 45, 96, 95, 41, 133, 248, 209, 37, 129, 45, 172,
			65, 99, 163, 150, 52, 155, 35, 193, 28, 194, 255, 53, 157, 229, 75,
			226, 135, 234, 98, 49, 155},
			"t1wbxhu3ypkuo6eyp6hjx6davuelxaxrvwb2kuwva"},
		{[]byte{4, 3, 237, 18, 200, 20, 182, 177, 13, 46, 224, 157, 149, 180,
			104, 141, 178, 209, 128, 208, 169, 163, 122, 107, 106, 125, 182, 61,
			41, 129, 30, 233, 115, 4, 121, 216, 239, 145, 57, 233, 18, 73, 202,
			189, 57, 50, 145, 207, 229, 210, 119, 186, 118, 222, 69, 227, 224,
			133, 163, 118, 129, 191, 54, 69, 210},
			"t1xtwapqc6nh4si2hcwpr3656iotzmlwumogqbuaa"},
		{[]byte{4, 247, 150, 129, 154, 142, 39, 22, 49, 175, 124, 24, 151, 151,
			181, 69, 214, 2, 37, 147, 97, 71, 230, 1, 14, 101, 98, 179, 206, 158,
			254, 139, 16, 20, 65, 97, 169, 30, 208, 180, 236, 137, 8, 0, 37, 63,
			166, 252, 32, 172, 144, 251, 241, 251, 242, 113, 48, 164, 236, 195,
			228, 3, 183, 5, 118},
			"t1xcbgdhkgkwht3hrrnui3jdopeejsoatkzmoltqy"},
		{[]byte{4, 66, 131, 43, 248, 124, 206, 158, 163, 69, 185, 3, 80, 222,
			125, 52, 149, 133, 156, 164, 73, 5, 156, 94, 136, 221, 231, 66, 133,
			223, 251, 158, 192, 30, 186, 188, 95, 200, 98, 104, 207, 234, 235,
			167, 174, 5, 191, 184, 214, 142, 183, 90, 82, 104, 120, 44, 248, 111,
			200, 112, 43, 239, 138, 31, 224},
			"t17uoq6tp427uzv7fztkbsnn64iwotfrristwpryy"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("testing secp256k1 address: %s", tc.expected), func(t *testing.T) {
			assert := assert.New(t)

			// Round trip encoding and decoding from string
			addr, err := NewSecp256k1Address(tc.input)
			assert.NoError(err)
			assert.Equal(tc.expected, addr.String())

			maybeAddr, err := NewFromString(tc.expected)
			assert.NoError(err)
			assert.Equal(SECP256K1, maybeAddr.Protocol())
			assert.Equal(addressHash(tc.input), maybeAddr.Payload())

			// Round trip to and from bytes
			maybeAddrBytes, err := NewFromBytes(maybeAddr.Bytes())
			assert.NoError(err)
			assert.Equal(maybeAddr, maybeAddrBytes)

			// Round trip encoding and decoding json
			b, err := addr.MarshalJSON()
			assert.NoError(err)

			var newAddr Address
			err = newAddr.UnmarshalJSON(b)
			assert.NoError(err)
			assert.Equal(addr, newAddr)
		})
	}
}

func TestRandomActorAddress(t *testing.T) {
	assert := assert.New(t)

	actorMsg := make([]byte, 20)
	rand.Read(actorMsg)

	addr, err := NewActorAddress(actorMsg)
	assert.NoError(err)
	assert.Equal(Actor, addr.Protocol())

	str, err := encode(Mainnet, addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)

}

func TestVectorActorAddress(t *testing.T) {
	testCases := []struct {
		input    []byte
		expected string
	}{
		{[]byte{118, 18, 129, 144, 205, 240, 104, 209, 65, 128, 68, 172, 192,
			62, 11, 103, 129, 151, 13, 96},
			"t24vg6ut43yw2h2jqydgbg2xq7x6f4kub3bg6as6i"},
		{[]byte{44, 175, 184, 226, 224, 107, 186, 152, 234, 101, 124, 92, 245,
			244, 32, 35, 170, 35, 232, 142},
			"t25nml2cfbljvn4goqtclhifepvfnicv6g7mfmmvq"},
		{[]byte{2, 44, 158, 14, 162, 157, 143, 64, 197, 106, 190, 195, 92, 141,
			88, 125, 160, 166, 76, 24},
			"t2nuqrg7vuysaue2pistjjnt3fadsdzvyuatqtfei"},
		{[]byte{223, 236, 3, 14, 32, 79, 15, 89, 216, 15, 29, 94, 233, 29, 253,
			6, 109, 127, 99, 189},
			"t24dd4ox4c2vpf5vk5wkadgyyn6qtuvgcpxxon64a"},
		{[]byte{61, 58, 137, 232, 221, 171, 84, 120, 50, 113, 108, 109, 70, 140,
			53, 96, 201, 244, 127, 216},
			"t2gfvuyh7v2sx3patm5k23wdzmhyhtmqctasbr23y"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("testing Actor address: %s", tc.expected), func(t *testing.T) {
			assert := assert.New(t)

			// Round trip encoding and decoding from string
			addr, err := NewActorAddress(tc.input)
			assert.NoError(err)
			assert.Equal(tc.expected, addr.String())

			maybeAddr, err := NewFromString(tc.expected)
			assert.NoError(err)
			assert.Equal(Actor, maybeAddr.Protocol())
			assert.Equal(addressHash(tc.input), maybeAddr.Payload())

			// Round trip to and from bytes
			maybeAddrBytes, err := NewFromBytes(maybeAddr.Bytes())
			assert.NoError(err)
			assert.Equal(maybeAddr, maybeAddrBytes)

			// Round trip encoding and decoding json
			b, err := addr.MarshalJSON()
			assert.NoError(err)

			var newAddr Address
			err = newAddr.UnmarshalJSON(b)
			assert.NoError(err)
			assert.Equal(addr, newAddr)
		})
	}
}

func TestRandomBLSAddress(t *testing.T) {
	assert := assert.New(t)

	pk := bls.PrivateKeyPublicKey(bls.PrivateKeyGenerate())

	addr, err := NewBLSAddress(pk[:])
	assert.NoError(err)
	assert.Equal(BLS, addr.Protocol())

	str, err := encode(Mainnet, addr)
	assert.NoError(err)

	maybe, err := decode(str)
	assert.NoError(err)
	assert.Equal(addr, maybe)

}

func TestVectorBLSAddress(t *testing.T) {
	testCases := []struct {
		input    []byte
		expected string
	}{
		{[]byte{173, 88, 223, 105, 110, 45, 78, 145, 234, 134, 200, 129, 233, 56,
			186, 78, 168, 27, 57, 94, 18, 121, 123, 132, 185, 207, 49, 75, 149, 70,
			112, 94, 131, 156, 122, 153, 214, 6, 178, 71, 221, 180, 249, 172, 122,
			52, 20, 221},
			"t3vvmn62lofvhjd2ugzca6sof2j2ubwok6cj4xxbfzz4yuxfkgobpihhd2thlanmsh3w2ptld2gqkn2jvlss4a"},
		{[]byte{179, 41, 79, 10, 46, 41, 224, 198, 110, 188, 35, 93, 47, 237,
			202, 86, 151, 191, 120, 74, 246, 5, 199, 90, 246, 8, 230, 166, 61, 92,
			211, 142, 168, 92, 168, 152, 158, 14, 253, 233, 24, 139, 56, 47,
			147, 114, 70, 13},
			"t3wmuu6crofhqmm3v4enos73okk2l366ck6yc4owxwbdtkmpk42ohkqxfitcpa57pjdcftql4tojda2poeruwa"},
		{[]byte{150, 161, 163, 228, 234, 122, 20, 212, 153, 133, 230, 97, 178,
			36, 1, 212, 79, 237, 64, 45, 29, 9, 37, 178, 67, 201, 35, 88, 156,
			15, 188, 126, 50, 205, 4, 226, 158, 215, 141, 21, 211, 125, 58, 170,
			63, 230, 218, 51},
			"t3s2q2hzhkpiknjgmf4zq3ejab2rh62qbndueslmsdzervrhapxr7dftie4kpnpdiv2n6tvkr743ndhrsw6d3a"},
		{[]byte{134, 180, 84, 37, 140, 88, 148, 117, 247, 209, 111, 90, 172, 1,
			138, 121, 246, 193, 22, 157, 32, 252, 51, 146, 29, 216, 181, 206, 28,
			172, 108, 52, 143, 144, 163, 96, 54, 36, 246, 174, 185, 27, 100, 81,
			140, 46, 128, 149},
			"t3q22fijmmlckhl56rn5nkyamkph3mcfu5ed6dheq53c244hfmnq2i7efdma3cj5voxenwiummf2ajlsbxc65a"},
		{[]byte{167, 114, 107, 3, 128, 34, 247, 90, 56, 70, 23, 88, 83, 96, 206,
			230, 41, 7, 10, 45, 157, 40, 113, 41, 101, 229, 242, 110, 204, 64,
			133, 131, 130, 128, 55, 36, 237, 52, 242, 114, 3, 54, 240, 157, 182,
			49, 240, 116},
			"t3u5zgwa4ael3vuocgc5mfgygo4yuqocrntuuhcklf4xzg5tcaqwbyfabxetwtj4tsam3pbhnwghyhijr5mixa"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("testing bls address: %s", tc.expected), func(t *testing.T) {
			assert := assert.New(t)

			// Round trip encoding and decoding from string
			addr, err := NewBLSAddress(tc.input)
			assert.NoError(err)
			assert.Equal(tc.expected, addr.String())

			maybeAddr, err := NewFromString(tc.expected)
			assert.NoError(err)
			assert.Equal(BLS, maybeAddr.Protocol())
			assert.Equal(tc.input, maybeAddr.Payload())

			// Round trip to and from bytes
			maybeAddrBytes, err := NewFromBytes(maybeAddr.Bytes())
			assert.NoError(err)
			assert.Equal(maybeAddr, maybeAddrBytes)

			// Round trip encoding and decoding json
			b, err := addr.MarshalJSON()
			assert.NoError(err)

			var newAddr Address
			err = newAddr.UnmarshalJSON(b)
			assert.NoError(err)
			assert.Equal(addr, newAddr)
		})
	}
}

func TestInvalidStringAddresses(t *testing.T) {
	testCases := []struct {
		input    string
		expetErr error
	}{
		{"Q2gfvuyh7v2sx3patm5k23wdzmhyhtmqctasbr23y", ErrUnknownNetwork},
		{"t4gfvuyh7v2sx3patm5k23wdzmhyhtmqctasbr23y", ErrUnknownProtocol},
		{"t2gfvuyh7v2sx3patm5k23wdzmhyhtmqctasbr24y", ErrInvalidChecksum},
		{"t0banananananannnnnnnnn", ErrInvalidLength},
		{"t0banananananannnnnnnn", ErrInvalidPayload},
		{"t2gfvuyh7v2sx3patm1k23wdzmhyhtmqctasbr24y", base32.CorruptInputError(16)}, // '1' is not in base32 alphabet
		{"t2gfvuyh7v2sx3paTm1k23wdzmhyhtmqctasbr24y", base32.CorruptInputError(14)}, // 'T' is not in base32 alphabet
		{"t2", ErrInvalidLength},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("testing string address: %s", tc.expetErr), func(t *testing.T) {
			assert := assert.New(t)

			_, err := NewFromString(tc.input)
			assert.Equal(tc.expetErr, err)
		})
	}

}

func TestInvalidByteAddresses(t *testing.T) {
	testCases := []struct {
		input    []byte
		expetErr error
	}{
		// Unknown Protocol
		{[]byte{4, 4, 4}, ErrUnknownProtocol},

		// ID protocol
		{[]byte{0}, ErrInvalidLength},

		// SECP256K1 Protocol
		{append([]byte{1}, make([]byte, PayloadHashLength-1)...), ErrInvalidPayload},
		{append([]byte{1}, make([]byte, PayloadHashLength+1)...), ErrInvalidPayload},
		// Actor Protocol
		{append([]byte{2}, make([]byte, PayloadHashLength-1)...), ErrInvalidPayload},
		{append([]byte{2}, make([]byte, PayloadHashLength+1)...), ErrInvalidPayload},

		// BLS Protocol
		{append([]byte{3}, make([]byte, bls.PublicKeyBytes-1)...), ErrInvalidPayload},
		{append([]byte{3}, make([]byte, bls.PrivateKeyBytes+1)...), ErrInvalidPayload},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("testing byte address: %s", tc.expetErr), func(t *testing.T) {
			assert := assert.New(t)

			_, err := NewFromBytes(tc.input)
			assert.Equal(tc.expetErr, err)
		})
	}

}

func TestChecksum(t *testing.T) {
	assert := assert.New(t)

	data := []byte("helloworld")
	bata := []byte("kittinmittins")

	cksm := Checksum(data)
	assert.Len(cksm, ChecksumHashLength)

	assert.True(ValidateChecksum(data, cksm))
	assert.False(ValidateChecksum(bata, cksm))

}

func TestAddressFormat(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	a, err := NewActorAddress([]byte("hello"))
	require.NoError(err)

	assert.Equal("t2wvjry4bx6bwj6kkhcmvgu5zafqyi5cjzbtet3va", a.String())
	assert.Equal("02B5531C7037F06C9F2947132A6A77202C308E8939", fmt.Sprintf("%X", a))
	assert.Equal("[2 - b5531c7037f06c9f2947132a6a77202c308e8939]", fmt.Sprintf("%v", a))

	assert.Equal("", fmt.Sprintf("%X", Undef))
	assert.Equal(UndefAddressString, Undef.String())
	assert.Equal(UndefAddressString, fmt.Sprintf("%v", Undef))
}

func TestCborMarshal(t *testing.T) {
	for _, a := range allTestAddresses {
		addr, err := NewFromString(a)
		if err != nil {
			t.Fatal(err)
		}

		buf := new(bytes.Buffer)
		if err := addr.MarshalCBOR(buf); err != nil {
			t.Fatal(err)
		}

		/*
			// Note: this is commented out because we're currently serializing addresses as cbor "text strings", not "byte strings".
			// This is to get around the restriction that refmt only allows string keys in maps.
			// if you change it to serialize to byte strings and uncomment this, the tests pass fine
			oldbytes, err := cbor.DumpObject(addr)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(oldbytes, buf.Bytes()) {
				t.Fatalf("serialization doesnt match old serialization: %s", a)
			}
		*/

		var out Address
		if err := out.UnmarshalCBOR(buf); err != nil {
			t.Fatal(err)
		}

		if out != addr {
			t.Fatalf("failed to round trip %s", a)
		}
	}
}

func BenchmarkOldCborMarshal(b *testing.B) {
	addr, err := NewFromString("t15ihq5ibzwki2b4ep2f46avlkrqzhpqgtga7pdrq")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := cbor.DumpObject(addr)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCborMarshal(b *testing.B) {
	addr, err := NewFromString("t15ihq5ibzwki2b4ep2f46avlkrqzhpqgtga7pdrq")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	buf := new(bytes.Buffer)
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := addr.MarshalCBOR(buf); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOldCborUnmarshal(b *testing.B) {
	addr, err := NewFromString("t15ihq5ibzwki2b4ep2f46avlkrqzhpqgtga7pdrq")
	if err != nil {
		b.Fatal(err)
	}

	buf := new(bytes.Buffer)
	if err := addr.MarshalCBOR(buf); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var a Address
		if err := cbor.DecodeInto(buf.Bytes(), &a); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCborUnmarshal(b *testing.B) {
	addr, err := NewFromString("t15ihq5ibzwki2b4ep2f46avlkrqzhpqgtga7pdrq")
	if err != nil {
		b.Fatal(err)
	}

	buf := new(bytes.Buffer)
	if err := addr.MarshalCBOR(buf); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var a Address
		if err := a.UnmarshalCBOR(bytes.NewReader(buf.Bytes())); err != nil {
			b.Fatal(err)
		}
	}
}
