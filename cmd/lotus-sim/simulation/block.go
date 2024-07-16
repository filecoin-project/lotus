package simulation

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

const beaconPrefix = "mockbeacon:"

// nextBeaconEntries returns a fake beacon entries for the next block.
func (sim *Simulation) nextBeaconEntries() []types.BeaconEntry {
	parentBeacons := sim.head.Blocks()[0].BeaconEntries
	lastBeacon := parentBeacons[len(parentBeacons)-1]
	beaconRound := lastBeacon.Round + 1

	buf := make([]byte, len(beaconPrefix)+8)
	copy(buf, beaconPrefix)
	binary.BigEndian.PutUint64(buf[len(beaconPrefix):], beaconRound)
	beaconRand := sha256.Sum256(buf)
	return []types.BeaconEntry{{
		Round: beaconRound,
		Data:  beaconRand[:],
	}}
}

// nextTicket returns a fake ticket for the next block.
func (sim *Simulation) nextTicket() *types.Ticket {
	newProof := sha256.Sum256(sim.head.MinTicket().VRFProof)
	return &types.Ticket{
		VRFProof: newProof[:],
	}
}

// makeTipSet generates and executes the next tipset from the given messages. This method:
//
// 1. Stores the given messages in the Chainstore.
// 2. Creates and persists a single block mined by the same miner as the parent.
// 3. Creates a tipset from this block and executes it.
// 4. Returns the resulting tipset.
//
// This method does _not_ mutate local state (although it does add blocks to the datastore).
func (sim *Simulation) makeTipSet(ctx context.Context, messages []*types.Message) (*types.TipSet, error) {
	parentTs := sim.head
	parentState, parentRec, err := sim.StateManager.TipSetState(ctx, parentTs)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute parent tipset: %w", err)
	}
	msgsCid, err := sim.storeMessages(ctx, messages)
	if err != nil {
		return nil, xerrors.Errorf("failed to store block messages: %w", err)
	}

	uts := parentTs.MinTimestamp() + buildconstants.BlockDelaySecs

	blks := []*types.BlockHeader{{
		Miner:                 parentTs.MinTicketBlock().Miner, // keep reusing the same miner.
		Ticket:                sim.nextTicket(),
		BeaconEntries:         sim.nextBeaconEntries(),
		Parents:               parentTs.Cids(),
		Height:                parentTs.Height() + 1,
		ParentStateRoot:       parentState,
		ParentMessageReceipts: parentRec,
		Messages:              msgsCid,
		ParentBaseFee:         abi.NewTokenAmount(0),
		Timestamp:             uts,
		ElectionProof:         &types.ElectionProof{WinCount: 1},
	}}

	newTipSet, err := types.NewTipSet(blks)
	if err != nil {
		return nil, xerrors.Errorf("failed to create new tipset: %w", err)
	}

	err = sim.Node.Chainstore.PersistTipsets(ctx, []*types.TipSet{newTipSet})
	if err != nil {
		return nil, xerrors.Errorf("failed to persist block headers: %w", err)
	}

	now := time.Now()
	_, _, err = sim.StateManager.TipSetState(ctx, newTipSet)
	if err != nil {
		return nil, xerrors.Errorf("failed to compute new tipset: %w", err)
	}
	duration := time.Since(now)
	log.Infow("computed tipset", "duration", duration, "height", newTipSet.Height())

	return newTipSet, nil
}
