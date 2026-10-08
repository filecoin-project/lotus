package reward

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	miner20 "github.com/filecoin-project/go-state-types/builtin/v20/miner"
	reward20 "github.com/filecoin-project/go-state-types/builtin/v20/reward"
	smoothing20 "github.com/filecoin-project/go-state-types/builtin/v20/util/smoothing"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state20)(nil)

func load20(store adt.Store, root cid.Cid) (State, error) {
	out := state20{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make20(store adt.Store, currRealizedPower abi.StoragePower) (State, error) {
	st, err := reward20.ConstructState(store, currRealizedPower)
	if err != nil {
		return nil, err
	}
	return &state20{State: *st, store: store}, nil
}

type state20 struct {
	reward20.State
	store adt.Store
}

func (s *state20) ThisEpochReward() (abi.TokenAmount, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state20) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {

	return builtin.FilterEstimate{
		PositionEstimate: s.State.ThisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: s.State.ThisEpochRewardSmoothed.VelocityEstimate,
	}, nil

}

func (s *state20) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state20) TotalStoragePowerReward() (abi.TokenAmount, error) {
	// Since v19 this adapter reports all FIL minted by f02, not only miner rewards.
	// Circulating supply subtracts the burnt-funds balance, cancelling f02's residual burn.
	return s.State.TotalMintedReward, nil
}

func (s *state20) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state20) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state20) CumsumBaseline() (reward20.Spacetime, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state20) CumsumRealized() (reward20.Spacetime, error) {
	return s.State.CumsumRealized, nil
}

func (s *state20) InitialPledgeForPower(qaPower abi.StoragePower, _ abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount, epochsSinceRampStart int64, rampDurationEpochs uint64) (abi.TokenAmount, error) {
	return miner20.InitialPledgeForPower(
		qaPower,
		s.State.ThisEpochBaselinePower,
		s.State.ThisEpochRewardSmoothed,
		smoothing20.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		circSupply,
		epochsSinceRampStart,
		rampDurationEpochs,
	), nil
}

func (s *state20) PreCommitDepositForPower(networkQAPower builtin.FilterEstimate, sectorWeight abi.StoragePower) (abi.TokenAmount, error) {
	return miner20.PreCommitDepositForPower(s.State.ThisEpochRewardSmoothed,
		smoothing20.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		sectorWeight), nil
}

func (s *state20) StreamLedger(epoch abi.ChainEpoch) (*StreamLedger, error) {
	streams, err := s.State.LoadStreams(s.store)
	if err != nil {
		return nil, err
	}

	accrued := make(map[StreamID]abi.TokenAmount, len(s.State.Accrued))
	for _, accrual := range s.State.Accrued {
		accrued[StreamID(accrual.ID)] = accrual.Amount
	}

	ledger := &StreamLedger{
		Epoch:               epoch,
		SWAActor:            s.State.SWAActor,
		SWATimelock:         s.State.SWATimelockEpochs,
		TotalMinted:         s.State.TotalMintedReward,
		TotalBurnMinted:     s.State.TotalBurnMinted,
		TotalExplicitMinted: s.State.TotalExplicitMinted,
		Streams:             make([]Stream, 0, len(streams.Streams)),
		Tombstones:          make([]Tombstone, 0, len(streams.Tombstones)),
		PendingWrites:       make([]PendingWrite, 0, len(streams.PendingWritesQueue)),
	}

	explicit := 0
	for _, stream := range streams.Streams {
		weight := WeightRecord(stream.Weight)
		out := Stream{
			ID:              StreamID(stream.ID),
			Weight:          weight,
			EvaluatedWeight: ComputeWeight(weight, epoch),
			Implicit:        stream.Distribution == nil,
			Accrued:         big.Zero(),
		}
		if stream.Distribution != nil {
			amount, ok := accrued[StreamID(stream.ID)]
			if !ok {
				return nil, fmt.Errorf("explicit stream %d has no accrual row", stream.ID)
			}
			explicit++
			out.Writer = stream.Distribution.Writer
			out.Accrued = amount
			out.Shares = fromV20RecipientShares(stream.Distribution.Shares)
			out.Payable = fromV20RecipientAmounts(stream.Distribution.Payable)
			out.ClaimedPeriod = fromV20RecipientAmounts(stream.Distribution.ClaimedPeriod)
		}
		ledger.Streams = append(ledger.Streams, out)
	}
	if explicit != len(s.State.Accrued) {
		return nil, fmt.Errorf("%d accrual rows for %d explicit streams", len(s.State.Accrued), explicit)
	}

	for _, tombstone := range streams.Tombstones {
		ledger.Tombstones = append(ledger.Tombstones, Tombstone{
			ID:      StreamID(tombstone.ID),
			Payable: fromV20RecipientAmounts(tombstone.Payable),
		})
	}

	for _, write := range streams.PendingWritesQueue {
		decoded, err := fromV20PendingWrite(write)
		if err != nil {
			return nil, err
		}
		ledger.PendingWrites = append(ledger.PendingWrites, decoded)
	}

	return ledger, nil
}

func fromV20RecipientShares(v20 []reward20.RecipientShare) []RecipientShare {
	if v20 == nil {
		return nil
	}
	shares := make([]RecipientShare, 0, len(v20))
	for _, share := range v20 {
		shares = append(shares, RecipientShare{Recipient: share.Recipient, Share: share.Share})
	}
	return shares
}

func fromV20RecipientAmounts(v20 []reward20.RecipientAmount) []RecipientAmount {
	if v20 == nil {
		return nil
	}
	amounts := make([]RecipientAmount, 0, len(v20))
	for _, row := range v20 {
		amounts = append(amounts, RecipientAmount{Recipient: row.Recipient, Amount: row.Amount})
	}
	return amounts
}

func fromV20PendingWrite(v20 reward20.PendingWrite) (PendingWrite, error) {
	out := PendingWrite{
		Op:             PendingWriteOp(v20.Op),
		EffectiveEpoch: v20.EffectiveEpoch,
	}
	if v20.ID != nil {
		id := StreamID(*v20.ID)
		out.ID = &id
	}
	switch out.Op {
	case OpSetWeightRecords, OpStepWeightRecords:
		var payload reward20.SetWeightRecordsParams
		if err := decodeStreamPayload(v20.Payload, &payload); err != nil {
			return PendingWrite{}, xerrors.Errorf("decoding pending %s payload: %w", out.Op, err)
		}
		out.Updates = make([]WeightRecordUpdate, 0, len(payload.Updates))
		for _, update := range payload.Updates {
			out.Updates = append(out.Updates, WeightRecordUpdate{
				ID:     StreamID(update.ID),
				Weight: WeightRecord(update.Weight),
			})
		}
	case OpRegisterStream:
		var payload reward20.RegisterStreamPayload
		if err := decodeStreamPayload(v20.Payload, &payload); err != nil {
			return PendingWrite{}, xerrors.Errorf("decoding pending %s payload: %w", out.Op, err)
		}
		out.Register = &RegisterStreamPayload{Weight: WeightRecord(payload.Weight)}
		if payload.Distribution != nil {
			out.Register.Distribution = &DistributionInit{
				Writer: payload.Distribution.Writer,
				Shares: fromV20RecipientShares(payload.Distribution.Shares),
			}
		}
	case OpRemoveStream:
		// A removal names its stream and carries an empty payload.
	case OpSetDistribution:
		var payload reward20.SetDistributionPayload
		if err := decodeStreamPayload(v20.Payload, &payload); err != nil {
			return PendingWrite{}, xerrors.Errorf("decoding pending %s payload: %w", out.Op, err)
		}
		out.Writer = payload.Writer
	default:
		return PendingWrite{}, fmt.Errorf("unknown pending write operation %d", uint8(out.Op))
	}
	return out, nil
}

func (s *state20) GetState() interface{} {
	return &s.State
}

func (s *state20) ActorKey() string {
	return manifest.RewardKey
}

func (s *state20) ActorVersion() actorstypes.Version {
	return actorstypes.Version20
}

func (s *state20) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
