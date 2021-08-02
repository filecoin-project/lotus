package store

import (
	"context"
	"encoding/binary"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/minio/blake2b-simd"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func DrawRandomness(rbase []byte, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	h := blake2b.New256()
	if err := binary.Write(h, binary.BigEndian, int64(pers)); err != nil {
		return nil, xerrors.Errorf("deriving randomness: %w", err)
	}
	VRFDigest := blake2b.Sum256(rbase)
	_, err := h.Write(VRFDigest[:])
	if err != nil {
		return nil, xerrors.Errorf("hashing VRFDigest: %w", err)
	}
	if err := binary.Write(h, binary.BigEndian, round); err != nil {
		return nil, xerrors.Errorf("deriving randomness: %w", err)
	}
	_, err = h.Write(entropy)
	if err != nil {
		return nil, xerrors.Errorf("hashing entropy: %w", err)
	}

	return h.Sum(nil), nil
}

func (cs *ChainStore) GetBeaconRandomnessLookingBack(ctx context.Context, blks []cid.Cid, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cs.GetBeaconRandomness(ctx, blks, pers, round, entropy, true)
}

func (cs *ChainStore) GetBeaconRandomnessLookingForward(ctx context.Context, blks []cid.Cid, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cs.GetBeaconRandomness(ctx, blks, pers, round, entropy, false)
}

func (cs *ChainStore) GetBeaconRandomness(ctx context.Context, blks []cid.Cid, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte, lookback bool) ([]byte, error) {
	_, span := trace.StartSpan(ctx, "store.GetBeaconRandomness")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("round", int64(round)))

	ts, err := cs.LoadTipSet(types.NewTipSetKey(blks...))
	if err != nil {
		return nil, err
	}

	if round > ts.Height() {
		return nil, xerrors.Errorf("cannot draw randomness from the future")
	}

	searchHeight := round
	if searchHeight < 0 {
		searchHeight = 0
	}

	randTs, err := cs.GetTipsetByHeight(ctx, searchHeight, ts, lookback)
	if err != nil {
		return nil, err
	}

	be, err := cs.GetLatestBeaconEntry(randTs)
	if err != nil {
		return nil, err
	}

	// if at (or just past -- for null epochs) appropriate epoch
	// or at genesis (works for negative epochs)
	return DrawRandomness(be.Data, pers, round, entropy)
}

func (cs *ChainStore) GetChainRandomnessLookingBack(ctx context.Context, blks []cid.Cid, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cs.GetChainRandomness(ctx, blks, pers, round, entropy, true)
}

func (cs *ChainStore) GetChainRandomnessLookingForward(ctx context.Context, blks []cid.Cid, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cs.GetChainRandomness(ctx, blks, pers, round, entropy, false)
}

func (cs *ChainStore) GetChainRandomness(ctx context.Context, blks []cid.Cid, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte, lookback bool) ([]byte, error) {
	_, span := trace.StartSpan(ctx, "store.GetChainRandomness")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("round", int64(round)))

	ts, err := cs.LoadTipSet(types.NewTipSetKey(blks...))
	if err != nil {
		return nil, err
	}

	if round > ts.Height() {
		return nil, xerrors.Errorf("cannot draw randomness from the future")
	}

	searchHeight := round
	if searchHeight < 0 {
		searchHeight = 0
	}

	randTs, err := cs.GetTipsetByHeight(ctx, searchHeight, ts, lookback)
	if err != nil {
		return nil, err
	}

	mtb := randTs.MinTicketBlock()

	// if at (or just past -- for null epochs) appropriate epoch
	// or at genesis (works for negative epochs)
	return DrawRandomness(mtb.Ticket.VRFProof, pers, round, entropy)
}

func (cs *ChainStore) GetLatestBeaconEntry(ts *types.TipSet) (*types.BeaconEntry, error) {
	cur := ts
	for i := 0; i < 20; i++ {
		cbe := cur.Blocks()[0].BeaconEntries
		if len(cbe) > 0 {
			return &cbe[len(cbe)-1], nil
		}

		if cur.Height() == 0 {
			return nil, xerrors.Errorf("made it back to genesis block without finding beacon entry")
		}

		next, err := cs.LoadTipSet(cur.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to load parents when searching back for latest beacon entry: %w", err)
		}
		cur = next
	}

	if os.Getenv("LOTUS_IGNORE_DRAND") == "_yes_" {
		return &types.BeaconEntry{
			Data: []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9},
		}, nil
	}

	return nil, xerrors.Errorf("found NO beacon entries in the 20 latest tipsets")
}

type chainRand struct {
	cs   *ChainStore
	blks []cid.Cid
}

func NewChainRand(cs *ChainStore, blks []cid.Cid) vm.Rand {
	return &chainRand{
		cs:   cs,
		blks: blks,
	}
}

func (cr *chainRand) GetChainRandomnessLookingBack(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cr.cs.GetChainRandomnessLookingBack(ctx, cr.blks, pers, round, entropy)
}

func (cr *chainRand) GetChainRandomnessLookingForward(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cr.cs.GetChainRandomnessLookingForward(ctx, cr.blks, pers, round, entropy)
}

func (cr *chainRand) GetBeaconRandomnessLookingBack(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cr.cs.GetBeaconRandomnessLookingBack(ctx, cr.blks, pers, round, entropy)
}

func (cr *chainRand) GetBeaconRandomnessLookingForward(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cr.cs.GetBeaconRandomnessLookingForward(ctx, cr.blks, pers, round, entropy)
}

func (cs *ChainStore) GetTipSetFromKey(tsk types.TipSetKey) (*types.TipSet, error) {
	if tsk.IsEmpty() {
		return cs.GetHeaviestTipSet(), nil
	}
	return cs.LoadTipSet(tsk)
}
