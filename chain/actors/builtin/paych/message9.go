package paych

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin9 "github.com/filecoin-project/go-state-types/builtin"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"
	init9 "github.com/filecoin-project/go-state-types/builtin/v9/init"
	paych9 "github.com/filecoin-project/go-state-types/builtin/v9/paych"

	"github.com/filecoin-project/lotus/chain/actors"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

type message9 struct{ from address.Address }

func (m message9) Create(to address.Address, initialAmount abi.TokenAmount) (*types.Message, error) {

	actorCodeID, ok := actors.GetActorCodeID(actorstypes.Version9, "paymentchannel")
	if !ok {
		return nil, xerrors.Errorf("error getting actor paymentchannel code id for actor version %d", 9)
	}

	params, aerr := actors.SerializeParams(&paych9.ConstructorParams{From: m.from, To: to})
	if aerr != nil {
		return nil, aerr
	}
	enc, aerr := actors.SerializeParams(&init9.ExecParams{
		CodeCID:           actorCodeID,
		ConstructorParams: params,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     init_.Address,
		From:   m.from,
		Value:  initialAmount,
		Method: builtin9.MethodsInit.Exec,
		Params: enc,
	}, nil
}

func (m message9) Update(paych address.Address, sv *paychtypes.SignedVoucher, secret []byte) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych9.UpdateChannelStateParams{

		Sv: toV9SignedVoucher(*sv),

		Secret: secret,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin9.MethodsPaych.UpdateChannelState,
		Params: params,
	}, nil
}

func toV9SignedVoucher(sv paychtypes.SignedVoucher) paych9.SignedVoucher {
	merges := make([]paych9.Merge, len(sv.Merges))
	for i := range sv.Merges {
		merges[i] = paych9.Merge{
			Lane:  sv.Merges[i].Lane,
			Nonce: sv.Merges[i].Nonce,
		}
	}

	return paych9.SignedVoucher{
		ChannelAddr:     sv.ChannelAddr,
		TimeLockMin:     sv.TimeLockMin,
		TimeLockMax:     sv.TimeLockMax,
		SecretHash:      sv.SecretHash,
		Extra:           (*paych9.ModVerifyParams)(sv.Extra),
		Lane:            sv.Lane,
		Nonce:           sv.Nonce,
		Amount:          sv.Amount,
		MinSettleHeight: sv.MinSettleHeight,
		Merges:          merges,
		Signature:       sv.Signature,
	}
}

func (m message9) Settle(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin9.MethodsPaych.Settle,
	}, nil
}

func (m message9) Collect(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin9.MethodsPaych.Collect,
	}, nil
}
