package drand

import (
	"bytes"
	"context"
	"time"

	dchain "github.com/drand/drand/chain"
	dclient "github.com/drand/drand/client"
	hclient "github.com/drand/drand/client/http"
	dcrypto "github.com/drand/drand/crypto"
	dlog "github.com/drand/drand/log"
	gclient "github.com/drand/drand/lp2p/client"
	"github.com/drand/kyber"
	lru "github.com/hashicorp/golang-lru/v2"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var log = logging.Logger("drand")

// DrandBeacon connects Lotus with a drand network in order to provide
// randomness to the system in a way that's aligned with Filecoin rounds/epochs.
//
// We connect to drand peers via their public HTTP endpoints. The peers are
// enumerated in the drandServers variable.
//
// The root trust for the Drand chain is configured from buildconstants.DrandConfigs
type DrandBeacon struct {
	isChained bool
	client    dclient.Client

	pubkey kyber.Point

	// seconds
	interval time.Duration

	drandGenTime uint64
	filGenTime   uint64
	filRoundTime uint64
	scheme       *dcrypto.Scheme

	localCache *lru.Cache[uint64, *types.BeaconEntry]
}

// IsChained tells us whether this particular beacon operates in "chained mode". Prior to Drand
// quicknet, beacons form a chain. After the introduction of quicknet, they do not, so we need to
// change how we interact with beacon entries. (See FIP-0063)
func (db *DrandBeacon) IsChained() bool {
	return db.isChained
}

// DrandHTTPClient interface overrides the user agent used by drand
type DrandHTTPClient interface {
	SetUserAgent(string)
}

type logger struct {
	*zap.SugaredLogger
}

func (l *logger) With(args ...interface{}) dlog.Logger {
	return &logger{l.SugaredLogger.With(args...)}
}

func (l *logger) Named(s string) dlog.Logger {
	return &logger{l.SugaredLogger.Named(s)}
}

func (l *logger) AddCallerSkip(skip int) dlog.Logger {
	return &logger{l.SugaredLogger.With(zap.AddCallerSkip(skip))}
}

func NewDrandBeacon(genesisTs, interval uint64, ps *pubsub.PubSub, config dtypes.DrandConfig) (*DrandBeacon, error) {
	if genesisTs == 0 {
		panic("what are you doing this cant be zero")
	}

	drandChain, err := dchain.InfoFromJSON(bytes.NewReader([]byte(config.ChainInfoJSON)))
	if err != nil {
		return nil, xerrors.Errorf("unable to unmarshal drand chain info: %w", err)
	}

	var clients []dclient.Client
	for _, url := range config.Servers {
		hc, err := hclient.NewWithInfo(url, drandChain, nil)
		if err != nil {
			return nil, xerrors.Errorf("could not create http drand client: %w", err)
		}
		hc.(DrandHTTPClient).SetUserAgent("drand-client-lotus/" + build.NodeBuildVersion)
		clients = append(clients, hc)
	}

	opts := []dclient.Option{
		dclient.WithChainInfo(drandChain),
		dclient.WithCacheSize(1024),
		dclient.WithLogger(&logger{&log.SugaredLogger}),
	}

	if ps != nil {
		opts = append(opts, gclient.WithPubsub(ps))
	} else {
		log.Info("drand beacon without pubsub")
		if len(clients) == 0 {
			// This hack is necessary to convince a drand beacon to start without any clients. For
			// historical becaons we need them to be able to verify old entries but we don't need to fetch
			// new ones. With pubsub enabled, it acts as a client so drand is happy, but if we don't have
			// pubsub then drand will complain about old beacons withotu clients. So we make one that
			// it'll think is a valid client and that it won't speed test (hence the need to mark it as
			// as "watcher").
			historicalClient := &historicalBeaconClient{}
			opts = append(opts, dclient.WithWatcher(func(chainInfo *dchain.Info, cache dclient.Cache) (dclient.Watcher, error) {
				return historicalClient, nil
			}))
		}
	}

	client, err := dclient.Wrap(clients, opts...)
	if err != nil {
		return nil, xerrors.Errorf("creating drand client: %w", err)
	}

	lc, err := lru.New[uint64, *types.BeaconEntry](1024)
	if err != nil {
		return nil, err
	}

	db := &DrandBeacon{
		isChained:  config.IsChained,
		client:     client,
		localCache: lc,
	}

	sch, err := dcrypto.GetSchemeByIDWithDefault(drandChain.Scheme)
	if err != nil {
		return nil, err
	}
	db.scheme = sch
	db.pubkey = drandChain.PublicKey
	db.interval = drandChain.Period
	db.drandGenTime = uint64(drandChain.GenesisTime)
	db.filRoundTime = interval
	db.filGenTime = genesisTs

	return db, nil
}

func (db *DrandBeacon) Entry(ctx context.Context, round uint64) <-chan beacon.Response {
	out := make(chan beacon.Response, 1)
	if round != 0 {
		be := db.getCachedValue(round)
		if be != nil {
			out <- beacon.Response{Entry: *be}
			close(out)
			return out
		}
	}

	go func() {
		start := build.Clock.Now()
		log.Debugw("start fetching randomness", "round", round)
		resp, err := db.client.Get(ctx, round)

		var br beacon.Response
		if err != nil {
			br.Err = xerrors.Errorf("drand failed Get request: %w", err)
		} else {
			br.Entry.Round = resp.Round()
			br.Entry.Data = resp.Signature()
		}
		log.Debugw("done fetching randomness", "round", round, "took", build.Clock.Since(start))
		out <- br
		close(out)
	}()

	return out
}
func (db *DrandBeacon) cacheValue(e types.BeaconEntry) {
	db.localCache.Add(e.Round, &e)
}

func (db *DrandBeacon) getCachedValue(round uint64) *types.BeaconEntry {
	v, _ := db.localCache.Get(round)
	return v
}

func (db *DrandBeacon) VerifyEntry(entry types.BeaconEntry, prevEntrySig []byte) error {
	if be := db.getCachedValue(entry.Round); be != nil {
		if !bytes.Equal(entry.Data, be.Data) {
			return xerrors.New("invalid beacon value, does not match cached good value")
		}
		// return no error if the value is in the cache already
		return nil
	}
	b := &dchain.Beacon{
		PreviousSig: prevEntrySig,
		Round:       entry.Round,
		Signature:   entry.Data,
	}

	err := db.scheme.VerifyBeacon(b, db.pubkey)
	if err != nil {
		return xerrors.Errorf("failed to verify beacon: %w", err)
	}

	db.cacheValue(entry)
	return nil
}

func (db *DrandBeacon) MaxBeaconRoundForEpoch(nv network.Version, filEpoch abi.ChainEpoch) uint64 {
	// TODO: sometimes the genesis time for filecoin is zero and this goes negative
	latestTs := ((uint64(filEpoch) * db.filRoundTime) + db.filGenTime) - db.filRoundTime

	if nv <= network.Version15 {
		return db.maxBeaconRoundV1(latestTs)
	}

	return db.maxBeaconRoundV2(latestTs)
}

func (db *DrandBeacon) maxBeaconRoundV1(latestTs uint64) uint64 {
	dround := (latestTs - db.drandGenTime) / uint64(db.interval.Seconds())
	return dround
}

func (db *DrandBeacon) maxBeaconRoundV2(latestTs uint64) uint64 {
	if latestTs < db.drandGenTime {
		return 1
	}

	fromGenesis := latestTs - db.drandGenTime
	// we take the time from genesis divided by the periods in seconds, that
	// gives us the number of periods since genesis.  We also add +1 because
	// round 1 starts at genesis time.
	return fromGenesis/uint64(db.interval.Seconds()) + 1
}

var _ beacon.RandomBeacon = (*DrandBeacon)(nil)

func BeaconScheduleFromDrandSchedule(dcs dtypes.DrandSchedule, genesisTime uint64, ps *pubsub.PubSub) (beacon.Schedule, error) {
	shd := beacon.Schedule{}
	for i, dc := range dcs {
		bc, err := NewDrandBeacon(genesisTime, buildconstants.BlockDelaySecs, ps, dc.Config)
		if err != nil {
			return nil, xerrors.Errorf("%d creating drand beacon: %w", i, err)
		}
		shd = append(shd, beacon.BeaconPoint{Start: dc.Start, Beacon: bc})
	}

	return shd, nil
}

var _ dclient.Client = historicalBeaconClient{}

// historicalBeaconClient is a drand client that doesn't actually do anything. It's used when
// we don't have a drand network to connect to but still need to provide a beacon client.
// We don't expect calls through to the client to be made since we should only be verifying old
// randomness, not fetching it.
type historicalBeaconClient struct{}

func (h historicalBeaconClient) Get(ctx context.Context, round uint64) (dclient.Result, error) {
	return nil, xerrors.Errorf("no historical randomness available")
}

func (h historicalBeaconClient) Watch(ctx context.Context) <-chan dclient.Result {
	return nil
}

func (h historicalBeaconClient) Info(ctx context.Context) (*dchain.Info, error) {
	return nil, xerrors.Errorf("no historical randomness available")
}

func (h historicalBeaconClient) RoundAt(time.Time) uint64 {
	return 0
}

func (h historicalBeaconClient) Close() error {
	return nil
}
