package retrievalimpl_test

// TODO(rvagg): this is a transitional package to test compatibility between
// cbor-gen and bindnode - it can be removed if/when cbor-gen is also removed

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/schema"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v8/paych"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

func TestIpldCompat_DealResponse(t *testing.T) {
	compareDealResponse := func(t *testing.T, drExpected retrievalmarket.DealResponse, drActual retrievalmarket.DealResponse) {
		assert.Equal(t, drExpected.ID, drActual.ID)
		assert.Equal(t, drExpected.Message, drActual.Message)
		compareBigInt(t, drExpected.PaymentOwed, drActual.PaymentOwed, "PaymentOwed")
		assert.Equal(t, drExpected.Status, drActual.Status)
	}

	for _, testCase := range []struct {
		label string
		dr    retrievalmarket.DealResponse
	}{
		{"empty", retrievalmarket.DealResponse{}},
		{"full", retrievalmarket.DealResponse{
			Status:      2,
			ID:          3,
			PaymentOwed: big.NewInt(10001),
			Message:     "bip bop",
		}},
		{"negPayment", retrievalmarket.DealResponse{ // maybe nonsense, but worth a test
			Status:      2,
			ID:          3,
			PaymentOwed: big.NewInt(-10001),
			Message:     "bip bop",
		}},
	} {
		t.Run(testCase.label, func(t *testing.T) {
			// encode the DealResponse with cbor-gen to bytes
			var originalBuf bytes.Buffer
			assert.Nil(t, testCase.dr.MarshalCBOR(&originalBuf))
			originalBytes := originalBuf.Bytes()

			// decode the bytes to DealResponse with bindnode
			nb := basicnode.Prototype.Any.NewBuilder()
			assert.Nil(t, dagcbor.Decode(nb, &originalBuf))
			node := nb.Build()
			drBindnodeIface, err := retrievalmarket.BindnodeRegistry.TypeFromNode(node, &retrievalmarket.DealResponse{})
			assert.Nil(t, err)
			drBindnode, ok := drBindnodeIface.(*retrievalmarket.DealResponse)
			assert.True(t, ok)

			// compare objects
			compareDealResponse(t, testCase.dr, *drBindnode)

			// encode the new DealResponse with bindnode to bytes
			node = retrievalmarket.BindnodeRegistry.TypeToNode(drBindnode)
			var bindnodeBuf bytes.Buffer
			dagcbor.Encode(node.(schema.TypedNode).Representation(), &bindnodeBuf)
			bindnodeBytes := bindnodeBuf.Bytes()

			compareCbor(t, originalBytes, bindnodeBytes)

			// decode the new bytes to DealResponse with cbor-gen
			var roundtripdr retrievalmarket.DealResponse
			assert.Nil(t, roundtripdr.UnmarshalCBOR(&bindnodeBuf))

			// compare objects
			compareDealResponse(t, testCase.dr, *drBindnode)
		})
	}
}

func TestIpldCompat_DealProposal(t *testing.T) {
	acid, err := cid.Decode("bafy2bzaceashdsqgbnisdg76gdhupvkpop4br5rs3veuy4whuxagnoco6px6e")
	assert.Nil(t, err)
	aselector := selectorparse.CommonSelector_MatchChildren

	compareDealProposal := func(t *testing.T, drExpected retrievalmarket.DealProposal, drActual retrievalmarket.DealProposal) {
		// for cbor-gen we get the selector as a deferred bytes, but an datamodel.Node for bindnode,
		// so we make them both bytes and compare those
		assert.Equal(t, drExpected.PayloadCID, drActual.PayloadCID, "PayloadCID")
		assert.Equal(t, drExpected.ID, drActual.ID, "ID")
		if !drExpected.Selector.IsNull() || !drActual.Selector.IsNull() {
			assert.True(t, ipld.DeepEqual(drExpected.Selector.Node, drActual.Selector.Node), "Selector")
		}
		assert.Equal(t, drExpected.Params.PieceCID, drActual.Params.PieceCID, "PieceCID")
		compareBigInt(t, drExpected.Params.PricePerByte, drActual.Params.PricePerByte, "PricePerByte")
		assert.Equal(t, drExpected.Params.PaymentInterval, drActual.Params.PaymentInterval, "PaymentInterval")
		assert.Equal(t, drExpected.Params.PaymentIntervalIncrease, drActual.Params.PaymentIntervalIncrease, "PaymentIntervalIncrease")
		compareBigInt(t, drExpected.Params.UnsealPrice, drActual.Params.UnsealPrice, "UnsealPrice")
	}

	for _, testCase := range []struct {
		label string
		dp    retrievalmarket.DealProposal
	}{
		{"empty", retrievalmarket.DealProposal{PayloadCID: acid}}, // PayloadCID is a required field
		{"optional", retrievalmarket.DealProposal{
			PayloadCID: acid,
			ID:         1010,
			Params: retrievalmarket.Params{
				Selector:                retrievalmarket.CborGenCompatibleNode{},
				PieceCID:                nil,
				PricePerByte:            abi.NewTokenAmount(1001),
				PaymentInterval:         20,
				PaymentIntervalIncrease: 30,
				UnsealPrice:             abi.NewTokenAmount(2002),
			},
		}},
		{"full", retrievalmarket.DealProposal{
			PayloadCID: acid,
			ID:         1010,
			Params: retrievalmarket.Params{
				Selector:                retrievalmarket.CborGenCompatibleNode{Node: aselector},
				PieceCID:                &acid,
				PricePerByte:            abi.NewTokenAmount(1001),
				PaymentInterval:         20,
				PaymentIntervalIncrease: 30,
				UnsealPrice:             abi.NewTokenAmount(2002),
			},
		}},
	} {
		t.Run(testCase.label, func(t *testing.T) {
			// encode the DealProposal with cbor-gen to bytes
			var originalBuf bytes.Buffer
			assert.Nil(t, testCase.dp.MarshalCBOR(&originalBuf))
			originalBytes := originalBuf.Bytes()

			// roundtrip the original DealProposal using cbor-gen so we can compare
			// decoded forms
			var roundtripdr retrievalmarket.DealProposal
			assert.Nil(t, roundtripdr.UnmarshalCBOR(bytes.NewReader(originalBytes)))

			// decode the bytes to DealProposal with bindnode
			nb := basicnode.Prototype.Any.NewBuilder()
			assert.Nil(t, dagcbor.Decode(nb, &originalBuf))
			node := nb.Build()
			dpBindnodeIface, err := retrievalmarket.BindnodeRegistry.TypeFromNode(node, &retrievalmarket.DealProposal{})
			assert.Nil(t, err)
			dpBindnode, ok := dpBindnodeIface.(*retrievalmarket.DealProposal)
			assert.True(t, ok)

			// compare objects
			compareDealProposal(t, testCase.dp, *dpBindnode)

			// encode the new DealProposal with bindnode to bytes
			node = retrievalmarket.BindnodeRegistry.TypeToNode(dpBindnode)
			var bindnodeBuf bytes.Buffer
			dagcbor.Encode(node.(schema.TypedNode).Representation(), &bindnodeBuf)
			bindnodeBytes := bindnodeBuf.Bytes()

			compareCbor(t, originalBytes, bindnodeBytes)

			// decode the new bytes to DealProposal with cbor-gen
			var roundtripFromBindnodeDr retrievalmarket.DealProposal
			assert.Nil(t, roundtripFromBindnodeDr.UnmarshalCBOR(&bindnodeBuf))

			// compare objects
			compareDealProposal(t, roundtripdr, *dpBindnode)
			// compareDealProposal(t, roundtripFromBindnodeDr, *dpBindnode)
		})
	}
}

func TestIpldCompat_DealPayment(t *testing.T) {
	compareDealPayment := func(t *testing.T, dpExpected retrievalmarket.DealPayment, dpActual retrievalmarket.DealPayment) {
		assert.Equal(t, dpExpected.ID, dpActual.ID, "ID")
		assert.Equal(t, dpExpected.PaymentChannel.String(), dpActual.PaymentChannel.String(), "PaymentChannel")
		assert.Equal(t, dpExpected.PaymentVoucher == nil, dpActual.PaymentVoucher == nil, "PaymentVoucher")
	}

	for _, testCase := range []struct {
		label string
		dp    retrievalmarket.DealPayment
	}{
		// addresses can't be null, and they're not marked nullable or optional, so we have to insert them
		{"empty", retrievalmarket.DealPayment{PaymentChannel: address.TestAddress}},
		{"empty voucher", retrievalmarket.DealPayment{
			ID:             1001,
			PaymentChannel: address.TestAddress,
			PaymentVoucher: &paych.SignedVoucher{
				ChannelAddr: address.TestAddress2,
			},
		}},
		{"mostly full voucher", retrievalmarket.DealPayment{
			ID:             1001,
			PaymentChannel: address.TestAddress,
			PaymentVoucher: &paych.SignedVoucher{
				ChannelAddr:     address.TestAddress2,
				TimeLockMin:     abi.ChainEpoch(2222),
				TimeLockMax:     abi.ChainEpoch(333333),
				SecretHash:      []byte("1234567890abcdef"),
				Extra:           nil,
				Lane:            100,
				Nonce:           200,
				Amount:          big.MustFromString("12345678901234567891234567890123456789012345678901234567890"),
				MinSettleHeight: abi.ChainEpoch(444444444),
				Signature:       nil,
			},
		}},
		{"full", retrievalmarket.DealPayment{
			ID:             1001,
			PaymentChannel: address.TestAddress,
			PaymentVoucher: &paych.SignedVoucher{
				ChannelAddr: address.TestAddress2,
				TimeLockMin: abi.ChainEpoch(2222),
				TimeLockMax: abi.ChainEpoch(333333),
				SecretHash:  []byte("1234567890abcdef"),
				Extra: &paych.ModVerifyParams{
					Actor:  address.TestAddress,
					Method: abi.MethodNum(50),
					Data:   []byte("doo-bee-doo"),
				},
				Lane:            100,
				Nonce:           200,
				Amount:          big.MustFromString("12345678901234567891234567890123456789012345678901234567890"),
				MinSettleHeight: abi.ChainEpoch(444444444),
				Signature: &crypto.Signature{
					Type: crypto.SigTypeBLS,
					Data: []byte("beep-boop-beep"),
				},
			},
		}},
		{"full secp256k1", retrievalmarket.DealPayment{
			ID:             1001,
			PaymentChannel: address.TestAddress,
			PaymentVoucher: &paych.SignedVoucher{
				ChannelAddr: address.TestAddress2,
				TimeLockMin: abi.ChainEpoch(2222),
				TimeLockMax: abi.ChainEpoch(333333),
				SecretHash:  []byte("1234567890abcdef"),
				Extra: &paych.ModVerifyParams{
					Actor:  address.TestAddress,
					Method: abi.MethodNum(50),
					Data:   []byte("doo-bee-doo"),
				},
				Lane:            100,
				Nonce:           200,
				Amount:          big.MustFromString("12345678901234567891234567890123456789012345678901234567890"),
				MinSettleHeight: abi.ChainEpoch(444444444),
				Signature: &crypto.Signature{
					Type: crypto.SigTypeSecp256k1,
					Data: []byte("bop-bee-bop"),
				},
			},
		}},
	} {
		t.Run(testCase.label, func(t *testing.T) {
			// encode the DealPayment with cbor-gen to bytes
			var originalBuf bytes.Buffer
			assert.Nil(t, testCase.dp.MarshalCBOR(&originalBuf))
			originalBytes := originalBuf.Bytes()

			// decode the bytes to DealPayment with bindnode
			nb := basicnode.Prototype.Any.NewBuilder()
			assert.Nil(t, dagcbor.Decode(nb, &originalBuf))
			node := nb.Build()
			dpBindnodeIface, err := retrievalmarket.BindnodeRegistry.TypeFromNode(node, &retrievalmarket.DealPayment{})
			assert.Nil(t, err)
			dpBindnode, ok := dpBindnodeIface.(*retrievalmarket.DealPayment)
			assert.True(t, ok)

			// compare objects
			compareDealPayment(t, testCase.dp, *dpBindnode)

			// encode the new DealPayment with bindnode to bytes
			node = retrievalmarket.BindnodeRegistry.TypeToNode(dpBindnode)
			var bindnodeBuf bytes.Buffer
			dagcbor.Encode(node.(schema.TypedNode).Representation(), &bindnodeBuf)
			bindnodeBytes := bindnodeBuf.Bytes()

			compareCbor(t, originalBytes, bindnodeBytes)

			// decode the new bytes to DealPayment with cbor-gen
			var roundtripdp retrievalmarket.DealPayment
			assert.Nil(t, roundtripdp.UnmarshalCBOR(&bindnodeBuf))

			// compare objects
			compareDealPayment(t, testCase.dp, *dpBindnode)
		})
	}
}

// this exists not because the encoding bytes are different but the unitialized
// form may be different in the empty case; functionally they should be the same
func compareBigInt(t *testing.T, expected big.Int, actual big.Int, msg string) {
	// special case `nil` because it ends up being an empty bytes (0x40) which is
	// big.Int(0) in a round-trip according to cbor-gen encoding
	if expected.Int == nil {
		expected = big.Zero()
	}
	assert.Equal(t, expected, actual, msg)
}

// needed because cbor-gen sorts maps differently, so we can't compare bytes
// so instead we round-trip them, untyped, through go-ipld-prime and compare the
// output bytes
func compareCbor(t *testing.T, cb1 []byte, cb2 []byte) {
	assert.Equal(t, len(cb1), len(cb2))
	rt := func(cb []byte) []byte {
		na := basicnode.Prototype.Any.NewBuilder()
		err := dagcbor.Decode(na, bytes.NewReader(cb))
		assert.Nil(t, err)
		n := na.Build()
		var buf bytes.Buffer
		err = dagcbor.Encode(n, &buf)
		assert.Nil(t, err)
		return buf.Bytes()
	}
	if !bytes.Equal(rt(cb1), rt(cb2)) {
		t.Logf(
			"Round-tripped node forms of CBOR are not equal:\n\tExpected: %s\n\tActual:   %s",
			hex.EncodeToString(cb1),
			hex.EncodeToString(cb2))
		assert.Fail(t, "decoded cbor different")
	}
}
