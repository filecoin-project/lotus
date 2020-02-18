package vm

import (
	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

func init() {
	mh.Codes[0xf104] = "filecoin"
}

// Actual type is defined in chain/types/vmcontext.go because the VMContext interface is there

func Syscalls(verifier sectorbuilder.Verifier) runtime.Syscalls {
	return &syscallShim{verifier}
}

type syscallShim struct {
	verifier sectorbuilder.Verifier
}

func (ss *syscallShim) ComputeUnsealedSectorCID(ssize abi.SectorSize, pieces []abi.PieceInfo) (cid.Cid, error) {
	// TODO: does this pull in unwanted dependencies?
	var ffipieces []ffi.PublicPieceInfo
	for _, p := range pieces {
		ffipieces = append(ffipieces, ffi.PublicPieceInfo{
			Size:  p.Size.Unpadded(),
			CommP: cidToCommD(p.PieceCID),
		})
	}

	commd, err := sectorbuilder.GenerateDataCommitment(ssize, ffipieces)
	if err != nil {
		log.Errorf("generate data commitment failed: %s", err)
		return cid.Undef, err
	}

	// TODO: pulling these numbers kinda out of an unmerged PR
	hb, err := mh.Encode(commd[:], 0xf104)
	if err != nil {
		log.Errorf("mh encode failed: %s", err)
		return cid.Undef, err
	}

	return cid.NewCidV1(0xf101, mh.Multihash(hb)), nil
}

func (ss *syscallShim) HashBlake2b(data []byte) [32]byte {
	panic("NYI")
}

func (ss *syscallShim) VerifyConsensusFault(a, b []byte) bool {
	panic("NYI")
}

func (ss *syscallShim) VerifyPoSt(ssize abi.SectorSize, proof abi.PoStVerifyInfo) (bool, error) {
	panic("NYI")
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

func (ss *syscallShim) VerifySeal(ssize abi.SectorSize, info abi.SealVerifyInfo) (bool, error) {
	//_, span := trace.StartSpan(ctx, "ValidatePoRep")
	//defer span.End()

	commD := cidToCommD(info.UnsealedCID)
	commR := cidToCommR(info.OnChain.SealedCID)

	miner, err := address.NewIDAddress(uint64(info.Miner))
	if err != nil {
		return false, xerrors.Errorf("weirdly failed to construct address: %w", err)
	}

	ticket := []byte(info.Randomness)
	proof := []byte(info.OnChain.Proof)
	seed := []byte(info.InteractiveRandomness)

	//func(ctx context.Context, maddr address.Address, ssize abi.SectorSize, commD, commR, ticket, proof, seed []byte, sectorID abi.SectorNumber)
	ok, err := ss.verifier.VerifySeal(ssize, commR[:], commD[:], miner, ticket, seed, info.SectorID.Number, proof)
	if err != nil {
		return false, xerrors.Errorf("failed to validate PoRep: %w", err)
	}

	return ok, nil
}

func (ss *syscallShim) VerifySignature(sig crypto.Signature, addr address.Address, input []byte) bool {
	return true
	/* // TODO: in genesis setup, we are currently faking signatures
	if err := ss.rt.vmctx.VerifySignature(&sig, addr, input); err != nil {
		return false
	}
	return true
	*/
}
