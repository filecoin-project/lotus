package reward

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	reward14 "github.com/filecoin-project/go-state-types/builtin/v14/reward"
	smoothing14 "github.com/filecoin-project/go-state-types/builtin/v14/util/smoothing"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state14)(nil)

func load14(store adt.Store, root cid.Cid) (State, error) {
	out := state14{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make14(store adt.Store, currRealizedPower abi.StoragePower) (State, error) {
	out := state14{store: store}
	out.State = *reward14.ConstructState(currRealizedPower)
	return &out, nil
}

type state14 struct {
	reward14.State
	store adt.Store
}

func (s *state14) ThisEpochReward() (abi.TokenAmount, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state14) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {

	return builtin.FilterEstimate{
		PositionEstimate: s.State.ThisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: s.State.ThisEpochRewardSmoothed.VelocityEstimate,
	}, nil

}

func (s *state14) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state14) TotalStoragePowerReward() (abi.TokenAmount, error) {
	return s.State.TotalStoragePowerReward, nil
}

func (s *state14) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state14) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state14) CumsumBaseline() (reward14.Spacetime, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state14) CumsumRealized() (reward14.Spacetime, error) {
	return s.State.CumsumRealized, nil
}

func (s *state14) InitialPledgeForPower(qaPower abi.StoragePower, _ abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount, epochsSinceRampStart int64, rampDurationEpochs uint64) (abi.TokenAmount, error) {
	return miner14.InitialPledgeForPower(
		qaPower,
		s.State.ThisEpochBaselinePower,
		s.State.ThisEpochRewardSmoothed,
		smoothing14.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		circSupply,
	), nil
}

func (s *state14) PreCommitDepositForPower(networkQAPower builtin.FilterEstimate, sectorWeight abi.StoragePower) (abi.TokenAmount, error) {
	return miner14.PreCommitDepositForPower(s.State.ThisEpochRewardSmoothed,
		smoothing14.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		sectorWeight), nil
}

func (s *state14) GetState() interface{} {
	return &s.State
}

func (s *state14) ActorKey() string {
	return manifest.RewardKey
}

func (s *state14) ActorVersion() actorstypes.Version {
	return actorstypes.Version14
}

func (s *state14) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
