package conformance

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/runtime/proof"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/vm"

	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"

	cbor "github.com/ipfs/go-ipld-cbor"
)

type testSyscalls struct {
	runtime.Syscalls
}

// TODO VerifySignature this will always succeed; but we want to be able to test failures too.
func (fss *testSyscalls) VerifySignature(_ crypto.Signature, _ address.Address, _ []byte) error {
	return nil
}

// TODO VerifySeal this will always succeed; but we want to be able to test failures too.
func (fss *testSyscalls) VerifySeal(_ proof.SealVerifyInfo) error {
	return nil
}

// TODO VerifyPoSt this will always succeed; but we want to be able to test failures too.
func (fss *testSyscalls) VerifyPoSt(_ proof.WindowPoStVerifyInfo) error {
	return nil
}

func mkFakedSigSyscalls(base vm.SyscallBuilder) vm.SyscallBuilder {
	return func(ctx context.Context, cstate *state.StateTree, cst cbor.IpldStore) runtime.Syscalls {
		return &testSyscalls{
			base(ctx, cstate, cst),
		}
	}
}
