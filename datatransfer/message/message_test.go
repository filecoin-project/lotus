package message_test

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/datatransfer"
	. "github.com/filecoin-project/lotus/datatransfer/message"
	"github.com/filecoin-project/lotus/datatransfer/testutil"
)

func TestNewRequest(t *testing.T) {
	baseCid := testutil.GenerateCids(1)[0]
	selector := testutil.RandomBytes(100)
	isPull := true
	id := datatransfer.TransferID(rand.Int31())
	vtype := "FakeVoucherType"
	voucher := testutil.RandomBytes(100)

	request := NewRequest(id, isPull, vtype, voucher, baseCid, selector)
	assert.Equal(t, id, request.TransferID())
	assert.False(t, request.IsCancel())
	assert.True(t, request.IsPull())
	assert.True(t, request.IsRequest())
	assert.Equal(t, baseCid.String(), request.BaseCid().String())
	assert.Equal(t, vtype, request.VoucherType())
	assert.Equal(t, voucher, request.Voucher())
	assert.Equal(t, selector, request.Selector())

	// Sanity check to make sure we can cast to DataTransferMessage
	msg, ok := request.(DataTransferMessage)
	require.True(t, ok)

	assert.True(t, msg.IsRequest())
	assert.Equal(t, request.TransferID(), msg.TransferID())
}
func TestTransferRequest_MarshalCBOR(t *testing.T) {
	// sanity check MarshalCBOR does its thing w/o error
	req := NewTestTransferRequest()
	wbuf := new(bytes.Buffer)
	require.NoError(t, req.MarshalCBOR(wbuf))
	assert.Greater(t, wbuf.Len(), 0)
}
func TestTransferRequest_UnmarshalCBOR(t *testing.T) {
	req := NewTestTransferRequest()
	wbuf := new(bytes.Buffer)
	require.NoError(t, req.MarshalCBOR(wbuf))

	desMsg, err := FromNet(strings.NewReader(wbuf.String()))
	require.NoError(t, err)

	// Verify round-trip
	assert.Equal(t, req.TransferID(), desMsg.TransferID())
	assert.Equal(t, req.IsRequest(), desMsg.IsRequest())

	desReq := desMsg.(DataTransferRequest)
	assert.Equal(t, req.IsPull(), desReq.IsPull())
	assert.Equal(t, req.IsCancel(), desReq.IsCancel())
	assert.Equal(t, req.BaseCid(), desReq.BaseCid())
	assert.Equal(t, req.VoucherType(), desReq.VoucherType())
	assert.Equal(t, req.Voucher(), desReq.Voucher())
	assert.Equal(t, req.Selector(), desReq.Selector())
}

func TestResponses(t *testing.T) {
	id := datatransfer.TransferID(rand.Int31())
	response := NewResponse(id, false) // not accepted
	assert.Equal(t, response.TransferID(), id)
	assert.False(t, response.Accepted())
	assert.False(t, response.IsRequest())

	// Sanity check to make sure we can cast to DataTransferMessage
	msg, ok := response.(DataTransferMessage)
	require.True(t, ok)

	assert.False(t, msg.IsRequest())
	assert.Equal(t, response.TransferID(), msg.TransferID())
}

func TestTransferResponse_MarshalCBOR(t *testing.T) {
	id := datatransfer.TransferID(rand.Int31())
	response := NewResponse(id, true) // accepted

	// sanity check that we can marshal data
	wbuf := new(bytes.Buffer)
	require.NoError(t, response.MarshalCBOR(wbuf))
	assert.Greater(t, wbuf.Len(), 0)
}

func TestTransferResponse_UnmarshalCBOR(t *testing.T) {
	id := datatransfer.TransferID(rand.Int31())
	response := NewResponse(id, true) // accepted

	wbuf := new(bytes.Buffer)
	require.NoError(t, response.MarshalCBOR(wbuf))

	// verify round trip
	desMsg, err := FromNet(strings.NewReader(wbuf.String()))
	require.NoError(t, err)
	assert.False(t, desMsg.IsRequest())
	assert.Equal(t, id, desMsg.TransferID())

	desResp, ok := desMsg.(DataTransferResponse)
	require.True(t, ok)
	assert.True(t, desResp.Accepted())
}

func TestRequestResponseCast(t *testing.T) {
	// Verify that we gracefully handle a mis-cast
	id := datatransfer.TransferID(rand.Int31())
	response := NewResponse(id, true) // accepted
	request := NewTestTransferRequest()

	cast, ok := response.(DataTransferMessage)
	require.True(t, ok)
	cast, ok = cast.(DataTransferRequest)
	require.True(t, ok)
	require.False(t, cast.IsRequest())

	// have to do this first or else it won't compile b/c DataTransferRequest
	// doesn't implement the DataTransferResponse interface. This also
	// simulates steps for deserializing a message from the net
	cast, ok = request.(DataTransferMessage)
	require.True(t, ok)
	cast, ok = cast.(DataTransferResponse)
	require.True(t, ok)
	require.True(t, cast.IsRequest())
}

func TestRequestCancel(t *testing.T) {
	id := datatransfer.TransferID(rand.Int31())
	req := CancelRequest(id)
	require.Equal(t, req.TransferID(), id)
	require.True(t, req.IsRequest())
	require.True(t, req.IsCancel())

	wbuf := new(bytes.Buffer)
	require.NoError(t, req.MarshalCBOR(wbuf))

	deserialized, err := FromNet(strings.NewReader(wbuf.String()))
	require.NoError(t, err)

	deserializedRequest, ok := deserialized.(DataTransferRequest)
	require.True(t, ok)
	require.Equal(t, deserializedRequest.TransferID(), req.TransferID())
	require.Equal(t, deserializedRequest.IsCancel(), req.IsCancel())
	require.Equal(t, deserializedRequest.IsRequest(), req.IsRequest())
}

func TestToNetFromNetEquivalency(t *testing.T) {
	baseCid := testutil.GenerateCids(1)[0]
	selector := testutil.RandomBytes(100)
	isPull := false
	id := datatransfer.TransferID(rand.Int31())
	accepted := false
	voucherType := "FakeVoucherType"
	voucher := testutil.RandomBytes(100)
	request := NewRequest(id, isPull, voucherType, voucher, baseCid, selector)
	buf := new(bytes.Buffer)
	err := request.ToNet(buf)
	require.NoError(t, err)
	require.Greater(t, buf.Len(), 0)
	deserialized, err := FromNet(buf)
	require.NoError(t, err)

	deserializedRequest, ok := deserialized.(DataTransferRequest)
	require.True(t, ok)

	require.Equal(t, deserializedRequest.TransferID(), request.TransferID())
	require.Equal(t, deserializedRequest.IsCancel(), request.IsCancel())
	require.Equal(t, deserializedRequest.IsPull(), request.IsPull())
	require.Equal(t, deserializedRequest.IsRequest(), request.IsRequest())
	require.Equal(t, deserializedRequest.BaseCid(), request.BaseCid())
	require.Equal(t, deserializedRequest.VoucherType(), request.VoucherType())
	require.Equal(t, deserializedRequest.Voucher(), request.Voucher())
	require.Equal(t, deserializedRequest.Selector(), request.Selector())

	response := NewResponse(id, accepted)
	err = response.ToNet(buf)
	require.NoError(t, err)
	deserialized, err = FromNet(buf)
	require.NoError(t, err)

	deserializedResponse, ok := deserialized.(DataTransferResponse)
	require.True(t, ok)

	require.Equal(t, deserializedResponse.TransferID(), response.TransferID())
	require.Equal(t, deserializedResponse.Accepted(), response.Accepted())
	require.Equal(t, deserializedResponse.IsRequest(), response.IsRequest())

	request = CancelRequest(id)
	err = request.ToNet(buf)
	require.NoError(t, err)
	deserialized, err = FromNet(buf)
	require.NoError(t, err)

	deserializedRequest, ok = deserialized.(DataTransferRequest)
	require.True(t, ok)

	require.Equal(t, deserializedRequest.TransferID(), request.TransferID())
	require.Equal(t, deserializedRequest.IsCancel(), request.IsCancel())
	require.Equal(t, deserializedRequest.IsRequest(), request.IsRequest())
}

func NewTestTransferRequest() DataTransferRequest {
	bcid := testutil.GenerateCids(1)[0]
	selector := testutil.RandomBytes(100)
	isPull := false
	id := datatransfer.TransferID(rand.Int31())
	vtype := "FakeVoucherType"
	v := testutil.RandomBytes(100)
	return NewRequest(id, isPull, vtype, v, bcid, selector)
}
