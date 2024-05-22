package itests

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// Manually onboard CC sectors, bypassing lotus-miner onboarding pathways
func TestManualCCOnboarding(t *testing.T) {
	req := require.New(t)

	for _, withMockProofs := range []bool{true, false} {
		testName := "WithoutMockProofs"
		if withMockProofs {
			testName = "WithMockProofs"
		}
		t.Run(testName, func(t *testing.T) {
			kit.QuietMiningLogs()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var (
				blocktime = 2 * time.Millisecond

				client kit.TestFullNode
				minerA kit.TestMiner // A is a standard genesis miner
				minerB kit.TestMiner // B is a CC miner we will onboard manually

				bSectorNum = abi.SectorNumber(22)

				cacheDirPath                         string
				unsealedSectorPath, sealedSectorPath string
				sealedCid, unsealedCid               cid.Cid
				sealTickets                          abi.SealRandomness
			)

			// Setup and begin mining with a single miner (A)

			kitOpts := []kit.EnsembleOpt{}
			if withMockProofs {
				kitOpts = append(kitOpts, kit.MockProofs())
			}
			nodeOpts := []kit.NodeOpt{kit.WithAllSubsystems()}
			ens := kit.NewEnsemble(t, kitOpts...).
				FullNode(&client, nodeOpts...).
				Miner(&minerA, &client, nodeOpts...).
				Start().
				InterconnectAll()
			ens.BeginMining(blocktime)

			nodeOpts = append(nodeOpts, kit.OwnerAddr(client.DefaultKey))
			ens.Miner(&minerB, &client, nodeOpts...).Start()

			maddrA, err := minerA.ActorAddress(ctx)
			req.NoError(err)

			build.Clock.Sleep(time.Second)

			mAddrB, err := minerB.ActorAddress(ctx)
			req.NoError(err)
			mAddrBBytes := new(bytes.Buffer)
			req.NoError(mAddrB.MarshalCBOR(mAddrBBytes))

			head, err := client.ChainHead(ctx)
			req.NoError(err)

			minerBInfo, err := client.StateMinerInfo(ctx, mAddrB, head.Key())
			req.NoError(err)

			t.Log("Checking initial power ...")

			// Miner A should have power
			p, err := client.StateMinerPower(ctx, maddrA, head.Key())
			req.NoError(err)
			t.Logf("MinerA RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())

			// Miner B should have no power
			p, err = client.StateMinerPower(ctx, mAddrB, head.Key())
			req.NoError(err)
			t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
			req.True(p.MinerPower.RawBytePower.IsZero())

			// Run precommit for a sector on miner B

			sealRandEpoch := policy.SealRandomnessLookback
			t.Logf("Waiting for at least epoch %d for seal randomness (current epoch %d) ...", sealRandEpoch+5, head.Height())
			client.WaitTillChain(ctx, kit.HeightAtLeast(sealRandEpoch+5))

			if withMockProofs {
				sealedCid = cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz")
			} else {
				cacheDirPath = t.TempDir()
				tmpDir := t.TempDir()
				unsealedSectorPath = filepath.Join(tmpDir, "unsealed")
				sealedSectorPath = filepath.Join(tmpDir, "sealed")

				sealTickets, sealedCid, unsealedCid = manualOnboardingGeneratePreCommit(
					t,
					ctx,
					client,
					cacheDirPath,
					unsealedSectorPath,
					sealedSectorPath,
					mAddrB,
					bSectorNum,
					sealRandEpoch,
				)
			}

			t.Log("Submitting PreCommitSector ...")

			preCommitParams := &miner.PreCommitSectorBatchParams2{
				Sectors: []miner.SectorPreCommitInfo{{
					Expiration:    2880 * 300,
					SectorNumber:  22,
					SealProof:     kit.TestSpt,
					SealedCID:     sealedCid,
					SealRandEpoch: sealRandEpoch,
				}},
			}

			enc := new(bytes.Buffer)
			req.NoError(preCommitParams.MarshalCBOR(enc))

			m, err := client.MpoolPushMessage(ctx, &types.Message{
				To:     mAddrB,
				From:   minerB.OwnerKey.Address,
				Value:  types.FromFil(1),
				Method: builtin.MethodsMiner.PreCommitSectorBatch2,
				Params: enc.Bytes(),
			}, nil)
			req.NoError(err)

			r, err := client.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
			req.NoError(err)
			req.True(r.Receipt.ExitCode.IsSuccess())

			preCommitInfo, err := client.StateSectorPreCommitInfo(ctx, mAddrB, bSectorNum, r.TipSet)
			req.NoError(err)

			// Run prove commit for the sector on miner B

			seedRandomnessHeight := preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()
			t.Logf("Waiting %d epochs for seed randomness at epoch %d (current epoch %d)...", seedRandomnessHeight-r.Height, seedRandomnessHeight, r.Height)
			client.WaitTillChain(ctx, kit.HeightAtLeast(seedRandomnessHeight+5))

			var sectorProof []byte
			if withMockProofs {
				sectorProof = []byte{0xde, 0xad, 0xbe, 0xef}
			} else {
				sectorProof = manualOnboardingGenerateProveCommit(
					t,
					ctx,
					client,
					cacheDirPath,
					sealedSectorPath,
					mAddrB,
					bSectorNum,
					sealedCid,
					unsealedCid,
					sealTickets,
				)
			}

			t.Log("Submitting ProveCommitSector ...")

			proveCommitParams := miner14.ProveCommitSectors3Params{
				SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: bSectorNum}},
				SectorProofs:             [][]byte{sectorProof},
				RequireActivationSuccess: true,
			}

			enc = new(bytes.Buffer)
			req.NoError(proveCommitParams.MarshalCBOR(enc))

			m, err = client.MpoolPushMessage(ctx, &types.Message{
				To:     minerB.ActorAddr,
				From:   minerB.OwnerKey.Address,
				Value:  types.FromFil(0),
				Method: builtin.MethodsMiner.ProveCommitSectors3,
				Params: enc.Bytes(),
			}, nil)
			req.NoError(err)

			r, err = client.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
			req.NoError(err)
			req.True(r.Receipt.ExitCode.IsSuccess())

			// Check power after proving, should still be zero until the PoSt is submitted
			p, err = client.StateMinerPower(ctx, mAddrB, r.TipSet)
			req.NoError(err)
			t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
			req.True(p.MinerPower.RawBytePower.IsZero())

			// Fetch on-chain sector properties

			soi, err := client.StateSectorGetInfo(ctx, mAddrB, bSectorNum, r.TipSet)
			req.NoError(err)
			t.Logf("SectorOnChainInfo %d: %+v", bSectorNum, soi)

			sp, err := client.StateSectorPartition(ctx, mAddrB, bSectorNum, r.TipSet)
			req.NoError(err)
			t.Logf("SectorPartition %d: %+v", bSectorNum, sp)
			bSectorDeadline := sp.Deadline
			bSectorPartition := sp.Partition

			// Wait for the deadline to come around and submit a PoSt

			di, err := client.StateMinerProvingDeadline(ctx, mAddrB, types.EmptyTSK)
			req.NoError(err)
			t.Logf("MinerB Deadline Info: %+v", di)

			// Use the current deadline to work out when the deadline we care about (bSectorDeadline) is open
			// and ready to receive posts
			deadlineCount := di.WPoStPeriodDeadlines
			epochsPerDeadline := uint64(di.WPoStChallengeWindow)
			currentDeadline := di.Index
			currentDeadlineStart := di.Open
			waitTillEpoch := abi.ChainEpoch((deadlineCount-currentDeadline+bSectorDeadline)*epochsPerDeadline) + currentDeadlineStart + 1

			t.Logf("Waiting %d until epoch %d to get to deadline %d", waitTillEpoch-di.CurrentEpoch, waitTillEpoch, bSectorDeadline)
			head = client.WaitTillChain(ctx, kit.HeightAtLeast(waitTillEpoch))

			// We should be up to the deadline we care about
			di, err = client.StateMinerProvingDeadline(ctx, mAddrB, types.EmptyTSK)
			req.NoError(err)
			req.Equal(bSectorDeadline, di.Index, "should be in the deadline of the sector to prove")

			var proofBytes []byte
			if withMockProofs {
				proofBytes = []byte{0xde, 0xad, 0xbe, 0xef}
			} else {
				proofBytes = manualOnboardingGenerateWindowPost(t, ctx, client, cacheDirPath, sealedSectorPath, mAddrB, bSectorNum, sealedCid)
			}

			t.Log("Submitting WindowedPoSt...")

			rand, err := client.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, di.Open, nil, head.Key())
			req.NoError(err)

			postParams := miner.SubmitWindowedPoStParams{
				ChainCommitEpoch: di.Open,
				ChainCommitRand:  rand,
				Deadline:         bSectorDeadline,
				Partitions:       []miner.PoStPartition{{Index: bSectorPartition}},
				Proofs:           []proof.PoStProof{{PoStProof: minerBInfo.WindowPoStProofType, ProofBytes: proofBytes}},
			}

			enc = new(bytes.Buffer)
			req.NoError(postParams.MarshalCBOR(enc))

			m, err = client.MpoolPushMessage(ctx, &types.Message{
				To:     mAddrB,
				From:   minerB.OwnerKey.Address,
				Value:  types.NewInt(0),
				Method: builtin.MethodsMiner.SubmitWindowedPoSt,
				Params: enc.Bytes(),
			}, nil)
			req.NoError(err)

			r, err = client.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
			req.NoError(err)
			req.True(r.Receipt.ExitCode.IsSuccess())

			if !withMockProofs {
				// Dispute the PoSt to confirm the validity of the PoSt since PoSt acceptance is optimistic
				manualOnboardingDisputeWindowPost(t, ctx, client, mAddrB, bSectorNum)
			}

			t.Log("Checking power after PoSt ...")

			// Miner B should now have power
			p, err = client.StateMinerPower(ctx, mAddrB, r.TipSet)
			req.NoError(err)
			t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
			req.Equal(uint64(2<<10), p.MinerPower.RawBytePower.Uint64())    // 2kiB RBP
			req.Equal(uint64(2<<10), p.MinerPower.QualityAdjPower.Uint64()) // 2kiB QaP
		})
	}
}

func manualOnboardingGeneratePreCommit(
	t *testing.T,
	ctx context.Context,
	client api.FullNode,
	cacheDirPath,
	unsealedSectorPath,
	sealedSectorPath string,
	minerAddr address.Address,
	sectorNumber abi.SectorNumber,
	sealRandEpoch abi.ChainEpoch,
) (abi.SealRandomness, cid.Cid, cid.Cid) {

	req := require.New(t)
	t.Log("Generating PreCommit ...")

	sectorSize := abi.SectorSize(2 << 10)
	unsealedSize := abi.PaddedPieceSize(sectorSize).Unpadded()
	req.NoError(os.WriteFile(unsealedSectorPath, make([]byte, unsealedSize), 0644))
	req.NoError(os.WriteFile(sealedSectorPath, make([]byte, sectorSize), 0644))

	head, err := client.ChainHead(ctx)
	req.NoError(err)

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(minerAddr.MarshalCBOR(minerAddrBytes))

	rand, err := client.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_SealRandomness, sealRandEpoch, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	sealTickets := abi.SealRandomness(rand)

	t.Logf("Running SealPreCommitPhase1 for sector %d...", sectorNumber)

	actorIdNum, err := address.IDFromAddress(minerAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	pc1, err := ffi.SealPreCommitPhase1(
		kit.TestSpt,
		cacheDirPath,
		unsealedSectorPath,
		sealedSectorPath,
		sectorNumber,
		actorId,
		sealTickets,
		[]abi.PieceInfo{},
	)
	req.NoError(err)
	req.NotNil(pc1)

	t.Logf("Running SealPreCommitPhase2 for sector %d...", sectorNumber)

	sealedCid, unsealedCid, err := ffi.SealPreCommitPhase2(
		pc1,
		cacheDirPath,
		sealedSectorPath,
	)
	req.NoError(err)

	t.Logf("Unsealed CID: %s", unsealedCid)
	t.Logf("Sealed CID: %s", sealedCid)

	return sealTickets, sealedCid, unsealedCid
}

func manualOnboardingGenerateProveCommit(
	t *testing.T,
	ctx context.Context,
	client api.FullNode,
	cacheDirPath,
	sealedSectorPath string,
	minerAddr address.Address,
	sectorNumber abi.SectorNumber,
	sealedCid, unsealedCid cid.Cid,
	sealTickets abi.SealRandomness,
) []byte {
	req := require.New(t)

	t.Log("Generating Sector Proof ...")

	head, err := client.ChainHead(ctx)
	req.NoError(err)

	preCommitInfo, err := client.StateSectorPreCommitInfo(ctx, minerAddr, sectorNumber, head.Key())
	req.NoError(err)

	seedRandomnessHeight := preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(minerAddr.MarshalCBOR(minerAddrBytes))

	rand, err := client.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, seedRandomnessHeight, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	seedRandomness := abi.InteractiveSealRandomness(rand)

	actorIdNum, err := address.IDFromAddress(minerAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	t.Logf("Running SealCommitPhase1 for sector %d...", sectorNumber)

	scp1, err := ffi.SealCommitPhase1(
		kit.TestSpt,
		sealedCid,
		unsealedCid,
		cacheDirPath,
		sealedSectorPath,
		sectorNumber,
		actorId,
		sealTickets,
		seedRandomness,
		[]abi.PieceInfo{},
	)
	req.NoError(err)

	t.Logf("Running SealCommitPhase2 for sector %d...", sectorNumber)

	sectorProof, err := ffi.SealCommitPhase2(scp1, sectorNumber, actorId)
	req.NoError(err)

	return sectorProof
}

func manualOnboardingGenerateWindowPost(
	t *testing.T,
	ctx context.Context,
	client api.FullNode,
	cacheDirPath string,
	sealedSectorPath string,
	minerAddr address.Address,
	sectorNumber abi.SectorNumber,
	sealedCid cid.Cid,
) []byte {

	req := require.New(t)

	head, err := client.ChainHead(ctx)
	req.NoError(err)

	minerInfo, err := client.StateMinerInfo(ctx, minerAddr, head.Key())
	req.NoError(err)

	di, err := client.StateMinerProvingDeadline(ctx, minerAddr, types.EmptyTSK)
	req.NoError(err)

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(minerAddr.MarshalCBOR(minerAddrBytes))

	rand, err := client.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_WindowedPoStChallengeSeed, di.Challenge, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	postRand := abi.PoStRandomness(rand)
	postRand[31] &= 0x3f // make fr32 compatible

	privateSectorInfo := ffi.PrivateSectorInfo{
		SectorInfo: proof.SectorInfo{
			SealProof:    kit.TestSpt,
			SectorNumber: sectorNumber,
			SealedCID:    sealedCid,
		},
		CacheDirPath:     cacheDirPath,
		PoStProofType:    minerInfo.WindowPoStProofType,
		SealedSectorPath: sealedSectorPath,
	}

	actorIdNum, err := address.IDFromAddress(minerAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	windowProofs, faultySectors, err := ffi.GenerateWindowPoSt(actorId, ffi.NewSortedPrivateSectorInfo(privateSectorInfo), postRand)
	req.NoError(err)
	req.Len(faultySectors, 0)
	req.Len(windowProofs, 1)
	req.Equal(minerInfo.WindowPoStProofType, windowProofs[0].PoStProof)
	proofBytes := windowProofs[0].ProofBytes

	info := proof.WindowPoStVerifyInfo{
		Randomness:        postRand,
		Proofs:            []proof.PoStProof{{PoStProof: minerInfo.WindowPoStProofType, ProofBytes: proofBytes}},
		ChallengedSectors: []proof.SectorInfo{{SealProof: kit.TestSpt, SectorNumber: sectorNumber, SealedCID: sealedCid}},
		Prover:            actorId,
	}

	verified, err := ffi.VerifyWindowPoSt(info)
	req.NoError(err)
	req.True(verified, "window post verification failed")

	return proofBytes
}

func manualOnboardingDisputeWindowPost(
	t *testing.T,
	ctx context.Context,
	client kit.TestFullNode,
	minerAddr address.Address,
	sectorNumber abi.SectorNumber,
) {

	req := require.New(t)

	head, err := client.ChainHead(ctx)
	req.NoError(err)

	sp, err := client.StateSectorPartition(ctx, minerAddr, sectorNumber, head.Key())
	req.NoError(err)

	di, err := client.StateMinerProvingDeadline(ctx, minerAddr, head.Key())
	req.NoError(err)

	disputeEpoch := di.Challenge + miner14.WPoStDisputeWindow + 5
	t.Logf("Waiting %d until epoch %d to submit dispute", disputeEpoch-head.Height(), disputeEpoch)

	client.WaitTillChain(ctx, kit.HeightAtLeast(disputeEpoch))

	t.Logf("Disputing WindowedPoSt to confirm validity...")

	disputeParams := &miner14.DisputeWindowedPoStParams{Deadline: sp.Deadline, PoStIndex: 0}
	enc := new(bytes.Buffer)
	req.NoError(disputeParams.MarshalCBOR(enc))

	disputeMsg := &types.Message{
		To:     minerAddr,
		Method: builtin.MethodsMiner.DisputeWindowedPoSt,
		Params: enc.Bytes(),
		Value:  types.NewInt(0),
		From:   client.DefaultKey.Address,
	}

	_, err = client.MpoolPushMessage(ctx, disputeMsg, nil)
	req.Error(err, "expected dispute to fail")
	req.Contains(err.Error(), "failed to dispute valid post")
	req.Contains(err.Error(), "(RetCode=16)")
}
