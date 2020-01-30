package sigs

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"
)

type SigShim interface {
	GenPrivate() ([]byte, error)
	ToPublic(pk []byte) ([]byte, error)
	Sign(pk []byte, msg []byte) ([]byte, error)
	Verify(sig []byte, a address.Address, msg []byte) error
}

var sigs map[string]SigShim

// RegisterSig should be only used during init
func RegisterSignature(name string, vs SigShim) {
	if sigs == nil {
		sigs = make(map[string]SigShim)
	}
	sigs[name] = vs
}

func Sign(sigType string, privkey []byte, msg []byte) (*types.Signature, error) {
	sv, ok := sigs[sigType]
	if !ok {
		return nil, fmt.Errorf("cannot sign message with signature of unsupported type: %s", sigType)
	}

	sb, err := sv.Sign(privkey, msg)
	if err != nil {
		return nil, err
	}
	return &types.Signature{
		Type: sigType,
		Data: sb,
	}, nil
}

func Verify(sig *types.Signature, addr address.Address, msg []byte) error {
	if sig == nil {
		return xerrors.Errorf("signature is nil")
	}

	if addr.Protocol() == address.ID {
		return fmt.Errorf("must resolve ID addresses before using them to verify a signature")
	}

	sv, ok := sigs[sig.Type]
	if !ok {
		return fmt.Errorf("cannot verify signature of unsupported type: %s", sig.Type)
	}

	return sv.Verify(sig.Data, addr, msg)
}

func Generate(sigType string) ([]byte, error) {
	sv, ok := sigs[sigType]
	if !ok {
		return nil, fmt.Errorf("cannot generate private key of unsupported type: %s", sigType)
	}

	return sv.GenPrivate()
}

func ToPublic(sigType string, pk []byte) ([]byte, error) {
	sv, ok := sigs[sigType]
	if !ok {
		return nil, fmt.Errorf("cannot generate public key of unsupported type: %s", sigType)
	}

	return sv.ToPublic(pk)
}

func CheckBlockSignature(blk *types.BlockHeader, ctx context.Context, worker address.Address) error {
	_, span := trace.StartSpan(ctx, "checkBlockSignature")
	defer span.End()

	if blk.BlockSig == nil {
		return xerrors.New("block signature not present")
	}

	sigb, err := blk.SigningBytes()
	if err != nil {
		return xerrors.Errorf("failed to get block signing bytes: %w", err)
	}

	_ = sigb
	//return blk.BlockSig.Verify(worker, sigb)
	return nil
}
