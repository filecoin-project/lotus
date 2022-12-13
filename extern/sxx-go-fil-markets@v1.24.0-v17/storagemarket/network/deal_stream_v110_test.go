package network

import (
	"bufio"
	"bytes"
	"testing"
	"unicode/utf8"

	"github.com/ipfs/go-cid"
	"github.com/polydawn/refmt/cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"
	marketOld "github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
)

// TestReceivev110DealProposal verifies that the provider will reject a v110
// deal that has a non-utf8 deal label
func TestReceivev110DealProposal(t *testing.T) {
	runTest := func(label string, validUtf8 bool) {
		dp := makeOldDealProposal()
		dp.Proposal.Label = label

		dpReq := migrations.Proposal1{
			DealProposal: &dp,
		}

		var buff bytes.Buffer
		err := dpReq.MarshalCBOR(&buff)
		require.NoError(t, err)

		ds := &dealStreamv110{
			buffered: bufio.NewReader(&buff),
		}
		prop, err := ds.ReadDealProposal()
		if validUtf8 {
			require.NoError(t, err)
			require.True(t, prop.DealProposal.Proposal.Label.IsString())
		} else {
			require.Error(t, err)
		}
	}

	t.Run("empty label", func(t *testing.T) {
		runTest("", true)
	})
	t.Run("string label", func(t *testing.T) {
		runTest("label", true)
	})
	t.Run("byte label", func(t *testing.T) {
		label := []byte{66, 250}
		require.False(t, utf8.Valid(label))
		runTest(string(label), false)
	})
}

func TestDealLabelCheck(t *testing.T) {
	err := checkDealLabel("")
	require.NoError(t, err)
	err = checkDealLabel("label")
	require.NoError(t, err)
	err = checkDealLabel(string([]byte{66, 250}))
	require.Error(t, err)
}

// Expect that CBOR marshaling a string will give the same result as marshaling
// a DealLabel with that string.
func TestLabelMatchingString(t *testing.T) {
	str := "testing"
	marshaledStr, err := cbor.Marshal(str)
	require.NoError(t, err)

	l, err := market.NewLabelFromString(str)
	require.NoError(t, err)
	var marshaledLabel bytes.Buffer
	err = l.MarshalCBOR(&marshaledLabel)
	require.NoError(t, err)

	require.Equal(t, marshaledLabel.Bytes(), marshaledStr)
}

// Expect that CBOR marshaling a string with bytes that are not valid utf8
// will give a different result than marshaling a DealLabel with those bytes.
func TestLabelMatchingBytes(t *testing.T) {
	bz := []byte{66, 250}
	require.False(t, utf8.Valid(bz))
	marshaledStr, err := cbor.Marshal(string(bz))
	require.NoError(t, err)

	l, err := market.NewLabelFromBytes(bz)
	require.NoError(t, err)
	var marshaledLabelFromBytes bytes.Buffer
	err = l.MarshalCBOR(&marshaledLabelFromBytes)
	require.NoError(t, err)

	require.NotEqual(t, marshaledLabelFromBytes.Bytes(), marshaledStr)
}

// TestSignedProposalCidMatching verifies that the ipld-marshaled signed deal
// proposal cid matches between the old deal proposal format and the new one
// for strings, but not for non-utf8 bytes
func TestSignedProposalCidMatching(t *testing.T) {
	runTest := func(label string, expectEqual bool) {
		oldDealProp := makeOldDealProposal()
		oldDealProp.Proposal.Label = label
		oldDealPropNd, err := cborutil.AsIpld(&oldDealProp)
		require.NoError(t, err)

		//t.Logf("testing label %s", oldDealProp.Proposal.Label)

		newDealProp, err := migrations.MigrateClientDealProposal0To1(oldDealProp)
		require.NoError(t, err)
		newDealPropNd, err := cborutil.AsIpld(newDealProp)
		require.NoError(t, err)

		require.Equal(t, expectEqual, oldDealPropNd.Cid() == newDealPropNd.Cid())
	}

	t.Run("empty label", func(t *testing.T) {
		runTest("", true)
	})
	t.Run("string label", func(t *testing.T) {
		runTest("label", true)
	})
	t.Run("byte label", func(t *testing.T) {
		label := []byte{66, 250}
		require.False(t, utf8.Valid(label))
		runTest(string(label), false)
	})
}

func makeOldDealProposal() marketOld.ClientDealProposal {
	pieceCid, err := cid.Parse("bafkqaaa")
	if err != nil {
		panic(err)
	}
	return marketOld.ClientDealProposal{
		Proposal: marketOld.DealProposal{
			PieceCID:  pieceCid,
			PieceSize: abi.PaddedPieceSize(2048),

			Client:   address.TestAddress,
			Provider: address.TestAddress2,
			Label:    "label",

			StartEpoch: abi.ChainEpoch(10),
			EndEpoch:   abi.ChainEpoch(20),

			StoragePricePerEpoch: abi.NewTokenAmount(1),
			ProviderCollateral:   abi.NewTokenAmount(2),
			ClientCollateral:     abi.NewTokenAmount(3),
		},
		ClientSignature: crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: []byte("signature data"),
		},
	}
}
