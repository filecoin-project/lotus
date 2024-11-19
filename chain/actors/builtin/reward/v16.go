package reward

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	reward16 "github.com/filecoin-project/go-state-types/builtin/v16/reward"
	smoothing16 "github.com/filecoin-project/go-state-types/builtin/v16/util/smoothing"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state16)(nil)

func load16(store adt.Store, root cid.Cid) (State, error) {
	out := state16{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make16(store adt.Store, currRealizedPower abi.StoragePower) (State, error) {
	out := state16{store: store}
	out.State = *reward16.ConstructState(currRealizedPower)
	return &out, nil
}

type state16 struct {
	reward16.State
	store adt.Store
}

func (s *state16) ThisEpochReward() (abi.TokenAmount, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state16) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {

	return builtin.FilterEstimate{
		PositionEstimate: s.State.ThisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: s.State.ThisEpochRewardSmoothed.VelocityEstimate,
	}, nil

}

func (s *state16) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state16) TotalStoragePowerReward() (abi.TokenAmount, error) {
	return s.State.TotalStoragePowerReward, nil
}

func (s *state16) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state16) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state16) CumsumBaseline() (reward16.Spacetime, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state16) CumsumRealized() (reward16.Spacetime, error) {
	return s.State.CumsumRealized, nil
}

func (s *state16) InitialPledgeForPower(qaPower abi.StoragePower, _ abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount, epochsSinceRampStart int64, rampDurationEpochs uint64) (abi.TokenAmount, error) {
	return miner16.InitialPledgeForPower(
		qaPower,
		s.State.ThisEpochBaselinePower,
		s.State.ThisEpochRewardSmoothed,
		smoothing16.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		circSupply,
		epochsSinceRampStart,
		rampDurationEpochs,
	), nil
}

func (s *state16) PreCommitDepositForPower(networkQAPower builtin.FilterEstimate, sectorWeight abi.StoragePower) (abi.TokenAmount, error) {
	return miner16.PreCommitDepositForPower(s.State.ThisEpochRewardSmoothed,
		smoothing16.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		sectorWeight), nil
}

func (s *state16) GetState() interface{} {
	return &s.State
}

func (s *state16) ActorKey() string {
	return manifest.RewardKey
}

func (s *state16) ActorVersion() actorstypes.Version {
	return actorstypes.Version16
}

func (s *state16) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
