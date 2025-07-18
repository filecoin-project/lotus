package blockstore

import (
	"context"
	"io"
	"time"

	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/exchange"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
)

// InstrumentedBlockService wraps a blockservice.BlockService to provide
// metrics tracking for local vs network block fetches
type InstrumentedBlockService struct {
	blockservice.BlockService
	local Blockstore // Local blockstore to check for local hits
}

// NewInstrumentedBlockService creates a new instrumented block service
func NewInstrumentedBlockService(bs Blockstore, exch exchange.Interface) *InstrumentedBlockService {
	return &InstrumentedBlockService{
		BlockService: blockservice.New(bs, exch),
		local:        bs,
	}
}

// instrumentedMetrics tracks metrics for block fetch operations
type instrumentedMetrics struct {
	isSession bool
}

// recordFetchMetrics tracks metrics for a batch of CID fetches
func (m *instrumentedMetrics) recordFetchMetrics(ctx context.Context, cids []cid.Cid, local Blockstore) (localCount int, networkCids []cid.Cid) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		stats.Record(ctx, MessageFetchMeasures.Duration.M(float64(duration.Milliseconds())))
	}()

	// Record total requested
	stats.Record(ctx, MessageFetchMeasures.Requested.M(int64(len(cids))))

	// Check which blocks are available locally
	networkCids = make([]cid.Cid, 0, len(cids))

	for _, c := range cids {
		if has, err := local.Has(ctx, c); err == nil && has {
			localCount++
		} else {
			networkCids = append(networkCids, c)
		}
	}

	// Record local vs network counts
	if localCount > 0 {
		stats.Record(ctx, MessageFetchMeasures.Local.M(int64(localCount)))
	}
	if len(networkCids) > 0 {
		stats.Record(ctx, MessageFetchMeasures.Network.M(int64(len(networkCids))))
	}

	// Log details for debugging
	if len(networkCids) > 0 {
		logLevel := "Debugw"
		logMsg := "message fetch requiring network"
		if m.isSession {
			logLevel = "Infow"
			logMsg = "bitswap session fetch required"
		}

		if logLevel == "Infow" {
			log.Infow(logMsg,
				"total_requested", len(cids),
				"local_hits", localCount,
				"network_fetches", len(networkCids),
				"network_cids", networkCids)
		} else {
			log.Debugw(logMsg,
				"total_requested", len(cids),
				"local_hits", localCount,
				"network_fetches", len(networkCids),
				"network_cids", networkCids)
		}
	} else {
		logMsg := "message fetch all local"
		if m.isSession {
			logMsg = "session fetch all local"
		}
		log.Debugw(logMsg,
			"total_requested", len(cids),
			"local_hits", localCount)
	}

	return localCount, networkCids
}

// recordSingleFetchMetrics tracks metrics for a single CID fetch
func (m *instrumentedMetrics) recordSingleFetchMetrics(ctx context.Context, c cid.Cid, local Blockstore) string {
	start := time.Now()

	// Check if available locally first
	fetchSource := "local"
	if has, err := local.Has(ctx, c); err != nil || !has {
		fetchSource = "network"
		logMsg := "single message fetch requiring network"
		if m.isSession {
			logMsg = "bitswap session single fetch required"
			log.Infow(logMsg, "cid", c)
		} else {
			log.Debugw(logMsg, "cid", c)
		}
	}

	// Record metrics with source tag
	ctx, _ = tag.New(ctx, tag.Upsert(FetchSource, fetchSource))
	defer func() {
		duration := time.Since(start)
		stats.Record(ctx, MessageFetchMeasures.Duration.M(float64(duration.Milliseconds())))
		stats.Record(ctx, MessageFetchMeasures.Requested.M(1))

		if fetchSource == "local" {
			stats.Record(ctx, MessageFetchMeasures.Local.M(1))
		} else {
			stats.Record(ctx, MessageFetchMeasures.Network.M(1))
		}
	}()

	return fetchSource
}

// GetBlocks wraps the underlying GetBlocks to track local vs network fetches
func (ibs *InstrumentedBlockService) GetBlocks(ctx context.Context, cids []cid.Cid) <-chan blocks.Block {
	m := &instrumentedMetrics{isSession: false}
	m.recordFetchMetrics(ctx, cids, ibs.local)
	return ibs.BlockService.GetBlocks(ctx, cids)
}

// GetBlock wraps the underlying GetBlock to track individual fetches
func (ibs *InstrumentedBlockService) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	m := &instrumentedMetrics{isSession: false}
	m.recordSingleFetchMetrics(ctx, c, ibs.local)
	return ibs.BlockService.GetBlock(ctx, c)
}

// NewSession creates a new instrumented session
func (ibs *InstrumentedBlockService) NewSession(ctx context.Context) *InstrumentedSession {
	session := blockservice.NewSession(ctx, ibs.BlockService)
	return &InstrumentedSession{
		BlockGetter: session,
		local:       ibs.local,
	}
}

// Implement the remaining BlockService interface methods by delegating to the wrapped service

// AddBlock adds a single block to the blockservice
func (ibs *InstrumentedBlockService) AddBlock(ctx context.Context, block blocks.Block) error {
	return ibs.BlockService.AddBlock(ctx, block)
}

// AddBlocks adds multiple blocks to the blockservice
func (ibs *InstrumentedBlockService) AddBlocks(ctx context.Context, blocks []blocks.Block) error {
	return ibs.BlockService.AddBlocks(ctx, blocks)
}

// DeleteBlock deletes a block from the blockservice
func (ibs *InstrumentedBlockService) DeleteBlock(ctx context.Context, c cid.Cid) error {
	return ibs.BlockService.DeleteBlock(ctx, c)
}

// Blockstore returns the underlying blockstore
func (ibs *InstrumentedBlockService) Blockstore() BasicBlockstore {
	return ibs.BlockService.Blockstore()
}

// Exchange returns the underlying exchange
func (ibs *InstrumentedBlockService) Exchange() exchange.Interface {
	return ibs.BlockService.Exchange()
}

// Close closes the blockservice
func (ibs *InstrumentedBlockService) Close() error {
	if closer, ok := ibs.BlockService.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// InstrumentedSession wraps a blockservice session to provide metrics
type InstrumentedSession struct {
	blockservice.BlockGetter
	local Blockstore
}

// GetBlocks wraps the session's GetBlocks to track metrics
func (is *InstrumentedSession) GetBlocks(ctx context.Context, cids []cid.Cid) <-chan blocks.Block {
	m := &instrumentedMetrics{isSession: true}
	m.recordFetchMetrics(ctx, cids, is.local)
	return is.BlockGetter.GetBlocks(ctx, cids)
}

// GetBlock wraps the session's GetBlock to track individual fetches
func (is *InstrumentedSession) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	m := &instrumentedMetrics{isSession: true}
	m.recordSingleFetchMetrics(ctx, c, is.local)
	return is.BlockGetter.GetBlock(ctx, c)
}

// Ensure InstrumentedSession implements the BlockGetter interface
var _ blockservice.BlockGetter = (*InstrumentedSession)(nil)

// Ensure InstrumentedBlockService implements the BlockService interface
var _ blockservice.BlockService = (*InstrumentedBlockService)(nil)
