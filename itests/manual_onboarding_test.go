package itests

import (
	"bytes"
	"context"
	"fmt"
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
				client    kit.TestFullNode
				minerA    kit.TestMiner // A is a standard genesis miner
			)

			// Setup and begin mining with a single miner (A)
			// Miner A will only be a genesis Miner with power allocated in the genesis block and will not onboard any sectors from here on
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

			// Instantiate MinerB to manually handle sector onboarding and power acquisition through sector activation.
			// Unlike other miners managed by the Lotus Miner storage infrastructure, MinerB operates independently,
			// performing all related tasks manually. Managed by the TestKit, MinerB has the capability to utilize actual proofs
			// for the processes of sector onboarding and activation.
			nodeOpts = append(nodeOpts, kit.OwnerAddr(client.DefaultKey))
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

			var bSectorNum abi.SectorNumber
			if withMockProofs {
				bSectorNum = minerB.OnboardCCSectorWithMockProofs(ctx, kit.TestSpt)
			} else {
				bSectorNum = minerB.OnboardCCSectorWithRealProofs(ctx, kit.TestSpt)
			}

			// Miner B should still not have power as power can only be gained after sector is activated i.e. the first WindowPost is submitted for it
			minerB.AssertNoPower(ctx)

			// start a background PoST scheduler for miner B
			bFirstCh, bErrCh := manualOnboardingRunWindowPost(
				ctx,
				withMockProofs,
				client,
				minerB,
				bSectorNum,
				minerB.CacheDirPaths[bSectorNum],
				minerB.SealedSectorPaths[bSectorNum],
				minerB.SealedCids[bSectorNum],
				kit.TestSpt,
			)

			checkPostSchedulers := func() {
				t.Helper()
				select {
				case err, ok := <-bErrCh:
					if ok {
						t.Fatalf("Received error from Miner B PoST scheduler: %v", err)
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
				// wait till the first PoST is submitted for MinerB and check again
				if isClosed(bFirstCh) {
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
			head, err = client.ChainHead(ctx)
			req.NoError(err)

			soi, err := client.StateSectorGetInfo(ctx, minerB.ActorAddr, bSectorNum, head.Key())
			req.NoError(err)
			t.Logf("Miner B SectorOnChainInfo %d: %+v", bSectorNum, soi)

			head = client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+5))

			checkPostSchedulers()

			t.Log("Checking power after PoSt ...")

			// Miner B should now have power
			minerB.AssertPower(ctx, (uint64(2 << 10)), (uint64(2 << 10)))

			checkPostSchedulers()
		})
	}
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
	miner *kit.TestUnmanagedMiner,
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
	from *kit.TestUnmanagedMiner,
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
	miner *kit.TestUnmanagedMiner,
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
	miner *kit.TestUnmanagedMiner,
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
