package evm

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin14 "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
)

var Methods = builtin14.MethodsEVM

// See https://github.com/filecoin-project/builtin-actors/blob/6e781444cee5965278c46ef4ffe1fb1970f18d7d/actors/evm/src/lib.rs#L35-L42
const (
	ErrReverted exitcode.ExitCode = iota + 33 // EVM exit codes start at 33
	ErrInvalidInstruction
	ErrUndefinedInstruction
	ErrStackUnderflow
	ErrStackOverflow
	ErrIllegalMemoryAccess
	ErrBadJumpdest
	ErrSelfdestructFailed
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.EvmKey {
			return nil, xerrors.Errorf("actor code is not evm: %s", name)
		}

		switch av {

		case actorstypes.Version10:
			return load10(store, act.Head)

		case actorstypes.Version11:
			return load11(store, act.Head)

		case actorstypes.Version12:
			return load12(store, act.Head)

		case actorstypes.Version13:
			return load13(store, act.Head)

		case actorstypes.Version14:
			return load14(store, act.Head)

		}
	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actorstypes.Version, bytecode cid.Cid) (State, error) {
	switch av {

	case actorstypes.Version10:
		return make10(store, bytecode)

	case actorstypes.Version11:
		return make11(store, bytecode)

	case actorstypes.Version12:
		return make12(store, bytecode)

	case actorstypes.Version13:
		return make13(store, bytecode)

	case actorstypes.Version14:
		return make14(store, bytecode)

	default:
		return nil, xerrors.Errorf("evm actor only valid for actors v10 and above, got %d", av)
	}
}

type State interface {
	cbor.Marshaler

	Nonce() (uint64, error)
	IsAlive() (bool, error)
	GetState() interface{}

	GetBytecode() ([]byte, error)
	GetBytecodeCID() (cid.Cid, error)
	GetBytecodeHash() ([32]byte, error)
}
