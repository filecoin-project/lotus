package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// lookbackCacheSize bounds each lookback cache, matching the chainstore's
// tipset cache.
const lookbackCacheSize = 8192

// networkParams holds the connected chain's genesis time and block delay,
// fetched from the backend on first use and kept for the life of the process.
type networkParams struct {
	mu         sync.Mutex
	genesis    time.Time
	blockDelay time.Duration
	loaded     bool
}

func (gw *Node) chainParams(ctx context.Context) (genesis time.Time, blockDelay time.Duration, err error) {
	p := &gw.networkParams
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.loaded {
		params, err := gw.v1Proxy.server.StateGetNetworkParams(ctx)
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("getting network params: %w", err)
		}
		if params.BlockDelaySecs == 0 {
			return time.Time{}, 0, errors.New("network params report a zero block delay")
		}
		p.genesis = time.Unix(int64(params.GenesisTimestamp), 0)
		p.blockDelay = time.Duration(params.BlockDelaySecs) * time.Second
		p.loaded = true
	}
	return p.genesis, p.blockDelay, nil
}

func epochTime(genesis time.Time, blockDelay time.Duration, h abi.ChainEpoch) time.Time {
	return genesis.Add(time.Duration(h) * blockDelay)
}

func (gw *Node) checkEpoch(ctx context.Context, h abi.ChainEpoch) error {
	if h < 0 {
		return fmt.Errorf("bad tipset height: negative height %d", h)
	}
	genesis, blockDelay, err := gw.chainParams(ctx)
	if err != nil {
		return err
	}
	// Compared in epochs so that absurd heights cannot overflow epochTime.
	if h > abi.ChainEpoch(time.Since(genesis)/blockDelay)+1 {
		return errors.New("tipset height in future")
	}
	if err := gw.checkTimestamp(epochTime(genesis, blockDelay, h)); err != nil {
		return fmt.Errorf("bad tipset height: %w", err)
	}
	return nil
}

// currentEpoch is the epoch at now, so head without fetching it.
func (gw *Node) currentEpoch(ctx context.Context) (abi.ChainEpoch, error) {
	genesis, blockDelay, err := gw.chainParams(ctx)
	if err != nil {
		return 0, err
	}
	return abi.ChainEpoch(time.Since(genesis) / blockDelay), nil
}

func (gw *Node) checkTimestamp(at time.Time) error {
	if time.Since(at) > gw.maxLookbackDuration {
		return gw.errLookback
	}
	return nil
}

func (gw *Node) checkTipSet(ctx context.Context, ts *types.TipSet) error {
	return gw.checkEpoch(ctx, ts.Height())
}

func (gw *Node) checkTipSetKey(ctx context.Context, tsk types.TipSetKey) error {
	if tsk.IsEmpty() {
		return nil
	}
	h, err := gw.tipSetHeight(ctx, tsk)
	if err != nil {
		return err
	}
	return gw.checkEpoch(ctx, h)
}

// checkKeyedTipSetHeight checks a v1 request for height h walked back from the
// anchor tipset tsk, or from head when tsk is empty. Both the anchor and h must
// be within the lookback bound, and h cannot be above the anchor.
func (gw *Node) checkKeyedTipSetHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) error {
	if !tsk.IsEmpty() {
		anchorHeight, err := gw.tipSetHeight(ctx, tsk)
		if err != nil {
			return err
		}
		if h > anchorHeight {
			return fmt.Errorf("height %d is above the anchor tipset at %d", h, anchorHeight)
		}
		if err := gw.checkEpoch(ctx, anchorHeight); err != nil {
			return err
		}
	}
	return gw.checkEpoch(ctx, h)
}

// tipSetHeight caches only found tipsets; a key the node lacks may arrive later.
func (gw *Node) tipSetHeight(ctx context.Context, tsk types.TipSetKey) (abi.ChainEpoch, error) {
	if h, ok := gw.tipSetHeights.Get(tsk); ok {
		return h, nil
	}
	ts, err := gw.v1Proxy.server.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return 0, err
	}
	if ts == nil {
		return 0, fmt.Errorf("tipset %s not found", tsk)
	}
	gw.tipSetHeights.Add(tsk, ts.Height())
	return ts.Height(), nil
}

func (gw *Node) checkEthBlockHash(ctx context.Context, blkHash ethtypes.EthHash) error {
	tsk, err := gw.tipSetKeyByEthHash(ctx, blkHash)
	if err != nil {
		return err
	}
	return gw.checkTipSetKey(ctx, tsk)
}

// tipSetKeyByEthHash reads the tipset key an eth block hash names through the
// v1 backend, which is the only one offering ChainReadObj.
func (gw *Node) tipSetKeyByEthHash(ctx context.Context, blkHash ethtypes.EthHash) (types.TipSetKey, error) {
	if tsk, ok := gw.tipSetKeysByEthHash.Get(blkHash); ok {
		return tsk, nil
	}
	tskBlk, err := gw.v1Proxy.server.ChainReadObj(ctx, blkHash.ToCid())
	if err != nil {
		return types.EmptyTSK, err
	}
	var tsk types.TipSetKey
	if err := tsk.UnmarshalCBOR(bytes.NewReader(tskBlk)); err != nil {
		return types.EmptyTSK, fmt.Errorf("cannot unmarshal block into tipset key: %w", err)
	}
	gw.tipSetKeysByEthHash.Add(blkHash, tsk)
	return tsk, nil
}

func (gw *Node) checkEthBlockParam(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, lookback ethtypes.EthUint64) error {
	switch {
	case blkParam.PredefinedBlock != nil:
		return gw.checkEthBlockNumber(ctx, *blkParam.PredefinedBlock, lookback)
	case blkParam.BlockNumber != nil:
		return gw.checkEpoch(ctx, abi.ChainEpoch(*blkParam.BlockNumber))
	case blkParam.BlockHash != nil:
		return gw.checkEthBlockHash(ctx, *blkParam.BlockHash)
	}
	return errors.New("invalid block param")
}

// checkEthBlockNumber checks an eth block number or tag, less lookback epochs
// for a tag.
func (gw *Node) checkEthBlockNumber(ctx context.Context, blkParam string, lookback ethtypes.EthUint64) error {
	var h abi.ChainEpoch
	switch blkParam {
	case "earliest":
		// also not supported in node impl
		return errors.New("block param \"earliest\" is not supported")
	case "pending", "latest":
		// Head is always ok.
		if lookback == 0 {
			return nil
		}
		cur, err := gw.currentEpoch(ctx)
		if err != nil {
			return err
		}
		h = cur
	case "safe", "finalized":
		// The static EC distance is the oldest either tag resolves to, so a
		// bound it passes needs no backend call. A tighter bound resolves the
		// real tipset, which F3 or the EC calculator usually place far nearer.
		cur, err := gw.currentEpoch(ctx)
		if err != nil {
			return err
		}
		distance := policy.ChainFinality
		if blkParam == "safe" {
			distance = buildconstants.SafeHeightDistance
		}
		if err := gw.checkEpoch(ctx, cur-distance-abi.ChainEpoch(lookback)); err == nil {
			return nil
		}
		tagH, err := gw.ethTagHeight(ctx, blkParam)
		if err != nil {
			return err
		}
		h = tagH
	default:
		var num ethtypes.EthUint64
		if err := num.UnmarshalJSON([]byte(`"` + blkParam + `"`)); err != nil {
			return fmt.Errorf("cannot parse block number: %v", err)
		}
		return gw.checkEpoch(ctx, abi.ChainEpoch(num))
	}
	return gw.checkEpoch(ctx, h-abi.ChainEpoch(lookback))
}

// ethTagHeight resolves "safe" and "finalized" through the v2 backend, which
// takes the further ahead of F3 and the FRC-0089 EC finality calculator, as
// the eth APIs of both versions do.
func (gw *Node) ethTagHeight(ctx context.Context, tag string) (abi.ChainEpoch, error) {
	var selector types.TipSetSelector
	switch tag {
	case "safe":
		selector = types.TipSetSelectors.Safe
	case "finalized":
		selector = types.TipSetSelectors.Finalized
	default:
		return 0, fmt.Errorf("unknown block tag: %s", tag)
	}
	ts, err := gw.v2Proxy.server.ChainGetTipSet(ctx, selector)
	if err != nil {
		return 0, err
	}
	if ts == nil {
		return 0, fmt.Errorf("no %s tipset", tag)
	}
	return ts.Height(), nil
}
