package reward

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"

	miner3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/miner"
	reward3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/reward"
	smoothing3 "github.com/filecoin-project/specs-actors/v3/actors/util/smoothing"
)

var _ State = (*state3)(nil)

func load3(store adt.Store, root cid.Cid) (State, error) {
	out := state3{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make3(store adt.Store, currRealizedPower abi.StoragePower) (State, error) {
	out := state3{store: store}
	out.State = *reward3.ConstructState(currRealizedPower)
	return &out, nil
}

type state3 struct {
	reward3.State
	store adt.Store
}

func (s *state3) ThisEpochReward() (abi.TokenAmount, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state3) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {

	return builtin.FilterEstimate{
		PositionEstimate: s.State.ThisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: s.State.ThisEpochRewardSmoothed.VelocityEstimate,
	}, nil

}

func (s *state3) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state3) TotalStoragePowerReward() (abi.TokenAmount, error) {
	return s.State.TotalStoragePowerReward, nil
}

func (s *state3) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state3) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state3) CumsumBaseline() (reward3.Spacetime, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state3) CumsumRealized() (reward3.Spacetime, error) {
	return s.State.CumsumRealized, nil
}

func (s *state3) InitialPledgeForPower(qaPower abi.StoragePower, networkTotalPledge abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount) (abi.TokenAmount, error) {
	return miner3.InitialPledgeForPower(
		qaPower,
		s.State.ThisEpochBaselinePower,
		s.State.ThisEpochRewardSmoothed,
		smoothing3.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		circSupply,
	), nil
}

func (s *state3) PreCommitDepositForPower(networkQAPower builtin.FilterEstimate, sectorWeight abi.StoragePower) (abi.TokenAmount, error) {
	return miner3.PreCommitDepositForPower(s.State.ThisEpochRewardSmoothed,
		smoothing3.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		sectorWeight), nil
}

func (s *state3) GetState() interface{} {
	return &s.State
}
