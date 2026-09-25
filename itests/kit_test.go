package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestMustPostHarness exercises MineBlocksMustPost against a live network. Requested null rounds
// land as a single gap of the requested width, the close guard stops block production rather than
// let a watched deadline close unproven, and chain and message waiters then return the failure
// instead of waiting on a stopped chain. Each stage builds on the chain state of the previous one.
func TestMustPostHarness(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		client  kit.TestFullNode
		genesis kit.TestMiner
	)
	sectorSize := abi.SectorSize(2 << 10)
	ens := kit.NewEnsemble(t, kit.MockProofs()).
		FullNode(&client, kit.SectorSize(sectorSize)).
		Miner(&genesis, &client, kit.PresealSectors(5), kit.SectorSize(sectorSize), kit.WithAllSubsystems()).
		Start().
		InterconnectAll()
	bm := ens.BeginMiningMustPost(2 * time.Millisecond)[0]
	ctx := context.Background()
	um, ens := ens.UnmanagedMiner(ctx, &client, kit.SectorSize(sectorSize), kit.OwnerAddr(client.DefaultKey))
	failure := ens.FailureContext(ctx)
	ens.Start()

	// The genesis miner is the only watched miner and holds all power until the unmanaged miner
	// onboards, so a round goes null only on a very rare lost election.
	if !t.Run("requested nulls land exactly", func(t *testing.T) {
		req := require.New(t)
		const nulls = 7

		// Early in a deadline the landing epoch stays far from its close, where the genesis miner's
		// post is still to come and the close guard would trip.
		var early *types.TipSet
		client.WaitTillChain(ctx, func(ts *types.TipSet) bool {
			di, err := client.StateMinerProvingDeadline(ctx, genesis.ActorAddr, ts.Key())
			req.NoError(err)
			if into := ts.Height() - di.Open; di.IsOpen() && into >= 5 && into < 20 {
				early = ts
				return true
			}
			return false
		})
		bm.InjectNulls(nulls)

		head := client.WaitTillChain(ctx, kit.HeightAtLeast(early.Height()+nulls+20))
		var maxGap abi.ChainEpoch
		for ts := head; ts.Height() > early.Height(); {
			parent, err := client.ChainGetTipSet(ctx, ts.Parents())
			req.NoError(err)
			maxGap = max(maxGap, ts.Height()-parent.Height()-1)
			ts = parent
		}
		t.Logf("largest gap between %d and %d: %d", early.Height(), head.Height(), maxGap)
		// Low probability, but a lost election adds a null round so we tolerate a couple.
		req.GreaterOrEqual(maxGap, abi.ChainEpoch(nulls))
		req.LessOrEqual(maxGap, abi.ChainEpoch(nulls+2))
		req.NoError(failure.Err(), "the harness is still mining")
	}) {
		return
	}

	// The close guard asserts the invariant that consumers rely on, which is that *no watched deadline
	// is crossed silently*.
	var cause error
	if !t.Run("close guard stops an unproven deadline", func(t *testing.T) {
		req := require.New(t)

		sectors, _ := um.OnboardSectors(abi.RegisteredSealProof_StackedDrg2KiBV1_1, kit.NewSectorBatch().AddEmptySectors(1))
		bm.WatchMinerForPost(um.ActorAddr)
		loc, err := client.StateSectorPartition(ctx, um.ActorAddr, sectors[0], types.EmptyTSK)
		req.NoError(err)

		head, err := client.ChainHead(ctx)
		req.NoError(err)
		di, err := client.StateMinerProvingDeadline(ctx, um.ActorAddr, head.Key())
		req.NoError(err)
		if di.IsOpen() && di.Index == loc.Deadline {
			// Stopping now would leave this occurrence unpostable, so let it be proved and close first.
			req.NoError(um.WaitTillPostCount(sectors[0], 1))
			head = client.WaitTillChain(ctx, kit.HeightAtLeast(di.Close))
		}
		// No post can arrive for the sector's deadline from here on, and none of its windows are open.
		um.Stop()

		stoppedAt, err := client.ChainHead(ctx)
		req.NoError(err)
		closeAt := kit.DeadlineCloseAfter(di, loc.Deadline, head.Height()+1)
		openAt := closeAt - di.WPoStChallengeWindow
		req.Greater(openAt, stoppedAt.Height(), "the chosen window opens after the miner stopped")

		head = client.WaitTillChain(ctx, kit.HeightAtLeast(openAt+1))
		di, err = client.StateMinerProvingDeadline(ctx, um.ActorAddr, head.Key())
		req.NoError(err)
		req.True(di.IsOpen(), "deadline %d open at %d", di.Index, head.Height())
		req.Equal(loc.Deadline, di.Index)
		parts, err := client.StateMinerPartitions(ctx, um.ActorAddr, di.Index, head.Key())
		req.NoError(err)
		req.Less(loc.Partition, uint64(len(parts)))
		live, err := parts[loc.Partition].LiveSectors.IsSet(uint64(sectors[0]))
		req.NoError(err)
		req.True(live, "sector %d is live in its partition", sectors[0])
		faulty, err := parts[loc.Partition].FaultySectors.IsSet(uint64(sectors[0]))
		req.NoError(err)
		req.False(faulty, "sector %d is not faulty, so the window requires proof", sectors[0])

		// ExpectFailure inverts the normal behaviour of the kit, allowing us to test how it handles failures.
		bm.ExpectFailure(func(err error) { t.Logf("expected MustPost failure: %s", err) })
		bm.InjectNulls(di.WPoStChallengeWindow + 5)

		select {
		case <-failure.Done():
		case <-time.After(time.Minute):
			req.FailNow("the close guard did not stop block production")
		}
		cause = context.Cause(failure)
		t.Logf("failure cause: %s", cause)
		req.NotErrorIs(cause, context.Canceled)
		req.ErrorContains(cause, "closed unproven under requested null rounds")
	}) {
		return
	}

	t.Run("waiters return the failure", func(t *testing.T) {
		req := require.New(t)
		// A regression shows as a deadline error rather than a hung test.
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		_, err := client.WaitTillChainOrError(ctx, kit.HeightAtLeast(1<<30))
		req.ErrorIs(err, cause)

		// Block production has stopped, so the message is never included.
		smsg, err := client.MpoolPushMessage(ctx, &types.Message{
			From:  client.DefaultKey.Address,
			To:    client.DefaultKey.Address,
			Value: big.Zero(),
		}, nil)
		req.NoError(err)
		_, err = client.WaitMsgResult(ctx, smsg.Cid(), 1)
		req.ErrorIs(err, cause)
	})
}
