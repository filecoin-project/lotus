package itests

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
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

func TestOnboardRawPieceVerified_WithActorEvents(t *testing.T) {
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

	/* --- Setup subscription channels for ActorEvents --- */

	// subscribe only to miner's actor events
	minerEvtsChan, err := miner.FullNode.SubscribeActorEvents(ctx, &types.ActorEventFilter{
		Addresses: []address.Address{miner.ActorAddr},
	})
	require.NoError(t, err)

	// subscribe only to sector-activated events
	sectorActivatedCbor := stringToEventKey(t, "sector-activated")
	sectorActivatedEvtsChan, err := miner.FullNode.SubscribeActorEvents(ctx, &types.ActorEventFilter{
		Fields: map[string][]types.ActorEventBlock{
			"$type": {
				{Codec: 0x51, Value: sectorActivatedCbor},
			},
		},
	})
	require.NoError(t, err)

	/* --- Start mining --- */

	ens.InterconnectAll().BeginMiningMustPost(blocktime)

	minerId, err := address.IDFromAddress(miner.ActorAddr)
	require.NoError(t, err)

	miner.PledgeSectors(ctx, 1, 0, nil)
	sl, err := miner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")

	snum := sl[0]

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	client.WaitForSectorActive(ctx, t, snum, maddr)

	/* --- Prepare piece for onboarding --- */

	pieceSize := abi.PaddedPieceSize(2048).Unpadded()
	pieceData := make([]byte, pieceSize)
	_, _ = rand.Read(pieceData)

	dc, err := miner.ComputeDataCid(ctx, pieceSize, bytes.NewReader(pieceData))
	require.NoError(t, err)

	/* --- Setup verified registry and client allocator --- */

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

	/* --- Allocate datacap for the piece by the verified client --- */

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

	// check that we have an allocation
	allocations, err := client.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, allocations, 1) // allocation waiting to be claimed

	var allocationId verifregtypes13.AllocationId
	var clientId abi.ActorID
	for key, value := range allocations {
		allocationId = verifregtypes13.AllocationId(key)
		clientId = value.Client
		break
	}

	/* --- Onboard the piece --- */

	head, err := client.ChainHead(ctx)
	require.NoError(t, err)

	// subscribe to actor events up until the current head
	initialEventsChan, err := miner.FullNode.SubscribeActorEvents(ctx, &types.ActorEventFilter{
		FromHeight: epochPtr(0),
		ToHeight:   epochPtr(int64(head.Height())),
	})
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

	/* --- Verify that the piece has been onboarded --- */

	si, err := miner.SectorsStatus(ctx, so.Sector, true)
	require.NoError(t, err)
	require.Equal(t, dc.PieceCID, *si.CommD)

	require.Equal(t, si.DealWeight, big.Zero())
	require.Equal(t, si.VerifiedDealWeight, big.Mul(big.NewInt(int64(dc.Size)), big.NewInt(int64(si.Expiration-si.Activation))))

	// check that we have no more allocations because the allocation has been claimed by the miner for the piece
	allocations, err = client.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, allocations, 0) // allocation has been claimed

	/* --- Tests for ActorEvents --- */

	// construct ActorEvents from messages and receipts
	eventsFromMessages := buildActorEventsFromMessages(ctx, t, miner.FullNode)
	fmt.Println("Events from message receipts:")
	printEvents(ctx, t, miner.FullNode, eventsFromMessages)

	require.GreaterOrEqual(t, len(eventsFromMessages), 8) // allow for additional events in the future
	// check for precisely these events
	for key, count := range map[string]int{
		"sector-precommitted": 2, // first to begin mining, second to onboard the piece
		"sector-activated":    2, // first to begin mining, second to onboard the piece
		"verifier-balance":    2, // first to setup the verifier, second to allocate datacap to the verified client
		"allocation":          1, // verified client allocates datacap to the miner
		"claim":               1, // miner claims the allocation for the piece
	} {
		keyBytes := stringToEventKey(t, key)
		found := 0
		for _, event := range eventsFromMessages {
			for _, e := range event.Entries {
				if e.Key == "$type" && bytes.Equal(e.Value, keyBytes) {
					found++
					break
				}
			}
		}
		require.Equal(t, count, found, "unexpected number of events for %s", key)
	}

	// verify that we can trace a datacap allocation through to a claim with the events, since this
	// information is not completely available from the state tree
	claims := buildClaimsFromMessages(ctx, t, eventsFromMessages, miner.FullNode)
	for _, claim := range claims {
		p, err := address.NewIDAddress(uint64(claim.Provider))
		require.NoError(t, err)
		c, err := address.NewIDAddress(uint64(claim.Client))
		require.NoError(t, err)
		fmt.Printf("Claim<provider=%s, client=%s, data=%s, size=%d, termMin=%d, termMax=%d, termStart=%d, sector=%d>\n",
			p, c, claim.Data, claim.Size, claim.TermMin, claim.TermMax, claim.TermStart, claim.Sector)
	}
	require.Equal(t, []*verifregtypes9.Claim{
		{
			Provider:  abi.ActorID(minerId),
			Client:    clientId,
			Data:      dc.PieceCID,
			Size:      dc.Size,
			TermMin:   verifregtypes13.MinimumVerifiedAllocationTerm,
			TermMax:   verifregtypes13.MaximumVerifiedAllocationTerm,
			TermStart: si.Activation,
			Sector:    so.Sector,
		},
	}, claims)

	// construct ActorEvents from GetActorEvents API
	t.Logf("Inspecting full events list from GetActorEvents")
	allEvtsFromGetAPI, err := miner.FullNode.GetActorEvents(ctx, &types.ActorEventFilter{
		FromHeight: epochPtr(0),
	})
	require.NoError(t, err)
	fmt.Println("Events from GetActorEvents:")
	printEvents(ctx, t, miner.FullNode, allEvtsFromGetAPI)
	// compare events from messages and receipts with events from GetActorEvents API
	require.Equal(t, eventsFromMessages, allEvtsFromGetAPI)

	// construct ActorEvents from subscription channel for just the miner actor
	t.Logf("Inspecting only miner's events list from SubscribeActorEvents")
	var subMinerEvts []*types.ActorEvent
	for evt := range minerEvtsChan {
		subMinerEvts = append(subMinerEvts, evt)
		if len(subMinerEvts) == 4 {
			break
		}
	}
	var allMinerEvts []*types.ActorEvent
	for _, evt := range eventsFromMessages {
		if evt.Emitter == miner.ActorAddr {
			allMinerEvts = append(allMinerEvts, evt)
		}
	}
	// compare events from messages and receipts with events from subscription channel
	require.Equal(t, allMinerEvts, subMinerEvts)

	// construct ActorEvents from subscription channels for just the sector-activated events
	var sectorActivatedEvts []*types.ActorEvent
	for _, evt := range eventsFromMessages {
		for _, entry := range evt.Entries {
			if entry.Key == "$type" && bytes.Equal(entry.Value, sectorActivatedCbor) {
				sectorActivatedEvts = append(sectorActivatedEvts, evt)
				break
			}
		}
	}
	require.Len(t, sectorActivatedEvts, 2) // sanity check

	t.Logf("Inspecting only sector-activated events list from real-time SubscribeActorEvents")
	var subscribedSectorActivatedEvts []*types.ActorEvent
	for evt := range sectorActivatedEvtsChan {
		subscribedSectorActivatedEvts = append(subscribedSectorActivatedEvts, evt)
		if len(subscribedSectorActivatedEvts) == 2 {
			break
		}
	}
	// compare events from messages and receipts with events from subscription channel
	require.Equal(t, sectorActivatedEvts, subscribedSectorActivatedEvts)

	// same thing but use historical event fetching to see the same list
	t.Logf("Inspecting only sector-activated events list from historical SubscribeActorEvents")
	sectorActivatedEvtsChan, err = miner.FullNode.SubscribeActorEvents(ctx, &types.ActorEventFilter{
		Fields: map[string][]types.ActorEventBlock{
			"$type": {
				{Codec: 0x51, Value: sectorActivatedCbor},
			},
		},
		FromHeight: epochPtr(0),
	})
	require.NoError(t, err)
	subscribedSectorActivatedEvts = subscribedSectorActivatedEvts[:0]
	for evt := range sectorActivatedEvtsChan {
		subscribedSectorActivatedEvts = append(subscribedSectorActivatedEvts, evt)
		if len(subscribedSectorActivatedEvts) == 2 {
			break
		}
	}
	// compare events from messages and receipts with events from subscription channel
	require.Equal(t, sectorActivatedEvts, subscribedSectorActivatedEvts)

	// check that our `ToHeight` filter works as expected
	t.Logf("Inspecting only initial list of events SubscribeActorEvents with ToHeight")
	var initialEvents []*types.ActorEvent
	for evt := range initialEventsChan {
		initialEvents = append(initialEvents, evt)
	}
	// sector-precommitted, sector-activated, verifier-balance, verifier-balance, allocation
	require.Equal(t, eventsFromMessages[0:5], initialEvents)

	// construct ActorEvents from subscription channel for all actor events
	t.Logf("Inspecting full events list from historical SubscribeActorEvents")
	allEvtsChan, err := miner.FullNode.SubscribeActorEvents(ctx, &types.ActorEventFilter{
		FromHeight: epochPtr(0),
	})
	require.NoError(t, err)
	var prefillEvts []*types.ActorEvent
	for evt := range allEvtsChan {
		prefillEvts = append(prefillEvts, evt)
		if len(prefillEvts) == len(eventsFromMessages) {
			break
		}
	}
	// compare events from messages and receipts with events from subscription channel
	require.Equal(t, eventsFromMessages, prefillEvts)
	t.Logf("All done comparing events")

	// NOTE: There is a delay in finishing this test because the SubscribeActorEvents
	// with the ToHeight (initialEventsChan) has to wait at least a full actual epoch before
	// realising that there's no more events for that filter. itests run with a different block
	// speed than the ActorEventHandler is aware of.
}

func buildClaimsFromMessages(ctx context.Context, t *testing.T, eventsFromMessages []*types.ActorEvent, node v1api.FullNode) []*verifregtypes9.Claim {
	claimKeyCbor := stringToEventKey(t, "claim")
	claims := make([]*verifregtypes9.Claim, 0)
	for _, event := range eventsFromMessages {
		var isClaim bool
		var claimId int64 = -1
		var providerId int64 = -1
		for _, e := range event.Entries {
			if e.Key == "$type" && bytes.Equal(e.Value, claimKeyCbor) {
				isClaim = true
			} else if isClaim && e.Key == "id" {
				nd, err := ipld.DecodeUsingPrototype(e.Value, dagcbor.Decode, bindnode.Prototype((*int64)(nil), nil))
				require.NoError(t, err)
				claimId = *bindnode.Unwrap(nd).(*int64)
			} else if isClaim && e.Key == "provider" {
				nd, err := ipld.DecodeUsingPrototype(e.Value, dagcbor.Decode, bindnode.Prototype((*int64)(nil), nil))
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
	}
	return claims
}

func buildActorEventsFromMessages(ctx context.Context, t *testing.T, node v1api.FullNode) []*types.ActorEvent {
	actorEvents := make([]*types.ActorEvent, 0)

	head, err := node.ChainHead(ctx)
	require.NoError(t, err)
	var lastts types.TipSetKey
	for height := 0; height < int(head.Height()); height++ {
		// for each tipset
		ts, err := node.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(height), types.EmptyTSK)
		require.NoError(t, err)
		if ts.Key() == lastts {
			continue
		}
		lastts = ts.Key()
		messages, err := node.ChainGetMessagesInTipset(ctx, ts.Key())
		require.NoError(t, err)
		if len(messages) == 0 {
			continue
		}
		for _, m := range messages {
			receipt, err := node.StateSearchMsg(ctx, types.EmptyTSK, m.Cid, -1, false)
			require.NoError(t, err)
			require.NotNil(t, receipt)
			// receipt
			if receipt.Receipt.EventsRoot != nil {
				events, err := node.ChainGetEvents(ctx, *receipt.Receipt.EventsRoot)
				require.NoError(t, err)
				for _, evt := range events {
					// for each event
					addr, err := address.NewIDAddress(uint64(evt.Emitter))
					require.NoError(t, err)

					actorEvents = append(actorEvents, &types.ActorEvent{
						Entries:   evt.Entries,
						Emitter:   addr,
						Reverted:  false,
						Height:    ts.Height(),
						TipSetKey: ts.Key(),
						MsgCid:    m.Cid,
					})
				}
			}
		}
	}
	return actorEvents
}

func printEvents(ctx context.Context, t *testing.T, node v1api.FullNode, events []*types.ActorEvent) {
	for _, event := range events {
		entryStrings := []string{
			fmt.Sprintf("height=%d", event.Height),
			fmt.Sprintf("msg=%s", event.MsgCid),
			fmt.Sprintf("emitter=%s", event.Emitter),
			fmt.Sprintf("reverted=%t", event.Reverted),
		}
		for _, e := range event.Entries {
			// for each event entry
			entryStrings = append(entryStrings, fmt.Sprintf("%s=%s", e.Key, eventValueToDagJson(t, e.Codec, e.Value)))
		}
		fmt.Printf("Event<%s>\n", strings.Join(entryStrings, ", "))
	}
}

// stringToEventKey converts a string to a CBOR-encoded blob which matches what we expect from the
// actor events.
func stringToEventKey(t *testing.T, str string) []byte {
	dcb, err := ipld.Encode(basicnode.NewString(str), dagcbor.Encode)
	require.NoError(t, err)
	return dcb
}

// eventValueToDagJson converts an ActorEvent value to a JSON string for printing.
func eventValueToDagJson(t *testing.T, codec uint64, data []byte) string {
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

func epochPtr(ei int64) *abi.ChainEpoch {
	ep := abi.ChainEpoch(ei)
	return &ep
}
