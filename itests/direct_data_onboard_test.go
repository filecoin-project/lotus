package itests

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-commp-utils/nonffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	market2 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	minertypes "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
)

func TestActors13Migration(t *testing.T) {

	var (
		blocktime = 2 * time.Millisecond
		ctx       = context.Background()
	)
	client, _, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.UpgradeSchedule(stmgr.Upgrade{
		Network: network.Version21,
		Height:  -1,
	}, stmgr.Upgrade{
		Network:   network.Version22,
		Height:    10,
		Migration: filcns.UpgradeActorsV13,
	}))
	ens.InterconnectAll().BeginMiningMustPost(blocktime)

	// mine until 15
	client.WaitTillChain(ctx, kit.HeightAtLeast(15))
}

func TestOnboardRawPiece(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		ctx       = context.Background()
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC())
	ens.InterconnectAll().BeginMiningMustPost(blocktime)

	pieceSize := abi.PaddedPieceSize(2048).Unpadded()
	pieceData := make([]byte, pieceSize)
	_, _ = rand.Read(pieceData)

	dc, err := miner.ComputeDataCid(ctx, pieceSize, bytes.NewReader(pieceData))
	require.NoError(t, err)

	head, err := client.ChainHead(ctx)
	require.NoError(t, err)

	so, err := miner.SectorAddPieceToAny(ctx, pieceSize, bytes.NewReader(pieceData), piece.PieceDealInfo{
		PublishCid:   nil,
		DealID:       0,
		DealProposal: nil,
		DealSchedule: piece.DealSchedule{
			StartEpoch: head.Height() + 2880*2,
			EndEpoch:   head.Height() + 2880*400,
		},
		KeepUnsealed: true,
		PieceActivationManifest: &minertypes.PieceActivationManifest{
			CID:                   dc.PieceCID,
			Size:                  dc.Size,
			VerifiedAllocationKey: nil,
			Notify:                nil,
		},
	})
	require.NoError(t, err)

	// wait for sector to commit

	// wait for sector to commit and enter proving state
	toCheck := map[abi.SectorNumber]struct{}{
		so.Sector: {},
	}

	miner.WaitSectorsProving(ctx, toCheck)

	si, err := miner.SectorsStatus(ctx, so.Sector, false)
	require.NoError(t, err)
	require.Equal(t, dc.PieceCID, *si.CommD)
}

func TestOnboardMixedMarketDDO(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		ctx       = context.Background()
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.MutateSealingConfig(func(sc *config.SealingConfig) {
		sc.RequireActivationSuccess = true
		sc.RequireNotificationSuccess = true
	}))
	ens.InterconnectAll().BeginMiningMustPost(blocktime)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	mi, err := client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	var pieces []abi.PieceInfo
	var dealID abi.DealID

	{
		// market piece
		pieceSize := abi.PaddedPieceSize(2048 / 2).Unpadded()
		pieceData := make([]byte, pieceSize)
		_, _ = rand.Read(pieceData)

		dc, err := miner.ComputeDataCid(ctx, pieceSize, bytes.NewReader(pieceData))
		require.NoError(t, err)
		pieces = append(pieces, dc)

		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		// PSD

		psdParams := market2.PublishStorageDealsParams{
			Deals: []market2.ClientDealProposal{
				makeMarketDealProposal(t, client, miner, dc.PieceCID, pieceSize.Padded(), head.Height()+2880*2, head.Height()+2880*400),
			},
		}

		psdMsg := &types.Message{
			To:   market.Address,
			From: mi.Worker,

			Method: market.Methods.PublishStorageDeals,
			Params: must.One(cborutil.Dump(&psdParams)),
		}

		smsg, err := client.MpoolPushMessage(ctx, psdMsg, nil)
		require.NoError(t, err)

		r, err := client.StateWaitMsg(ctx, smsg.Cid(), 1, stmgr.LookbackNoLimit, true)
		require.NoError(t, err)

		require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)

		nv, err := client.StateNetworkVersion(ctx, types.EmptyTSK)
		require.NoError(t, err)

		res, err := market.DecodePublishStorageDealsReturn(r.Receipt.Return, nv)
		require.NoError(t, err)
		dealID = must.One(res.DealIDs())[0]

		mcid := smsg.Cid()

		so, err := miner.SectorAddPieceToAny(ctx, pieceSize, bytes.NewReader(pieceData), piece.PieceDealInfo{
			PublishCid:   &mcid,
			DealID:       dealID,
			DealProposal: &psdParams.Deals[0].Proposal,
			DealSchedule: piece.DealSchedule{
				StartEpoch: head.Height() + 2880*2,
				EndEpoch:   head.Height() + 2880*400,
			},
			PieceActivationManifest: nil,
			KeepUnsealed:            true,
		})
		require.NoError(t, err)

		require.Equal(t, abi.PaddedPieceSize(0), so.Offset)
		require.Equal(t, abi.SectorNumber(2), so.Sector)
	}

	{
		// raw ddo piece

		pieceSize := abi.PaddedPieceSize(2048 / 2).Unpadded()
		pieceData := make([]byte, pieceSize)
		_, _ = rand.Read(pieceData)

		dc, err := miner.ComputeDataCid(ctx, pieceSize, bytes.NewReader(pieceData))
		require.NoError(t, err)
		pieces = append(pieces, dc)

		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		so, err := miner.SectorAddPieceToAny(ctx, pieceSize, bytes.NewReader(pieceData), piece.PieceDealInfo{
			PublishCid:   nil,
			DealID:       0,
			DealProposal: nil,
			DealSchedule: piece.DealSchedule{
				StartEpoch: head.Height() + 2880*2,
				EndEpoch:   head.Height() + 2880*400,
			},
			KeepUnsealed: false,
			PieceActivationManifest: &minertypes.PieceActivationManifest{
				CID:                   dc.PieceCID,
				Size:                  dc.Size,
				VerifiedAllocationKey: nil,
				Notify:                nil,
			},
		})
		require.NoError(t, err)

		require.Equal(t, abi.PaddedPieceSize(1024), so.Offset)
		require.Equal(t, abi.SectorNumber(2), so.Sector)
	}

	toCheck := map[abi.SectorNumber]struct{}{
		2: {},
	}

	miner.WaitSectorsProving(ctx, toCheck)

	expectCommD, err := nonffi.GenerateUnsealedCID(abi.RegisteredSealProof_StackedDrg2KiBV1_1, pieces)
	require.NoError(t, err)

	si, err := miner.SectorsStatus(ctx, 2, false)
	require.NoError(t, err)
	require.Equal(t, expectCommD, *si.CommD)

	ds, err := client.StateMarketStorageDeal(ctx, dealID, types.EmptyTSK)
	require.NoError(t, err)

	require.NotEqual(t, -1, ds.State.SectorStartEpoch)

	{
		deals, err := client.StateMarketDeals(ctx, types.EmptyTSK)
		require.NoError(t, err)
		for id, deal := range deals {
			fmt.Println("Deal", id, deal.Proposal.PieceCID, deal.Proposal.PieceSize, deal.Proposal.Client, deal.Proposal.Provider)
		}

		// check actor events, verify deal-published is as expected
		minerIdAddr, err := client.StateLookupID(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		minerId, err := address.IDFromAddress(minerIdAddr)
		require.NoError(t, err)
		caddr, err := client.WalletDefaultAddress(context.Background())
		require.NoError(t, err)
		clientIdAddr, err := client.StateLookupID(ctx, caddr, types.EmptyTSK)
		require.NoError(t, err)
		clientId, err := address.IDFromAddress(clientIdAddr)
		require.NoError(t, err)

		fmt.Println("minerId", minerId, "clientId", clientId)
		for _, piece := range pieces {
			fmt.Println("piece", piece.PieceCID, piece.Size)
		}

		// check "deal-published" actor event
		var epochZero abi.ChainEpoch
		allEvents, err := miner.FullNode.GetActorEvents(ctx, &types.ActorEventFilter{
			FromHeight: &epochZero,
		})
		require.NoError(t, err)
		for _, key := range []string{"deal-published", "deal-activated", "sector-precommitted", "sector-activated"} {
			var found bool
			keyBytes := must.One(ipld.Encode(basicnode.NewString(key), dagcbor.Encode))
			for _, event := range allEvents {
				for _, e := range event.Entries {
					if e.Key == "$type" && bytes.Equal(e.Value, keyBytes) {
						found = true
						switch key {
						case "deal-published", "deal-activated":
							expectedEntries := []types.EventEntry{
								{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: keyBytes},
								{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "id", Value: must.One(ipld.Encode(basicnode.NewInt(2), dagcbor.Encode))},
								{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "client", Value: must.One(ipld.Encode(basicnode.NewInt(int64(clientId)), dagcbor.Encode))},
								{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "provider", Value: must.One(ipld.Encode(basicnode.NewInt(int64(minerId)), dagcbor.Encode))},
							}
							require.ElementsMatch(t, expectedEntries, event.Entries)
						}
						break
					}
				}
			}
			require.True(t, found, "expected to find event %s", key)
		}
	}
}

func TestOnboardRawPieceSnap(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		ctx       = context.Background()
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.MutateSealingConfig(func(sc *config.SealingConfig) {
		sc.PreferNewSectorsForDeals = false
		sc.MakeNewSectorForDeals = false
		sc.MakeCCSectorsAvailable = true
		sc.AggregateCommits = false
	}))
	ens.InterconnectAll().BeginMiningMustPost(blocktime)

	miner.PledgeSectors(ctx, 1, 0, nil)
	sl, err := miner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")

	snum := sl[0]

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	client.WaitForSectorActive(ctx, t, snum, maddr)

	pieceSize := abi.PaddedPieceSize(2048).Unpadded()
	pieceData := make([]byte, pieceSize)
	_, _ = rand.Read(pieceData)

	dc, err := miner.ComputeDataCid(ctx, pieceSize, bytes.NewReader(pieceData))
	require.NoError(t, err)

	head, err := client.ChainHead(ctx)
	require.NoError(t, err)

	so, err := miner.SectorAddPieceToAny(ctx, pieceSize, bytes.NewReader(pieceData), piece.PieceDealInfo{
		PublishCid:   nil,
		DealID:       0,
		DealProposal: nil,
		DealSchedule: piece.DealSchedule{
			StartEpoch: head.Height() + 2880*2,
			EndEpoch:   head.Height() + 2880*400, // todo set so that it works with the sector
		},
		KeepUnsealed: false,
		PieceActivationManifest: &minertypes.PieceActivationManifest{
			CID:                   dc.PieceCID,
			Size:                  dc.Size,
			VerifiedAllocationKey: nil,
			Notify:                nil,
		},
	})
	require.NoError(t, err)

	// wait for sector to commit

	// wait for sector to commit and enter proving state
	toCheck := map[abi.SectorNumber]struct{}{
		so.Sector: {},
	}

	miner.WaitSectorsProving(ctx, toCheck)
}

func makeMarketDealProposal(t *testing.T, client *kit.TestFullNode, miner *kit.TestMiner, data cid.Cid, ps abi.PaddedPieceSize, start, end abi.ChainEpoch) market2.ClientDealProposal {
	ca, err := client.WalletDefaultAddress(context.Background())
	require.NoError(t, err)

	ma, err := miner.ActorAddress(context.Background())
	require.NoError(t, err)

	dp := market2.DealProposal{
		PieceCID:             data,
		PieceSize:            ps,
		VerifiedDeal:         false,
		Client:               ca,
		Provider:             ma,
		Label:                must.One(market2.NewLabelFromString("wat")),
		StartEpoch:           start,
		EndEpoch:             end,
		StoragePricePerEpoch: big.Zero(),
		ProviderCollateral:   abi.TokenAmount{}, // below
		ClientCollateral:     big.Zero(),
	}

	cb, err := client.StateDealProviderCollateralBounds(context.Background(), dp.PieceSize, dp.VerifiedDeal, types.EmptyTSK)
	require.NoError(t, err)
	dp.ProviderCollateral = big.Div(big.Mul(cb.Min, big.NewInt(2)), big.NewInt(2))

	buf, err := cborutil.Dump(&dp)
	require.NoError(t, err)
	sig, err := client.WalletSign(context.Background(), ca, buf)
	require.NoError(t, err)

	return market2.ClientDealProposal{
		Proposal:        dp,
		ClientSignature: *sig,
	}

}
