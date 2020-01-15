package actors

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
)

type CronActor struct{}

type callTuple struct {
	addr   address.Address
	method uint64
}

var CronActors = []callTuple{
	{StoragePowerAddress, SPAMethods.CheckProofSubmissions},
}

type CronActorState struct{}

type cAMethods struct {
	EpochTick uint64
}

var CAMethods = cAMethods{2}

func (ca CronActor) Exports() []interface{} {
	return []interface{}{
		1: nil,
		2: ca.EpochTick,
	}
}

func (ca CronActor) EpochTick(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	if vmctx.Message().From != CronAddress {
		return nil, aerrors.New(1, "EpochTick is only callable as a part of tipset state computation")
	}

	for _, call := range CronActors {
		_, err := vmctx.Send(call.addr, call.method, types.NewInt(0), nil)
		if err != nil {
			return nil, err // todo: this very bad?
		}
	}

	return nil, nil
}
