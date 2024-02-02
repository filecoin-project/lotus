package itests

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-commp-utils/nonffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes13 "github.com/filecoin-project/go-state-types/builtin/v13/miner"
	verifregtypes13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	datacap2 "github.com/filecoin-project/go-state-types/builtin/v9/datacap"
	market2 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	verifregtypes9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	minertypes "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
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

func TestOnboardRawPieceVerified(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		ctx       = context.Background()
	)

	rootKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifier1Key, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifiedClientKey, err := key.GenerateKey(types.KTBLS)
	require.NoError(t, err)

	bal, err := types.ParseFIL("100fil")
	require.NoError(t, err)

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(),
		kit.RootVerifier(rootKey, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifier1Key, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifiedClientKey, abi.NewTokenAmount(bal.Int64())),
	)

	evtChan, err := miner.FullNode.SubscribeActorEvents(ctx, &types.SubActorEventFilter{
		Filter: types.ActorEventFilter{
			MinEpoch: -1,
			MaxEpoch: -1,
		},
		Prefill: true,
	})
	require.NoError(t, err)

	events := make([]types.ActorEvent, 0)
	go func() {
		for e := range evtChan {
			fmt.Printf("%s Got ActorEvent: %+v", time.Now().Format(time.StampMilli), e)
			events = append(events, *e)
		}
	}()

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

	// get VRH
	vrh, err := client.StateVerifiedRegistryRootKey(ctx, types.TipSetKey{})
	fmt.Println(vrh.String())
	require.NoError(t, err)

	// import the root key.
	rootAddr, err := client.WalletImport(ctx, &rootKey.KeyInfo)
	require.NoError(t, err)

	// import the verifiers' keys.
	verifier1Addr, err := client.WalletImport(ctx, &verifier1Key.KeyInfo)
	require.NoError(t, err)

	// import the verified client's key.
	verifiedClientAddr, err := client.WalletImport(ctx, &verifiedClientKey.KeyInfo)
	require.NoError(t, err)

	// make the 2 verifiers

	mkVerifier(ctx, t, client.FullNode.(*api.FullNodeStruct), rootAddr, verifier1Addr)

	// assign datacap to a client
	initialDatacap := big.NewInt(10000)

	params, err := actors.SerializeParams(&verifregtypes13.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: initialDatacap})
	require.NoError(t, err)

	msg := &types.Message{
		From:   verifier1Addr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifiedClient,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	res, err := client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	minerId, err := address.IDFromAddress(miner.ActorAddr)
	require.NoError(t, err)

	allocationRequest := verifregtypes13.AllocationRequest{
		Provider:   abi.ActorID(minerId),
		Data:       dc.PieceCID,
		Size:       dc.Size,
		TermMin:    verifregtypes13.MinimumVerifiedAllocationTerm,
		TermMax:    verifregtypes13.MaximumVerifiedAllocationTerm,
		Expiration: verifregtypes13.MaximumVerifiedAllocationExpiration,
	}

	allocationRequests := verifregtypes13.AllocationRequests{
		Allocations: []verifregtypes13.AllocationRequest{allocationRequest},
	}

	receiverParams, err := actors.SerializeParams(&allocationRequests)
	require.NoError(t, err)

	transferParams, err := actors.SerializeParams(&datacap2.TransferParams{
		To:           builtin.VerifiedRegistryActorAddr,
		Amount:       big.Mul(big.NewInt(int64(dc.Size)), builtin.TokenPrecision),
		OperatorData: receiverParams,
	})
	require.NoError(t, err)

	msg = &types.Message{
		To:     builtin.DatacapActorAddr,
		From:   verifiedClientAddr,
		Method: datacap.Methods.TransferExported,
		Params: transferParams,
		Value:  big.Zero(),
	}

	sm, err = client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	res, err = client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	allocations, err := client.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, 1, len(allocations))

	var allocationId verifregtypes13.AllocationId
	var clientId abi.ActorID
	for key, value := range allocations {
		allocationId = verifregtypes13.AllocationId(key)
		clientId = value.Client
		break
	}

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
			VerifiedAllocationKey: &minertypes13.VerifiedAllocationKey{Client: clientId, ID: allocationId},
			Notify:                nil,
		},
	})
	require.NoError(t, err)

	// wait for sector to commit
	miner.WaitSectorsProving(ctx, map[abi.SectorNumber]struct{}{
		so.Sector: {},
	})

	si, err := miner.SectorsStatus(ctx, so.Sector, true)
	require.NoError(t, err)
	require.Equal(t, dc.PieceCID, *si.CommD)

	require.Equal(t, si.DealWeight, big.Zero())
	require.Equal(t, si.VerifiedDealWeight, big.Mul(big.NewInt(int64(dc.Size)), big.NewInt(int64(si.Expiration-si.Activation))))

	allocations, err = client.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, allocations, 0)

	evts, err := miner.FullNode.GetActorEvents(ctx, &types.ActorEventFilter{
		MinEpoch: -1,
		MaxEpoch: -1,
	})
	require.NoError(t, err)
	for _, evt := range evts {
		fmt.Printf("Got ActorEvent: %+v", evt)
	}

	eventsFromMessages := buildActorEventsFromMessages(t, ctx, miner.FullNode)
	writeEventsToFile(t, ctx, miner.FullNode, eventsFromMessages)
	for _, evt := range evts {
		fmt.Printf("Got ActorEvent from messages: %+v", evt)
	}

	// TODO: compare GetActorEvents & SubscribeActorEvents & eventsFromMessages for equality
}

func buildActorEventsFromMessages(t *testing.T, ctx context.Context, node v1api.FullNode) []types.ActorEvent {
	actorEvents := make([]types.ActorEvent, 0)

	head, err := node.ChainHead(ctx)
	require.NoError(t, err)
	for height := 0; height < int(head.Height()); height++ {
		// for each tipset
		ts, err := node.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(height), types.EmptyTSK)
		require.NoError(t, err)
		for _, b := range ts.Blocks() {
			// for each block
			// alternative here is to go straight to receipts, but we need the message CID for our event
			// list: node.ChainGetParentReceipts(ctx, b.Cid())
			messages, err := node.ChainGetParentMessages(ctx, b.Cid())
			require.NoError(t, err)
			if len(messages) == 0 {
				continue
			}
			for _, m := range messages {
				receipt, err := node.StateSearchMsg(ctx, ts.Key(), m.Cid, -1, false)
				require.NoError(t, err)
				// receipt
				if receipt.Receipt.EventsRoot != nil {
					events, err := node.ChainGetEvents(ctx, *receipt.Receipt.EventsRoot)
					require.NoError(t, err)
					for _, evt := range events {
						// for each event
						addr, err := address.NewIDAddress(uint64(evt.Emitter))
						require.NoError(t, err)
						tsCid, err := ts.Key().Cid()
						require.NoError(t, err)

						actorEvents = append(actorEvents, types.ActorEvent{
							Entries:     evt.Entries,
							EmitterAddr: addr,
							Reverted:    false,
							Height:      abi.ChainEpoch(height),
							TipSetKey:   tsCid,
							MsgCid:      m.Cid,
						})
					}
				}
			}
		}
	}
	return actorEvents
}

func writeEventsToFile(t *testing.T, ctx context.Context, node v1api.FullNode, events []types.ActorEvent) {
	file, err := os.Create("block.out")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, file.Close())
	}()
	write := func(s string) {
		_, err := file.WriteString(s)
		require.NoError(t, err)
	}
	claimKeyCbor, err := ipld.Encode(basicnode.NewString("claim"), dagcbor.Encode)
	require.NoError(t, err)

	for _, event := range events {
		entryStrings := []string{
			fmt.Sprintf("height=%d", event.Height),
			fmt.Sprintf("msg=%s", event.MsgCid),
			fmt.Sprintf("emitter=%s", event.EmitterAddr),
			fmt.Sprintf("reverted=%t", event.Reverted),
		}
		claims := make([]*verifregtypes9.Claim, 0)
		var isClaim bool
		var claimId int64 = -1
		var providerId int64 = -1

		for _, e := range event.Entries {
			// for each event entry
			entryStrings = append(entryStrings, fmt.Sprintf("%s=%s", e.Key, toDagJson(t, e.Codec, e.Value)))
			if e.Key == "$type" && bytes.Equal(e.Value, claimKeyCbor) {
				isClaim = true
			} else if isClaim && e.Key == "id" {
				nd, err := ipld.DecodeUsingPrototype([]byte(e.Value), dagcbor.Decode, bindnode.Prototype((*int64)(nil), nil))
				require.NoError(t, err)
				claimId = *bindnode.Unwrap(nd).(*int64)
			} else if isClaim && e.Key == "provider" {
				nd, err := ipld.DecodeUsingPrototype([]byte(e.Value), dagcbor.Decode, bindnode.Prototype((*int64)(nil), nil))
				require.NoError(t, err)
				providerId = *bindnode.Unwrap(nd).(*int64)
			}
			if isClaim && claimId != -1 && providerId != -1 {
				provider, err := address.NewIDAddress(uint64(providerId))
				require.NoError(t, err)
				claim, err := node.StateGetClaim(ctx, provider, verifregtypes9.ClaimId(claimId), types.EmptyTSK)
				require.NoError(t, err)
				claims = append(claims, claim)
			}
		}
		write(fmt.Sprintf("Event<%s>\n", strings.Join(entryStrings, ", ")))
		if len(claims) > 0 {
			for _, claim := range claims {
				p, err := address.NewIDAddress(uint64(claim.Provider))
				require.NoError(t, err)
				c, err := address.NewIDAddress(uint64(claim.Client))
				require.NoError(t, err)
				write(fmt.Sprintf("  Claim<provider=%s, client=%s, data=%s, size=%d, termMin=%d, termMax=%d, termStart=%d, sector=%d>\n",
					p, c, claim.Data, claim.Size, claim.TermMin, claim.TermMax, claim.TermStart, claim.Sector))
			}
		}
	}
}

func toDagJson(t *testing.T, codec uint64, data []byte) string {
	switch codec {
	case 0x51:
		nd, err := ipld.Decode(data, dagcbor.Decode)
		require.NoError(t, err)
		byts, err := ipld.Encode(nd, dagjson.Encode)
		require.NoError(t, err)
		return string(byts)
	default:
		return fmt.Sprintf("0x%x", data)
	}
}

func mkVerifier(ctx context.Context, t *testing.T, api *api.FullNodeStruct, rootAddr address.Address, addr address.Address) {
	allowance := big.NewInt(100000000000)
	params, aerr := actors.SerializeParams(&verifregtypes13.AddVerifierParams{Address: addr, Allowance: allowance})
	require.NoError(t, aerr)

	msg := &types.Message{
		From:   rootAddr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifier,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err := api.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err, "AddVerifier failed")

	res, err := api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	verifierAllowance, err := api.StateVerifierStatus(ctx, addr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, allowance, *verifierAllowance)
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
