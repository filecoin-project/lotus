package message_test

import (
	"bytes"
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
	require.Equal(t, request.TransferID(), id)
	require.False(t, request.IsCancel())
	require.False(t, request.IsPull())
	require.True(t, request.IsRequest())
	require.Equal(t, request.BaseCid().String(), baseCid.String())
	require.Equal(t, request.VoucherIdentifier(), voucherIdentifier)
	require.Equal(t, request.Voucher(), voucher)
	require.Equal(t, request.Selector(), selector)

	pbMessage := request.ToProto()
	// TODO: Uncomment once implemented
	//require.True(t, pbMessage.IsRequest)
	//pbRequest := pbMessage.Request
	//require.Equal(t, pbRequest.TransferID, int32(id))
	//require.False(t, pbRequest.Cancel)
	//require.False(t, pbRequest.Pull)
	//require.Equal(t, pbRequest.BaseCid, baseCid.Bytes())
	//require.Equal(t, pbRequest.VoucherIdentifier, voucherIdentifier)
	//require.Equal(t, pbRequest.Voucher, voucher)
	//require.Equal(t, pbRequest.Selector, selector)

	deserialized, err := message.NewMessageFromProto(*pbMessage)
	require.NoError(t, err)

	deserializedRequest, ok := deserialized.(message.DataTransferRequest)
	require.True(t, ok)

	require.Equal(t, deserializedRequest.TransferID(), request.TransferID())
	require.Equal(t, deserializedRequest.IsCancel(), request.IsCancel())
	require.Equal(t, deserializedRequest.IsPull(), request.IsPull())
	require.Equal(t, deserializedRequest.IsRequest(), request.IsRequest())
	require.Equal(t, deserializedRequest.BaseCid().String(), request.BaseCid().String())
	require.Equal(t, deserializedRequest.VoucherIdentifier(), request.VoucherIdentifier())
	require.Equal(t, deserializedRequest.Voucher(), request.Voucher())
	require.Equal(t, deserializedRequest.Selector(), request.Selector())

}

// TODO: get passing to complete
// https://github.com/filecoin-project/go-data-transfer/issues/37
func TestResponses(t *testing.T) {
	accepted := false
	id := datatransfer.TransferID(rand.Int31())
	response := message.NewResponse(id, accepted)
	require.Equal(t, response.TransferID(), id)
	require.False(t, response.Accepted())
	require.False(t, response.IsRequest())

	pbMessage := response.ToProto()
	// TODO: Uncomment once implemented
	//require.False(t, pbMessage.IsRequest)
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
	require.Equal(t, deserializedRequest.BaseCid().String(), request.BaseCid().String())
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
