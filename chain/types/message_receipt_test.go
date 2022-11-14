package types

import (
	"bytes"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
)

func TestMessageReceiptSerdeRoundrip(t *testing.T) {
	var (
		buf    = new(bytes.Buffer)
		err    error
		assert = assert.New(t)
	)

	randomCid, err := cid.Decode("bafy2bzacecu7n7wbtogznrtuuvf73dsz7wasgyneqasksdblxupnyovmtwxxu")
	assert.NoError(err)

	//
	// Version 0
	//
	mr := NewMessageReceiptV0(0, []byte{0x00, 0x01, 0x02, 0x04}, 42)

	// marshal
	err = mr.MarshalCBOR(buf)
	assert.NoError(err)

	// unmarshal
	var mr2 MessageReceipt
	err = mr2.UnmarshalCBOR(buf)
	assert.NoError(err)
	assert.Equal(mr, mr2)

	// version 0 with an events root -- should not serialize the events root!
	mr.EventsRoot = &randomCid

	buf.Reset()

	// marshal
	err = mr.MarshalCBOR(buf)
	assert.NoError(err)

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

	// unmarshal
	mr2 = MessageReceipt{}
	err = mr2.UnmarshalCBOR(buf)
	assert.NoError(err)
	assert.Equal(mr, mr2)
	assert.NotNil(mr2.EventsRoot)
}
