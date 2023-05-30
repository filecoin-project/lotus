package drand

import (
	"bytes"
	"context"
	"time"

	dchain "github.com/drand/drand/chain"
	dclient "github.com/drand/drand/client"
	hclient "github.com/drand/drand/client/http"
	"github.com/drand/drand/common/scheme"
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
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var log = logging.Logger("drand")

type drandPeer struct {
	addr string
	tls  bool
}

func (dp *drandPeer) Address() string {
	return dp.addr
}

func (dp *drandPeer) IsTLS() bool {
	return dp.tls
}

// DrandBeacon connects Lotus with a drand network in order to provide
// randomness to the system in a way that's aligned with Filecoin rounds/epochs.
//
// We connect to drand peers via their public HTTP endpoints. The peers are
// enumerated in the drandServers variable.
//
// The root trust for the Drand chain is configured from build.DrandChain.
type DrandBeacon struct {
	client dclient.Client

	pubkey kyber.Point

	// seconds
	interval time.Duration

	drandGenTime uint64
	filGenTime   uint64
	filRoundTime uint64

	localCache *lru.Cache[uint64, *types.BeaconEntry]
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
		hc.(DrandHTTPClient).SetUserAgent("drand-client-lotus/" + build.BuildVersion)
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
		client:     client,
		localCache: lc,
	}

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

func (db *DrandBeacon) VerifyEntry(curr types.BeaconEntry, prev types.BeaconEntry) error {
	if prev.Round == 0 {
		// TODO handle genesis better
		return nil
	}

	if curr.Round != prev.Round+1 {
		return xerrors.Errorf("invalid beacon entry: cur (%d) != prev (%d) + 1", curr.Round, prev.Round)
	}

	if be := db.getCachedValue(curr.Round); be != nil {
		if !bytes.Equal(curr.Data, be.Data) {
			return xerrors.New("invalid beacon value, does not match cached good value")
		}
		// return no error if the value is in the cache already
		return nil
	}
	b := &dchain.Beacon{
		PreviousSig: prev.Data,
		Round:       curr.Round,
		Signature:   curr.Data,
	}
	err := dchain.NewVerifier(scheme.GetSchemeFromEnv()).VerifyBeacon(*b, db.pubkey)
	if err == nil {
		db.cacheValue(curr)
	}
	return err
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
