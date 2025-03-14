package itests

import (
	"context"
	"fmt"
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
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

// All real-proofs test cases in here are skipped by default, gated with the LOTUS_RUN_VERY_EXPENSIVE_TESTS env var.
// Set this env var to "1" to run these tests.
// Some tests are also explicitly skipped with a `skipped` bool which can be manually commented out here and run.
// They are both very-very expensive, and are mainly useful to test very specific scenarios. You may need a -timeout
// of more than 10m to run these.

func TestManualNISectorOnboarding(t *testing.T) {
	req := require.New(t)

	const blocktime = 2 * time.Millisecond
	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
	sealProofType, err := miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version23, miner.SealProofVariant_NonInteractive)
	req.NoError(err)

	mkgood := func(c int) []exitcode.ExitCode {
		out := make([]exitcode.ExitCode, c)
		for i := range out {
			out[i] = exitcode.Ok
		}
		return out
	}

	type testCaseMiner struct {
		// sectorsToOnboard tells us (a) how many sectors to prepare and (b) how many of them should pass the onboarding process
		// with an Ok exit code
		sectorsToOnboard []exitcode.ExitCode
		// allOrNothing tells us whether all sectors should pass onboarding or none
		allOrNothing bool
		expectPower  uint64
		// snapDeal tells us whether to snap a deal into one of the sectors after activation
		snapDeal bool
	}

	testCases := []struct {
		name       string
		skip       bool
		mockProofs bool
		miners     []testCaseMiner
	}{
		{
			name:       "mock proofs, miners with 1, 3 (1 bad) and 6 sectors",
			mockProofs: true,
			miners: []testCaseMiner{
				{
					sectorsToOnboard: mkgood(1),
					allOrNothing:     true,
					expectPower:      uint64(defaultSectorSize),
					snapDeal:         true,
				},
				{
					sectorsToOnboard: []exitcode.ExitCode{exitcode.Ok, exitcode.ErrIllegalArgument, exitcode.Ok},
					allOrNothing:     false,
					expectPower:      uint64(defaultSectorSize * 2),
					snapDeal:         true,
				},
				{
					// should trigger a non-zero aggregate fee (>5 for niporep)
					sectorsToOnboard: mkgood(6),
					allOrNothing:     true,
					expectPower:      uint64(defaultSectorSize * 6),
					snapDeal:         true,
				},
			},
		},
		{
			name:       "mock proofs, 1 miner with 65 sectors",
			mockProofs: true,
			miners: []testCaseMiner{
				{
					sectorsToOnboard: mkgood(65),
					allOrNothing:     true,
					expectPower:      uint64(defaultSectorSize * 65),
					snapDeal:         true,
				},
			},
		},
		{
			name:       "real proofs, 1 miner with 1 sector",
			mockProofs: false,
			skip:       true, // uncomment if you want to run this test manually
			miners: []testCaseMiner{
				{
					sectorsToOnboard: mkgood(1),
					allOrNothing:     true,
					expectPower:      uint64(defaultSectorSize),
					snapDeal:         true,
				},
			},
		},
		{
			name:       "real proofs, 1 miner with 2 sectors",
			mockProofs: false,
			skip:       true, // uncomment if you want to run this test manually
			miners: []testCaseMiner{
				{
					sectorsToOnboard: mkgood(2),
					allOrNothing:     true,
					expectPower:      uint64(defaultSectorSize * 2),
					snapDeal:         true,
				},
			},
		},
		{
			name:       "real proofs, 1 miner with 3 sectors",
			mockProofs: false,
			skip:       true, // uncomment if you want to run this test manually
			miners: []testCaseMiner{
				{
					sectorsToOnboard: mkgood(3),
					allOrNothing:     true,
					expectPower:      uint64(defaultSectorSize * 3),
					snapDeal:         true,
				},
			},
		},
		{
			name:       "real proofs, 1 miner with 4 sectors",
			mockProofs: false,
			skip:       true, // uncomment if you want to run this test manually
			miners: []testCaseMiner{
				{
					sectorsToOnboard: mkgood(4),
					allOrNothing:     true,
					expectPower:      uint64(defaultSectorSize * 4),
					snapDeal:         true,
				},
			},
		},
		{
			// useful for testing aggregate fee
			// "aggregate seal verify failed: invalid aggregate"
			name:       "real proofs, 1 miner with 7 sectors",
			mockProofs: false,
			skip:       true, // uncomment if you want to run this test manually
			miners: []testCaseMiner{
				{
					sectorsToOnboard: mkgood(7),
					allOrNothing:     true,
					expectPower:      uint64(defaultSectorSize * 7),
					snapDeal:         true,
				},
			},
		},
		{
			// useful for testing aggregate fee and successful activation with the aggregate proof even
			// though one of them is bad
			name:       "real proofs, 1 miner with 7 sectors, 1 bad",
			mockProofs: false,
			miners: []testCaseMiner{
				{
					sectorsToOnboard: []exitcode.ExitCode{exitcode.Ok, exitcode.ErrIllegalArgument, exitcode.Ok, exitcode.Ok, exitcode.Ok, exitcode.Ok, exitcode.Ok},
					allOrNothing:     false,
					expectPower:      uint64(defaultSectorSize * 6),
					snapDeal:         true,
				},
			},
		},
		{
			name:       "real proofs, 1 miner with 65 sectors",
			mockProofs: false,
			skip:       true, // uncomment if you want to run this test manually
			miners: []testCaseMiner{
				{
					sectorsToOnboard: mkgood(65),
					allOrNothing:     true,
					expectPower:      uint64(defaultSectorSize * 65),
					snapDeal:         true,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("skipping test")
			}
			if !tc.mockProofs {
				kit.VeryExpensive(t)
			}
			req := require.New(t)

			kit.QuietMiningLogs()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var (
				client       kit.TestFullNode
				genesisMiner kit.TestMiner
			)

			// Setup and begin mining with a single genesis block miner, this miner won't be used
			// in the test beyond mining the chain and maintaining power
			ens := kit.NewEnsemble(t, kit.MockProofs(tc.mockProofs)).
				FullNode(&client, kit.SectorSize(defaultSectorSize)).
				// preseal more than the default number of sectors to ensure that the genesis miner has power
				// because our unmanaged miners won't produce blocks so we may get null rounds
				Miner(&genesisMiner, &client, kit.PresealSectors(5), kit.SectorSize(defaultSectorSize), kit.WithAllSubsystems()).
				Start().
				InterconnectAll()
			blockMiners := ens.BeginMiningMustPost(blocktime)
			req.Len(blockMiners, 1)
			blockMiner := blockMiners[0]

			// Instantiate our test miners to manually handle sector onboarding and power acquisition through sector activation.
			nodeOpts := []kit.NodeOpt{kit.SectorSize(defaultSectorSize), kit.OwnerAddr(client.DefaultKey)}
			miners := make([]*kit.TestUnmanagedMiner, len(tc.miners))
			for i := range tc.miners {
				miners[i], _ = ens.UnmanagedMiner(ctx, &client, nodeOpts...)
				defer miners[i].Stop()
			}

			ens.Start()

			build.Clock.Sleep(time.Second)

			t.Log("Checking initial power ...")

			// The genesis miner A should have power as it has already onboarded sectors in the genesis block
			head, err := client.ChainHead(ctx)
			req.NoError(err)
			p, err := client.StateMinerPower(ctx, genesisMiner.ActorAddr, head.Key())
			req.NoError(err)
			t.Logf("genesisMiner RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())

			// Our test miners should have no power as they have yet to onboard and activate any sectors
			for _, miner := range miners {
				miner.AssertNoPower()
			}

			sectors := make([][]abi.SectorNumber, len(tc.miners))

			for i, tcMiner := range tc.miners {
				miner := miners[i]
				var expectSuccesses int
				for _, ec := range tcMiner.sectorsToOnboard {
					if ec.IsSuccess() {
						expectSuccesses++
					}
				}

				head, err = client.ChainHead(ctx)
				req.NoError(err)

				// Onboard CC sectors to this test miner using NI-PoRep
				sectors[i], _ = miner.OnboardSectors(
					sealProofType,
					kit.NewSectorBatch().AddEmptySectors(len(tcMiner.sectorsToOnboard)),
					kit.WithExpectedExitCodes(tcMiner.sectorsToOnboard),
					kit.WithRequireActivationSuccess(tcMiner.allOrNothing),
					kit.WithModifyNIActivationsBeforeSubmit(func(activations []miner14.SectorNIActivationInfo) []miner14.SectorNIActivationInfo {
						head, err := client.ChainHead(ctx)
						req.NoError(err)

						for j, ec := range tcMiner.sectorsToOnboard {
							if !ec.IsSuccess() {
								// Set the expiration in the past to ensure the sector fails onboarding
								activations[j].Expiration = head.Height() - 100
							}
						}
						return activations
					}),
				)

				req.Len(sectors[i], expectSuccesses)

				// Miner B should still not have power as power can only be gained after sector is activated i.e. the first WindowPost is submitted for it
				miner.AssertNoPower()

				// Check that the sector-activated event was emitted
				{
					expectedEntries := make([][]types.EventEntry, 0)
					for _, sectorNumber := range sectors[i] {
						expectedEntries = append(expectedEntries, []types.EventEntry{
							{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: must.One(ipld.Encode(basicnode.NewString("sector-activated"), dagcbor.Encode))},
							{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "sector", Value: must.One(ipld.Encode(basicnode.NewInt(int64(sectorNumber)), dagcbor.Encode))},
							{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "unsealed-cid", Value: must.One(ipld.Encode(datamodel.Null, dagcbor.Encode))},
						})
					}
					from := head.Height()
					recentEvents, err := client.FullNode.GetActorEventsRaw(ctx, &types.ActorEventFilter{FromHeight: &from})
					req.NoError(err)
					req.Len(recentEvents, len(sectors[i]))
					for i, event := range recentEvents {
						req.Equal(expectedEntries[i], event.Entries)
					}
				}

				// Ensure that the block miner checks for and waits for posts during the appropriate proving window from our new miner with a sector
				blockMiner.WatchMinerForPost(miner.ActorAddr)
			}

			// Wait till each miners' sectors have had their first post and are activated and check that this is reflected in miner power
			for i, miner := range miners {
				miner.WaitTillActivatedAndAssertPower(sectors[i], tc.miners[i].expectPower, tc.miners[i].expectPower)
			}

			for i, tcMiner := range tc.miners {
				if !tcMiner.snapDeal {
					continue
				}

				miner := miners[i]
				head, err = client.ChainHead(ctx)
				req.NoError(err)

				// Snap a deal into the first of the successfully onboarded CC sectors for this miner
				snapPieces, _ := miner.SnapDeal(sectors[i][0], kit.SectorWithPiece(kit.BogusPieceCid2))

				// Check "sector-updated" event happned after snap
				{
					expectedEntries := []types.EventEntry{
						{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: must.One(ipld.Encode(basicnode.NewString("sector-updated"), dagcbor.Encode))},
						{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "sector", Value: must.One(ipld.Encode(basicnode.NewInt(int64(sectors[i][0])), dagcbor.Encode))},
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
			}
		})
	}
}

func TestNISectorFailureCases(t *testing.T) {
	req := require.New(t)

	const blocktime = 2 * time.Millisecond
	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
	sealProofType, err := miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version23, miner.SealProofVariant_NonInteractive)
	req.NoError(err)

	kit.QuietMiningLogs()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		client       kit.TestFullNode
		genesisMiner kit.TestMiner
	)
	ens := kit.NewEnsemble(t, kit.MockProofs(true)).
		FullNode(&client, kit.SectorSize(defaultSectorSize)).
		Miner(&genesisMiner, &client, kit.PresealSectors(5), kit.SectorSize(defaultSectorSize), kit.WithAllSubsystems()).
		Start().
		InterconnectAll()
	_ = ens.BeginMining(blocktime)

	miner, _ := ens.UnmanagedMiner(ctx, &client, kit.SectorSize(defaultSectorSize), kit.OwnerAddr(client.DefaultKey))
	defer miner.Stop()

	ens.Start()

	build.Clock.Sleep(time.Second)

	// We have to onboard a sector first to get the miner enrolled in cron; although we don't need to wait for it to prove
	_, _ = miner.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))

	// Utility functions and variables for our failure cases

	actorIdNum, err := address.IDFromAddress(miner.ActorAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	// make sure we are working with a deadline that has no chance of being immutable (current or next)
	di, err := miner.FullNode.StateMinerProvingDeadline(ctx, miner.ActorAddr, types.EmptyTSK)
	req.NoError(err)
	currentDeadlineIdx := kit.CurrentDeadlineIndex(di)
	provingDeadline := currentDeadlineIdx - 2
	if currentDeadlineIdx <= 2 {
		provingDeadline = miner14.WPoStPeriodDeadlines - 1
	}

	head, err := miner.FullNode.ChainHead(ctx)
	req.NoError(err)

	sectorNumber := abi.SectorNumber(5000)

	submitAndFail := func(params *miner14.ProveCommitSectorsNIParams, errMsg string, errCode int) {
		t.Helper()
		r, err := miner.SubmitMessage(params, 1, builtin.MethodsMiner.ProveCommitSectorsNI)
		req.Error(err, "expected error: [%s]", errMsg)
		req.Contains(err.Error(), errMsg)
		if errCode > 0 {
			req.Contains(err.Error(), fmt.Sprintf("(RetCode=%d)", errCode))
		}
		req.Nil(r)
	}
	mkSai := func() miner14.SectorNIActivationInfo {
		sectorNumber++ // unique per sector
		return miner14.SectorNIActivationInfo{
			SealingNumber: sectorNumber,
			SealerID:      actorId,
			SealedCID:     cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz"),
			SectorNumber:  sectorNumber,
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

	// Failure cases

	t.Run("message rejection on no sectors", func(t *testing.T) {
		params := mkParams()
		params.Sectors = []miner14.SectorNIActivationInfo{}
		submitAndFail(&params, "too few sectors", 16)
	})

	t.Run("message rejection on too many sectors", func(t *testing.T) {
		sectorInfos := make([]miner14.SectorNIActivationInfo, 66)
		for i := range sectorInfos {
			sectorInfos[i] = mkSai()
		}
		params := mkParams()
		params.Sectors = sectorInfos
		submitAndFail(&params, "too many sectors", 16)
	})

	t.Run("bad aggregation proof type", func(t *testing.T) {
		params := mkParams()
		params.AggregateProofType = abi.RegisteredAggregationProof_SnarkPackV1
		submitAndFail(&params, "aggregate proof type", 16)
	})

	t.Run("bad SealerID", func(t *testing.T) {
		params := mkParams()
		params.Sectors[1].SealerID = 1234
		submitAndFail(&params, "invalid NI commit 1 while requiring activation success", 16)
	})

	t.Run("bad SealingNumber", func(t *testing.T) {
		params := mkParams()
		params.Sectors[1].SealingNumber = 1234
		submitAndFail(&params, "invalid NI commit 1 while requiring activation success", 16)
	})

	t.Run("bad SealedCID", func(t *testing.T) {
		params := mkParams()
		params.Sectors[1].SealedCID = kit.BogusPieceCid1
		submitAndFail(&params, "invalid NI commit 1 while requiring activation success", 16)
	})

	t.Run("bad SealRandEpoch", func(t *testing.T) {
		head, err = miner.FullNode.ChainHead(ctx)
		req.NoError(err)
		params := mkParams()
		params.Sectors[1].SealRandEpoch = head.Height() + builtin.EpochsInDay
		submitAndFail(&params, fmt.Sprintf("seal challenge epoch %d must be before now", params.Sectors[1].SealRandEpoch), 16)
		params.Sectors[1].SealRandEpoch = head.Height() - miner14.MaxProveCommitNiLookback - builtin.EpochsInDay
		submitAndFail(&params, "invalid NI commit 1 while requiring activation success", 16)
	})

	t.Run("immutable deadlines", func(t *testing.T) {
		di, err = miner.FullNode.StateMinerProvingDeadline(ctx, miner.ActorAddr, head.Key())
		req.NoError(err)
		currentDeadlineIdx = kit.CurrentDeadlineIndex(di)

		t.Logf("Validating submission failure for current and next deadline. Current Deadline Info: %+v, calculated current deadline: %d.", di, currentDeadlineIdx)

		params := mkParams()
		params.ProvingDeadline = currentDeadlineIdx
		submitAndFail(&params, fmt.Sprintf("proving deadline %d must not be the current or next deadline", currentDeadlineIdx), 18)
		params.ProvingDeadline = currentDeadlineIdx + 1
		if params.ProvingDeadline == di.WPoStPeriodDeadlines {
			params.ProvingDeadline = 0
		}
		msgdline := currentDeadlineIdx + 1
		if msgdline == di.WPoStPeriodDeadlines {
			msgdline = 0
		}
		submitAndFail(&params, fmt.Sprintf("proving deadline %d must not be the current or next deadline", msgdline), 18)
		params.ProvingDeadline = di.WPoStPeriodDeadlines // too big
		submitAndFail(&params, fmt.Sprintf("proving deadline index %d invalid", di.WPoStPeriodDeadlines), 16)
	})
}
