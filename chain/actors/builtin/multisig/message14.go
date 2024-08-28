package multisig

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	multisig14 "github.com/filecoin-project/go-state-types/builtin/v14/multisig"
	init15 "github.com/filecoin-project/go-state-types/builtin/v15/init"
	"github.com/filecoin-project/go-state-types/manifest"

	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/lotus/chain/actors"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

type message14 struct{ message0 }

func (m message14) Create(
	signers []address.Address, threshold uint64,
	unlockStart, unlockDuration abi.ChainEpoch,
	initialAmount abi.TokenAmount,
) (*types.Message, error) {

	lenAddrs := uint64(len(signers))

	if lenAddrs < threshold {
		return nil, xerrors.Errorf("cannot require signing of more addresses than provided for multisig")
	}

	if threshold == 0 {
		threshold = lenAddrs
	}

	if m.from == address.Undef {
		return nil, xerrors.Errorf("must provide source address")
	}

	// Set up constructor parameters for multisig
	msigParams := &multisig14.ConstructorParams{
		Signers:               signers,
		NumApprovalsThreshold: threshold,
		UnlockDuration:        unlockDuration,
		StartEpoch:            unlockStart,
	}

	enc, actErr := actors.SerializeParams(msigParams)
	if actErr != nil {
		return nil, actErr
	}

	code, ok := actors.GetActorCodeID(actorstypes.Version14, manifest.MultisigKey)
	if !ok {
		return nil, xerrors.Errorf("failed to get multisig code ID")
	}

	// new actors are created by invoking 'exec' on the init actor with the constructor params
	execParams := &init15.ExecParams{
		CodeCID:           code,
		ConstructorParams: enc,
	}

	enc, actErr = actors.SerializeParams(execParams)
	if actErr != nil {
		return nil, actErr
	}

	return &types.Message{
		To:     init_.Address,
		From:   m.from,
		Method: builtintypes.MethodsInit.Exec,
		Params: enc,
		Value:  initialAmount,
	}, nil
}
