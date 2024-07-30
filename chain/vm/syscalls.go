package vm

import (
	"bytes"
	"context"
	"fmt"
	goruntime "runtime"
	"sync"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/minio/blake2b-simd"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	runtime7 "github.com/filecoin-project/specs-actors/v7/actors/runtime"
	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/proofs"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
)

func init() {
	mh.Codes[0xf104] = "filecoin"
}

// Actual type is defined in chain/types/vmcontext.go because the VMContext interface is there

type SyscallBuilder func(ctx context.Context, rt *Runtime) runtime7.Syscalls

func Syscalls(verifier proofs.Verifier) SyscallBuilder {
	return func(ctx context.Context, rt *Runtime) runtime7.Syscalls {

		return &syscallShim{
			ctx:            ctx,
			epoch:          rt.CurrEpoch(),
			networkVersion: rt.NetworkVersion(),

			actor:   rt.Receiver(),
			cstate:  rt.state,
			cst:     rt.cst,
			lbState: rt.vm.lbStateGet,

			verifier: verifier,
		}
	}
}

type syscallShim struct {
	ctx context.Context

	epoch          abi.ChainEpoch
	networkVersion network.Version
	lbState        LookbackStateGetter
	actor          address.Address
	cstate         *state.StateTree
	cst            cbor.IpldStore
	verifier       proofs.Verifier
}

func (ss *syscallShim) ComputeUnsealedSectorCID(st abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	commd, err := proofs.GenerateUnsealedCID(st, pieces)
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
func (ss *syscallShim) VerifyConsensusFault(a, b, extra []byte) (*runtime7.ConsensusFault, error) {
	// Note that block syntax is not validated. Any validly signed block will be accepted pursuant to the below conditions.
	// Whether or not it could ever have been accepted in a chain is not checked/does not matter here.
	// for that reason when checking block parent relationships, rather than instantiating a Tipset to do so
	// (which runs a syntactic check), we do it directly on the CIDs.

	// (0) cheap preliminary checks

	// can blocks be decoded properly?
	var blockA, blockB types.BlockHeader
	if decodeErr := blockA.UnmarshalCBOR(bytes.NewReader(a)); decodeErr != nil {
		return nil, xerrors.Errorf("cannot decode first block header: %w", decodeErr)
	}

	// A _valid_ block must use an ID address, but that's not what we're checking here. We're
	// just making sure that adding additional address protocols won't lead to consensus issues.
	if !abi.AddressValidForNetworkVersion(blockA.Miner, ss.networkVersion) {
		return nil, xerrors.Errorf("address protocol unsupported in current network version: %d", blockA.Miner.Protocol())
	}

	if decodeErr := blockB.UnmarshalCBOR(bytes.NewReader(b)); decodeErr != nil {
		return nil, xerrors.Errorf("cannot decode second block header: %f", decodeErr)
	}

	if !abi.AddressValidForNetworkVersion(blockB.Miner, ss.networkVersion) {
		return nil, xerrors.Errorf("address protocol unsupported in current network version: %d", blockB.Miner.Protocol())
	}

	// workaround chain halt
	if buildconstants.IsNearUpgrade(blockA.Height, buildconstants.UpgradeOrangeHeight) {
		return nil, xerrors.Errorf("consensus reporting disabled around Upgrade Orange")
	}
	if buildconstants.IsNearUpgrade(blockB.Height, buildconstants.UpgradeOrangeHeight) {
		return nil, xerrors.Errorf("consensus reporting disabled around Upgrade Orange")
	}

	// are blocks the same?
	if blockA.Cid().Equals(blockB.Cid()) {
		return nil, fmt.Errorf("no consensus fault: submitted blocks are the same")
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
	var consensusFault *runtime7.ConsensusFault

	// (a) double-fork mining fault
	if blockA.Height == blockB.Height {
		consensusFault = &runtime7.ConsensusFault{
			Target: blockA.Miner,
			Epoch:  blockB.Height,
			Type:   runtime7.ConsensusFaultDoubleForkMining,
		}
	}

	// (b) time-offset mining fault
	// strictly speaking no need to compare heights based on double fork mining check above,
	// but at same height this would be a different fault.
	if types.CidArrsEqual(blockA.Parents, blockB.Parents) && blockA.Height != blockB.Height {
		consensusFault = &runtime7.ConsensusFault{
			Target: blockA.Miner,
			Epoch:  blockB.Height,
			Type:   runtime7.ConsensusFaultTimeOffsetMining,
		}
	}

	// (c) parent-grinding fault
	// Here extra is the "witness", a third block that shows the connection between A and B as
	// A's sibling and B's parent.
	// Specifically, since A is of lower height, it must be that B was mined omitting A from its tipset
	//
	//      B
	//      |
	//  [A, C]
	var blockC types.BlockHeader
	if len(extra) > 0 {
		if decodeErr := blockC.UnmarshalCBOR(bytes.NewReader(extra)); decodeErr != nil {
			return nil, xerrors.Errorf("cannot decode extra: %w", decodeErr)
		}

		if !abi.AddressValidForNetworkVersion(blockC.Miner, ss.networkVersion) {
			return nil, xerrors.Errorf("address protocol unsupported in current network version: %d", blockC.Miner.Protocol())
		}

		if types.CidArrsEqual(blockA.Parents, blockC.Parents) && blockA.Height == blockC.Height &&
			types.CidArrsContains(blockB.Parents, blockC.Cid()) && !types.CidArrsContains(blockB.Parents, blockA.Cid()) {
			consensusFault = &runtime7.ConsensusFault{
				Target: blockA.Miner,
				Epoch:  blockB.Height,
				Type:   runtime7.ConsensusFaultParentGrinding,
			}
		}
	}

	// (3) return if no consensus fault by now
	if consensusFault == nil {
		return nil, xerrors.Errorf("no consensus fault detected")
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
		return nil, xerrors.Errorf("cannot verify second block sig: %w", sigErr)
	}

	return consensusFault, nil
}

func (ss *syscallShim) VerifyBlockSig(blk *types.BlockHeader) error {
	waddr, err := ss.workerKeyAtLookback(blk.Height)
	if err != nil {
		return err
	}

	if err := sigs.CheckBlockSignature(ss.ctx, blk, waddr); err != nil {
		return err
	}

	return nil
}

func (ss *syscallShim) workerKeyAtLookback(height abi.ChainEpoch) (address.Address, error) {
	if ss.networkVersion >= network.Version7 && height < ss.epoch-policy.ChainFinality {
		return address.Undef, xerrors.Errorf("cannot get worker key (currEpoch %d, height %d)", ss.epoch, height)
	}

	lbState, err := ss.lbState(ss.ctx, height)
	if err != nil {
		return address.Undef, err
	}
	// get appropriate miner actor
	act, err := lbState.GetActor(ss.actor)
	if err != nil {
		return address.Undef, err
	}

	// use that to get the miner state
	mas, err := miner.Load(adt.WrapStore(ss.ctx, ss.cst), act)
	if err != nil {
		return address.Undef, err
	}

	info, err := mas.Info()
	if err != nil {
		return address.Undef, err
	}

	return ResolveToDeterministicAddr(ss.cstate, ss.cst, info.Worker)
}

func (ss *syscallShim) VerifyPoSt(info proof7.WindowPoStVerifyInfo) error {
	ok, err := ss.verifier.VerifyWindowPoSt(context.TODO(), info)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("proof was invalid")
	}
	return nil
}

func (ss *syscallShim) VerifySeal(info proof7.SealVerifyInfo) error {
	// _, span := trace.StartSpan(ctx, "ValidatePoRep")
	// defer span.End()

	miner, err := address.NewIDAddress(uint64(info.Miner))
	if err != nil {
		return xerrors.Errorf("weirdly failed to construct address: %w", err)
	}

	ticket := []byte(info.Randomness)
	proof := info.Proof
	seed := []byte(info.InteractiveRandomness)

	log.Debugf("Verif r:%s; d:%s; m:%s; t:%x; s:%x; N:%d; p:%x", info.SealedCID, info.UnsealedCID, miner, ticket, seed, info.SectorID.Number, proof)

	// func(ctx context.Context, maddr address.Address, ssize abi.SectorSize, commD, commR, ticket, proof, seed []byte, sectorID abi.SectorNumber)
	ok, err := ss.verifier.VerifySeal(info)
	if err != nil {
		return xerrors.Errorf("failed to validate PoRep: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid proof")
	}

	return nil
}

func (ss *syscallShim) VerifyAggregateSeals(aggregate proof7.AggregateSealVerifyProofAndInfos) error {
	ok, err := ss.verifier.VerifyAggregateSeals(aggregate)
	if err != nil {
		return xerrors.Errorf("failed to verify aggregated PoRep: %w", err)
	}

	if !ok {
		return fmt.Errorf("invalid aggregate proof")
	}

	return nil
}

func (ss *syscallShim) VerifyReplicaUpdate(update proof7.ReplicaUpdateInfo) error {
	ok, err := ss.verifier.VerifyReplicaUpdate(update)
	if err != nil {
		return xerrors.Errorf("failed to verify replica update: %w", err)
	}

	if !ok {
		return fmt.Errorf("invalid replica update")
	}

	return nil
}

func (ss *syscallShim) VerifySignature(sig crypto.Signature, addr address.Address, input []byte) error {
	// TODO: in genesis setup, we are currently faking signatures

	kaddr, err := ResolveToDeterministicAddr(ss.cstate, ss.cst, addr)
	if err != nil {
		return err
	}

	return sigs.Verify(&sig, kaddr, input)
}

var BatchSealVerifyParallelism = goruntime.NumCPU()

func (ss *syscallShim) BatchVerifySeals(inp map[address.Address][]proof7.SealVerifyInfo) (map[address.Address][]bool, error) {
	out := make(map[address.Address][]bool)

	sema := make(chan struct{}, BatchSealVerifyParallelism)

	var wg sync.WaitGroup
	for addr, seals := range inp {
		results := make([]bool, len(seals))
		out[addr] = results

		for i, s := range seals {
			wg.Add(1)
			go func(ma address.Address, ix int, svi proof7.SealVerifyInfo, res []bool) {
				defer wg.Done()
				sema <- struct{}{}

				if err := ss.VerifySeal(svi); err != nil {
					log.Warnw("seal verify in batch failed", "miner", ma, "sectorNumber", svi.SectorID.Number, "err", err)
					res[ix] = false
				} else {
					res[ix] = true
				}

				<-sema
			}(addr, i, s, results)
		}
	}
	wg.Wait()

	return out, nil
}
