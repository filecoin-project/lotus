package kit

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/proof"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TestUnmanagedMiner is a miner that's not managed by the storage/infrastructure, all tasks must be manually executed, managed and scheduled by the test or test kit.
// Note: `TestUnmanagedMiner` is not thread safe and assumes linear access of it's methods
type TestUnmanagedMiner struct {
	t                 *testing.T
	options           nodeOpts
	unsealedSectorDir string
	sealedSectorDir   string
	sectorSize        abi.SectorSize
	currentSectorNum  abi.SectorNumber

	CacheDir string

	UnsealedSectorPaths map[abi.SectorNumber]string
	SealedSectorPaths   map[abi.SectorNumber]string
	SealedCids          map[abi.SectorNumber]cid.Cid
	UnsealedCids        map[abi.SectorNumber]cid.Cid
	SealTickets         map[abi.SectorNumber]abi.SealRandomness

	ActorAddr address.Address
	OwnerKey  *key.Key
	FullNode  *TestFullNode
	Libp2p    struct {
		PeerID  peer.ID
		PrivKey libp2pcrypto.PrivKey
	}

	activationSectors  map[abi.SectorNumber]sectorActivation
	sectorActivationCh chan WindowPostReq
	mockProofs         map[abi.SectorNumber]bool
	proofType          map[abi.SectorNumber]abi.RegisteredSealProof
}

type sectorActivation struct {
	SectorNumber abi.SectorNumber
	RespCh       chan WindowPostResp
	NextPost     abi.ChainEpoch
}

type WindowPostReq struct {
	SectorNumber abi.SectorNumber
	RespCh       chan WindowPostResp
}

type WindowPostResp struct {
	SectorNumber abi.SectorNumber
	Error        error
}

func NewTestUnmanagedMiner(t *testing.T, full *TestFullNode, actorAddr address.Address, opts ...NodeOpt) *TestUnmanagedMiner {
	require.NotNil(t, full, "full node required when instantiating miner")

	options := DefaultNodeOpts
	for _, o := range opts {
		err := o(&options)
		require.NoError(t, err)
	}

	privkey, _, err := libp2pcrypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)

	require.NotNil(t, options.ownerKey, "manual miner key can't be null if initializing a miner after genesis")

	peerId, err := peer.IDFromPrivateKey(privkey)
	require.NoError(t, err)
	tmpDir := t.TempDir()

	cacheDir := filepath.Join(tmpDir, fmt.Sprintf("cache-%s", actorAddr))
	unsealedSectorDir := filepath.Join(tmpDir, fmt.Sprintf("unsealed-%s", actorAddr))
	sealedSectorDir := filepath.Join(tmpDir, fmt.Sprintf("sealed-%s", actorAddr))

	_ = os.Mkdir(cacheDir, 0755)
	_ = os.Mkdir(unsealedSectorDir, 0755)
	_ = os.Mkdir(sealedSectorDir, 0755)

	tm := TestUnmanagedMiner{
		t:                 t,
		options:           options,
		CacheDir:          cacheDir,
		unsealedSectorDir: unsealedSectorDir,
		sealedSectorDir:   sealedSectorDir,

		UnsealedSectorPaths: make(map[abi.SectorNumber]string),
		SealedSectorPaths:   make(map[abi.SectorNumber]string),
		SealedCids:          make(map[abi.SectorNumber]cid.Cid),
		UnsealedCids:        make(map[abi.SectorNumber]cid.Cid),
		SealTickets:         make(map[abi.SectorNumber]abi.SealRandomness),

		ActorAddr:          actorAddr,
		OwnerKey:           options.ownerKey,
		FullNode:           full,
		currentSectorNum:   101,
		activationSectors:  make(map[abi.SectorNumber]sectorActivation),
		mockProofs:         make(map[abi.SectorNumber]bool),
		proofType:          make(map[abi.SectorNumber]abi.RegisteredSealProof),
		sectorActivationCh: make(chan WindowPostReq),
	}
	tm.Libp2p.PeerID = peerId
	tm.Libp2p.PrivKey = privkey

	return &tm
}

func (tm *TestUnmanagedMiner) Start(ctx context.Context) {
	go tm.wdPostScheduler(ctx)
}

func (tm *TestUnmanagedMiner) AssertNoPower(ctx context.Context) {
	p := tm.CurrentPower(ctx)
	tm.t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
	require.True(tm.t, p.MinerPower.RawBytePower.IsZero())
}

func (tm *TestUnmanagedMiner) CurrentPower(ctx context.Context) *api.MinerPower {
	head, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	p, err := tm.FullNode.StateMinerPower(ctx, tm.ActorAddr, head.Key())
	require.NoError(tm.t, err)

	return p
}

func (tm *TestUnmanagedMiner) AssertPower(ctx context.Context, raw uint64, qa uint64) {
	req := require.New(tm.t)
	p := tm.CurrentPower(ctx)
	tm.t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
	req.Equal(raw, p.MinerPower.RawBytePower.Uint64())
	req.Equal(qa, p.MinerPower.QualityAdjPower.Uint64())
}

func (tm *TestUnmanagedMiner) OnboardCCSectorWithMockProofs(ctx context.Context, proofType abi.RegisteredSealProof) (abi.SectorNumber, chan WindowPostResp) {
	req := require.New(tm.t)
	sectorNumber := tm.currentSectorNum
	tm.currentSectorNum++

	tm.SealedCids[sectorNumber] = cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz")

	preCommitSealRand := tm.waitPreCommitSealRandomness(ctx)

	// Step 4 : Submit the Pre-Commit to the network
	r := tm.submitMessage(ctx, &miner14.PreCommitSectorBatchParams2{
		Sectors: []miner14.SectorPreCommitInfo{{
			Expiration:    2880 * 300,
			SectorNumber:  sectorNumber,
			SealProof:     TestSpt,
			SealedCID:     tm.SealedCids[sectorNumber],
			SealRandEpoch: preCommitSealRand,
		}},
	}, 1, builtin.MethodsMiner.PreCommitSectorBatch2)
	req.True(r.Receipt.ExitCode.IsSuccess())

	sectorProof := []byte{0xde, 0xad, 0xbe, 0xef}

	_ = tm.proveCommitWaitSeed(ctx, sectorNumber)

	r = tm.submitMessage(ctx, &miner14.ProveCommitSectors3Params{
		SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: sectorNumber}},
		SectorProofs:             [][]byte{sectorProof},
		RequireActivationSuccess: true,
	}, 0, builtin.MethodsMiner.ProveCommitSectors3)
	req.True(r.Receipt.ExitCode.IsSuccess())

	tm.mockProofs[sectorNumber] = true
	tm.proofType[sectorNumber] = proofType

	// schedule WindowPosts for this sector
	respCh := make(chan WindowPostResp, 1)
	tm.sectorActivationCh <- WindowPostReq{SectorNumber: sectorNumber, RespCh: respCh}

	return sectorNumber, respCh
}

func (tm *TestUnmanagedMiner) createCCSector(ctx context.Context, sectorNumber abi.SectorNumber) {
	req := require.New(tm.t)

	unsealedSectorPath := filepath.Join(tm.unsealedSectorDir, fmt.Sprintf("%d", sectorNumber))
	sealedSectorPath := filepath.Join(tm.sealedSectorDir, fmt.Sprintf("%d", sectorNumber))
	unsealedSize := abi.PaddedPieceSize(tm.sectorSize).Unpadded()
	req.NoError(os.WriteFile(unsealedSectorPath, make([]byte, unsealedSize), 0644))
	req.NoError(os.WriteFile(sealedSectorPath, make([]byte, tm.sectorSize), 0644))
	tm.t.Logf("Wrote unsealed sector to %s", unsealedSectorPath)
	tm.t.Logf("Wrote sealed sector to %s", sealedSectorPath)

	tm.UnsealedSectorPaths[sectorNumber] = unsealedSectorPath
	tm.SealedSectorPaths[sectorNumber] = sealedSectorPath

	return
}

func (tm *TestUnmanagedMiner) OnboardCCSectorWithRealProofs(ctx context.Context, proofType abi.RegisteredSealProof) (abi.SectorNumber, chan WindowPostResp) {
	req := require.New(tm.t)
	sectorNumber := tm.currentSectorNum
	tm.currentSectorNum++

	// --------------------Create pre-commit for the CC sector -> we'll just pre-commit `sector size` worth of 0s for this CC sector

	// Step 1: Wait for the pre-commitseal randomness to be available (we can only draw seal randomness from tipsets that have already achieved finality)
	preCommitSealRand := tm.waitPreCommitSealRandomness(ctx)

	// Step 2: Write empty 32 bytes that we want to seal i.e. create our CC sector
	tm.createCCSector(ctx, sectorNumber)

	// Step 3: Generate a Pre-Commit for the CC sector -> this persists the proof on the `TestUnmanagedMiner` Miner State
	tm.generatePreCommit(ctx, sectorNumber, preCommitSealRand, proofType)

	// Step 4 : Submit the Pre-Commit to the network
	r := tm.submitMessage(ctx, &miner14.PreCommitSectorBatchParams2{
		Sectors: []miner14.SectorPreCommitInfo{{
			Expiration:    2880 * 300,
			SectorNumber:  sectorNumber,
			SealProof:     TestSpt,
			SealedCID:     tm.SealedCids[sectorNumber],
			SealRandEpoch: preCommitSealRand,
		}},
	}, 1, builtin.MethodsMiner.PreCommitSectorBatch2)
	req.True(r.Receipt.ExitCode.IsSuccess())

	// Step 5: Generate a ProveCommit for the CC sector
	waitSeedRandomness := tm.proveCommitWaitSeed(ctx, sectorNumber)

	proveCommit := tm.generateProveCommit(ctx, sectorNumber, proofType, waitSeedRandomness)

	// Step 6: Submit the ProveCommit to the network
	tm.t.Log("Submitting ProveCommitSector ...")

	r = tm.submitMessage(ctx, &miner14.ProveCommitSectors3Params{
		SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: sectorNumber}},
		SectorProofs:             [][]byte{proveCommit},
		RequireActivationSuccess: true,
	}, 0, builtin.MethodsMiner.ProveCommitSectors3)
	req.True(r.Receipt.ExitCode.IsSuccess())

	tm.mockProofs[sectorNumber] = false
	tm.proofType[sectorNumber] = proofType

	// schedule WindowPosts for this sector
	tm.t.Logf("Scheduling WindowPost for sector %d", sectorNumber)
	respCh := make(chan WindowPostResp, 1)
	tm.sectorActivationCh <- WindowPostReq{SectorNumber: sectorNumber, RespCh: respCh}

	return sectorNumber, respCh
}

func (tm *TestUnmanagedMiner) wdPostScheduler(ctx context.Context) {
	for {
		select {
		case req := <-tm.sectorActivationCh:
			currentEpoch, nextPost, err := tm.calculateNextPostEpoch(ctx, req.SectorNumber)
			tm.t.Logf("Activating sector %d, next post %d, current epoch %d", req.SectorNumber, nextPost, currentEpoch)
			if err != nil {
				req.RespCh <- WindowPostResp{SectorNumber: req.SectorNumber, Error: err}
				close(req.RespCh)
				delete(tm.activationSectors, req.SectorNumber)
				continue
			}
			nextPost += 5 // give a little buffer
			tm.activationSectors[req.SectorNumber] = sectorActivation{SectorNumber: req.SectorNumber, RespCh: req.RespCh, NextPost: nextPost}

			go func() {
				tm.FullNode.WaitTillChain(ctx, HeightAtLeast(nextPost))
				err := tm.submitWindowPost(ctx, req.SectorNumber)
				req.RespCh <- WindowPostResp{SectorNumber: req.SectorNumber, Error: err}
				close(req.RespCh)
				delete(tm.activationSectors, req.SectorNumber)
			}()

		case <-ctx.Done():
			tm.t.Logf("Context cancelled, stopping window post scheduler")
			for sector, sa := range tm.activationSectors {
				sa.RespCh <- WindowPostResp{SectorNumber: sector, Error: fmt.Errorf("context cancelled")}
				close(sa.RespCh)
			}
			return
		}
	}
}

func (tm *TestUnmanagedMiner) submitWindowPost(ctx context.Context, sectorNumber abi.SectorNumber) error {
	fmt.Printf("WindowPoST(%d): Running WindowPoSt ...\n", sectorNumber)

	head, err := tm.FullNode.ChainHead(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chain head: %w", err)
	}

	sp, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get sector partition: %w", err)
	}

	// We should be up to the deadline we care about
	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get proving deadline: %w", err)
	}
	fmt.Printf("WindowPoST(%d): SectorPartition: %+v, ProvingDeadline: %+v\n", sectorNumber, sp, di)
	if di.Index != sp.Deadline {
		return fmt.Errorf("sector %d is not in the deadline %d, but %d", sectorNumber, sp.Deadline, di.Index)
	}

	var proofBytes []byte
	if tm.mockProofs[sectorNumber] {
		proofBytes = []byte{0xde, 0xad, 0xbe, 0xef}
	} else {
		proofBytes, err = tm.generateWindowPost(ctx, sectorNumber)
		if err != nil {
			return fmt.Errorf("failed to generate window post: %w", err)
		}
	}

	fmt.Printf("WindowedPoSt(%d) Submitting ...\n", sectorNumber)

	chainRandomnessEpoch := di.Challenge
	chainRandomness, err := tm.FullNode.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, chainRandomnessEpoch,
		nil, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get chain randomness: %w", err)
	}

	minerInfo, err := tm.FullNode.StateMinerInfo(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get miner info: %w", err)
	}

	r := tm.submitMessage(ctx, &miner14.SubmitWindowedPoStParams{
		ChainCommitEpoch: chainRandomnessEpoch,
		ChainCommitRand:  chainRandomness,
		Deadline:         sp.Deadline,
		Partitions:       []miner14.PoStPartition{{Index: sp.Partition}},
		Proofs:           []proof.PoStProof{{PoStProof: minerInfo.WindowPoStProofType, ProofBytes: proofBytes}},
	}, 0, builtin.MethodsMiner.SubmitWindowedPoSt)

	fmt.Println("WindowedPoSt(%d) Submitted ...", sectorNumber, r.Receipt.ExitCode)

	if !r.Receipt.ExitCode.IsSuccess() {
		return fmt.Errorf("submitting PoSt failed: %s", r.Receipt.ExitCode)
	}

	return nil
}

func (tm *TestUnmanagedMiner) generateWindowPost(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
) ([]byte, error) {
	head, err := tm.FullNode.ChainHead(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain head: %w", err)
	}

	minerInfo, err := tm.FullNode.StateMinerInfo(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return nil, fmt.Errorf("failed to get miner info: %w", err)
	}

	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, types.EmptyTSK)
	if err != nil {
		return nil, fmt.Errorf("failed to get proving deadline: %w", err)
	}

	minerAddrBytes := new(bytes.Buffer)
	if err := tm.ActorAddr.MarshalCBOR(minerAddrBytes); err != nil {
		return nil, fmt.Errorf("failed to marshal miner address: %w", err)
	}

	rand, err := tm.FullNode.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_WindowedPoStChallengeSeed, di.Challenge, minerAddrBytes.Bytes(), head.Key())
	if err != nil {
		return nil, fmt.Errorf("failed to get randomness: %w", err)
	}
	postRand := abi.PoStRandomness(rand)
	postRand[31] &= 0x3f // make fr32 compatible

	privateSectorInfo := ffi.PrivateSectorInfo{
		SectorInfo: proof.SectorInfo{
			SealProof:    tm.proofType[sectorNumber],
			SectorNumber: sectorNumber,
			SealedCID:    tm.SealedCids[sectorNumber],
		},
		CacheDirPath:     tm.CacheDir,
		PoStProofType:    minerInfo.WindowPoStProofType,
		SealedSectorPath: tm.SealedSectorPaths[sectorNumber],
	}

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
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
		ChallengedSectors: []proof.SectorInfo{{SealProof: tm.proofType[sectorNumber], SectorNumber: sectorNumber, SealedCID: tm.SealedCids[sectorNumber]}},
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

func (tm *TestUnmanagedMiner) waitPreCommitSealRandomness(ctx context.Context) abi.ChainEpoch {
	// we want to draw seal randomess from a tipset that has already achieved finality as PreCommits are expensive to re-generate.
	// See if we already have an epoch that is already final and wait for such an epoch if we don't have one
	head, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	var sealRandEpoch abi.ChainEpoch
	if head.Height() > policy.SealRandomnessLookback {
		sealRandEpoch = head.Height() - policy.SealRandomnessLookback
	} else {
		sealRandEpoch = policy.SealRandomnessLookback
		tm.t.Logf("Waiting for at least epoch %d for seal randomness (current epoch %d) ...", sealRandEpoch+5, head.Height())
		tm.FullNode.WaitTillChain(ctx, HeightAtLeast(sealRandEpoch+5))
	}

	return sealRandEpoch
}

// manualOnboardingCalculateNextPostEpoch calculates the first epoch of the deadline proving window
// that we want for the given sector for the given miner.
func (tm *TestUnmanagedMiner) calculateNextPostEpoch(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
) (abi.ChainEpoch, abi.ChainEpoch, error) {

	head, err := tm.FullNode.ChainHead(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get chain head: %w", err)
	}

	sp, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get sector partition: %w", err)
	}

	fmt.Printf("WindowPoST(%d): SectorPartition: %+v\n", sectorNumber, sp)

	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, head.Key())
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

func (tm *TestUnmanagedMiner) generatePreCommit(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
	sealRandEpoch abi.ChainEpoch,
	proofType abi.RegisteredSealProof,
) {
	req := require.New(tm.t)
	tm.t.Logf("Generating proof type %d PreCommit ...", proofType)

	head, err := tm.FullNode.ChainHead(ctx)
	req.NoError(err)

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes))

	rand, err := tm.FullNode.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_SealRandomness, sealRandEpoch, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	sealTickets := abi.SealRandomness(rand)

	tm.t.Logf("Running proof type %d SealPreCommitPhase1 for sector %d...", proofType, sectorNumber)

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	pc1, err := ffi.SealPreCommitPhase1(
		proofType,
		tm.CacheDir,
		tm.UnsealedSectorPaths[sectorNumber],
		tm.SealedSectorPaths[sectorNumber],
		sectorNumber,
		actorId,
		sealTickets,
		[]abi.PieceInfo{},
	)
	req.NoError(err)
	req.NotNil(pc1)

	tm.t.Logf("Running proof type %d SealPreCommitPhase2 for sector %d...", proofType, sectorNumber)

	sealedCid, unsealedCid, err := ffi.SealPreCommitPhase2(
		pc1,
		tm.CacheDir,
		tm.SealedSectorPaths[sectorNumber],
	)
	req.NoError(err)

	tm.t.Logf("Unsealed CID: %s", unsealedCid)
	tm.t.Logf("Sealed CID: %s", sealedCid)

	tm.SealTickets[sectorNumber] = sealTickets
	tm.SealedCids[sectorNumber] = sealedCid
	tm.UnsealedCids[sectorNumber] = unsealedCid
}

func (tm *TestUnmanagedMiner) proveCommitWaitSeed(ctx context.Context, sectorNumber abi.SectorNumber) abi.InteractiveSealRandomness {
	req := require.New(tm.t)
	head, err := tm.FullNode.ChainHead(ctx)
	req.NoError(err)

	preCommitInfo, err := tm.FullNode.StateSectorPreCommitInfo(ctx, tm.ActorAddr, sectorNumber, head.Key())
	req.NoError(err)
	seedRandomnessHeight := preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

	tm.t.Logf("Waiting %d epochs for seed randomness at epoch %d (current epoch %d)...", seedRandomnessHeight-head.Height(), seedRandomnessHeight, head.Height())
	tm.FullNode.WaitTillChain(ctx, HeightAtLeast(seedRandomnessHeight+5))

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes))

	head, err = tm.FullNode.ChainHead(ctx)
	req.NoError(err)

	rand, err := tm.FullNode.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, seedRandomnessHeight, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	seedRandomness := abi.InteractiveSealRandomness(rand)

	tm.t.Logf("Got seed randomness for sector %d: %x", sectorNumber, seedRandomness)
	return seedRandomness
}

func (tm *TestUnmanagedMiner) generateProveCommit(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
	proofType abi.RegisteredSealProof,
	seedRandomness abi.InteractiveSealRandomness,
) []byte {
	tm.t.Logf("Generating proof type %d Sector Proof for sector %d...", proofType, sectorNumber)
	req := require.New(tm.t)

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	tm.t.Logf("Running proof type %d SealCommitPhase1 for sector %d...", proofType, sectorNumber)

	scp1, err := ffi.SealCommitPhase1(
		proofType,
		tm.SealedCids[sectorNumber],
		tm.UnsealedCids[sectorNumber],
		tm.CacheDir,
		tm.SealedSectorPaths[sectorNumber],
		sectorNumber,
		actorId,
		tm.SealTickets[sectorNumber],
		seedRandomness,
		[]abi.PieceInfo{},
	)
	req.NoError(err)

	tm.t.Logf("Running proof type %d SealCommitPhase2 for sector %d...", proofType, sectorNumber)

	sectorProof, err := ffi.SealCommitPhase2(scp1, sectorNumber, actorId)
	req.NoError(err)

	tm.t.Logf("Got proof type %d sector proof of length %d", proofType, len(sectorProof))

	return sectorProof
}

func (tm *TestUnmanagedMiner) submitMessage(
	ctx context.Context,
	params cbg.CBORMarshaler,
	value uint64,
	method abi.MethodNum,
) *api.MsgLookup {
	enc, aerr := actors.SerializeParams(params)
	require.NoError(tm.t, aerr)

	m, err := tm.FullNode.MpoolPushMessage(ctx, &types.Message{
		To:     tm.ActorAddr,
		From:   tm.OwnerKey.Address,
		Value:  types.FromFil(value),
		Method: method,
		Params: enc,
	}, nil)
	require.NoError(tm.t, err)

	msg, err := tm.FullNode.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	require.NoError(tm.t, err)
	return msg
}
