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
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes13 "github.com/filecoin-project/go-state-types/builtin/v13/miner"
	verifregtypes13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	datacap2 "github.com/filecoin-project/go-state-types/builtin/v9/datacap"
	verifregtypes9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	minertypes "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
)

func TestOnboardRawPieceVerified_WithActorEvents(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		ctx       = context.Background()
	)

	rootKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifierKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifiedClientKey, err := key.GenerateKey(types.KTBLS)
	require.NoError(t, err)

	bal, err := types.ParseFIL("100fil")
	require.NoError(t, err)

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(),
		kit.RootVerifier(rootKey, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifierKey, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifiedClientKey, abi.NewTokenAmount(bal.Int64())),
	)

	/* --- Setup subscription channels for ActorEvents --- */

	// subscribe only to miner's actor events
	minerEvtsChan, err := miner.FullNode.SubscribeActorEvents(ctx, &types.ActorEventFilter{
		Addresses: []address.Address{miner.ActorAddr},
	})
	require.NoError(t, err)

	// subscribe only to sector-activated events
	sectorActivatedCbor := must.One(ipld.Encode(basicnode.NewString("sector-activated"), dagcbor.Encode))
	sectorActivatedEvtsChan, err := miner.FullNode.SubscribeActorEvents(ctx, &types.ActorEventFilter{
		Fields: map[string][]types.ActorEventBlock{
			"$type": {
				{Codec: uint64(multicodec.Cbor), Value: sectorActivatedCbor},
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

	/* --- Setup verified registry and client and allocate datacap to client */

	verifierAddr, verifiedClientAddr := ddoVerifiedSetupVerifiedClient(ctx, t, client, rootKey, verifierKey, verifiedClientKey)

	/* --- Prepare piece for onboarding --- */

	pieceSize := abi.PaddedPieceSize(2048).Unpadded()
	pieceData := make([]byte, pieceSize)
	_, _ = rand.Read(pieceData)

	dc, err := miner.ComputeDataCid(ctx, pieceSize, bytes.NewReader(pieceData))
	require.NoError(t, err)

	/* --- Allocate datacap for the piece by the verified client --- */

	clientId, allocationId := ddoVerifiedSetupAllocations(ctx, t, client, minerId, dc, verifiedClientAddr)

	head, err := client.ChainHead(ctx)
	require.NoError(t, err)

	// subscribe to actor events up until the current head
	initialEventsChan, err := miner.FullNode.SubscribeActorEvents(ctx, &types.ActorEventFilter{
		FromHeight: epochPtr(0),
		ToHeight:   epochPtr(int64(head.Height())),
	})
	require.NoError(t, err)

	/* --- Onboard the piece --- */

	so, si := ddoVerifiedOnboardPiece(ctx, t, miner, clientId, allocationId, dc, pieceData)

	// check that we have one allocation because the real allocation has been claimed by the miner for the piece
	allocations, err := client.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, allocations, 1) // allocation has been claimed, leaving the bogus one

	ddoVerifiedRemoveAllocations(ctx, t, client, verifiedClientAddr, clientId)

	// check that we have no more allocations
	allocations, err = client.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, allocations, 0)

	/* --- Tests for ActorEvents --- */

	t.Logf("Inspecting events as they appear in message receipts")

	// construct ActorEvents from messages and receipts
	eventsFromMessages := ddoVerifiedBuildActorEventsFromMessages(ctx, t, miner.FullNode)
	fmt.Println("Events from message receipts:")
	printEvents(t, eventsFromMessages)

	// check for precisely these events and ensure they contain what we expect; don't be strict on
	// other events to make sure we're forward-compatible as new events are added

	{
		precommitedEvents := filterEvents(eventsFromMessages, "sector-precommitted")
		require.Len(t, precommitedEvents, 2)

		expectedEntries := []types.EventEntry{
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: must.One(ipld.Encode(basicnode.NewString("sector-precommitted"), dagcbor.Encode))},
			// first sector to start mining is CC
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "sector", Value: must.One(ipld.Encode(basicnode.NewInt(int64(so.Sector)-1), dagcbor.Encode))},
		}
		require.ElementsMatch(t, expectedEntries, precommitedEvents[0].Entries)

		// second sector has our piece
		expectedEntries[1].Value = must.One(ipld.Encode(basicnode.NewInt(int64(so.Sector)), dagcbor.Encode))
		require.ElementsMatch(t, expectedEntries, precommitedEvents[1].Entries)
	}

	{
		activatedEvents := filterEvents(eventsFromMessages, "sector-activated")
		require.Len(t, activatedEvents, 2)

		expectedEntries := []types.EventEntry{
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: must.One(ipld.Encode(basicnode.NewString("sector-activated"), dagcbor.Encode))},
			// first sector to start mining is CC
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "sector", Value: must.One(ipld.Encode(basicnode.NewInt(int64(so.Sector)-1), dagcbor.Encode))},
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "unsealed-cid", Value: must.One(ipld.Encode(datamodel.Null, dagcbor.Encode))},
		}
		require.ElementsMatch(t, expectedEntries, activatedEvents[0].Entries)

		// second sector has our piece, and only our piece, so usealed-cid matches piece-cid,
		// unfortunately we don't have a case with multiple pieces
		expectedEntries[1].Value = must.One(ipld.Encode(basicnode.NewInt(int64(so.Sector)), dagcbor.Encode))
		expectedEntries[2].Value = must.One(ipld.Encode(basicnode.NewLink(cidlink.Link{Cid: dc.PieceCID}), dagcbor.Encode))
		expectedEntries = append(expectedEntries,
			types.EventEntry{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "piece-cid", Value: must.One(ipld.Encode(basicnode.NewLink(cidlink.Link{Cid: dc.PieceCID}), dagcbor.Encode))},
			types.EventEntry{Flags: 0x01, Codec: uint64(multicodec.Cbor), Key: "piece-size", Value: must.One(ipld.Encode(basicnode.NewInt(int64(pieceSize.Padded())), dagcbor.Encode))},
		)
		require.ElementsMatch(t, expectedEntries, activatedEvents[1].Entries)
	}

	{
		verifierBalanceEvents := filterEvents(eventsFromMessages, "verifier-balance")
		require.Len(t, verifierBalanceEvents, 2)

		verifierIdAddr, err := client.StateLookupID(ctx, verifierAddr, types.EmptyTSK)
		require.NoError(t, err)
		verifierId, err := address.IDFromAddress(verifierIdAddr)
		require.NoError(t, err)

		verifierEntry := types.EventEntry{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "verifier", Value: must.One(ipld.Encode(basicnode.NewInt(int64(verifierId)), dagcbor.Encode))}
		require.Contains(t, verifierBalanceEvents[0].Entries, verifierEntry)
		require.Contains(t, verifierBalanceEvents[1].Entries, verifierEntry)
	}

	{
		allocationEvents := filterEvents(eventsFromMessages, "allocation")
		require.Len(t, allocationEvents, 2)

		expectedEntries := []types.EventEntry{
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: must.One(ipld.Encode(basicnode.NewString("allocation"), dagcbor.Encode))},
			// first, bogus, allocation
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "id", Value: must.One(ipld.Encode(basicnode.NewInt(int64(allocationId)-1), dagcbor.Encode))},
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "provider", Value: must.One(ipld.Encode(basicnode.NewInt(int64(minerId)), dagcbor.Encode))},
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "client", Value: must.One(ipld.Encode(basicnode.NewInt(int64(clientId)), dagcbor.Encode))},
		}
		require.ElementsMatch(t, expectedEntries, allocationEvents[0].Entries)

		// the second, real allocation
		expectedEntries[1].Value = must.One(ipld.Encode(basicnode.NewInt(int64(allocationId)), dagcbor.Encode))
		require.ElementsMatch(t, expectedEntries, allocationEvents[1].Entries)
	}

	{
		allocationEvents := filterEvents(eventsFromMessages, "allocation-removed")
		require.Len(t, allocationEvents, 1)

		// manual removal of the bogus allocation
		expectedEntries := []types.EventEntry{
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: must.One(ipld.Encode(basicnode.NewString("allocation-removed"), dagcbor.Encode))},
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "id", Value: must.One(ipld.Encode(basicnode.NewInt(int64(allocationId)-1), dagcbor.Encode))},
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "provider", Value: must.One(ipld.Encode(basicnode.NewInt(int64(minerId)), dagcbor.Encode))},
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "client", Value: must.One(ipld.Encode(basicnode.NewInt(int64(clientId)), dagcbor.Encode))},
		}
		require.ElementsMatch(t, expectedEntries, allocationEvents[0].Entries)
	}

	{
		claimEvents := filterEvents(eventsFromMessages, "claim")
		require.Len(t, claimEvents, 1)

		expectedEntries := []types.EventEntry{
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: must.One(ipld.Encode(basicnode.NewString("claim"), dagcbor.Encode))},
			// claimId inherits from its original allocationId
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "id", Value: must.One(ipld.Encode(basicnode.NewInt(int64(allocationId)), dagcbor.Encode))},
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "provider", Value: must.One(ipld.Encode(basicnode.NewInt(int64(minerId)), dagcbor.Encode))},
			{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "client", Value: must.One(ipld.Encode(basicnode.NewInt(int64(clientId)), dagcbor.Encode))},
		}
		require.ElementsMatch(t, expectedEntries, claimEvents[0].Entries)
	}

	// verify that we can trace a datacap allocation through to a claim with the events, since this
	// information is not completely available from the state tree
	claims := ddoVerifiedBuildClaimsFromMessages(ctx, t, eventsFromMessages, miner.FullNode)
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
	printEvents(t, allEvtsFromGetAPI)
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
				{Codec: uint64(multicodec.Cbor), Value: sectorActivatedCbor},
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
	// sector-precommitted, sector-activated, verifier-balance, verifier-balance, allocation, allocation
	require.Equal(t, eventsFromMessages[0:6], initialEvents)

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

func ddoVerifiedSetupAllocations(
	ctx context.Context,
	t *testing.T,
	node v1api.FullNode,
	minerId uint64,
	dc abi.PieceInfo,
	verifiedClientAddr address.Address,
) (clientID abi.ActorID, allocationID verifregtypes13.AllocationId) {

	head, err := node.ChainHead(ctx)
	require.NoError(t, err)

	// design this one to expire so we can observe allocation-removed
	expiringAllocationHeight := head.Height() + 100
	allocationRequestBork := verifregtypes13.AllocationRequest{
		Provider:   abi.ActorID(minerId),
		Data:       cid.MustParse("baga6ea4seaaqa"),
		Size:       dc.Size,
		TermMin:    verifregtypes13.MinimumVerifiedAllocationTerm,
		TermMax:    verifregtypes13.MaximumVerifiedAllocationTerm,
		Expiration: expiringAllocationHeight,
	}
	allocationRequest := verifregtypes13.AllocationRequest{
		Provider:   abi.ActorID(minerId),
		Data:       dc.PieceCID,
		Size:       dc.Size,
		TermMin:    verifregtypes13.MinimumVerifiedAllocationTerm,
		TermMax:    verifregtypes13.MaximumVerifiedAllocationTerm,
		Expiration: verifregtypes13.MaximumVerifiedAllocationExpiration,
	}

	allocationRequests := verifregtypes13.AllocationRequests{
		Allocations: []verifregtypes13.AllocationRequest{allocationRequestBork, allocationRequest},
	}

	receiverParams, aerr := actors.SerializeParams(&allocationRequests)
	require.NoError(t, aerr)

	transferParams, aerr := actors.SerializeParams(&datacap2.TransferParams{
		To:           builtin.VerifiedRegistryActorAddr,
		Amount:       big.Mul(big.NewInt(int64(dc.Size*2)), builtin.TokenPrecision),
		OperatorData: receiverParams,
	})
	require.NoError(t, aerr)

	msg := &types.Message{
		To:     builtin.DatacapActorAddr,
		From:   verifiedClientAddr,
		Method: datacap.Methods.TransferExported,
		Params: transferParams,
		Value:  big.Zero(),
	}

	sm, err := node.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	res, err := node.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	// check that we have an allocation
	allocations, err := node.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, allocations, 2) // allocation waiting to be claimed

	for key, value := range allocations {
		if value.Data == dc.PieceCID {
			allocationID = verifregtypes13.AllocationId(key)
			clientID = value.Client
			break
		}
	}
	require.NotEqual(t, verifregtypes13.AllocationId(0), allocationID) // found it in there
	return clientID, allocationID
}

func ddoVerifiedOnboardPiece(ctx context.Context, t *testing.T, miner *kit.TestMiner, clientId abi.ActorID, allocationId verifregtypes13.AllocationId, dc abi.PieceInfo, pieceData []byte) (lapi.SectorOffset, lapi.SectorInfo) {
	head, err := miner.FullNode.ChainHead(ctx)
	require.NoError(t, err)

	so, err := miner.SectorAddPieceToAny(ctx, dc.Size.Unpadded(), bytes.NewReader(pieceData), piece.PieceDealInfo{
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

	// Verify that the piece has been onboarded

	si, err := miner.SectorsStatus(ctx, so.Sector, true)
	require.NoError(t, err)
	require.Equal(t, dc.PieceCID, *si.CommD)

	require.Equal(t, si.DealWeight, big.Zero())
	require.Equal(t, si.VerifiedDealWeight, big.Mul(big.NewInt(int64(dc.Size)), big.NewInt(int64(si.Expiration-si.Activation))))

	return so, si
}

func ddoVerifiedRemoveAllocations(ctx context.Context, t *testing.T, node v1api.FullNode, verifiedClientAddr address.Address, clientId abi.ActorID) {
	// trigger an allocation removal
	removalParams, aerr := actors.SerializeParams(&verifregtypes13.RemoveExpiredAllocationsParams{Client: clientId})
	require.NoError(t, aerr)

	msg := &types.Message{
		To:     builtin.VerifiedRegistryActorAddr,
		From:   verifiedClientAddr,
		Method: verifreg.Methods.RemoveExpiredAllocations,
		Params: removalParams,
		Value:  big.Zero(),
	}

	sm, err := node.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	res, err := node.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)
}

func ddoVerifiedBuildClaimsFromMessages(ctx context.Context, t *testing.T, eventsFromMessages []*types.ActorEvent, node v1api.FullNode) []*verifregtypes9.Claim {
	claimKeyCbor := must.One(ipld.Encode(basicnode.NewString("claim"), dagcbor.Encode))
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

func ddoVerifiedBuildActorEventsFromMessages(ctx context.Context, t *testing.T, node v1api.FullNode) []*types.ActorEvent {
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

func ddoVerifiedSetupVerifiedClient(ctx context.Context, t *testing.T, client *kit.TestFullNode, rootKey *key.Key, verifierKey *key.Key, verifiedClientKey *key.Key) (address.Address, address.Address) {
	// import the root key.
	rootAddr, err := client.WalletImport(ctx, &rootKey.KeyInfo)
	require.NoError(t, err)

	// import the verifiers' keys.
	verifierAddr, err := client.WalletImport(ctx, &verifierKey.KeyInfo)
	require.NoError(t, err)

	// import the verified client's key.
	verifiedClientAddr, err := client.WalletImport(ctx, &verifiedClientKey.KeyInfo)
	require.NoError(t, err)

	allowance := big.NewInt(100000000000)
	params, aerr := actors.SerializeParams(&verifregtypes13.AddVerifierParams{Address: verifierAddr, Allowance: allowance})
	require.NoError(t, aerr)

	msg := &types.Message{
		From:   rootAddr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifier,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err, "AddVerifier failed")

	res, err := client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	verifierAllowance, err := client.StateVerifierStatus(ctx, verifierAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, allowance, *verifierAllowance)

	// assign datacap to a client
	initialDatacap := big.NewInt(10000)

	params, aerr = actors.SerializeParams(&verifregtypes13.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: initialDatacap})
	require.NoError(t, aerr)

	msg = &types.Message{
		From:   verifierAddr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifiedClient,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err = client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	res, err = client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	return verifierAddr, verifiedClientAddr
}

func filterEvents(events []*types.ActorEvent, key string) []*types.ActorEvent {
	keyBytes := must.One(ipld.Encode(basicnode.NewString(key), dagcbor.Encode))
	filtered := make([]*types.ActorEvent, 0)
	for _, event := range events {
		for _, e := range event.Entries {
			if e.Key == "$type" && bytes.Equal(e.Value, keyBytes) {
				filtered = append(filtered, event)
				break
			}
		}
	}
	return filtered
}

func printEvents(t *testing.T, events []*types.ActorEvent) {
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

// eventValueToDagJson converts an ActorEvent value to a JSON string for printing.
func eventValueToDagJson(t *testing.T, codec uint64, data []byte) string {
	switch codec {
	case uint64(multicodec.Cbor):
		nd, err := ipld.Decode(data, dagcbor.Decode)
		require.NoError(t, err)
		byts, err := ipld.Encode(nd, dagjson.Encode)
		require.NoError(t, err)
		return string(byts)
	default:
		return fmt.Sprintf("0x%x", data)
	}
}

func epochPtr(ei int64) *abi.ChainEpoch {
	ep := abi.ChainEpoch(ei)
	return &ep
}
