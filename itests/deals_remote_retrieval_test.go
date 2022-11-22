package itests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/ipld/go-car"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestNetStoreRetrieval(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()

	full, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blocktime)

	time.Sleep(5 * time.Second)

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	dealStartEpoch := abi.ChainEpoch(2 << 12)

	rseed := 7

	dh := kit.NewDealHarness(t, full, miner, miner)
	dealCid, res, _ := dh.MakeOnlineDeal(context.Background(), kit.MakeFullDealParams{
		Rseed:                    rseed,
		StartEpoch:               dealStartEpoch,
		UseCARFileForStorageDeal: true,
	})

	// create deal store
	id := uuid.New()
	rstore := bstore.NewMemorySync()

	au, err := url.Parse(full.ListenURL)
	require.NoError(t, err)

	switch au.Scheme {
	case "http":
		au.Scheme = "ws"
	case "https":
		au.Scheme = "wss"
	}

	au.Path = path.Join(au.Path, "/rest/v0/store/"+id.String())

	conn, _, err := websocket.DefaultDialer.Dial(au.String(), nil)
	require.NoError(t, err)

	_ = bstore.HandleNetBstoreWS(ctx, rstore, conn)

	dh.PerformRetrievalWithOrder(ctx, dealCid, res.Root, false, func(offer api.QueryOffer, address address.Address) api.RetrievalOrder {
		order := offer.Order(address)

		order.RemoteStore = &id

		return order
	})

	// check blockstore blocks
	carv1FilePath, _ := kit.CreateRandomCARv1(t, rseed, 200)
	cb, err := os.ReadFile(carv1FilePath)
	require.NoError(t, err)

	cr, err := car.NewCarReader(bytes.NewReader(cb))
	require.NoError(t, err)

	var blocks int
	for {
		cb, err := cr.Next()
		if err == io.EOF {
			fmt.Println("blocks: ", blocks)
			return
		}
		require.NoError(t, err)

		sb, err := rstore.Get(ctx, cb.Cid())
		require.NoError(t, err)
		require.EqualValues(t, cb.RawData(), sb.RawData())

		blocks++
	}
}
