package reward

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"

	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	reward5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/reward"
	smoothing5 "github.com/filecoin-project/specs-actors/v5/actors/util/smoothing"
)

var _ State = (*state5)(nil)

func load5(store adt.Store, root cid.Cid) (State, error) {
	out := state5{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make5(store adt.Store, currRealizedPower abi.StoragePower) (State, error) {
	out := state5{store: store}
	out.State = *reward5.ConstructState(currRealizedPower)
	return &out, nil
}

type state5 struct {
	reward5.State
	store adt.Store
}

func (s *state5) ThisEpochReward() (abi.TokenAmount, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state5) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {

	return builtin.FilterEstimate{
		PositionEstimate: s.State.ThisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: s.State.ThisEpochRewardSmoothed.VelocityEstimate,
	}, nil

}

func (s *state5) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state5) TotalStoragePowerReward() (abi.TokenAmount, error) {
	return s.State.TotalStoragePowerReward, nil
}

func (s *state5) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state5) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state5) CumsumBaseline() (reward5.Spacetime, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state5) CumsumRealized() (reward5.Spacetime, error) {
	return s.State.CumsumRealized, nil
}

func (s *state5) InitialPledgeForPower(qaPower abi.StoragePower, networkTotalPledge abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount) (abi.TokenAmount, error) {
	return miner5.InitialPledgeForPower(
		qaPower,
		s.State.ThisEpochBaselinePower,
		s.State.ThisEpochRewardSmoothed,
		smoothing5.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		circSupply,
	), nil
}

func (s *state5) PreCommitDepositForPower(networkQAPower builtin.FilterEstimate, sectorWeight abi.StoragePower) (abi.TokenAmount, error) {
	return miner5.PreCommitDepositForPower(s.State.ThisEpochRewardSmoothed,
		smoothing5.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		sectorWeight), nil
}

func (s *state5) GetState() interface{} {
	return &s.State
}
