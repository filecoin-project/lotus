package consensus

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/crypto"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

var ErrTemporal = errors.New("temporal error")

func VerifyBlsAggregate(ctx context.Context, sig *crypto.Signature, msgs []cid.Cid, pubks [][]byte) error {
	_, span := trace.StartSpan(ctx, "syncer.VerifyBlsAggregate")
	defer span.End()
	span.AddAttributes(
		trace.Int64Attribute("msgCount", int64(len(msgs))),
	)

	msgsS := make([]ffi.Message, len(msgs))
	pubksS := make([]ffi.PublicKey, len(msgs))
	for i := 0; i < len(msgs); i++ {
		msgsS[i] = msgs[i].Bytes()
		copy(pubksS[i][:], pubks[i][:ffi.PublicKeyBytes])
	}

	sigS := new(ffi.Signature)
	copy(sigS[:], sig.Data[:ffi.SignatureBytes])

	if len(msgs) == 0 {
		return nil
	}

	valid := ffi.HashVerify(sigS, msgsS, pubksS)
	if !valid {
		return xerrors.New("bls aggregate signature failed to verify")
	}
	return nil
}

func AggregateSignatures(sigs []crypto.Signature) (*crypto.Signature, error) {
	sigsS := make([]ffi.Signature, len(sigs))
	for i := 0; i < len(sigs); i++ {
		copy(sigsS[i][:], sigs[i].Data[:ffi.SignatureBytes])
	}

	aggSig := ffi.Aggregate(sigsS)
	if aggSig == nil {
		if len(sigs) > 0 {
			return nil, xerrors.Errorf("bls.Aggregate returned nil with %d signatures", len(sigs))
		}

		zeroSig := ffi.CreateZeroSignature()

		// Note: for blst this condition should not happen - nil should not
		// be returned
		return &crypto.Signature{
			Type: crypto.SigTypeBLS,
			Data: zeroSig[:],
		}, nil
	}
	return &crypto.Signature{
		Type: crypto.SigTypeBLS,
		Data: aggSig[:],
	}, nil
}

func ToMessagesArray(store blockadt.Store, cids []cid.Cid) (cid.Cid, error) {
	arr := blockadt.MakeEmptyArray(store)
	for i, c := range cids {
		oc := cbg.CborCid(c)
		if err := arr.Set(uint64(i), &oc); err != nil {
			return cid.Undef, err
		}
	}
	return arr.Root()
}
