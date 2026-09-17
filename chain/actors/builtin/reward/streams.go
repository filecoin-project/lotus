package reward

import (
	"bytes"
	"encoding/json"
	"fmt"
	mathbig "math/big"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/cbor"
)

// PendingWriteOp is a deferred stream-weights-actor operation. Its values are in reward.go,
// with the rest of the ledger's actor-version aliases.
type PendingWriteOp uint8

var pendingWriteOpNames = map[PendingWriteOp]string{
	OpSetWeightRecords:  "SetWeightRecords",
	OpStepWeightRecords: "StepWeightRecords",
	OpRegisterStream:    "RegisterStream",
	OpRemoveStream:      "RemoveStream",
	OpSetDistribution:   "SetDistribution",
}

func (op PendingWriteOp) String() string {
	if name, ok := pendingWriteOpNames[op]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", uint8(op))
}

func (op PendingWriteOp) MarshalJSON() ([]byte, error) {
	return json.Marshal(op.String())
}

func (op *PendingWriteOp) UnmarshalJSON(in []byte) error {
	var name string
	if err := json.Unmarshal(in, &name); err != nil {
		return err
	}
	for candidate, candidateName := range pendingWriteOpNames {
		if candidateName == name {
			*op = candidate
			return nil
		}
	}
	return fmt.Errorf("unknown pending write operation %q", name)
}

// StreamLedger is the reward actor's stream ledger read at one epoch: the live streams with
// their weights evaluated, what retired streams still owe, and the writes the stream weights
// actor has queued.
type StreamLedger struct {
	// Epoch is the epoch the weights are evaluated at.
	Epoch abi.ChainEpoch
	// SWAActor is the only caller the reward actor takes stream configuration from.
	SWAActor address.Address
	// SWATimelock is the hold in epochs between a queued write and its effective epoch.
	SWATimelock abi.ChainEpoch
	// TotalMinted is all FIL minted through block rewards.
	TotalMinted abi.TokenAmount
	// TotalBurnMinted is the part of TotalMinted sent to the burnt funds actor.
	TotalBurnMinted abi.TokenAmount
	// TotalExplicitMinted is the part of TotalMinted accrued to explicit streams.
	TotalExplicitMinted abi.TokenAmount
	// Streams are the live streams, ascending by ID.
	Streams []Stream
	// Tombstones hold what removed streams still owe, ascending by ID.
	Tombstones []Tombstone
	// PendingWrites are the deferred writes, ordered by effective epoch.
	PendingWrites []PendingWrite
}

// Stream is one live reward stream, its weight evaluated at the ledger's epoch.
type Stream struct {
	// ID is the stream's identifier.
	ID StreamID
	// Weight is the stored record every award evaluates.
	Weight WeightRecord
	// EvaluatedWeight is Weight at the ledger's epoch: this stream's part of a block reward,
	// in Denom fixed point.
	EvaluatedWeight uint64
	// Implicit marks the stream that pays block producers, leaving the rest to pay recipients.
	Implicit bool
	// Writer installs this stream's share maps and is empty on an implicit stream.
	Writer address.Address
	// Accrued is the current period's accrual and is zero on an implicit stream.
	Accrued abi.TokenAmount
	// Shares is the current period's recipient allocation. What it leaves of Denom burns.
	Shares []RecipientShare
	// Payable holds settled amounts from earlier periods that recipients have not claimed.
	Payable []RecipientAmount
	// ClaimedPeriod is what recipients have already drawn against Accrued.
	ClaimedPeriod []RecipientAmount
}

// Tombstone holds what a removed stream still owes its recipients.
type Tombstone struct {
	// ID is the removed stream's identifier.
	ID StreamID
	// Payable is each recipient's unclaimed balance, ascending by recipient.
	Payable []RecipientAmount
}

// PendingWrite is one deferred write with its payload decoded.
type PendingWrite struct {
	// ID names the stream a per-stream write targets and is nil for a schedule-wide write.
	ID *StreamID
	// Op is the deferred operation.
	Op PendingWriteOp
	// EffectiveEpoch is the first epoch the write is due at.
	EffectiveEpoch abi.ChainEpoch
	// Updates are the weight records a SetWeightRecords or StepWeightRecords write installs.
	Updates []WeightRecordUpdate
	// Register is the stream a RegisterStream write creates.
	Register *RegisterStreamPayload
	// Writer is the replacement writer a SetDistribution write installs.
	Writer address.Address
}

// ComputeWeight evaluates a weight record at an epoch: its line clamped to the record's
// inclusive floor and cap, in Denom fixed point. It mirrors the actor's own evaluation, which
// go-state-types carries only as an unexported invariant helper.
func ComputeWeight(record WeightRecord, epoch abi.ChainEpoch) uint64 {
	delta := new(mathbig.Int).Sub(mathbig.NewInt(int64(epoch)), mathbig.NewInt(int64(record.TStart)))
	value := new(mathbig.Int).Mul(mathbig.NewInt(record.Slope), delta)
	value.Add(value, new(mathbig.Int).SetUint64(record.VStart))
	if upper := new(mathbig.Int).SetUint64(record.Cap); value.Cmp(upper) > 0 {
		value = upper
	}
	if lower := new(mathbig.Int).SetUint64(record.Floor); value.Cmp(lower) < 0 {
		value = lower
	}
	return value.Uint64()
}

// WeightTotal is the part of a block reward the live streams take at the ledger's epoch, in
// Denom fixed point.
func (l *StreamLedger) WeightTotal() *mathbig.Int {
	total := new(mathbig.Int)
	for _, stream := range l.Streams {
		total.Add(total, new(mathbig.Int).SetUint64(stream.EvaluatedWeight))
	}
	return total
}

// BurnWeight is what the live streams leave of Denom at the ledger's epoch: the part of a block
// reward that burns. An award refuses a schedule that makes it negative.
func (l *StreamLedger) BurnWeight() *mathbig.Int {
	return new(mathbig.Int).Sub(new(mathbig.Int).SetUint64(Denom), l.WeightTotal())
}

// MinerMinted is the part of TotalMinted paid to block producers.
func (l *StreamLedger) MinerMinted() abi.TokenAmount {
	return big.Sub(big.Sub(l.TotalMinted, l.TotalBurnMinted), l.TotalExplicitMinted)
}

// Liability is the explicit-stream value the reward actor holds for recipients, live and
// tombstoned. Its balance beyond this backs future awards.
func (l *StreamLedger) Liability() abi.TokenAmount {
	total := big.Zero()
	for _, stream := range l.Streams {
		total = big.Add(total, stream.Liability())
	}
	for _, tombstone := range l.Tombstones {
		total = big.Add(total, tombstone.Liability())
	}
	return total
}

// ShareTotal is the part of this stream's portion its recipients take, in Denom fixed point.
func (s Stream) ShareTotal() *mathbig.Int {
	total := new(mathbig.Int)
	for _, share := range s.Shares {
		total.Add(total, new(mathbig.Int).SetUint64(share.Share))
	}
	return total
}

// ShareBurn is what this stream's share map leaves of Denom: the part of its portion that burns.
func (s Stream) ShareBurn() *mathbig.Int {
	return new(mathbig.Int).Sub(new(mathbig.Int).SetUint64(Denom), s.ShareTotal())
}

// Liability is what this stream owes its recipients: the unclaimed part of the current period
// plus the balances carried from earlier ones.
func (s Stream) Liability() abi.TokenAmount {
	total := s.Accrued
	for _, row := range s.ClaimedPeriod {
		total = big.Sub(total, row.Amount)
	}
	for _, row := range s.Payable {
		total = big.Add(total, row.Amount)
	}
	return total
}

// Liability is what this tombstone owes the recipients of the stream it retired.
func (t Tombstone) Liability() abi.TokenAmount {
	total := big.Zero()
	for _, row := range t.Payable {
		total = big.Add(total, row.Amount)
	}
	return total
}

// decodeStreamPayload reads one queued write's operation tuple.
func decodeStreamPayload(payload []byte, out cbor.Unmarshaler) error {
	reader := bytes.NewReader(payload)
	if err := out.UnmarshalCBOR(reader); err != nil {
		return err
	}
	if reader.Len() != 0 {
		return fmt.Errorf("%d trailing bytes", reader.Len())
	}
	return nil
}
