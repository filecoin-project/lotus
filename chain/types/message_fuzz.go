//+build gofuzz

package types

import "bytes"

func FuzzMessage(data []byte) int {
	var msg Message
	err := msg.UnmarshalCBOR(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	reData, err := msg.Serialize()
	if err != nil {
		panic(err)
	}
	var msg2 Message
	err = msg2.UnmarshalCBOR(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	reData2, err := msg.Serialize()
	if err != nil {
		panic(err)
	}
	if !bytes.Equal(reData, reData2) {
		panic("reencoding not equal")
	}
	return 1
}
