package itests

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

func TestManualNISectorOnboarding(t *testing.T) {
	req := require.New(t)

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
	sealProofType, err := miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version23, miner.SealProofVariant_NonInteractive)
	req.NoError(err)

	for _, withMockProofs := range []bool{true, false} {
		testName := "WithRealProofs"
		if withMockProofs {
			testName = "WithMockProofs"
		}
		t.Run(testName, func(t *testing.T) {
			if !withMockProofs {
				kit.VeryExpensive(t)
			}
			kit.QuietMiningLogs()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var (
				// need to pick a balance value so that the test is not racy on CI by running through it's WindowPostDeadlines too fast
				blocktime = 2 * time.Millisecond
				client    kit.TestFullNode
				minerA    kit.TestMiner // A is a standard genesis miner
			)

			// Setup and begin mining with a single miner (A)
			// Miner A will only be a genesis Miner with power allocated in the genesis block and will not onboard any sectors from here on
			ens := kit.NewEnsemble(t, kit.MockProofs(withMockProofs)).
				FullNode(&client, kit.SectorSize(defaultSectorSize)).
				// preseal more than the default number of sectors to ensure that the genesis miner has power
				// because our unmanaged miners won't produce blocks so we may get null rounds
				Miner(&minerA, &client, kit.PresealSectors(5), kit.SectorSize(defaultSectorSize), kit.WithAllSubsystems()).
				Start().
				InterconnectAll()
			blockMiners := ens.BeginMiningMustPost(blocktime)
			req.Len(blockMiners, 1)
			blockMiner := blockMiners[0]

			// Instantiate MinerB to manually handle sector onboarding and power acquisition through sector activation.
			// Unlike other miners managed by the Lotus Miner storage infrastructure, MinerB operates independently,
			// performing all related tasks manually. Managed by the TestKit, MinerB has the capability to utilize actual proofs
			// for the processes of sector onboarding and activation.
			nodeOpts := []kit.NodeOpt{kit.SectorSize(defaultSectorSize), kit.OwnerAddr(client.DefaultKey)}
			minerB, ens := ens.UnmanagedMiner(&client, nodeOpts...)

			ens.Start()

			build.Clock.Sleep(time.Second)

			t.Log("Checking initial power ...")

			// Miner A should have power as it has already onboarded sectors in the genesis block
			head, err := client.ChainHead(ctx)
			req.NoError(err)
			p, err := client.StateMinerPower(ctx, minerA.ActorAddr, head.Key())
			req.NoError(err)
			t.Logf("MinerA RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())

			// Miner B should have no power as it has yet to onboard and activate any sectors
			minerB.AssertNoPower(ctx)

			// Verify that ProveCommitSectorsNI rejects messages with invalid parameters
			verifyProveCommitSectorsNIErrorConditions(ctx, t, minerB, sealProofType)

			// ---- Miner B onboards a CC sector
			var bSectorNum abi.SectorNumber
			var bRespCh chan kit.WindowPostResp
			var bWdPostCancelF context.CancelFunc

			// Onboard a CC sector with Miner B using NI-PoRep
			bSectorNum, bRespCh, bWdPostCancelF = minerB.OnboardCCSector(ctx, sealProofType)
			// Miner B should still not have power as power can only be gained after sector is activated i.e. the first WindowPost is submitted for it
			minerB.AssertNoPower(ctx)

			// Check that the sector-activated event was emitted
			{
				expectedEntries := []types.EventEntry{
					{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: must.One(ipld.Encode(basicnode.NewString("sector-activated"), dagcbor.Encode))},
					{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "sector", Value: must.One(ipld.Encode(basicnode.NewInt(int64(bSectorNum)), dagcbor.Encode))},
					{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "unsealed-cid", Value: must.One(ipld.Encode(datamodel.Null, dagcbor.Encode))},
				}
				from := head.Height()
				recentEvents, err := client.FullNode.GetActorEventsRaw(ctx, &types.ActorEventFilter{FromHeight: &from})
				req.NoError(err)
				req.Len(recentEvents, 1)
				req.Equal(expectedEntries, recentEvents[0].Entries)
			}

			// Ensure that the block miner checks for and waits for posts during the appropriate proving window from our new miner with a sector
			blockMiner.WatchMinerForPost(minerB.ActorAddr)

			// Wait till both miners' sectors have had their first post and are activated and check that this is reflected in miner power
			minerB.WaitTillActivatedAndAssertPower(ctx, bRespCh, bSectorNum)

			head, err = client.ChainHead(ctx)
			req.NoError(err)

			// Miner B has activated the CC sector -> upgrade it with snapdeals
			snapPieces := minerB.SnapDeal(ctx, kit.TestSpt, bSectorNum)
			// cancel the WdPost for the CC sector as the corresponding CommR is no longer valid
			bWdPostCancelF()

			// Check "sector-updated" event happned after snap
			{
				expectedEntries := []types.EventEntry{
					{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: must.One(ipld.Encode(basicnode.NewString("sector-updated"), dagcbor.Encode))},
					{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "sector", Value: must.One(ipld.Encode(basicnode.NewInt(int64(bSectorNum)), dagcbor.Encode))},
					{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "unsealed-cid", Value: must.One(ipld.Encode(basicnode.NewLink(cidlink.Link{Cid: snapPieces[0].PieceCID}), dagcbor.Encode))},
					{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "piece-cid", Value: must.One(ipld.Encode(basicnode.NewLink(cidlink.Link{Cid: snapPieces[0].PieceCID}), dagcbor.Encode))},
					{Flags: 0x01, Codec: uint64(multicodec.Cbor), Key: "piece-size", Value: must.One(ipld.Encode(basicnode.NewInt(int64(snapPieces[0].Size)), dagcbor.Encode))},
				}
				from := head.Height()
				recentEvents, err := client.FullNode.GetActorEventsRaw(ctx, &types.ActorEventFilter{FromHeight: &from})
				req.NoError(err)
				req.Len(recentEvents, 1)
				req.Equal(expectedEntries, recentEvents[0].Entries)
			}
		})
	}
}

func verifyProveCommitSectorsNIErrorConditions(ctx context.Context, t *testing.T, miner *kit.TestUnmanagedMiner, sealProofType abi.RegisteredSealProof) {
	req := require.New(t)

	head, err := miner.FullNode.ChainHead(ctx)
	req.NoError(err)

	actorIdNum, err := address.IDFromAddress(miner.ActorAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	var provingDeadline uint64 = 7
	if miner.IsImmutableDeadline(ctx, provingDeadline) {
		// avoid immutable deadlines
		provingDeadline = 5
	}

	submitAndFail := func(params *miner14.ProveCommitSectorsNIParams, errMsg string, errCode int) {
		t.Helper()
		r, err := miner.SubmitMessage(ctx, params, 1, builtin.MethodsMiner.ProveCommitSectorsNI)
		req.Error(err)
		req.Contains(err.Error(), errMsg)
		if errCode > 0 {
			req.Contains(err.Error(), fmt.Sprintf("(RetCode=%d)", errCode))
		}
		req.Nil(r)
	}

	sn := abi.SectorNumber(5000)
	mkSai := func() miner14.SectorNIActivationInfo {
		sn++
		return miner14.SectorNIActivationInfo{
			SealingNumber: sn,
			SealerID:      actorId,
			SealedCID:     cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz"),
			SectorNumber:  sn,
			SealRandEpoch: head.Height() - 10,
			Expiration:    2880 * 300,
		}
	}
	mkParams := func() miner14.ProveCommitSectorsNIParams {
		return miner14.ProveCommitSectorsNIParams{
			Sectors:                  []miner14.SectorNIActivationInfo{mkSai(), mkSai()},
			AggregateProof:           []byte{0xca, 0xfe, 0xbe, 0xef},
			SealProofType:            sealProofType,
			AggregateProofType:       abi.RegisteredAggregationProof_SnarkPackV2,
			ProvingDeadline:          provingDeadline,
			RequireActivationSuccess: true,
		}
	}

	// Test message rejection on no sectors
	params := mkParams()
	params.Sectors = []miner14.SectorNIActivationInfo{}
	submitAndFail(&params, "too few sectors", 16)

	// Test message rejection on too many sectors
	sectorInfos := make([]miner14.SectorNIActivationInfo, 66)
	for i := range sectorInfos {
		sectorInfos[i] = mkSai()
	}
	params = mkParams()
	params.Sectors = sectorInfos
	submitAndFail(&params, "too many sectors", 16)

	// Test bad aggregation proof type
	params = mkParams()
	params.AggregateProofType = abi.RegisteredAggregationProof_SnarkPackV1
	submitAndFail(&params, "aggregate proof type", 16)

	// Test bad SealerID
	params = mkParams()
	params.Sectors[1].SealerID = 1234
	submitAndFail(&params, "invalid NI commit 1 while requiring activation success", 16)

	// Test bad SealingNumber
	params = mkParams()
	params.Sectors[1].SealingNumber = 1234
	submitAndFail(&params, "invalid NI commit 1 while requiring activation success", 16)

	// Test bad SealedCID
	params = mkParams()
	params.Sectors[1].SealedCID = cid.MustParse("baga6ea4seaqjtovkwk4myyzj56eztkh5pzsk5upksan6f5outesy62bsvl4dsha")
	submitAndFail(&params, "invalid NI commit 1 while requiring activation success", 16)

	// Test bad SealRandEpoch
	head, err = miner.FullNode.ChainHead(ctx)
	req.NoError(err)
	params = mkParams()
	params.Sectors[1].SealRandEpoch = head.Height() + builtin.EpochsInDay
	submitAndFail(&params, fmt.Sprintf("seal challenge epoch %d must be before now", params.Sectors[1].SealRandEpoch), 16)
	params.Sectors[1].SealRandEpoch = head.Height() - 190*builtin.EpochsInDay
	submitAndFail(&params, "invalid NI commit 1 while requiring activation success", 16)

	// Immutable/bad deadlines
	di, err := miner.FullNode.StateMinerProvingDeadline(ctx, miner.ActorAddr, head.Key())
	req.NoError(err)
	currentDeadlineIdx := uint64(math.Abs(float64((di.CurrentEpoch - di.PeriodStart) / di.WPoStChallengeWindow)))
	req.Less(currentDeadlineIdx, di.WPoStPeriodDeadlines)
	params = mkParams()
	params.ProvingDeadline = currentDeadlineIdx
	submitAndFail(&params, fmt.Sprintf("proving deadline %d must not be the current or next deadline", currentDeadlineIdx), 18)
	params.ProvingDeadline = currentDeadlineIdx + 1
	submitAndFail(&params, fmt.Sprintf("proving deadline %d must not be the current or next deadline", currentDeadlineIdx+1), 18)
	params.ProvingDeadline = di.WPoStPeriodDeadlines // too big
	submitAndFail(&params, fmt.Sprintf("proving deadline index %d invalid", di.WPoStPeriodDeadlines), 16)
}
