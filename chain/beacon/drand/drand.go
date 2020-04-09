package drand

import (
	"context"
	"sync"

	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"

	dnet "github.com/drand/drand/net"
	dproto "github.com/drand/drand/protobuf/drand"
)

type DrandBeacon struct {
	client dnet.Client

	cacheLk    sync.Mutex
	localCache map[int64]types.BeaconEntry
}

func NewDrandBeacon() *DrandBeacon {
	return &DrandBeacon{
		client:     dnet.NewGrpcClient(),
		localCache: make(map[int64]types.BeaconEntry),
	}
}

//func (db *DrandBeacon)

func (db *DrandBeacon) Entry(ctx context.Context, round uint64) <-chan beacon.Response {
	// check cache, it it if there, otherwise query the endpoint
	resp, err := db.client.PublicRand(nil, &dproto.PublicRandRequest{Round: round})
	_, _ = resp, err

	return nil
}

func (db *DrandBeacon) VerifyEntry(types.BeaconEntry, types.BeaconEntry) error {
	return nil
}

func (db *DrandBeacon) MaxBeaconRoundForEpoch(abi.ChainEpoch, types.BeaconEntry) uint64 {
	// this is just some local math
	return 0
}

var _ beacon.RandomBeacon = (*DrandBeacon)(nil)
