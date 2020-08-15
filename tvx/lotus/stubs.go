package lotus

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/vm"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"

	cbor "github.com/ipfs/go-ipld-cbor"
)

type vmRand struct{}

var _ vm.Rand = (*vmRand)(nil)

func (r *vmRand) GetChainRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return []byte("i_am_random_____i_am_random_____"), nil // 32 bytes.
}

func (r *vmRand) GetBeaconRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return []byte("i_am_random_____i_am_random_____"), nil // 32 bytes.
}

type fakedSigSyscalls struct {
	runtime.Syscalls
}

// TODO VerifySignature this will always succeed; but we want to be able to test failures too.
func (fss *fakedSigSyscalls) VerifySignature(_ crypto.Signature, _ address.Address, _ []byte) error {
	return nil
}

// TODO VerifySeal this will always succeed; but we want to be able to test failures too.
func (fss *fakedSigSyscalls) VerifySeal(_ abi.SealVerifyInfo) error {
	return nil
}

// TODO VerifyPoSt this will always succeed; but we want to be able to test failures too.
func (fss *fakedSigSyscalls) VerifyPoSt(_ abi.WindowPoStVerifyInfo) error {
	return nil
}

func mkFakedSigSyscalls(base vm.SyscallBuilder) vm.SyscallBuilder {
	return func(ctx context.Context, cstate *state.StateTree, cst cbor.IpldStore) runtime.Syscalls {
		return &fakedSigSyscalls{
			base(ctx, cstate, cst),
		}
	}
}
