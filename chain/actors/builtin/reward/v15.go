package reward

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	reward15 "github.com/filecoin-project/go-state-types/builtin/v15/reward"
	smoothing15 "github.com/filecoin-project/go-state-types/builtin/v15/util/smoothing"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state15)(nil)

func load15(store adt.Store, root cid.Cid) (State, error) {
	out := state15{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make15(store adt.Store, currRealizedPower abi.StoragePower) (State, error) {
	out := state15{store: store}
	out.State = *reward15.ConstructState(currRealizedPower)
	return &out, nil
}

type state15 struct {
	reward15.State
	store adt.Store
}

func (s *state15) ThisEpochReward() (abi.TokenAmount, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state15) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {

	return builtin.FilterEstimate{
		PositionEstimate: s.State.ThisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: s.State.ThisEpochRewardSmoothed.VelocityEstimate,
	}, nil

}

func (s *state15) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state15) TotalStoragePowerReward() (abi.TokenAmount, error) {
	return s.State.TotalStoragePowerReward, nil
}

func (s *state15) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state15) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state15) CumsumBaseline() (reward15.Spacetime, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state15) CumsumRealized() (reward15.Spacetime, error) {
	return s.State.CumsumRealized, nil
}

func (s *state15) InitialPledgeForPower(qaPower abi.StoragePower, _ abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount, epochsSinceRampStart int64, rampDurationEpochs uint64) (abi.TokenAmount, error) {
	return miner15.InitialPledgeForPower(
		qaPower,
		s.State.ThisEpochBaselinePower,
		s.State.ThisEpochRewardSmoothed,
		smoothing15.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		circSupply,
		epochsSinceRampStart,
		rampDurationEpochs,
	), nil
}

func (s *state15) PreCommitDepositForPower(networkQAPower builtin.FilterEstimate, sectorWeight abi.StoragePower) (abi.TokenAmount, error) {
	return miner15.PreCommitDepositForPower(s.State.ThisEpochRewardSmoothed,
		smoothing15.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		sectorWeight), nil
}

func (s *state15) GetState() interface{} {
	return &s.State
}

func (s *state15) ActorKey() string {
	return manifest.RewardKey
}

func (s *state15) ActorVersion() actorstypes.Version {
	return actorstypes.Version15
}

func (s *state15) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
