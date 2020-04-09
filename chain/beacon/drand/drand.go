package drand

import (
	"context"

	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"

	dnet "github.com/drand/drand/net"
	dproto "github.com/drand/drand/protobuf/drand"
)

type DrandBeacon struct {
	client dnet.Client

	cacheLk sync.Mutex
	localCache map[int64]types.BeaconEntry
}

func NewDrandBeacon() *DrandBeacon {
	return &DrandBeacon{dnet.NewGrpcClient()}
}

func (db *DrandBeacon) 

func (db *DrandBeacon) Entry(ctx context.Context, round uint64) <-chan beacon.Response {
	// check cache, it it if there, otherwise query the endpoint
	resp, err := db.client.PublicRand(ctx, &dproto.PublicRandRequest{round})

}

func (db *DrandBeacon) VerifyEntry(types.BeaconEntry, types.BeaconEntry) error {
}

func (db *DrandBeacon) MaxBeaconRoundForEpoch(abi.ChainEpoch, types.BeaconEntry) uint64 {
	// this is just some local math
}

var _ beacon.RandomBeacon = (*DrandBeacon)(nil)
