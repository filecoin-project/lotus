package types

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
)

func TestMessageReceiptSerdeRoundrip(t *testing.T) {
	var (
		assert = assert.New(t)
		buf    = new(bytes.Buffer)
		err    error
	)

	randomCid, err := cid.Decode("bafy2bzacecu7n7wbtogznrtuuvf73dsz7wasgyneqasksdblxupnyovmtwxxu")
	assert.NoError(err)

	randomCid2, err := cid.Decode("bafy2bzacedwviarwqx6oldifyt2k4lsreru7gkuouqpmhqjvy54m5kza7k6to")
	assert.NoError(err)

	//
	// Version 0
	//
	mr := NewMessageReceiptV0(0, []byte{0x00, 0x01, 0x02, 0x04}, 42)

	// marshal
	err = mr.MarshalCBOR(buf)
	assert.NoError(err)

	t.Logf("version 0: %s\n", hex.EncodeToString(buf.Bytes()))

	// unmarshal
	var mr2 MessageReceipt
	err = mr2.UnmarshalCBOR(buf)
	assert.NoError(err)
	assert.Equal(mr, mr2)
	assert.Equal(MessageReceiptV0, mr2.Version())

	// version 0 with an events root -- should not serialize the events root!
	mr.EventsRoot = &randomCid

	buf.Reset()

	// marshal
	err = mr.MarshalCBOR(buf)
	assert.NoError(err)

	t.Logf("version 0 (with root): %s\n", hex.EncodeToString(buf.Bytes()))

	// unmarshal
	mr2 = MessageReceipt{}
	err = mr2.UnmarshalCBOR(buf)
	assert.NoError(err)
	assert.NotEqual(mr, mr2)
	assert.Nil(mr2.EventsRoot)

	//
	// Version 1
	//
	buf.Reset()
	mr = NewMessageReceiptV1(0, []byte{0x00, 0x01, 0x02, 0x04}, 42, &randomCid)

	// marshal
	err = mr.MarshalCBOR(buf)
	assert.NoError(err)

	t.Logf("version 1: %s\n", hex.EncodeToString(buf.Bytes()))

	// unmarshal
	mr2 = MessageReceipt{}
	err = mr2.UnmarshalCBOR(buf)
	assert.NoError(err)
	assert.Equal(mr, mr2)
	assert.NotNil(mr2.EventsRoot)
	assert.Equal(MessageReceiptV1, mr2.Version())

	//
	// Version 2 -- all fields populated
	//
	buf.Reset()
	codec := uint64(0x51) // dag-cbor
	mr = NewMessageReceiptV2(0, []byte{0x00, 0x01, 0x02, 0x04}, 42, &randomCid, &codec, &randomCid2)

	err = mr.MarshalCBOR(buf)
	assert.NoError(err)

	t.Logf("version 2 (full): %s\n", hex.EncodeToString(buf.Bytes()))

	mr2 = MessageReceipt{}
	err = mr2.UnmarshalCBOR(buf)
	assert.NoError(err)
	assert.Equal(mr, mr2)
	assert.Equal(MessageReceiptV2, mr2.Version())
	assert.NotNil(mr2.EventsRoot)
	assert.Equal(randomCid, *mr2.EventsRoot)
	assert.NotNil(mr2.IpldCodec)
	assert.Equal(uint64(0x51), *mr2.IpldCodec)
	assert.NotNil(mr2.Message)
	assert.Equal(randomCid2, *mr2.Message)

	//
	// Version 2 -- nil optional fields
	//
	buf.Reset()
	mr = NewMessageReceiptV2(1, []byte{0xde, 0xad}, 100, nil, nil, nil)

	err = mr.MarshalCBOR(buf)
	assert.NoError(err)

	t.Logf("version 2 (nil fields): %s\n", hex.EncodeToString(buf.Bytes()))

	mr2 = MessageReceipt{}
	err = mr2.UnmarshalCBOR(buf)
	assert.NoError(err)
	assert.Equal(mr, mr2)
	assert.Equal(MessageReceiptV2, mr2.Version())
	assert.Nil(mr2.EventsRoot)
	assert.Nil(mr2.IpldCodec)
	assert.Nil(mr2.Message)

	//
	// Version 2 -- mixed nil/non-nil
	//
	buf.Reset()
	mr = NewMessageReceiptV2(0, nil, 0, nil, &codec, &randomCid)

	err = mr.MarshalCBOR(buf)
	assert.NoError(err)

	t.Logf("version 2 (mixed): %s\n", hex.EncodeToString(buf.Bytes()))

	mr2 = MessageReceipt{}
	err = mr2.UnmarshalCBOR(buf)
	assert.NoError(err)
	assert.Equal(mr, mr2)
	assert.Nil(mr2.EventsRoot)
	assert.NotNil(mr2.IpldCodec)
	assert.NotNil(mr2.Message)

	//
	// Version 2 -- non-zero exit code
	//
	buf.Reset()
	mr = NewMessageReceiptV2(16, []byte{0xff}, 999, &randomCid, &codec, &randomCid2)

	err = mr.MarshalCBOR(buf)
	assert.NoError(err)

	mr2 = MessageReceipt{}
	err = mr2.UnmarshalCBOR(buf)
	assert.NoError(err)
	assert.True(mr.Equals(&mr2))
}

func TestMessageReceiptV2Equals(t *testing.T) {
	assert := assert.New(t)

	randomCid, err := cid.Decode("bafy2bzacecu7n7wbtogznrtuuvf73dsz7wasgyneqasksdblxupnyovmtwxxu")
	assert.NoError(err)

	randomCid2, err := cid.Decode("bafy2bzacedwviarwqx6oldifyt2k4lsreru7gkuouqpmhqjvy54m5kza7k6to")
	assert.NoError(err)

	codec1 := uint64(0x51)
	codec2 := uint64(0x55)

	mr1 := NewMessageReceiptV2(0, []byte{0x01}, 42, &randomCid, &codec1, &randomCid2)
	mr2 := NewMessageReceiptV2(0, []byte{0x01}, 42, &randomCid, &codec1, &randomCid2)
	assert.True(mr1.Equals(&mr2))

	// different codec
	mr3 := NewMessageReceiptV2(0, []byte{0x01}, 42, &randomCid, &codec2, &randomCid2)
	assert.False(mr1.Equals(&mr3))

	// nil vs non-nil codec
	mr4 := NewMessageReceiptV2(0, []byte{0x01}, 42, &randomCid, nil, &randomCid2)
	assert.False(mr1.Equals(&mr4))

	// nil vs non-nil message
	mr5 := NewMessageReceiptV2(0, []byte{0x01}, 42, &randomCid, &codec1, nil)
	assert.False(mr1.Equals(&mr5))

	// both nil codec and message
	mr6 := NewMessageReceiptV2(0, []byte{0x01}, 42, &randomCid, nil, nil)
	mr7 := NewMessageReceiptV2(0, []byte{0x01}, 42, &randomCid, nil, nil)
	assert.True(mr6.Equals(&mr7))

	// V1 vs V2 should not be equal even with matching fields
	mr8 := NewMessageReceiptV1(0, []byte{0x01}, 42, &randomCid)
	assert.False(mr8.Equals(&mr6))
}
