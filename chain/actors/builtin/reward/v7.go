package reward

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"

	miner7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/miner"
	reward7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/reward"
	smoothing7 "github.com/filecoin-project/specs-actors/v7/actors/util/smoothing"
)

var _ State = (*state7)(nil)

func load7(store adt.Store, root cid.Cid) (State, error) {
	out := state7{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make7(store adt.Store, currRealizedPower abi.StoragePower) (State, error) {
	out := state7{store: store}
	out.State = *reward7.ConstructState(currRealizedPower)
	return &out, nil
}

type state7 struct {
	reward7.State
	store adt.Store
}

func (s *state7) ThisEpochReward() (abi.TokenAmount, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state7) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {

	return builtin.FilterEstimate{
		PositionEstimate: s.State.ThisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: s.State.ThisEpochRewardSmoothed.VelocityEstimate,
	}, nil

}

func (s *state7) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state7) TotalStoragePowerReward() (abi.TokenAmount, error) {
	return s.State.TotalStoragePowerReward, nil
}

func (s *state7) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state7) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state7) CumsumBaseline() (reward7.Spacetime, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state7) CumsumRealized() (reward7.Spacetime, error) {
	return s.State.CumsumRealized, nil
}

func (s *state7) InitialPledgeForPower(qaPower abi.StoragePower, networkTotalPledge abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount) (abi.TokenAmount, error) {
	return miner7.InitialPledgeForPower(
		qaPower,
		s.State.ThisEpochBaselinePower,
		s.State.ThisEpochRewardSmoothed,
		smoothing7.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		circSupply,
	), nil
}

func (s *state7) PreCommitDepositForPower(networkQAPower builtin.FilterEstimate, sectorWeight abi.StoragePower) (abi.TokenAmount, error) {
	return miner7.PreCommitDepositForPower(s.State.ThisEpochRewardSmoothed,
		smoothing7.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		sectorWeight), nil
}

func (s *state7) GetState() interface{} {
	return &s.State
}
