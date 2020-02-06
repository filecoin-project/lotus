//+build gofuzz

package types

import (
	"bytes"
	"github.com/filecoin-project/go-address"
)

// FIXME: Separate this into `UnmarshalCBOR` calls of the underlying elements first.
func FuzzMessageDecoder(data []byte) int {
	var msg Message
	err := msg.UnmarshalCBOR(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	reData, err := msg.Serialize()
	if err != nil {
		panic(err) // ok
	}
	var msg2 Message
	err = msg2.UnmarshalCBOR(bytes.NewReader(data))
	if err != nil {
		panic(err) // ok
	}
	reData2, err := msg.Serialize()
	if err != nil {
		panic(err) // ok
	}
	if !bytes.Equal(reData, reData2) {
		panic("reencoding not equal") // ok
	}
	return 1
}

func FuzzAddressEncoder(data []byte) int {
	ads, err := address.NewFromBytes(data)
	if err != nil {
		return -1
	}
	// FIXME: The fuzzer should produce as many valid addresses as possible, `newAddress`
	// should be partially expanded here. At the moment we signal invalid ones with `-1`.

	if err := ads.MarshalCBOR(new(bytes.Buffer)); err != nil {
		panic(err)
	}

	return 1
}

func FuzzBigIntEncoder(data []byte) int {
	bi, err := fromCborBytes(data)
	if err != nil {
		return -1
	}

	buf := new(bytes.Buffer)
	if err := bi.MarshalCBOR(buf); err != nil {
		panic(err)
	}

	//if !bytes.Equal(data, buf.Bytes()) {
	//	panic("reencoding not equal")
	//}
	// FIXME: We can't check this because we're not interpreting the input `data` as CBOR
	// (to avoid using the same `UnmarshalCBOR` we're trying to fuzz), `fromCborBytes` is
	// a bit misleading, here `data` is just the payload.

	return 1
}

// FIXME: Encapsulate params encoding logic from `Message.MarshalCBOR` and fuzz it in a
// separate function.
