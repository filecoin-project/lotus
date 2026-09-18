package reward

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	miner19 "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	smoothing19 "github.com/filecoin-project/go-state-types/builtin/v19/util/smoothing"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var _ State = (*state19)(nil)

func load19(store adt.Store, root cid.Cid) (State, error) {
	out := state19{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make19(store adt.Store, currRealizedPower abi.StoragePower) (State, error) {
	st, err := reward19.ConstructState(store, currRealizedPower)
	if err != nil {
		return nil, err
	}
	return &state19{State: *st, store: store}, nil
}

type state19 struct {
	reward19.State
	store adt.Store
}

func (s *state19) ThisEpochReward() (abi.TokenAmount, error) {
	return s.State.ThisEpochReward, nil
}

func (s *state19) ThisEpochRewardSmoothed() (builtin.FilterEstimate, error) {

	return builtin.FilterEstimate{
		PositionEstimate: s.State.ThisEpochRewardSmoothed.PositionEstimate,
		VelocityEstimate: s.State.ThisEpochRewardSmoothed.VelocityEstimate,
	}, nil

}

func (s *state19) ThisEpochBaselinePower() (abi.StoragePower, error) {
	return s.State.ThisEpochBaselinePower, nil
}

func (s *state19) TotalStoragePowerReward() (abi.TokenAmount, error) {
	// Since v19 this adapter reports all FIL minted by f02, not only miner rewards.
	// Circulating supply subtracts the burnt-funds balance, cancelling f02's residual burn.
	return s.State.TotalMintedReward, nil
}

func (s *state19) EffectiveBaselinePower() (abi.StoragePower, error) {
	return s.State.EffectiveBaselinePower, nil
}

func (s *state19) EffectiveNetworkTime() (abi.ChainEpoch, error) {
	return s.State.EffectiveNetworkTime, nil
}

func (s *state19) CumsumBaseline() (reward19.Spacetime, error) {
	return s.State.CumsumBaseline, nil
}

func (s *state19) CumsumRealized() (reward19.Spacetime, error) {
	return s.State.CumsumRealized, nil
}

func (s *state19) InitialPledgeForPower(qaPower abi.StoragePower, _ abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount, epochsSinceRampStart int64, rampDurationEpochs uint64) (abi.TokenAmount, error) {
	return miner19.InitialPledgeForPower(
		qaPower,
		s.State.ThisEpochBaselinePower,
		s.State.ThisEpochRewardSmoothed,
		smoothing19.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		circSupply,
		epochsSinceRampStart,
		rampDurationEpochs,
	), nil
}

func (s *state19) PreCommitDepositForPower(networkQAPower builtin.FilterEstimate, sectorWeight abi.StoragePower) (abi.TokenAmount, error) {
	return miner19.PreCommitDepositForPower(s.State.ThisEpochRewardSmoothed,
		smoothing19.FilterEstimate{
			PositionEstimate: networkQAPower.PositionEstimate,
			VelocityEstimate: networkQAPower.VelocityEstimate,
		},
		sectorWeight), nil
}

func (s *state19) StreamLedger(epoch abi.ChainEpoch) (*StreamLedger, error) {
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
			out.Shares = fromV19RecipientShares(stream.Distribution.Shares)
			out.Payable = fromV19RecipientAmounts(stream.Distribution.Payable)
			out.ClaimedPeriod = fromV19RecipientAmounts(stream.Distribution.ClaimedPeriod)
		}
		ledger.Streams = append(ledger.Streams, out)
	}
	if explicit != len(s.State.Accrued) {
		return nil, fmt.Errorf("%d accrual rows for %d explicit streams", len(s.State.Accrued), explicit)
	}

	for _, tombstone := range streams.Tombstones {
		ledger.Tombstones = append(ledger.Tombstones, Tombstone{
			ID:      StreamID(tombstone.ID),
			Payable: fromV19RecipientAmounts(tombstone.Payable),
		})
	}

	for _, write := range streams.PendingWritesQueue {
		decoded, err := fromV19PendingWrite(write)
		if err != nil {
			return nil, err
		}
		ledger.PendingWrites = append(ledger.PendingWrites, decoded)
	}

	return ledger, nil
}

func fromV19RecipientShares(v19 []reward19.RecipientShare) []RecipientShare {
	if v19 == nil {
		return nil
	}
	shares := make([]RecipientShare, 0, len(v19))
	for _, share := range v19 {
		shares = append(shares, RecipientShare{Recipient: share.Recipient, Share: share.Share})
	}
	return shares
}

func fromV19RecipientAmounts(v19 []reward19.RecipientAmount) []RecipientAmount {
	if v19 == nil {
		return nil
	}
	amounts := make([]RecipientAmount, 0, len(v19))
	for _, row := range v19 {
		amounts = append(amounts, RecipientAmount{Recipient: row.Recipient, Amount: row.Amount})
	}
	return amounts
}

func fromV19PendingWrite(v19 reward19.PendingWrite) (PendingWrite, error) {
	out := PendingWrite{
		Op:             PendingWriteOp(v19.Op),
		EffectiveEpoch: v19.EffectiveEpoch,
	}
	if v19.ID != nil {
		id := StreamID(*v19.ID)
		out.ID = &id
	}
	switch out.Op {
	case OpSetWeightRecords, OpStepWeightRecords:
		var payload reward19.SetWeightRecordsParams
		if err := decodeStreamPayload(v19.Payload, &payload); err != nil {
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
		var payload reward19.RegisterStreamPayload
		if err := decodeStreamPayload(v19.Payload, &payload); err != nil {
			return PendingWrite{}, xerrors.Errorf("decoding pending %s payload: %w", out.Op, err)
		}
		out.Register = &RegisterStreamPayload{Weight: WeightRecord(payload.Weight)}
		if payload.Distribution != nil {
			out.Register.Distribution = &DistributionInit{
				Writer: payload.Distribution.Writer,
				Shares: fromV19RecipientShares(payload.Distribution.Shares),
			}
		}
	case OpRemoveStream:
		// A removal names its stream and carries an empty payload.
	case OpSetDistribution:
		var payload reward19.SetDistributionPayload
		if err := decodeStreamPayload(v19.Payload, &payload); err != nil {
			return PendingWrite{}, xerrors.Errorf("decoding pending %s payload: %w", out.Op, err)
		}
		out.Writer = payload.Writer
	default:
		return PendingWrite{}, fmt.Errorf("unknown pending write operation %d", uint8(out.Op))
	}
	return out, nil
}

func (s *state19) GetState() interface{} {
	return &s.State
}

func (s *state19) ActorKey() string {
	return manifest.RewardKey
}

func (s *state19) ActorVersion() actorstypes.Version {
	return actorstypes.Version19
}

func (s *state19) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
