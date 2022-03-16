package lp2p

import (
	"context"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"

	"go.uber.org/fx"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	rcmgr "github.com/libp2p/go-libp2p-resource-manager"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node/repo"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
)

func ResourceManager(connMgrHi uint) func(lc fx.Lifecycle, repo repo.LockedRepo) (network.ResourceManager, error) {
	return func(lc fx.Lifecycle, repo repo.LockedRepo) (network.ResourceManager, error) {
		envvar := os.Getenv("LOTUS_RCMGR")
		if envvar == "" || envvar == "0" {
			// TODO opt-in for now -- flip this to enabled by default once we are comfortable with testing
			log.Info("libp2p resource manager is disabled")
			return network.NullResourceManager, nil
		}

		log.Info("libp2p resource manager is enabled")
		// enable debug logs for rcmgr
		logging.SetLogLevel("rcmgr", "debug")

		// Adjust default limits
		// - give it more memory, up to 4G, min of 1G
		// - if maxconns are too high, adjust Conn/FD/Stream limits
		defaultLimits := rcmgr.DefaultLimits.WithSystemMemory(.125, 1<<30, 4<<30)
		maxconns := int(connMgrHi)
		if 2*maxconns > defaultLimits.SystemBaseLimit.ConnsInbound {
			// adjust conns to 2x to allow for two conns per peer (TCP+QUIC)
			defaultLimits.SystemBaseLimit.ConnsInbound = logScale(2 * maxconns)
			defaultLimits.SystemBaseLimit.ConnsOutbound = logScale(2 * maxconns)
			defaultLimits.SystemBaseLimit.Conns = logScale(4 * maxconns)

			defaultLimits.SystemBaseLimit.StreamsInbound = logScale(16 * maxconns)
			defaultLimits.SystemBaseLimit.StreamsOutbound = logScale(64 * maxconns)
			defaultLimits.SystemBaseLimit.Streams = logScale(64 * maxconns)

			if 2*maxconns > defaultLimits.SystemBaseLimit.FD {
				defaultLimits.SystemBaseLimit.FD = logScale(2 * maxconns)
			}

			defaultLimits.ServiceBaseLimit.StreamsInbound = logScale(8 * maxconns)
			defaultLimits.ServiceBaseLimit.StreamsOutbound = logScale(32 * maxconns)
			defaultLimits.ServiceBaseLimit.Streams = logScale(32 * maxconns)

			defaultLimits.ProtocolBaseLimit.StreamsInbound = logScale(8 * maxconns)
			defaultLimits.ProtocolBaseLimit.StreamsOutbound = logScale(32 * maxconns)
			defaultLimits.ProtocolBaseLimit.Streams = logScale(32 * maxconns)

			log.Info("adjusted default resource manager limits")
		}

		// initialize
		var limiter *rcmgr.BasicLimiter
		var opts []rcmgr.Option

		repoPath := repo.Path()

		// create limiter -- parse $repo/limits.json if exists
		limitsFile := filepath.Join(repoPath, "limits.json")
		limitsIn, err := os.Open(limitsFile)
		switch {
		case err == nil:
			defer limitsIn.Close() //nolint:errcheck
			limiter, err = rcmgr.NewLimiterFromJSON(limitsIn, defaultLimits)
			if err != nil {
				return nil, fmt.Errorf("error parsing limit file: %w", err)
			}

		case errors.Is(err, os.ErrNotExist):
			limiter = rcmgr.NewStaticLimiter(defaultLimits)

		default:
			return nil, err
		}

		// TODO: also set appropriate default limits for lotus protocols
		libp2p.SetDefaultServiceLimits(limiter)

		opts = append(opts, rcmgr.WithMetrics(rcmgrMetrics{}))

		if os.Getenv("LOTUS_DEBUG_RCMGR") != "" {
			debugPath := filepath.Join(repoPath, "debug")
			if err := os.MkdirAll(debugPath, 0755); err != nil {
				return nil, fmt.Errorf("error creating debug directory: %w", err)
			}
			traceFile := filepath.Join(debugPath, "rcmgr.json.gz")
			opts = append(opts, rcmgr.WithTrace(traceFile))
		}

		mgr, err := rcmgr.NewResourceManager(limiter, opts...)
		if err != nil {
			return nil, fmt.Errorf("error creating resource manager: %w", err)
		}

		lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				return mgr.Close()
			}})

		return mgr, nil
	}
}

func logScale(val int) int {
	bitlen := bits.Len(uint(val))
	return 1 << bitlen
}

func ResourceManagerOption(mgr network.ResourceManager) Libp2pOpts {
	return Libp2pOpts{
		Opts: []libp2p.Option{libp2p.ResourceManager(mgr)},
	}
}

type rcmgrMetrics struct{}

func (r rcmgrMetrics) AllowConn(dir network.Direction, usefd bool) {
	ctx := context.Background()
	if dir == network.DirInbound {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.Direction, "inbound"))
	} else {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.Direction, "outbound"))
	}
	if usefd {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.UseFD, "true"))
	} else {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.UseFD, "false"))
	}
	stats.Record(ctx, metrics.RcmgrAllowConn.M(1))
}

func (r rcmgrMetrics) BlockConn(dir network.Direction, usefd bool) {
	ctx := context.Background()
	if dir == network.DirInbound {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.Direction, "inbound"))
	} else {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.Direction, "outbound"))
	}
	if usefd {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.UseFD, "true"))
	} else {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.UseFD, "false"))
	}
	stats.Record(ctx, metrics.RcmgrBlockConn.M(1))
}

func (r rcmgrMetrics) AllowStream(p peer.ID, dir network.Direction) {
	ctx := context.Background()
	if dir == network.DirInbound {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.Direction, "inbound"))
	} else {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.Direction, "outbound"))
	}
	stats.Record(ctx, metrics.RcmgrAllowStream.M(1))
}

func (r rcmgrMetrics) BlockStream(p peer.ID, dir network.Direction) {
	ctx := context.Background()
	if dir == network.DirInbound {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.Direction, "inbound"))
	} else {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.Direction, "outbound"))
	}
	stats.Record(ctx, metrics.RcmgrBlockStream.M(1))
}

func (r rcmgrMetrics) AllowPeer(p peer.ID) {
	ctx := context.Background()
	stats.Record(ctx, metrics.RcmgrAllowPeer.M(1))
}

func (r rcmgrMetrics) BlockPeer(p peer.ID) {
	ctx := context.Background()
	stats.Record(ctx, metrics.RcmgrBlockPeer.M(1))
}

func (r rcmgrMetrics) AllowProtocol(proto protocol.ID) {
	ctx := context.Background()
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.ProtocolID, string(proto)))
	stats.Record(ctx, metrics.RcmgrAllowProto.M(1))
}

func (r rcmgrMetrics) BlockProtocol(proto protocol.ID) {
	ctx := context.Background()
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.ProtocolID, string(proto)))
	stats.Record(ctx, metrics.RcmgrBlockProto.M(1))
}

func (r rcmgrMetrics) BlockProtocolPeer(proto protocol.ID, p peer.ID) {
	ctx := context.Background()
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.ProtocolID, string(proto)))
	stats.Record(ctx, metrics.RcmgrBlockProtoPeer.M(1))
}

func (r rcmgrMetrics) AllowService(svc string) {
	ctx := context.Background()
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.ServiceID, svc))
	stats.Record(ctx, metrics.RcmgrAllowSvc.M(1))
}

func (r rcmgrMetrics) BlockService(svc string) {
	ctx := context.Background()
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.ServiceID, svc))
	stats.Record(ctx, metrics.RcmgrBlockSvc.M(1))
}

func (r rcmgrMetrics) BlockServicePeer(svc string, p peer.ID) {
	ctx := context.Background()
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.ServiceID, svc))
	stats.Record(ctx, metrics.RcmgrBlockSvcPeer.M(1))
}

func (r rcmgrMetrics) AllowMemory(size int) {
	stats.Record(context.Background(), metrics.RcmgrAllowMem.M(1))
}

func (r rcmgrMetrics) BlockMemory(size int) {
	stats.Record(context.Background(), metrics.RcmgrBlockMem.M(1))
}
