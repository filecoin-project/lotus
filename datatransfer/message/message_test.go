package message_test

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/datatransfer/message"
	"github.com/filecoin-project/lotus/datatransfer/testutil"
)

// TODO: get passing to complete
// https://github.com/filecoin-project/go-data-transfer/issues/37
func TestRequests(t *testing.T) {
	baseCid := testutil.GenerateCids(1)[0]
	selector := testutil.RandomBytes(100)
	isPull := false
	id := datatransfer.TransferID(rand.Int31())
	voucherIdentifier := "FakeVoucherType"
	voucher := testutil.RandomBytes(100)
	request := message.NewRequest(id, isPull, voucherIdentifier, voucher, baseCid, selector)
	assert.Equal(t, id, request.TransferID())
	assert.False(t, request.IsCancel())
	assert.False(t, request.IsPull())
	assert.True(t, request.IsRequest())
	assert.Equal(t, baseCid.String(), request.BaseCid())
	assert.Equal(t,  voucherIdentifier, request.VoucherIdentifier())
	assert.Equal(t, voucher, request.Voucher())
	assert.Equal(t, selector, request.Selector())

	pbMessage := request.ToProto()
	// TODO: Uncomment once implemented
	assert.True(t, pbMessage.IsRequest)  // IsRequest?
	pbRequest := pbMessage.Request
	assert.Equal(t, uint64(id), pbRequest.TransferID)
	assert.False(t, pbRequest.IsCancel)
	assert.False(t, pbRequest.IsPull)
	assert.Equal(t, baseCid.Bytes(), pbRequest.BaseCid)
	assert.Equal(t, pbRequest.TransferID, voucherIdentifier)
	assert.Equal(t, voucher, pbRequest.Voucher)
	assert.Equal(t, selector, pbRequest.Selector)

	deserialized, err := message.NewMessageFromProto(*pbMessage)
	assert.NoError(t, err)

	deserializedRequest, ok := deserialized.(message.DataTransferRequest)
	assert.True(t, ok)

	assert.Equal(t, request.TransferID(), deserializedRequest.TransferID())
	assert.Equal(t, request.IsCancel(), deserializedRequest.IsCancel())
	assert.Equal(t, request.IsPull(), deserializedRequest.IsPull())
	assert.Equal(t, request.IsRequest(), deserializedRequest.IsRequest())
	assert.Equal(t, request.BaseCid(), deserializedRequest.BaseCid())
	assert.Equal(t, request.VoucherIdentifier(), deserializedRequest.VoucherIdentifier())
	assert.Equal(t, request.Voucher(), deserializedRequest.Voucher())
	assert.Equal(t, request.Selector(), deserializedRequest.Selector())

}

// TODO: get passing to complete
// https://github.com/filecoin-project/go-data-transfer/issues/37
func TestResponses(t *testing.T) {
	accepted := false
	id := datatransfer.TransferID(rand.Int31())
	response := message.NewResponse(id, accepted)
	assert.Equal(t, response.TransferID(), id)
	assert.False(t, response.Accepted())
	assert.False(t, response.IsRequest())

	pbMessage := response.ToProto()
	// TODO: Uncomment once implemented
	//assert.False(t, pbMessage.IsRequest)
	//pbResponse := pbMessage.Response
	//require.Equal(t, pbResponse.TransferID, int32(id))
	//require.False(t, pbResponse.Accepted

	deserialized, err := message.NewMessageFromProto(*pbMessage)
	require.NoError(t, err)

	deserializedResponse, ok := deserialized.(message.DataTransferResponse)
	require.True(t, ok)

	require.Equal(t, deserializedResponse.TransferID(), response.TransferID())
	require.Equal(t, deserializedResponse.Accepted(), response.Accepted())
	require.Equal(t, deserializedResponse.IsRequest(), response.IsRequest())
}

// TODO: get passing to complete
// https://github.com/filecoin-project/go-data-transfer/issues/37
func TestRequestCancel(t *testing.T) {
	id := datatransfer.TransferID(rand.Int31())
	request := message.CancelRequest(id)
	require.Equal(t, request.TransferID(), id)
	require.True(t, request.IsRequest())
	require.True(t, request.IsCancel())

	pbMessage := request.ToProto()
	// TODO: Uncomment once implemented
	//require.True(t, pbMessage.IsRequest)
	//pbRequest := pbMessage.Request
	//require.Equal(t, pbRequest.TransferID, int32(id))
	//require.True(t, pbRequest.Cancel)

	deserialized, err := message.NewMessageFromProto(*pbMessage)
	require.NoError(t, err)

	deserializedRequest, ok := deserialized.(message.DataTransferRequest)
	require.True(t, ok)

	require.Equal(t, deserializedRequest.TransferID(), request.TransferID())
	require.Equal(t, deserializedRequest.IsCancel(), request.IsCancel())
	require.Equal(t, deserializedRequest.IsRequest(), request.IsRequest())
}

// TODO: get passing to complete
// https://github.com/filecoin-project/go-data-transfer/issues/37
func TestToNetFromNetEquivalency(t *testing.T) {
	baseCid := testutil.GenerateCids(1)[0]
	selector := testutil.RandomBytes(100)
	isPull := false
	id := datatransfer.TransferID(rand.Int31())
	accepted := false
	voucherIdentifier := "FakeVoucherType"
	voucher := testutil.RandomBytes(100)
	request := message.NewRequest(id, isPull, voucherIdentifier, voucher, baseCid, selector)
	buf := new(bytes.Buffer)
	err := request.ToNet(buf)
	require.NoError(t, err)
	deserialized, err := message.FromNet(buf)
	require.NoError(t, err)

	deserializedRequest, ok := deserialized.(message.DataTransferRequest)
	require.True(t, ok)

	require.Equal(t, deserializedRequest.TransferID(), request.TransferID())
	require.Equal(t, deserializedRequest.IsCancel(), request.IsCancel())
	require.Equal(t, deserializedRequest.IsPull(), request.IsPull())
	require.Equal(t, deserializedRequest.IsRequest(), request.IsRequest())
	require.Equal(t, deserializedRequest.BaseCid(), request.BaseCid())
	require.Equal(t, deserializedRequest.VoucherIdentifier(), request.VoucherIdentifier())
	require.Equal(t, deserializedRequest.Voucher(), request.Voucher())
	require.Equal(t, deserializedRequest.Selector(), request.Selector())

	response := message.NewResponse(id, accepted)
	err = request.ToNet(buf)
	require.NoError(t, err)
	deserialized, err = message.FromNet(buf)
	require.NoError(t, err)

	deserializedResponse, ok := deserialized.(message.DataTransferResponse)
	require.True(t, ok)

	require.Equal(t, deserializedResponse.TransferID(), response.TransferID())
	require.Equal(t, deserializedResponse.Accepted(), response.Accepted())
	require.Equal(t, deserializedResponse.IsRequest(), response.IsRequest())

	request = message.CancelRequest(id)
	err = request.ToNet(buf)
	require.NoError(t, err)
	deserialized, err = message.FromNet(buf)
	require.NoError(t, err)

	deserializedRequest, ok = deserialized.(message.DataTransferRequest)
	require.True(t, ok)

	require.Equal(t, deserializedRequest.TransferID(), request.TransferID())
	require.Equal(t, deserializedRequest.IsCancel(), request.IsCancel())
	require.Equal(t, deserializedRequest.IsRequest(), request.IsRequest())
}
