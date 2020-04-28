package crand

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"sync"
	"time"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/crand/pb"
	"github.com/filecoin-project/specs-actors/actors/abi"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
)

var log = logging.Logger("crand")

var crandServers = []string{
	"localhost:17000",
}

var crandPubKey ffi.PublicKey

func init() {
	_, err := hex.Decode(crandPubKey[:], []byte("98066719FE8C859863FCE1F65AA08D5D96962591C5D5DC36D58D6337562994E16C0C7FE1CC8BF58488C4C315D4A9F0AF"))
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

type CrandBeacon struct {
	client pb.CrandClient

	// seconds
	interval time.Duration

	drandGenTime uint64
	filGenTime   uint64
	filRoundTime uint64

	cacheLk    sync.Mutex
	localCache map[uint64]types.BeaconEntry
}

func NewCrandBeacon(genesisTs uint64, interval uint64) (*CrandBeacon, error) {
	if genesisTs == 0 {
		panic("what are you doing this cant be zero")
	}

	conn, err := grpc.Dial(crandServers[0], grpc.WithInsecure())
	if err != nil {
		return nil, xerrors.Errorf("dialing crand: %w", err)
	}

	db := &CrandBeacon{
		client:     pb.NewCrandClient(conn),
		localCache: make(map[uint64]types.BeaconEntry),
	}

	infoResp, err := db.client.GetInfo(context.TODO(), &pb.InfoRequest{})
	if err != nil {
		return nil, xerrors.Errorf("failed to get info response from beacon peer: %w", err)
	}

	// TODO: verify these values are what we expect them to be
	if !bytes.Equal(infoResp.GetPubkey(), crandPubKey[:]) {
		return nil, xerrors.Errorf("public key does not match")
	}
	// fmt.Printf("Crand Pubkey:\n%#v\n", kgroup.PublicKey.TOML()) // use to print public key
	db.interval = time.Duration(infoResp.GetRound())
	db.drandGenTime = uint64(infoResp.GetGenesisTs())
	db.filRoundTime = interval
	db.filGenTime = genesisTs

	// TODO: the stream currently gives you back *all* values since drand genesis.
	// Having the stream in the background is merely an optimization, so not a big deal to disable it for now
	//go db.handleStreamingUpdates()

	return db, nil
}

func (db *CrandBeacon) Entry(ctx context.Context, round uint64) <-chan beacon.Response {
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
		resp, err := db.client.GetRandomness(ctx, &pb.RandomnessRequest{Round: round})

		var br beacon.Response
		if err != nil {
			br.Err = err
		} else {
			br.Entry.Round = round
			br.Entry.Data = resp.GetRandomness()
		}

		out <- br
		close(out)
	}()

	return out
}

func (db *CrandBeacon) cacheValue(e types.BeaconEntry) {
	db.cacheLk.Lock()
	defer db.cacheLk.Unlock()
	db.localCache[e.Round] = e
}

func (db *CrandBeacon) getCachedValue(round uint64) *types.BeaconEntry {
	db.cacheLk.Lock()
	defer db.cacheLk.Unlock()
	v, ok := db.localCache[round]
	if !ok {
		return nil
	}
	return &v
}

func (db *CrandBeacon) VerifyEntry(curr types.BeaconEntry, prev types.BeaconEntry) error {
	if prev.Round == 0 {
		// TODO handle genesis better
		return nil
	}

	if curr.Round-1 != prev.Round {
		return xerrors.Errorf("inconsistent beacons")
	}

	var sig ffi.Signature
	copy(sig[:], curr.Data)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, curr.Round)
	res := ffi.Verify(&sig, []ffi.Digest{ffi.Hash(ffi.Message(buf))}, []ffi.PublicKey{crandPubKey})
	if !res {
		return xerrors.Errorf("signature failed to verify")
	}
	db.cacheValue(curr)

	return nil
}

func (db *CrandBeacon) MaxBeaconRoundForEpoch(filEpoch abi.ChainEpoch, prevEntry types.BeaconEntry) uint64 {
	// TODO: sometimes the genesis time for filecoin is zero and this goes negative
	latestTs := ((uint64(filEpoch) * db.filRoundTime) + db.filGenTime) - db.filRoundTime
	dround := (latestTs - db.drandGenTime) / uint64(db.interval/time.Second)
	return dround
}

var _ beacon.RandomBeacon = (*CrandBeacon)(nil)
