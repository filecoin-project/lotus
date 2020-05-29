package drand

import (
	"bytes"
	"context"
	"sync"
	"time"

	dchain "github.com/drand/drand/chain"
	dclient "github.com/drand/drand/client"
	gclient "github.com/drand/drand/cmd/relay-gossip/client"
	dlog "github.com/drand/drand/log"
	"github.com/drand/kyber"
	kzap "github.com/go-kit/kit/log/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/xerrors"

	logging "github.com/ipfs/go-log"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

var log = logging.Logger("drand")

var drandServers = []string{
	"https://pl-eu.testnet.drand.sh",
	"https://pl-us.testnet.drand.sh",
	"https://pl-sin.testnet.drand.sh",
}

var drandChain *dchain.Info

func init() {

	var err error
	drandChain, err = dchain.InfoFromJSON(bytes.NewReader([]byte(build.DrandChain)))
	if err != nil {
		panic("could not unmarshal chain info: " + err.Error())
	}
}

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

type DrandBeacon struct {
	client dclient.Client

	pubkey kyber.Point

	// seconds
	interval time.Duration

	drandGenTime uint64
	filGenTime   uint64
	filRoundTime uint64

	cacheLk    sync.Mutex
	localCache map[uint64]types.BeaconEntry
}

func NewDrandBeacon(genesisTs, interval uint64, ps *pubsub.PubSub) (*DrandBeacon, error) {
	if genesisTs == 0 {
		panic("what are you doing this cant be zero")
	}

	dlogger := dlog.NewKitLoggerFrom(kzap.NewZapSugarLogger(
		log.SugaredLogger.Desugar(), zapcore.InfoLevel))
	opts := []dclient.Option{
		dclient.WithChainInfo(drandChain),
		dclient.WithHTTPEndpoints(drandServers),
		dclient.WithCacheSize(1024),
		dclient.WithLogger(dlogger),
	}

	if ps != nil {
		opts = append(opts, gclient.WithPubsub(ps))
	} else {
		log.Info("drand beacon without pubsub")
	}

	client, err := dclient.New(opts...)
	if err != nil {
		return nil, xerrors.Errorf("creating drand client")
	}

	go func() {
		// Explicitly Watch until that is fixed in drand
		ch := client.Watch(context.Background())
		for range ch {
		}
		log.Error("dranch Watch bork")
	}()

	db := &DrandBeacon{
		client:     client,
		localCache: make(map[uint64]types.BeaconEntry),
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
		start := time.Now()
		log.Infow("start fetching randomness", "round", round)
		resp, err := db.client.Get(ctx, round)

		var br beacon.Response
		if err != nil {
			br.Err = xerrors.Errorf("drand failed Get request: %w", err)
		} else {
			br.Entry.Round = resp.Round()
			br.Entry.Data = resp.Signature()
		}
		log.Infow("done fetching randomness", "round", round, "took", time.Since(start))
		out <- br
		close(out)
	}()

	return out
}
func (db *DrandBeacon) cacheValue(e types.BeaconEntry) {
	db.cacheLk.Lock()
	defer db.cacheLk.Unlock()
	db.localCache[e.Round] = e
}

func (db *DrandBeacon) getCachedValue(round uint64) *types.BeaconEntry {
	db.cacheLk.Lock()
	defer db.cacheLk.Unlock()
	v, ok := db.localCache[round]
	if !ok {
		return nil
	}
	return &v
}

func (db *DrandBeacon) VerifyEntry(curr types.BeaconEntry, prev types.BeaconEntry) error {
	if prev.Round == 0 {
		// TODO handle genesis better
		return nil
	}
	b := &dchain.Beacon{
		PreviousSig: prev.Data,
		Round:       curr.Round,
		Signature:   curr.Data,
	}
	err := dchain.VerifyBeacon(db.pubkey, b)
	if err == nil {
		db.cacheValue(curr)
	}
	return err
}

func (db *DrandBeacon) MaxBeaconRoundForEpoch(filEpoch abi.ChainEpoch, prevEntry types.BeaconEntry) uint64 {
	// TODO: sometimes the genesis time for filecoin is zero and this goes negative
	latestTs := ((uint64(filEpoch) * db.filRoundTime) + db.filGenTime) - db.filRoundTime
	dround := (latestTs - db.drandGenTime) / uint64(db.interval.Seconds())
	return dround
}

var _ beacon.RandomBeacon = (*DrandBeacon)(nil)
