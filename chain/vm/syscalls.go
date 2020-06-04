package vm

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/minio/blake2b-simd"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
)

func init() {
	mh.Codes[0xf104] = "filecoin"
}

// Actual type is defined in chain/types/vmcontext.go because the VMContext interface is there

func Syscalls(verifier ffiwrapper.Verifier) runtime.Syscalls {
	return &syscallShim{verifier: verifier}
}

type syscallShim struct {
	ctx context.Context

	cstate   *state.StateTree
	cst      *cbor.BasicIpldStore
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
	return blake2b.Sum256(data)
}

// Checks validity of the submitted consensus fault with the two block headers needed to prove the fault
// and an optional extra one to check common ancestry (as needed).
// Note that the blocks are ordered: the method requires a.Epoch() <= b.Epoch().
func (ss *syscallShim) VerifyConsensusFault(a, b, extra []byte) (*runtime.ConsensusFault, error) {
	// Note that block syntax is not validated. Any validly signed block will be accepted pursuant to the below conditions.
	// Whether or not it could ever have been accepted in a chain is not checked/does not matter here.
	// for that reason when checking block parent relationships, rather than instantiating a Tipset to do so
	// (which runs a syntactic check), we do it directly on the CIDs.

	// (0) cheap preliminary checks

	// are blocks the same?
	if bytes.Equal(a, b) {
		return nil, fmt.Errorf("no consensus fault: submitted blocks are the same")
	}

	// can blocks be decoded properly?
	var blockA, blockB types.BlockHeader
	if decodeErr := blockA.UnmarshalCBOR(bytes.NewReader(a)); decodeErr != nil {
		return nil, xerrors.Errorf("cannot decode first block header: %w", decodeErr)
	}

	if decodeErr := blockB.UnmarshalCBOR(bytes.NewReader(b)); decodeErr != nil {
		return nil, xerrors.Errorf("cannot decode second block header: %f", decodeErr)
	}

	// (1) check conditions necessary to any consensus fault

	// were blocks mined by same miner?
	if blockA.Miner != blockB.Miner {
		return nil, fmt.Errorf("no consensus fault: blocks not mined by same miner")
	}

	// block a must be earlier or equal to block b, epoch wise (ie at least as early in the chain).
	if blockB.Height < blockA.Height {
		return nil, fmt.Errorf("first block must not be of higher height than second")
	}

	// (2) check for the consensus faults themselves
	var consensusFault *runtime.ConsensusFault

	// (a) double-fork mining fault
	if blockA.Height == blockB.Height {
		consensusFault = &runtime.ConsensusFault{
			Target: blockA.Miner,
			Epoch:  blockB.Height,
			Type:   runtime.ConsensusFaultDoubleForkMining,
		}
	}

	// (b) time-offset mining fault
	// strictly speaking no need to compare heights based on double fork mining check above,
	// but at same height this would be a different fault.
	if !types.CidArrsEqual(blockA.Parents, blockB.Parents) && blockA.Height != blockB.Height {
		consensusFault = &runtime.ConsensusFault{
			Target: blockA.Miner,
			Epoch:  blockB.Height,
			Type:   runtime.ConsensusFaultTimeOffsetMining,
		}
	}

	// (c) parent-grinding fault
	// Here extra is the "witness", a third block that shows the connection between A and B as
	// A's sibling and B's parent.
	// Specifically, since A is of lower height, it must be that B was mined omitting A from its tipset
	var blockC types.BlockHeader
	if len(extra) > 0 {
		if decodeErr := blockC.UnmarshalCBOR(bytes.NewReader(extra)); decodeErr != nil {
			return nil, xerrors.Errorf("cannot decode extra: %w", decodeErr)
		}

		if types.CidArrsEqual(blockA.Parents, blockC.Parents) && blockA.Height == blockC.Height &&
			types.CidArrsContains(blockB.Parents, blockC.Cid()) && !types.CidArrsContains(blockB.Parents, blockA.Cid()) {
			consensusFault = &runtime.ConsensusFault{
				Target: blockA.Miner,
				Epoch:  blockB.Height,
				Type:   runtime.ConsensusFaultParentGrinding,
			}
		}
	}

	// (3) return if no consensus fault by now
	if consensusFault == nil {
		return consensusFault, nil
	}

	// else
	// (4) expensive final checks

	// check blocks are properly signed by their respective miner
	// note we do not need to check extra's: it is a parent to block b
	// which itself is signed, so it was willingly included by the miner
	if sigErr := ss.VerifyBlockSig(&blockA); sigErr != nil {
		return nil, xerrors.Errorf("cannot verify first block sig: %w", sigErr)
	}

	if sigErr := ss.VerifyBlockSig(&blockB); sigErr != nil {
		return nil, xerrors.Errorf("cannot verify first block sig: %w", sigErr)
	}

	return consensusFault, nil
}

func (ss *syscallShim) VerifyBlockSig(blk *types.BlockHeader) error {

	// get appropriate miner actor
	act, err := ss.cstate.GetActor(blk.Miner)
	if err != nil {
		return err
	}

	// use that to get the miner state
	var mas miner.State
	if err = ss.cst.Get(ss.ctx, act.Head, &mas); err != nil {
		return err
	}

	// and use to get resolved workerKey
	waddr, err := ResolveToKeyAddr(ss.cstate, ss.cst, mas.Info.Worker)
	if err != nil {
		return err
	}

	if err := sigs.CheckBlockSignature(blk, ss.ctx, waddr); err != nil {
		return err
	}

	return nil
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
	proof := []byte(info.Proof)
	seed := []byte(info.InteractiveRandomness)

	log.Debugf("Verif r:%x; d:%x; m:%s; t:%x; s:%x; N:%d; p:%x", info.SealedCID, info.UnsealedCID, miner, ticket, seed, info.SectorID.Number, proof)

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
	// TODO: in genesis setup, we are currently faking signatures

	kaddr, err := ResolveToKeyAddr(ss.cstate, ss.cst, addr)
	if err != nil {
		return err
	}

	return sigs.Verify(&sig, kaddr, input)
}
