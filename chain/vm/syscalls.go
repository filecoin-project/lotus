package vm

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
)

func init() {
	mh.Codes[0xf104] = "filecoin"
}

// Actual type is defined in chain/types/vmcontext.go because the VMContext interface is there

func Syscalls(verifier ffiwrapper.Verifier) runtime.Syscalls {
	return &syscallShim{verifier}
}

type syscallShim struct {
	verifier ffiwrapper.Verifier
}

func (ss *syscallShim) ComputeUnsealedSectorCID(st abi.RegisteredProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	var sum abi.PaddedPieceSize
	for _, p := range pieces {
		sum += p.Size
	}

	commd, err := ffiwrapper.GenerateUnsealedCID(st, pieces)
	if err != nil {
		log.Errorf("generate data commitment failed: %s", err)
		return cid.Undef, err
	}

	return commd, nil
}

func (ss *syscallShim) HashBlake2b(data []byte) [32]byte {
	panic("NYI")
}

func (ss *syscallShim) VerifyConsensusFault(a, b, extra []byte, epoch abi.ChainEpoch) (*runtime.ConsensusFault, error) {
	panic("NYI")
}

func (ss *syscallShim) VerifyPoSt(proof abi.WindowPoStVerifyInfo) error {
	ok, err := ss.verifier.VerifyWindowPoSt(context.TODO(), proof)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("proof was invalid")
	}
	return nil
}

func cidToCommD(c cid.Cid) [32]byte {
	b := c.Bytes()
	var out [32]byte
	copy(out[:], b[len(b)-32:])
	return out
}

func cidToCommR(c cid.Cid) [32]byte {
	b := c.Bytes()
	var out [32]byte
	copy(out[:], b[len(b)-32:])
	return out
}

func (ss *syscallShim) VerifySeal(info abi.SealVerifyInfo) error {
	//_, span := trace.StartSpan(ctx, "ValidatePoRep")
	//defer span.End()

	miner, err := address.NewIDAddress(uint64(info.Miner))
	if err != nil {
		return xerrors.Errorf("weirdly failed to construct address: %w", err)
	}

	ticket := []byte(info.Randomness)
	proof := []byte(info.OnChain.Proof)
	seed := []byte(info.InteractiveRandomness)

	log.Infof("Verif r:%x; d:%x; m:%s; t:%x; s:%x; N:%d; p:%x", info.OnChain.SealedCID, info.UnsealedCID, miner, ticket, seed, info.SectorID.Number, proof)

	//func(ctx context.Context, maddr address.Address, ssize abi.SectorSize, commD, commR, ticket, proof, seed []byte, sectorID abi.SectorNumber)
	ok, err := ss.verifier.VerifySeal(info)
	if err != nil {
		return xerrors.Errorf("failed to validate PoRep: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid proof")
	}

	return nil
}

func (ss *syscallShim) VerifySignature(sig crypto.Signature, addr address.Address, input []byte) error {
	return nil
	/* // TODO: in genesis setup, we are currently faking signatures
	if err := ss.rt.vmctx.VerifySignature(&sig, addr, input); err != nil {
		return false
	}
	return true
	*/
}
