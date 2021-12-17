package vm

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/specs-actors/v7/actors/runtime"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/state"
	cbor "github.com/ipfs/go-ipld-cbor"

	ffi "github.com/filecoin-project/filecoin-ffi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

var _ VMI = (*FVM)(nil)

type Extern interface {
	GetRandomnessFromTickets(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) abi.Randomness
	GetRandomnessFromBeacon(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) abi.Randomness
	VerifyConsensusFault(a, b, extra []byte) (address.Address, abi.ChainEpoch, runtime.ConsensusFaultType)
}
type FvmExtern struct {
	rand Rand
}

func (e *FvmExtern) GetRandomnessFromTickets(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) abi.Randomness {
	res, err := e.rand.GetChainRandomness(context.Background(), personalization, randEpoch, entropy)

	if err != nil {
		panic(aerrors.Fatalf("could not get ticket randomness: %s", err))
	}
	return res
}

func (e *FvmExtern) GetRandomnessFromBeacon(personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) abi.Randomness {
	res, err := e.rand.GetBeaconRandomness(context.Background(), personalization, randEpoch, entropy)

	if err != nil {
		panic(aerrors.Fatalf("could not get ticket randomness: %s", err))
	}
	return res
}

type FVM struct {
	machineId uint64
	extern    FvmExtern
}

func NewFVM(ctx context.Context, opts *VMOpts) (*FVM, error) {
	buf := blockstore.NewBuffered(opts.Bstore)
	cst := cbor.NewCborStore(buf)
	state, err := state.LoadStateTree(cst, opts.StateBase)
	if err != nil {
		return nil, err
	}

	baseCirc, err := opts.CircSupplyCalc(ctx, opts.Epoch, state)
	if err != nil {
		return nil, err
	}

	id, err := ffi.CreateFVM(0, opts.Epoch, opts.BaseFee, baseCirc, opts.NetworkVersion, opts.StateBase)
	if err != nil {
		return nil, err
	}

	return &FVM{
		extern:    FvmExtern{rand: opts.Rand},
		machineId: id,
	}, nil
}

func (vm *FVM) ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error) {
	msgBytes, err := cmsg.VMMessage().Serialize()
	if err != nil {
		return nil, xerrors.Errorf("serializing msg: %w", err)
	}

	ret, err := ffi.ApplyMessage(vm.machineId, msgBytes)
	if err != nil {
		return nil, xerrors.Errorf("applying msg: %w", err)
	}

	return &ApplyRet{
		MessageReceipt: types.MessageReceipt{
			Return:   ret.Return,
			ExitCode: exitcode.ExitCode(ret.ExitCode),
			GasUsed:  ret.GasUsed,
		},
		GasCosts: &GasOutputs{
			// TODO: do the other optional fields eventually
			BaseFeeBurn:        abi.TokenAmount{},
			OverEstimationBurn: abi.TokenAmount{},
			MinerPenalty:       ret.MinerPenalty,
			MinerTip:           ret.MinerTip,
			Refund:             abi.TokenAmount{},
			GasRefund:          0,
			GasBurned:          0,
		},
		// TODO: do these eventually, not consensus critical
		ActorErr:       nil,
		ExecutionTrace: types.ExecutionTrace{},
		Duration:       0,
	}, nil
}

func (vm *FVM) ApplyImplicitMessage(ctx context.Context, cmsg *types.Message) (*ApplyRet, error) {
	msgBytes, err := cmsg.VMMessage().Serialize()
	if err != nil {
		return nil, xerrors.Errorf("serializing msg: %w", err)
	}

	ret, err := ffi.ApplyMessage(vm.machineId, msgBytes)
	if err != nil {
		return nil, xerrors.Errorf("applying msg: %w", err)
	}

	return &ApplyRet{
		MessageReceipt: types.MessageReceipt{
			Return:   ret.Return,
			ExitCode: exitcode.ExitCode(ret.ExitCode),
			GasUsed:  ret.GasUsed,
		},
		GasCosts: &GasOutputs{
			// TODO: do the other optional fields eventually
			BaseFeeBurn:        abi.TokenAmount{},
			OverEstimationBurn: abi.TokenAmount{},
			MinerPenalty:       ret.MinerPenalty,
			MinerTip:           ret.MinerTip,
			Refund:             abi.TokenAmount{},
			GasRefund:          0,
			GasBurned:          0,
		},
		// TODO: do these eventually, not consensus critical
		ActorErr:       nil,
		ExecutionTrace: types.ExecutionTrace{},
		Duration:       0,
	}, nil
}

func (vm *FVM) Flush(ctx context.Context) (cid.Cid, error) {
	return cid.Undef, nil
}
