package itests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	minerActor "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
	proof3 "github.com/filecoin-project/specs-actors/v3/actors/runtime/proof"
	"github.com/stretchr/testify/require"
)

func TestWindowPostDispute(t *testing.T) {
	if os.Getenv("LOTUS_TEST_WINDOW_POST") != "1" {
		t.Skip("this takes a few minutes, set LOTUS_TEST_WINDOW_POST=1 to run")
	}

	kit.QuietMiningLogs()

	b := kit.MockMinerBuilder
	blocktime := 2 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First, we configure two miners. After sealing, we're going to turn off the first miner so
	// it doesn't submit proofs.
	//
	// Then we're going to manually submit bad proofs.
	n, sn := b(t,
		[]kit.FullNodeOpts{kit.FullNodeWithLatestActorsAt(-1)},
		[]kit.StorageMiner{
			{Full: 0, Preseal: kit.PresealGenesis},
			{Full: 0},
		})

	client := n[0].FullNode.(*impl.FullNodeAPI)
	chainMiner := sn[0]
	evilMiner := sn[1]

	{
		addrinfo, err := client.NetAddrsListen(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if err := chainMiner.NetConnect(ctx, addrinfo); err != nil {
			t.Fatal(err)
		}

		if err := evilMiner.NetConnect(ctx, addrinfo); err != nil {
			t.Fatal(err)
		}
	}

	defaultFrom, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	build.Clock.Sleep(time.Second)

	// Mine with the _second_ node (the good one).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			build.Clock.Sleep(blocktime)
			if err := chainMiner.MineOne(ctx, kit.MineNext); err != nil {
				if ctx.Err() != nil {
					// context was canceled, ignore the error.
					return
				}
				t.Error(err)
			}
		}
	}()
	defer func() {
		cancel()
		<-done
	}()

	// Give the chain miner enough sectors to win every block.
	kit.PledgeSectors(t, ctx, chainMiner, 10, 0, nil)
	// And the evil one 1 sector. No cookie for you.
	kit.PledgeSectors(t, ctx, evilMiner, 1, 0, nil)

	// Let the evil miner's sectors gain power.
	evilMinerAddr, err := evilMiner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)

	fmt.Printf("Running one proving period\n")
	fmt.Printf("End for head.Height > %d\n", di.PeriodStart+di.WPoStProvingPeriod*2)

	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > di.PeriodStart+di.WPoStProvingPeriod*2 {
			fmt.Printf("Now head.Height = %d\n", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	p, err := client.StateMinerPower(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := evilMiner.ActorSectorSize(ctx, evilMinerAddr)
	require.NoError(t, err)

	// make sure it has gained power.
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)))

	evilSectors, err := evilMiner.SectorsList(ctx)
	require.NoError(t, err)
	evilSectorNo := evilSectors[0] // only one.
	evilSectorLoc, err := client.StateSectorPartition(ctx, evilMinerAddr, evilSectorNo, types.EmptyTSK)
	require.NoError(t, err)

	fmt.Println("evil miner stopping")

	// Now stop the evil miner, and start manually submitting bad proofs.
	require.NoError(t, evilMiner.Stop(ctx))

	fmt.Println("evil miner stopped")

	// Wait until we need to prove our sector.
	for {
		di, err = client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)
		if di.Index == evilSectorLoc.Deadline && di.CurrentEpoch-di.PeriodStart > 1 {
			break
		}
		build.Clock.Sleep(blocktime)
	}

	err = submitBadProof(ctx, client, evilMinerAddr, di, evilSectorLoc.Deadline, evilSectorLoc.Partition)
	require.NoError(t, err, "evil proof not accepted")

	// Wait until after the proving period.
	for {
		di, err = client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)
		if di.Index != evilSectorLoc.Deadline {
			break
		}
		build.Clock.Sleep(blocktime)
	}

	fmt.Println("accepted evil proof")

	// Make sure the evil node didn't lose any power.
	p, err = client.StateMinerPower(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)))

	// OBJECTION! The good miner files a DISPUTE!!!!
	{
		params := &minerActor.DisputeWindowedPoStParams{
			Deadline:  evilSectorLoc.Deadline,
			PoStIndex: 0,
		}

		enc, aerr := actors.SerializeParams(params)
		require.NoError(t, aerr)

		msg := &types.Message{
			To:     evilMinerAddr,
			Method: minerActor.Methods.DisputeWindowedPoSt,
			Params: enc,
			Value:  types.NewInt(0),
			From:   defaultFrom,
		}
		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)

		fmt.Println("waiting dispute")
		rec, err := client.StateWaitMsg(ctx, sm.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Zero(t, rec.Receipt.ExitCode, "dispute not accepted: %s", rec.Receipt.ExitCode.Error())
	}

	// Objection SUSTAINED!
	// Make sure the evil node lost power.
	p, err = client.StateMinerPower(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.True(t, p.MinerPower.RawBytePower.IsZero())

	// Now we begin the redemption arc.
	require.True(t, p.MinerPower.RawBytePower.IsZero())

	// First, recover the sector.

	{
		minerInfo, err := client.StateMinerInfo(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)

		params := &minerActor.DeclareFaultsRecoveredParams{
			Recoveries: []minerActor.RecoveryDeclaration{{
				Deadline:  evilSectorLoc.Deadline,
				Partition: evilSectorLoc.Partition,
				Sectors:   bitfield.NewFromSet([]uint64{uint64(evilSectorNo)}),
			}},
		}

		enc, aerr := actors.SerializeParams(params)
		require.NoError(t, aerr)

		msg := &types.Message{
			To:     evilMinerAddr,
			Method: minerActor.Methods.DeclareFaultsRecovered,
			Params: enc,
			Value:  types.FromFil(30), // repay debt.
			From:   minerInfo.Owner,
		}
		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)

		rec, err := client.StateWaitMsg(ctx, sm.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Zero(t, rec.Receipt.ExitCode, "recovery not accepted: %s", rec.Receipt.ExitCode.Error())
	}

	// Then wait for the deadline.
	for {
		di, err = client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)
		if di.Index == evilSectorLoc.Deadline {
			break
		}
		build.Clock.Sleep(blocktime)
	}

	// Now try to be evil again
	err = submitBadProof(ctx, client, evilMinerAddr, di, evilSectorLoc.Deadline, evilSectorLoc.Partition)
	require.Error(t, err)
	require.Contains(t, err.Error(), "message execution failed: exit 16, reason: window post failed: invalid PoSt")

	// It didn't work because we're recovering.
}

func TestWindowPostDisputeFails(t *testing.T) {
	if os.Getenv("LOTUS_TEST_WINDOW_POST") != "1" {
		t.Skip("this takes a few minutes, set LOTUS_TEST_WINDOW_POST=1 to run")
	}

	kit.QuietMiningLogs()

	b := kit.MockMinerBuilder
	blocktime := 2 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := b(t, []kit.FullNodeOpts{kit.FullNodeWithLatestActorsAt(-1)}, kit.OneMiner)

	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	{
		addrinfo, err := client.NetAddrsListen(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if err := miner.NetConnect(ctx, addrinfo); err != nil {
			t.Fatal(err)
		}
	}

	defaultFrom, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	build.Clock.Sleep(time.Second)

	// Mine with the _second_ node (the good one).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			build.Clock.Sleep(blocktime)
			if err := miner.MineOne(ctx, kit.MineNext); err != nil {
				if ctx.Err() != nil {
					// context was canceled, ignore the error.
					return
				}
				t.Error(err)
			}
		}
	}()
	defer func() {
		cancel()
		<-done
	}()

	kit.PledgeSectors(t, ctx, miner, 10, 0, nil)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	fmt.Printf("Running one proving period\n")
	fmt.Printf("End for head.Height > %d\n", di.PeriodStart+di.WPoStProvingPeriod*2)

	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > di.PeriodStart+di.WPoStProvingPeriod*2 {
			fmt.Printf("Now head.Height = %d\n", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)
	expectedPower := types.NewInt(uint64(ssz) * (kit.GenesisPreseals + 10))

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	// make sure it has gained power.
	require.Equal(t, p.MinerPower.RawBytePower, expectedPower)

	// Wait until a proof has been submitted.
	var targetDeadline uint64
waitForProof:
	for {
		deadlines, err := client.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		for dlIdx, dl := range deadlines {
			nonEmpty, err := dl.PostSubmissions.IsEmpty()
			require.NoError(t, err)
			if nonEmpty {
				targetDeadline = uint64(dlIdx)
				break waitForProof
			}
		}

		build.Clock.Sleep(blocktime)
	}

	for {
		di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		// wait until the deadline finishes.
		if di.Index == ((targetDeadline + 1) % di.WPoStPeriodDeadlines) {
			break
		}

		build.Clock.Sleep(blocktime)
	}

	// Try to object to the proof. This should fail.
	{
		params := &minerActor.DisputeWindowedPoStParams{
			Deadline:  targetDeadline,
			PoStIndex: 0,
		}

		enc, aerr := actors.SerializeParams(params)
		require.NoError(t, aerr)

		msg := &types.Message{
			To:     maddr,
			Method: minerActor.Methods.DisputeWindowedPoSt,
			Params: enc,
			Value:  types.NewInt(0),
			From:   defaultFrom,
		}
		_, err := client.MpoolPushMessage(ctx, msg, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to dispute valid post (RetCode=16)")
	}
}

func submitBadProof(
	ctx context.Context,
	client api.FullNode, maddr address.Address,
	di *dline.Info, dlIdx, partIdx uint64,
) error {
	head, err := client.ChainHead(ctx)
	if err != nil {
		return err
	}

	from, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		return err
	}

	minerInfo, err := client.StateMinerInfo(ctx, maddr, head.Key())
	if err != nil {
		return err
	}

	commEpoch := di.Open
	commRand, err := client.ChainGetRandomnessFromTickets(
		ctx, head.Key(), crypto.DomainSeparationTag_PoStChainCommit,
		commEpoch, nil,
	)
	if err != nil {
		return err
	}
	params := &minerActor.SubmitWindowedPoStParams{
		ChainCommitEpoch: commEpoch,
		ChainCommitRand:  commRand,
		Deadline:         dlIdx,
		Partitions:       []minerActor.PoStPartition{{Index: partIdx}},
		Proofs: []proof3.PoStProof{{
			PoStProof:  minerInfo.WindowPoStProofType,
			ProofBytes: []byte("I'm soooo very evil."),
		}},
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return aerr
	}

	msg := &types.Message{
		To:     maddr,
		Method: minerActor.Methods.SubmitWindowedPoSt,
		Params: enc,
		Value:  types.NewInt(0),
		From:   from,
	}
	sm, err := client.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return err
	}

	rec, err := client.StateWaitMsg(ctx, sm.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		return err
	}
	if rec.Receipt.ExitCode.IsError() {
		return rec.Receipt.ExitCode
	}
	return nil
}
