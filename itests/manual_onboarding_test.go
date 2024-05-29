package itests

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

const sectorSize = abi.SectorSize(2 << 10) // 2KiB

// Manually onboard CC sectors, bypassing lotus-miner onboarding pathways
func TestManualCCOnboarding(t *testing.T) {
	req := require.New(t)

	for _, withMockProofs := range []bool{true, false} {
		testName := "WithRealProofs"
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
				minerA kit.TestMiner          // A is a standard genesis miner
				minerB kit.TestUnmanagedMiner // B is a CC miner we will onboard manually
				minerC kit.TestUnmanagedMiner // C is a CC miner we will onboard manually with NI-PoRep

				// TODO: single sector per miner for now, but this isn't going to scale when refactored into
				// TestUnmanagedMiner - they'll need their own list of sectors and their own copy of each of
				// the things below, including per-sector maps of some of these too.
				//
				// Misc thoughts:
				// Each TestUnmanagedMiner should have its own temp dir, within which it can have a cache dir
				// and a place to put sealed and unsealed sectors. We can't share these between miners.
				// We should have a way to "add" CC sectors, which will setup the sealed and unsealed files
				// and can move many of the manualOnboarding*() methods into the TestUnmanagedMiner struct.
				//
				// The manualOnboardingRunWindowPost() Go routine should be owned by TestUnmanagedMiner and
				// a simple "miner.StartWindowPost()" should suffice to make it observe all of the sectors
				// it knows about and start posting for them. We should be able to ignore most (all?) of the
				// special cases that lotus-miner currently has to deal with.

				// sector numbers, make them unique for each miner so our maps work
				bSectorNum = abi.SectorNumber(22)
				cSectorNum = abi.SectorNumber(33)

				tmpDir = t.TempDir()

				cacheDirPath                         = map[abi.SectorNumber]string{} // can't share a cacheDir between miners
				unsealedSectorPath, sealedSectorPath = map[abi.SectorNumber]string{}, map[abi.SectorNumber]string{}
				sealedCid, unsealedCid               = map[abi.SectorNumber]cid.Cid{}, map[abi.SectorNumber]cid.Cid{}

				// note we'll use the same randEpoch for both miners
				sealRandEpoch = policy.SealRandomnessLookback
				sealTickets   = map[abi.SectorNumber]abi.SealRandomness{}
			)

			// Setup and begin mining with a single miner (A)

			kitOpts := []kit.EnsembleOpt{}
			if withMockProofs {
				kitOpts = append(kitOpts, kit.MockProofs())
			}
			nodeOpts := []kit.NodeOpt{kit.SectorSize(sectorSize), kit.WithAllSubsystems()}
			ens := kit.NewEnsemble(t, kitOpts...).
				FullNode(&client, nodeOpts...).
				Miner(&minerA, &client, nodeOpts...).
				Start().
				InterconnectAll()
			ens.BeginMining(blocktime)

			nodeOpts = append(nodeOpts, kit.OwnerAddr(client.DefaultKey))
			ens.UnmanagedMiner(&minerB, &client, nodeOpts...).Start()
			ens.UnmanagedMiner(&minerC, &client, nodeOpts...).Start()

			build.Clock.Sleep(time.Second)

			head, err := client.ChainHead(ctx)
			req.NoError(err)

			t.Log("Checking initial power ...")

			// Miner A should have power
			p, err := client.StateMinerPower(ctx, minerA.ActorAddr, head.Key())
			req.NoError(err)
			t.Logf("MinerA RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())

			// Miner B should have no power
			p, err = client.StateMinerPower(ctx, minerB.ActorAddr, head.Key())
			req.NoError(err)
			t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
			req.True(p.MinerPower.RawBytePower.IsZero())

			// Miner C should have no power
			p, err = client.StateMinerPower(ctx, minerC.ActorAddr, head.Key())
			req.NoError(err)
			t.Logf("MinerC RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
			req.True(p.MinerPower.RawBytePower.IsZero())

			// Run precommit for a sector on miner B

			t.Logf("Waiting for at least epoch %d for seal randomness (current epoch %d) ...", sealRandEpoch+5, head.Height())
			client.WaitTillChain(ctx, kit.HeightAtLeast(sealRandEpoch+5))

			if withMockProofs {
				sealedCid[bSectorNum] = cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz")
			} else {
				cacheDirPath[bSectorNum] = filepath.Join(tmpDir, "cacheb")
				unsealedSectorPath[bSectorNum] = filepath.Join(tmpDir, "unsealedb")
				sealedSectorPath[bSectorNum] = filepath.Join(tmpDir, "sealedb")

				sealTickets[bSectorNum], sealedCid[bSectorNum], unsealedCid[bSectorNum] = manualOnboardingGeneratePreCommit(
					ctx,
					t,
					client,
					cacheDirPath[bSectorNum],
					unsealedSectorPath[bSectorNum],
					sealedSectorPath[bSectorNum],
					minerB.ActorAddr,
					bSectorNum,
					sealRandEpoch,
					kit.TestSpt,
				)
			}

			t.Log("Submitting MinerB PreCommitSector ...")

			r, err := manualOnboardingSubmitMessage(ctx, client, minerB, &miner14.PreCommitSectorBatchParams2{
				Sectors: []miner14.SectorPreCommitInfo{{
					Expiration:    2880 * 300,
					SectorNumber:  bSectorNum,
					SealProof:     kit.TestSpt,
					SealedCID:     sealedCid[bSectorNum],
					SealRandEpoch: sealRandEpoch,
				}},
			}, 1, builtin.MethodsMiner.PreCommitSectorBatch2)
			req.NoError(err)
			req.True(r.Receipt.ExitCode.IsSuccess())

			preCommitInfo, err := client.StateSectorPreCommitInfo(ctx, minerB.ActorAddr, bSectorNum, r.TipSet)
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
					ctx,
					t,
					client,
					cacheDirPath[bSectorNum],
					sealedSectorPath[bSectorNum],
					minerB.ActorAddr,
					bSectorNum,
					sealedCid[bSectorNum],
					unsealedCid[bSectorNum],
					sealTickets[bSectorNum],
					kit.TestSpt,
				)
			}

			t.Log("Submitting MinerB ProveCommitSector ...")

			r, err = manualOnboardingSubmitMessage(ctx, client, minerB, &miner14.ProveCommitSectors3Params{
				SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: bSectorNum}},
				SectorProofs:             [][]byte{sectorProof},
				RequireActivationSuccess: true,
			}, 0, builtin.MethodsMiner.ProveCommitSectors3)
			req.NoError(err)
			req.True(r.Receipt.ExitCode.IsSuccess())

			// Check power after proving, should still be zero until the PoSt is submitted
			p, err = client.StateMinerPower(ctx, minerB.ActorAddr, r.TipSet)
			req.NoError(err)
			t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
			req.True(p.MinerPower.RawBytePower.IsZero())

			// start a background PoST scheduler for miner B
			bFirstCh, bErrCh := manualOnboardingRunWindowPost(
				ctx,
				withMockProofs,
				client,
				minerB,
				bSectorNum,
				cacheDirPath[bSectorNum],
				sealedSectorPath[bSectorNum],
				sealedCid[bSectorNum],
				kit.TestSpt,
			)

			// NI-PoRep

			if withMockProofs {
				sectorProof = []byte{0xde, 0xad, 0xbe, 0xef}
				sealedCid[cSectorNum] = cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz")
			} else {
				// NI-PoRep compresses precommit and commit into one step
				cacheDirPath[cSectorNum] = filepath.Join(tmpDir, "cachec")
				unsealedSectorPath[cSectorNum] = filepath.Join(tmpDir, "unsealedc")
				sealedSectorPath[cSectorNum] = filepath.Join(tmpDir, "sealedc")

				sealTickets[cSectorNum], sealedCid[cSectorNum], unsealedCid[cSectorNum] = manualOnboardingGeneratePreCommit(
					ctx,
					t,
					client,
					cacheDirPath[cSectorNum],
					unsealedSectorPath[cSectorNum],
					sealedSectorPath[cSectorNum],
					minerC.ActorAddr,
					cSectorNum,
					sealRandEpoch,
					kit.TestSptNi,
				)

				sectorProof = manualOnboardingGenerateProveCommit(
					ctx,
					t,
					client,
					cacheDirPath[cSectorNum],
					sealedSectorPath[cSectorNum],
					minerC.ActorAddr,
					cSectorNum,
					sealedCid[cSectorNum],
					unsealedCid[cSectorNum],
					sealTickets[cSectorNum],
					kit.TestSptNi,
				)
			}

			t.Log("Submitting MinerC ProveCommitSectorsNI ...")

			actorIdNum, err := address.IDFromAddress(minerC.ActorAddr)
			req.NoError(err)
			actorId := abi.ActorID(actorIdNum)

			r, err = manualOnboardingSubmitMessage(ctx, client, minerC, &miner14.ProveCommitSectorsNIParams{
				Sectors: []miner14.SectorNIActivationInfo{{
					SealingNumber: cSectorNum,
					SealerID:      actorId,
					SectorNumber:  cSectorNum,
					SealedCID:     sealedCid[cSectorNum],
					SealRandEpoch: sealRandEpoch,
					Expiration:    2880 * 300,
				}},
				SealProofType:            kit.TestSptNi,
				SectorProofs:             [][]byte{sectorProof},
				RequireActivationSuccess: true,
			}, 1, builtin.MethodsMiner.ProveCommitSectorsNI)
			req.NoError(err)
			req.True(r.Receipt.ExitCode.IsSuccess())

			// start a background PoST scheduler for miner C
			cFirstCh, cErrCh := manualOnboardingRunWindowPost(
				ctx,
				withMockProofs,
				client,
				minerC,
				cSectorNum,
				cacheDirPath[cSectorNum],
				sealedSectorPath[cSectorNum],
				sealedCid[cSectorNum],
				kit.TestSptNi,
			)

			checkPostSchedulers := func() {
				t.Helper()
				select {
				case err, ok := <-bErrCh:
					if ok {
						t.Fatalf("Received error from Miner B PoST scheduler: %v", err)
					}
				case err, ok := <-cErrCh:
					if ok {
						t.Fatalf("Received error from Miner C PoST scheduler: %v", err)
					}
				default:
				}
			}

			isClosed := func(ch <-chan struct{}) bool {
				select {
				case <-ch:
					return true
				default:
				}
				return false
			}

			for ctx.Err() == nil {
				checkPostSchedulers()
				// wait till the first PoST is submitted for both by checking if both bFirstCh and cFirstCh are closed, if so, break, otherwise sleep for 500ms and check again
				if isClosed(bFirstCh) && isClosed(cFirstCh) {
					break
				}
				t.Log("Waiting for first PoST to be submitted for all miners ...")
				select {
				case <-time.After(2 * time.Second):
				case <-ctx.Done():
					t.Fatal("Context cancelled")
				}
			}

			// Fetch on-chain sector properties

			soi, err := client.StateSectorGetInfo(ctx, minerB.ActorAddr, bSectorNum, r.TipSet)
			req.NoError(err)
			t.Logf("Miner B SectorOnChainInfo %d: %+v", bSectorNum, soi)
			soi, err = client.StateSectorGetInfo(ctx, minerC.ActorAddr, cSectorNum, r.TipSet)
			req.NoError(err)
			t.Logf("Miner C SectorOnChainInfo %d: %+v", cSectorNum, soi)

			head, err = client.ChainHead(ctx)
			req.NoError(err)

			head = client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+5))

			checkPostSchedulers()

			t.Log("Checking power after PoSt ...")

			// Miner B should now have power
			p, err = client.StateMinerPower(ctx, minerB.ActorAddr, head.Key())
			req.NoError(err)
			t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
			req.Equal(uint64(2<<10), p.MinerPower.RawBytePower.Uint64())    // 2kiB RBP
			req.Equal(uint64(2<<10), p.MinerPower.QualityAdjPower.Uint64()) // 2kiB QaP

			// Miner C should now have power
			p, err = client.StateMinerPower(ctx, minerC.ActorAddr, head.Key())
			req.NoError(err)
			t.Logf("MinerC RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
			req.Equal(uint64(2<<10), p.MinerPower.RawBytePower.Uint64())    // 2kiB RBP
			req.Equal(uint64(2<<10), p.MinerPower.QualityAdjPower.Uint64()) // 2kiB QaP

			checkPostSchedulers()
		})
	}
}

func manualOnboardingGeneratePreCommit(
	ctx context.Context,
	t *testing.T,
	client api.FullNode,
	cacheDirPath,
	unsealedSectorPath,
	sealedSectorPath string,
	minerAddr address.Address,
	sectorNumber abi.SectorNumber,
	sealRandEpoch abi.ChainEpoch,
	proofType abi.RegisteredSealProof,
) (abi.SealRandomness, cid.Cid, cid.Cid) {

	req := require.New(t)
	t.Logf("Generating proof type %d PreCommit ...", proofType)

	_ = os.Mkdir(cacheDirPath, 0755)
	unsealedSize := abi.PaddedPieceSize(sectorSize).Unpadded()
	req.NoError(os.WriteFile(unsealedSectorPath, make([]byte, unsealedSize), 0644))
	req.NoError(os.WriteFile(sealedSectorPath, make([]byte, sectorSize), 0644))

	t.Logf("Wrote unsealed sector to %s", unsealedSectorPath)
	t.Logf("Wrote sealed sector to %s", sealedSectorPath)

	head, err := client.ChainHead(ctx)
	req.NoError(err)

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(minerAddr.MarshalCBOR(minerAddrBytes))

	rand, err := client.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_SealRandomness, sealRandEpoch, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	sealTickets := abi.SealRandomness(rand)

	t.Logf("Running proof type %d SealPreCommitPhase1 for sector %d...", proofType, sectorNumber)

	actorIdNum, err := address.IDFromAddress(minerAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	pc1, err := ffi.SealPreCommitPhase1(
		proofType,
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

	t.Logf("Running proof type %d SealPreCommitPhase2 for sector %d...", proofType, sectorNumber)

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
	ctx context.Context,
	t *testing.T,
	client api.FullNode,
	cacheDirPath,
	sealedSectorPath string,
	minerAddr address.Address,
	sectorNumber abi.SectorNumber,
	sealedCid, unsealedCid cid.Cid,
	sealTickets abi.SealRandomness,
	proofType abi.RegisteredSealProof,
) []byte {
	req := require.New(t)

	t.Logf("Generating proof type %d Sector Proof ...", proofType)

	head, err := client.ChainHead(ctx)
	req.NoError(err)

	var seedRandomnessHeight abi.ChainEpoch

	if proofType >= abi.RegisteredSealProof_StackedDrg2KiBV1_2_Feat_NiPoRep && proofType <= abi.RegisteredSealProof_StackedDrg64GiBV1_2_Feat_NiPoRep {
		// this just needs to be somewhere between 6 months and chain finality for NI-PoRep,
		// and there's no PreCommitInfo because it's non-interactive!
		seedRandomnessHeight = head.Height() - policy.ChainFinality
	} else {
		preCommitInfo, err := client.StateSectorPreCommitInfo(ctx, minerAddr, sectorNumber, head.Key())
		req.NoError(err)
		seedRandomnessHeight = preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()
	}

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(minerAddr.MarshalCBOR(minerAddrBytes))

	rand, err := client.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, seedRandomnessHeight, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	seedRandomness := abi.InteractiveSealRandomness(rand)

	actorIdNum, err := address.IDFromAddress(minerAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	t.Logf("Running proof type %d SealCommitPhase1 for sector %d...", proofType, sectorNumber)

	scp1, err := ffi.SealCommitPhase1(
		proofType,
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

	t.Logf("Running proof type %d SealCommitPhase2 for sector %d...", proofType, sectorNumber)

	sectorProof, err := ffi.SealCommitPhase2(scp1, sectorNumber, actorId)
	req.NoError(err)

	t.Logf("Got proof type %d sector proof of length %d", proofType, len(sectorProof))

	/* this variant would be used for aggregating NI-PoRep proofs
	sectorProof, err := ffi.SealCommitPhase2CircuitProofs(scp1, sectorNumber)
	req.NoError(err)
	*/

	return sectorProof
}

func manualOnboardingGenerateWindowPost(
	ctx context.Context,
	client api.FullNode,
	cacheDirPath string,
	sealedSectorPath string,
	minerAddr address.Address,
	sectorNumber abi.SectorNumber,
	sealedCid cid.Cid,
	proofType abi.RegisteredSealProof,
) ([]byte, error) {

	head, err := client.ChainHead(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain head: %w", err)
	}

	minerInfo, err := client.StateMinerInfo(ctx, minerAddr, head.Key())
	if err != nil {
		return nil, fmt.Errorf("failed to get miner info: %w", err)
	}

	di, err := client.StateMinerProvingDeadline(ctx, minerAddr, types.EmptyTSK)
	if err != nil {
		return nil, fmt.Errorf("failed to get proving deadline: %w", err)
	}

	minerAddrBytes := new(bytes.Buffer)
	if err := minerAddr.MarshalCBOR(minerAddrBytes); err != nil {
		return nil, fmt.Errorf("failed to marshal miner address: %w", err)
	}

	rand, err := client.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_WindowedPoStChallengeSeed, di.Challenge, minerAddrBytes.Bytes(), head.Key())
	if err != nil {
		return nil, fmt.Errorf("failed to get randomness: %w", err)
	}
	postRand := abi.PoStRandomness(rand)
	postRand[31] &= 0x3f // make fr32 compatible

	privateSectorInfo := ffi.PrivateSectorInfo{
		SectorInfo: proof.SectorInfo{
			SealProof:    proofType,
			SectorNumber: sectorNumber,
			SealedCID:    sealedCid,
		},
		CacheDirPath:     cacheDirPath,
		PoStProofType:    minerInfo.WindowPoStProofType,
		SealedSectorPath: sealedSectorPath,
	}

	actorIdNum, err := address.IDFromAddress(minerAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor ID: %w", err)
	}
	actorId := abi.ActorID(actorIdNum)

	windowProofs, faultySectors, err := ffi.GenerateWindowPoSt(actorId, ffi.NewSortedPrivateSectorInfo(privateSectorInfo), postRand)
	if err != nil {
		return nil, fmt.Errorf("failed to generate window post: %w", err)
	}
	if len(faultySectors) > 0 {
		return nil, fmt.Errorf("post failed for sectors: %v", faultySectors)
	}
	if len(windowProofs) != 1 {
		return nil, fmt.Errorf("expected 1 proof, got %d", len(windowProofs))
	}
	if windowProofs[0].PoStProof != minerInfo.WindowPoStProofType {
		return nil, fmt.Errorf("expected proof type %d, got %d", minerInfo.WindowPoStProofType, windowProofs[0].PoStProof)
	}
	proofBytes := windowProofs[0].ProofBytes

	info := proof.WindowPoStVerifyInfo{
		Randomness:        postRand,
		Proofs:            []proof.PoStProof{{PoStProof: minerInfo.WindowPoStProofType, ProofBytes: proofBytes}},
		ChallengedSectors: []proof.SectorInfo{{SealProof: proofType, SectorNumber: sectorNumber, SealedCID: sealedCid}},
		Prover:            actorId,
	}

	verified, err := ffi.VerifyWindowPoSt(info)
	if err != nil {
		return nil, fmt.Errorf("failed to verify window post: %w", err)
	}
	if !verified {
		return nil, fmt.Errorf("window post verification failed")
	}

	return proofBytes, nil
}

func manualOnboardingDisputeWindowPost(
	ctx context.Context,
	client kit.TestFullNode,
	miner kit.TestUnmanagedMiner,
	sectorNumber abi.SectorNumber,
) error {

	head, err := client.ChainHead(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chain head: %w", err)
	}

	sp, err := client.StateSectorPartition(ctx, miner.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get sector partition: %w", err)
	}

	di, err := client.StateMinerProvingDeadline(ctx, miner.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get proving deadline: %w", err)
	}

	disputeEpoch := di.Close + 5
	fmt.Printf("WindowPoST(%d): Dispute: Waiting %d until epoch %d to submit dispute\n", sectorNumber, disputeEpoch-head.Height(), disputeEpoch)

	client.WaitTillChain(ctx, kit.HeightAtLeast(disputeEpoch))

	fmt.Printf("WindowPoST(%d): Dispute: Disputing WindowedPoSt to confirm validity...\n", sectorNumber)

	_, err = manualOnboardingSubmitMessage(ctx, client, miner, &miner14.DisputeWindowedPoStParams{
		Deadline:  sp.Deadline,
		PoStIndex: 0,
	}, 0, builtin.MethodsMiner.DisputeWindowedPoSt)
	if err == nil {
		return fmt.Errorf("expected dispute to fail")
	}
	if !strings.Contains(err.Error(), "failed to dispute valid post") {
		return fmt.Errorf("expected dispute to fail with 'failed to dispute valid post', got: %w", err)
	}
	if !strings.Contains(err.Error(), "(RetCode=16)") {
		return fmt.Errorf("expected dispute to fail with RetCode=16, got: %w", err)
	}
	return nil
}

func manualOnboardingSubmitMessage(
	ctx context.Context,
	client api.FullNode,
	from kit.TestUnmanagedMiner,
	params cbg.CBORMarshaler,
	value uint64,
	method abi.MethodNum,
) (*api.MsgLookup, error) {

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return nil, fmt.Errorf("failed to serialize params: %w", aerr)
	}

	m, err := client.MpoolPushMessage(ctx, &types.Message{
		To:     from.ActorAddr,
		From:   from.OwnerKey.Address,
		Value:  types.FromFil(value),
		Method: method,
		Params: enc,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to push message: %w", err)
	}

	return client.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
}

// manualOnboardingCalculateNextPostEpoch calculates the first epoch of the deadline proving window
// that we want for the given sector for the given miner.
func manualOnboardingCalculateNextPostEpoch(
	ctx context.Context,
	client api.FullNode,
	minerAddr address.Address,
	sectorNumber abi.SectorNumber,
) (abi.ChainEpoch, abi.ChainEpoch, error) {

	head, err := client.ChainHead(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get chain head: %w", err)
	}

	sp, err := client.StateSectorPartition(ctx, minerAddr, sectorNumber, head.Key())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get sector partition: %w", err)
	}

	fmt.Printf("WindowPoST(%d): SectorPartition: %+v\n", sectorNumber, sp)

	di, err := client.StateMinerProvingDeadline(ctx, minerAddr, head.Key())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get proving deadline: %w", err)
	}

	fmt.Printf("WindowPoST(%d): ProvingDeadline: %+v\n", sectorNumber, di)

	// periodStart tells us the first epoch of the current proving period (24h)
	// although it may be in the future if we don't need to submit post in this period
	periodStart := di.PeriodStart
	if di.PeriodStart < di.CurrentEpoch && sp.Deadline <= di.Index {
		// the deadline we want has past in this current proving period, so wait till the next one
		periodStart += di.WPoStProvingPeriod
	}
	provingEpoch := periodStart + (di.WPoStProvingPeriod/abi.ChainEpoch(di.WPoStPeriodDeadlines))*abi.ChainEpoch(sp.Deadline)

	return di.CurrentEpoch, provingEpoch, nil
}

func manualOnboardingSubmitWindowPost(
	ctx context.Context,
	withMockProofs bool,
	client kit.TestFullNode,
	miner kit.TestUnmanagedMiner,
	sectorNumber abi.SectorNumber,
	cacheDirPath, sealedSectorPath string,
	sealedCid cid.Cid,
	proofType abi.RegisteredSealProof,
) error {
	fmt.Printf("WindowPoST(%d): Running WindowPoSt ...\n", sectorNumber)

	head, err := client.ChainHead(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chain head: %w", err)
	}

	sp, err := client.StateSectorPartition(ctx, miner.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get sector partition: %w", err)
	}

	// We should be up to the deadline we care about
	di, err := client.StateMinerProvingDeadline(ctx, miner.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get proving deadline: %w", err)
	}
	fmt.Printf("WindowPoST(%d): SectorPartition: %+v, ProvingDeadline: %+v\n", sectorNumber, sp, di)
	if di.Index != sp.Deadline {
		return fmt.Errorf("sector %d is not in the deadline %d, but %d", sectorNumber, sp.Deadline, di.Index)
	}

	var proofBytes []byte
	if withMockProofs {
		proofBytes = []byte{0xde, 0xad, 0xbe, 0xef}
	} else {
		proofBytes, err = manualOnboardingGenerateWindowPost(ctx, client, cacheDirPath, sealedSectorPath, miner.ActorAddr, sectorNumber, sealedCid, proofType)
		if err != nil {
			return fmt.Errorf("failed to generate window post: %w", err)
		}
	}

	fmt.Printf("WindowedPoSt(%d) Submitting ...\n", sectorNumber)

	chainRandomnessEpoch := di.Challenge
	chainRandomness, err := client.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, chainRandomnessEpoch, nil, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get chain randomness: %w", err)
	}

	minerInfo, err := client.StateMinerInfo(ctx, miner.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get miner info: %w", err)
	}

	r, err := manualOnboardingSubmitMessage(ctx, client, miner, &miner14.SubmitWindowedPoStParams{
		ChainCommitEpoch: chainRandomnessEpoch,
		ChainCommitRand:  chainRandomness,
		Deadline:         sp.Deadline,
		Partitions:       []miner14.PoStPartition{{Index: sp.Partition}},
		Proofs:           []proof.PoStProof{{PoStProof: minerInfo.WindowPoStProofType, ProofBytes: proofBytes}},
	}, 0, builtin.MethodsMiner.SubmitWindowedPoSt)
	if err != nil {
		return fmt.Errorf("failed to submit PoSt: %w", err)
	}
	if !r.Receipt.ExitCode.IsSuccess() {
		return fmt.Errorf("submitting PoSt failed: %s", r.Receipt.ExitCode)
	}

	if !withMockProofs {
		// Dispute the PoSt to confirm the validity of the PoSt since PoSt acceptance is optimistic
		if err := manualOnboardingDisputeWindowPost(ctx, client, miner, sectorNumber); err != nil {
			return fmt.Errorf("failed to dispute PoSt: %w", err)
		}
	}
	return nil
}

// manualOnboardingRunWindowPost runs a goroutine to continually submit PoSTs for the given sector
// and miner. It will wait until the next proving period for the sector and then submit the PoSt.
// It will continue to do this until the context is cancelled.
// It returns a channel that will be closed when the first PoSt is submitted and a channel that will
// receive any errors that occur.
func manualOnboardingRunWindowPost(
	ctx context.Context,
	withMockProofs bool,
	client kit.TestFullNode,
	miner kit.TestUnmanagedMiner,
	sectorNumber abi.SectorNumber,
	cacheDirPath,
	sealedSectorPath string,
	sealedCid cid.Cid,
	proofType abi.RegisteredSealProof,
) (chan struct{}, chan error) {

	first := make(chan struct{})
	errCh := make(chan error)

	go func() {
		for ctx.Err() == nil {
			currentEpoch, nextPost, err := manualOnboardingCalculateNextPostEpoch(ctx, client, miner.ActorAddr, sectorNumber)
			if err != nil {
				errCh <- err
				return
			}
			if ctx.Err() != nil {
				return
			}
			nextPost += 5 // give a little buffer
			fmt.Printf("WindowPoST(%d) Waiting %d until epoch %d to submit PoSt\n", sectorNumber, nextPost-currentEpoch, nextPost)

			// Create channel to listen for chain head
			heads, err := client.ChainNotify(ctx)
			if err != nil {
				errCh <- err
				return
			}
			// Wait for nextPost epoch
			for chg := range heads {
				var ts *types.TipSet
				for _, c := range chg {
					if c.Type != "apply" {
						continue
					}
					ts = c.Val
					if ts.Height() >= nextPost {
						break
					}
				}
				if ctx.Err() != nil {
					return
				}
				if ts != nil && ts.Height() >= nextPost {
					break
				}
			}
			if ctx.Err() != nil {
				return
			}

			err = manualOnboardingSubmitWindowPost(ctx, withMockProofs, client, miner, sectorNumber, cacheDirPath, sealedSectorPath, sealedCid, proofType)
			if err != nil {
				errCh <- err
				return
			}

			// signal first post is done
			select {
			case <-first:
			default:
				close(first)
			}
		}
	}()

	return first, errCh
}
