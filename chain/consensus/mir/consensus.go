//go:generate go run ./gen/gen.go

// Package mir implements Mir integration in Lotus as an alternative consensus.
package mir

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	xerrors "golang.org/x/xerrors"
)

var _ consensus.Consensus = &Mir{}

var RewardFunc = func(ctx context.Context, vmi vm.Interface, em stmgr.ExecMonitor,
	epoch abi.ChainEpoch, ts *types.TipSet, params []byte) error {
	// TODO: No RewardFunc implemented for mir yet
	return nil
}

type Mir struct {
	beacon  beacon.Schedule
	sm      *stmgr.StateManager
	genesis *types.TipSet
}

func NewConsensus(
	ctx context.Context,
	sm *stmgr.StateManager,
	b beacon.Schedule,
	g chain.Genesis,
	netName dtypes.NetworkName,
) (consensus.Consensus, error) {
	return &Mir{
		beacon:  b,
		sm:      sm,
		genesis: g,
	}, nil
}

// CreateBlock creates a Filecoin block from the block template provided by Mir.
func (bft *Mir) CreateBlock(ctx context.Context, w lapi.Wallet, bt *lapi.BlockTemplate) (*types.FullBlock, error) {
	pts, err := bft.sm.ChainStore().LoadTipSet(ctx, bt.Parents)
	if err != nil {
		return nil, fmt.Errorf("failed to load parent tipset: %w", err)
	}

	st, recpts, err := bft.sm.TipSetState(ctx, pts)
	if err != nil {
		return nil, fmt.Errorf("failed to load tipset state: %w", err)
	}

	next := &types.BlockHeader{
		Miner:         builtin.SystemActorAddr, // Mir blocks are not signed, we use system addr as miner.
		Parents:       bt.Parents.Cids(),
		Ticket:        bt.Ticket,
		ElectionProof: bt.Eproof,

		BeaconEntries:         bt.BeaconValues,
		Height:                bt.Epoch,
		Timestamp:             bt.Timestamp,
		WinPoStProof:          bt.WinningPoStProof,
		ParentStateRoot:       st,
		ParentMessageReceipts: recpts,
	}
	blsMessages, secpkMessages, err := consensus.MsgsFromBlockTemplate(ctx, bft.sm, next, pts, bt)
	if err != nil {
		return nil, xerrors.Errorf("failed to process messages from block template: %w", err)
	}

	return &types.FullBlock{
		Header:        next,
		BlsMessages:   blsMessages,
		SecpkMessages: secpkMessages,
	}, nil
}

func (bft *Mir) ValidateBlockHeader(ctx context.Context, b *types.BlockHeader) (rejectReason string, err error) {
	// TODO: This is missing.
	log.Warn("oh oh! No specific block header validation implemented for Mir yet")
	return "", nil
}

func (bft *Mir) ValidateBlock(ctx context.Context, b *types.FullBlock) (err error) {
	log.Infof("starting block validation process at @%d", b.Header.Height)

	if err := blockSanityChecks(b.Header); err != nil {
		return xerrors.Errorf("incoming header failed basic sanity checks: %w", err)
	}

	h := b.Header

	baseTs, err := bft.sm.ChainStore().LoadTipSet(ctx, types.NewTipSetKey(h.Parents...))
	if err != nil {
		return xerrors.Errorf("load parent tipset failed (%s): %w", h.Parents, err)
	}
	if h.Height <= baseTs.Height() {
		return xerrors.Errorf("block height not greater than parent height: %d != %d", h.Height, baseTs.Height())
	}

	// Allow a small block drift
	now := uint64(build.Clock.Now().Unix())
	if h.Timestamp > now+build.AllowableClockDriftSecs {
		return xerrors.Errorf("block was from the future (now=%d, blk=%d): %w", now, h.Timestamp, consensus.ErrTemporal)
	}
	if h.Timestamp > now {
		log.Warn("got block from the future, but within threshold", h.Timestamp, build.Clock.Now().Unix())
	}

	pweight, err := bft.sm.ChainStore().Weight(ctx, baseTs)
	if err != nil {
		return xerrors.Errorf("getting parent weight: %w", err)
	}

	if types.BigCmp(pweight, b.Header.ParentWeight) != 0 {
		return xerrors.Errorf("parrent weight different: %s (header) != %s (computed)",
			b.Header.ParentWeight, pweight)
	}

	return consensus.RunAsyncChecks(ctx, consensus.CommonBlkChecks(ctx, bft.sm, bft.sm.ChainStore(), b, baseTs))
}

func blockSanityChecks(h *types.BlockHeader) error {
	// TODO: empty for now, when we include checkpoints for block validation, this may not be the case.
	empty := types.ElectionProof{}
	if h.ElectionProof.WinCount != empty.WinCount || !bytes.Equal(h.ElectionProof.VRFProof, empty.VRFProof) {
		return xerrors.Errorf("mir expects empty election proof")
	}

	if h.Ticket.VRFProof != nil {
		return xerrors.Errorf("mir block have nil ticket")
	}

	if h.BlockSig != nil {
		return xerrors.Errorf("mir blocks have no signature")
	}

	if h.BLSAggregate == nil {
		return xerrors.Errorf("block had nil bls aggregate signature")
	}

	if len(h.Parents) != 1 {
		return xerrors.Errorf("must have 1 parent")
	}

	if h.Miner.Protocol() != address.ID {
		return xerrors.Errorf("block had non-ID miner address")
	}

	if h.Miner != builtin.SystemActorAddr {
		return xerrors.Errorf("mir blocks include the systemActor addr as miner")
	}

	return nil
}

// IsEpochBeyondCurrMax is used in Filcns to detect delayed blocks.
// We are currently using defaults here and not worrying about it.
// We will consider potential changes of Consensus interface in https://github.com/filecoin-project/eudico/issues/143.
func (bft *Mir) IsEpochBeyondCurrMax(epoch abi.ChainEpoch) bool {
	return false
}

// Weight in mir uses a default approach where the height determines the weight.
//
// Every tipset in mir has a single block.
func Weight(ctx context.Context, stateBs bstore.Blockstore, ts *types.TipSet) (types.BigInt, error) {
	if ts == nil {
		return types.NewInt(0), nil
	}

	return big.NewInt(int64(ts.Height() + 1)), nil
}
