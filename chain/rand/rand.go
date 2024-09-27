package rand

import (
	"context"
	"encoding/binary"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/must"
)

var log = logging.Logger("rand")

func DrawRandomnessFromBase(rbase []byte, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return DrawRandomnessFromDigest(blake2b.Sum256(rbase), pers, round, entropy)
}

func DrawRandomnessFromDigest(digest [32]byte, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	h := must.One(blake2b.New256(nil))
	if err := binary.Write(h, binary.BigEndian, int64(pers)); err != nil {
		return nil, xerrors.Errorf("deriving randomness: %w", err)
	}
	_, err := h.Write(digest[:])
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

func (sr *stateRand) GetBeaconRandomnessTipset(ctx context.Context, round abi.ChainEpoch, lookback bool) (*types.TipSet, error) {
	_, span := trace.StartSpan(ctx, "store.GetBeaconRandomness")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("round", int64(round)))

	ts, err := sr.cs.LoadTipSet(ctx, types.NewTipSetKey(sr.blks...))
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

	randTs, err := sr.cs.GetTipsetByHeight(ctx, searchHeight, ts, lookback)
	if err != nil {
		return nil, err
	}

	return randTs, nil
}

func (sr *stateRand) getChainRandomness(ctx context.Context, round abi.ChainEpoch, lookback bool) ([32]byte, error) {
	_, span := trace.StartSpan(ctx, "store.GetChainRandomness")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("round", int64(round)))

	ts, err := sr.cs.LoadTipSet(ctx, types.NewTipSetKey(sr.blks...))
	if err != nil {
		return [32]byte{}, err
	}

	if round > ts.Height() {
		return [32]byte{}, xerrors.Errorf("cannot draw randomness from the future")
	}

	searchHeight := round
	if searchHeight < 0 {
		searchHeight = 0
	}

	randTs, err := sr.cs.GetTipsetByHeight(ctx, searchHeight, ts, lookback)
	if err != nil {
		return [32]byte{}, err
	}

	return blake2b.Sum256(randTs.MinTicketBlock().Ticket.VRFProof), nil
}

type NetworkVersionGetter func(context.Context, abi.ChainEpoch) network.Version

type stateRand struct {
	cs                   *store.ChainStore
	blks                 []cid.Cid
	beacon               beacon.Schedule
	networkVersionGetter NetworkVersionGetter
}

type Rand interface {
	GetChainRandomness(ctx context.Context, round abi.ChainEpoch) ([32]byte, error)
	GetBeaconEntry(ctx context.Context, round abi.ChainEpoch) (*types.BeaconEntry, error)
	GetBeaconRandomness(ctx context.Context, round abi.ChainEpoch) ([32]byte, error)
}

func NewStateRand(cs *store.ChainStore, blks []cid.Cid, b beacon.Schedule, networkVersionGetter NetworkVersionGetter) Rand {
	return &stateRand{
		cs:                   cs,
		blks:                 blks,
		beacon:               b,
		networkVersionGetter: networkVersionGetter,
	}
}

// network v0-12
func (sr *stateRand) getBeaconEntryV1(ctx context.Context, round abi.ChainEpoch) (*types.BeaconEntry, error) {
	randTs, err := sr.GetBeaconRandomnessTipset(ctx, round, true)
	if err != nil {
		return nil, err
	}
	return sr.cs.GetLatestBeaconEntry(ctx, randTs)
}

// network v13
func (sr *stateRand) getBeaconEntryV2(ctx context.Context, round abi.ChainEpoch) (*types.BeaconEntry, error) {
	randTs, err := sr.GetBeaconRandomnessTipset(ctx, round, false)
	if err != nil {
		return nil, err
	}
	return sr.cs.GetLatestBeaconEntry(ctx, randTs)
}

// network v14 and on
func (sr *stateRand) getBeaconEntryV3(ctx context.Context, filecoinEpoch abi.ChainEpoch) (*types.BeaconEntry, error) {
	if filecoinEpoch < 0 {
		return sr.getBeaconEntryV2(ctx, filecoinEpoch)
	}

	randTs, err := sr.GetBeaconRandomnessTipset(ctx, filecoinEpoch, false)
	if err != nil {
		return nil, err
	}

	nv := sr.networkVersionGetter(ctx, filecoinEpoch)

	round := sr.beacon.BeaconForEpoch(filecoinEpoch).MaxBeaconRoundForEpoch(nv, filecoinEpoch)

	// Search back for the beacon entry, in normal operation it should be in randTs but for devnets
	// where the blocktime is faster than the beacon period we may need to search back a bit to find
	// the beacon entry for the requested round.
	for i := 0; i < 20; i++ {
		cbe := randTs.Blocks()[0].BeaconEntries
		for _, v := range cbe {
			if v.Round == round {
				return &v, nil
			}
		}

		next, err := sr.cs.LoadTipSet(ctx, randTs.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to load parents when searching back for beacon entry: %w", err)
		}

		randTs = next
	}

	return nil, xerrors.Errorf("didn't find beacon for round %d (epoch %d)", round, filecoinEpoch)
}

func (sr *stateRand) GetChainRandomness(ctx context.Context, filecoinEpoch abi.ChainEpoch) ([32]byte, error) {
	nv := sr.networkVersionGetter(ctx, filecoinEpoch)

	if nv >= network.Version13 {
		return sr.getChainRandomness(ctx, filecoinEpoch, false)
	}

	return sr.getChainRandomness(ctx, filecoinEpoch, true)
}

func (sr *stateRand) GetBeaconEntry(ctx context.Context, filecoinEpoch abi.ChainEpoch) (*types.BeaconEntry, error) {
	nv := sr.networkVersionGetter(ctx, filecoinEpoch)

	if nv >= network.Version14 {
		be, err := sr.getBeaconEntryV3(ctx, filecoinEpoch)
		if err != nil {
			log.Errorf("failed to get beacon entry as expected: %s", err)
		}
		return be, err
	} else if nv == network.Version13 {
		return sr.getBeaconEntryV2(ctx, filecoinEpoch)
	}
	return sr.getBeaconEntryV1(ctx, filecoinEpoch)
}

func (sr *stateRand) GetBeaconRandomness(ctx context.Context, filecoinEpoch abi.ChainEpoch) ([32]byte, error) {
	be, err := sr.GetBeaconEntry(ctx, filecoinEpoch)
	if err != nil {
		return [32]byte{}, err
	}
	return blake2b.Sum256(be.Data), nil
}

func (sr *stateRand) DrawChainRandomness(ctx context.Context, pers crypto.DomainSeparationTag, filecoinEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	digest, err := sr.GetChainRandomness(ctx, filecoinEpoch)

	if err != nil {
		return nil, xerrors.Errorf("failed to get chain randomness: %w", err)
	}

	ret, err := DrawRandomnessFromDigest(digest, pers, filecoinEpoch, entropy)
	if err != nil {
		return nil, xerrors.Errorf("failed to draw chain randomness: %w", err)
	}

	return ret, nil
}

func (sr *stateRand) DrawBeaconRandomness(ctx context.Context, pers crypto.DomainSeparationTag, filecoinEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	digest, err := sr.GetBeaconRandomness(ctx, filecoinEpoch)

	if err != nil {
		return nil, xerrors.Errorf("failed to get beacon randomness: %w", err)
	}

	ret, err := DrawRandomnessFromDigest(digest, pers, filecoinEpoch, entropy)
	if err != nil {
		return nil, xerrors.Errorf("failed to draw beacon randomness: %w", err)
	}

	return ret, nil
}
