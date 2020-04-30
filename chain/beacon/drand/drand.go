package drand

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"

	logging "github.com/ipfs/go-log"

	dbeacon "github.com/drand/drand/beacon"
	"github.com/drand/drand/core"
	dkey "github.com/drand/drand/key"
	dnet "github.com/drand/drand/net"
	dproto "github.com/drand/drand/protobuf/drand"
)

var log = logging.Logger("drand")

var drandServers = []string{
	"nicolas.drand.fil-test.net:443",
	"philipp.drand.fil-test.net:443",
	"mathilde.drand.fil-test.net:443",
	"ludovic.drand.fil-test.net:443",
	"gabbi.drand.fil-test.net:443",
	"linus.drand.fil-test.net:443",
	"jeff.drand.fil-test.net:443",
}

var drandPubKey *dkey.DistPublic

func init() {
	drandPubKey = new(dkey.DistPublic)
	err := drandPubKey.FromTOML(&dkey.DistPublicTOML{Coefficients: build.DrandCoeffs})
	if err != nil {
		panic(err)
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
	client dnet.Client

	peers         []dnet.Peer
	peersIndex    int
	peersIndexMtx sync.Mutex

	pubkey *dkey.DistPublic

	// seconds
	interval time.Duration

	drandGenTime uint64
	filGenTime   uint64
	filRoundTime uint64

	cacheLk    sync.Mutex
	localCache map[uint64]types.BeaconEntry
}

func NewDrandBeacon(genesisTs, interval uint64) (*DrandBeacon, error) {
	if genesisTs == 0 {
		panic("what are you doing this cant be zero")
	}
	db := &DrandBeacon{
		client:     dnet.NewGrpcClient(),
		localCache: make(map[uint64]types.BeaconEntry),
	}
	for _, ds := range drandServers {
		db.peers = append(db.peers, &drandPeer{addr: ds, tls: true})
	}

	db.peersIndex = rand.Intn(len(db.peers))

	groupResp, err := db.client.Group(context.TODO(), db.peers[db.peersIndex], &dproto.GroupRequest{})
	if err != nil {
		return nil, xerrors.Errorf("failed to get group response from beacon peer: %w", err)
	}

	kgroup, err := core.ProtoToGroup(groupResp)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse group response: %w", err)
	}

	// TODO: verify these values are what we expect them to be
	if !kgroup.PublicKey.Equal(drandPubKey) {
		return nil, xerrors.Errorf("public key does not match")
	}
	// fmt.Printf("Drand Pubkey:\n%#v\n", kgroup.PublicKey.TOML()) // use to print public key
	db.pubkey = drandPubKey
	db.interval = kgroup.Period
	db.drandGenTime = uint64(kgroup.GenesisTime)
	db.filRoundTime = interval
	db.filGenTime = genesisTs

	// TODO: the stream currently gives you back *all* values since drand genesis.
	// Having the stream in the background is merely an optimization, so not a big deal to disable it for now
	// go db.handleStreamingUpdates()

	return db, nil
}

func (db *DrandBeacon) rotatePeersIndex() {
	db.peersIndexMtx.Lock()
	defer db.peersIndexMtx.Unlock()
	db.peersIndex = (db.peersIndex + 1) % len(db.peers)
}

func (db *DrandBeacon) handleStreamingUpdates() {
	for {
		ch, err := db.client.PublicRandStream(context.Background(), db.peers[db.peersIndex], &dproto.PublicRandRequest{})
		if err != nil {
			log.Warnf("failed to get public rand stream: %s", err)
			log.Warnf("trying again in 10 seconds")
			db.rotatePeersIndex()
			time.Sleep(time.Second * 10)
			continue
		}

		for e := range ch {
			db.cacheValue(types.BeaconEntry{
				Round: e.Round,
				Data:  e.Signature,
			})
		}

		log.Warn("drand beacon stream broke, reconnecting in 10 seconds")
		db.rotatePeersIndex()
		time.Sleep(time.Second * 10)
	}
}

func (db *DrandBeacon) Entry(ctx context.Context, round uint64) <-chan beacon.Response {
	// check cache, it it if there, otherwise query the endpoint
	cres := db.getCachedValue(round)
	if cres != nil {
		out := make(chan beacon.Response, 1)
		out <- beacon.Response{Entry: *cres}
		close(out)
		return out
	}

	out := make(chan beacon.Response, 1)

	go func() {
		resp, err := db.client.PublicRand(ctx, db.peers[db.peersIndex], &dproto.PublicRandRequest{Round: round})

		var br beacon.Response
		if err != nil {
			db.rotatePeersIndex()
			br.Err = err
		} else {
			br.Entry.Round = resp.GetRound()
			br.Entry.Data = resp.GetSignature()
		}

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
	b := &dbeacon.Beacon{
		PreviousSig: prev.Data,
		Round:       curr.Round,
		Signature:   curr.Data,
	}
	//log.Warnw("VerifyEntry", "beacon", b)
	err := dbeacon.VerifyBeacon(db.pubkey.Key(), b)
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
