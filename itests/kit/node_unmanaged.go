package kit

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-cid"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
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
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
)

// TestUnmanagedMiner is a miner that's not managed by the storage/infrastructure, all tasks must be manually executed, managed and scheduled by the test or test kit.
// Note: `TestUnmanagedMiner` is not thread safe and assumes linear access of it's methods
type TestUnmanagedMiner struct {
	t       *testing.T
	options nodeOpts

	cacheDir          string
	unsealedSectorDir string
	sealedSectorDir   string
	currentSectorNum  abi.SectorNumber

	cacheDirPaths       map[abi.SectorNumber]string
	unsealedSectorPaths map[abi.SectorNumber]string
	sealedSectorPaths   map[abi.SectorNumber]string
	sealedCids          map[abi.SectorNumber]cid.Cid
	unsealedCids        map[abi.SectorNumber]cid.Cid
	sealTickets         map[abi.SectorNumber]abi.SealRandomness

	proofType map[abi.SectorNumber]abi.RegisteredSealProof

	ActorAddr address.Address
	OwnerKey  *key.Key
	FullNode  *TestFullNode
	Libp2p    struct {
		PeerID  peer.ID
		PrivKey libp2pcrypto.PrivKey
	}
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

	require.NotNil(t, options.ownerKey, "owner key is required for initializing a miner")

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
		cacheDir:          cacheDir,
		unsealedSectorDir: unsealedSectorDir,
		sealedSectorDir:   sealedSectorDir,

		unsealedSectorPaths: make(map[abi.SectorNumber]string),
		cacheDirPaths:       make(map[abi.SectorNumber]string),
		sealedSectorPaths:   make(map[abi.SectorNumber]string),
		sealedCids:          make(map[abi.SectorNumber]cid.Cid),
		unsealedCids:        make(map[abi.SectorNumber]cid.Cid),
		sealTickets:         make(map[abi.SectorNumber]abi.SealRandomness),

		ActorAddr:        actorAddr,
		OwnerKey:         options.ownerKey,
		FullNode:         full,
		currentSectorNum: 101,
		proofType:        make(map[abi.SectorNumber]abi.RegisteredSealProof),
	}
	tm.Libp2p.PeerID = peerId
	tm.Libp2p.PrivKey = privkey

	return &tm
}

func (tm *TestUnmanagedMiner) AssertNoPower(ctx context.Context) {
	p := tm.CurrentPower(ctx)
	tm.t.Logf("Miner %s RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), tm.ActorAddr, p.MinerPower.RawBytePower.String())
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
	tm.t.Logf("Miner %s RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), tm.ActorAddr, p.MinerPower.RawBytePower.String())
	req.Equal(raw, p.MinerPower.RawBytePower.Uint64())
	req.Equal(qa, p.MinerPower.QualityAdjPower.Uint64())
}

func (tm *TestUnmanagedMiner) mkAndSavePiecesToOnboard(_ context.Context, sectorNumber abi.SectorNumber, pt abi.RegisteredSealProof) []abi.PieceInfo {
	paddedPieceSize := abi.PaddedPieceSize(tm.options.sectorSize)
	unpaddedPieceSize := paddedPieceSize.Unpadded()

	// Generate random bytes for the piece
	randomBytes := make([]byte, unpaddedPieceSize)
	_, err := io.ReadFull(rand.Reader, randomBytes)
	require.NoError(tm.t, err)

	// Create a temporary file for the first piece
	pieceFileA := requireTempFile(tm.t, bytes.NewReader(randomBytes), uint64(unpaddedPieceSize))

	// Generate the piece CID from the file
	pieceCIDA, err := ffi.GeneratePieceCIDFromFile(pt, pieceFileA, unpaddedPieceSize)
	require.NoError(tm.t, err)

	// Reset file offset to the beginning after CID generation
	_, err = pieceFileA.Seek(0, io.SeekStart)
	require.NoError(tm.t, err)

	unsealedSectorFile := requireTempFile(tm.t, bytes.NewReader([]byte{}), 0)
	defer func() {
		_ = unsealedSectorFile.Close()
	}()

	// Write the piece to the staged sector file without alignment
	writtenBytes, pieceCID, err := ffi.WriteWithoutAlignment(pt, pieceFileA, unpaddedPieceSize, unsealedSectorFile)
	require.NoError(tm.t, err)
	require.EqualValues(tm.t, unpaddedPieceSize, writtenBytes)
	require.True(tm.t, pieceCID.Equals(pieceCIDA))

	// Create a struct for the piece info
	publicPieces := []abi.PieceInfo{{
		Size:     paddedPieceSize,
		PieceCID: pieceCIDA,
	}}

	// Create a temporary file for the sealed sector
	sealedSectorFile := requireTempFile(tm.t, bytes.NewReader([]byte{}), 0)
	defer func() {
		_ = sealedSectorFile.Close()
	}()

	// Update paths for the sector
	tm.sealedSectorPaths[sectorNumber] = sealedSectorFile.Name()
	tm.unsealedSectorPaths[sectorNumber] = unsealedSectorFile.Name()
	tm.cacheDirPaths[sectorNumber] = filepath.Join(tm.cacheDir, fmt.Sprintf("%d", sectorNumber))

	// Ensure the cache directory exists
	_ = os.Mkdir(tm.cacheDirPaths[sectorNumber], 0755)

	return publicPieces
}

func (tm *TestUnmanagedMiner) makeAndSaveCCSector(_ context.Context, sectorNumber abi.SectorNumber) {
	requirements := require.New(tm.t)

	// Create cache directory
	cacheDirPath := filepath.Join(tm.cacheDir, fmt.Sprintf("%d", sectorNumber))
	requirements.NoError(os.Mkdir(cacheDirPath, 0755))
	tm.t.Logf("Miner %s: Sector %d: created cache directory at %s", tm.ActorAddr, sectorNumber, cacheDirPath)

	// Define paths for unsealed and sealed sectors
	unsealedSectorPath := filepath.Join(tm.unsealedSectorDir, fmt.Sprintf("%d", sectorNumber))
	sealedSectorPath := filepath.Join(tm.sealedSectorDir, fmt.Sprintf("%d", sectorNumber))
	unsealedSize := abi.PaddedPieceSize(tm.options.sectorSize).Unpadded()

	// Write unsealed sector file
	requirements.NoError(os.WriteFile(unsealedSectorPath, make([]byte, unsealedSize), 0644))
	tm.t.Logf("Miner %s: Sector %d: wrote unsealed CC sector to %s", tm.ActorAddr, sectorNumber, unsealedSectorPath)

	// Write sealed sector file
	requirements.NoError(os.WriteFile(sealedSectorPath, make([]byte, tm.options.sectorSize), 0644))
	tm.t.Logf("Miner %s: Sector %d: wrote sealed CC sector to %s", tm.ActorAddr, sectorNumber, sealedSectorPath)

	// Update paths in the struct
	tm.unsealedSectorPaths[sectorNumber] = unsealedSectorPath
	tm.sealedSectorPaths[sectorNumber] = sealedSectorPath
	tm.cacheDirPaths[sectorNumber] = cacheDirPath
}

func (tm *TestUnmanagedMiner) OnboardSectorWithPiecesAndRealProofs(ctx context.Context, proofType abi.RegisteredSealProof) (abi.SectorNumber, chan WindowPostResp) {
	req := require.New(tm.t)
	sectorNumber := tm.currentSectorNum
	tm.currentSectorNum++

	// Step 1: Wait for the pre-commitseal randomness to be available (we can only draw seal randomness from tipsets that have already achieved finality)
	preCommitSealRand := tm.waitPreCommitSealRandomness(ctx, sectorNumber)

	// Step 2: Build a sector with non 0 Pieces that we want to onboard
	pieces := tm.mkAndSavePiecesToOnboard(ctx, sectorNumber, proofType)

	// Step 3: Generate a Pre-Commit for the CC sector -> this persists the proof on the `TestUnmanagedMiner` Miner State
	tm.generatePreCommit(ctx, sectorNumber, preCommitSealRand, proofType, pieces)

	// Step 4 : Submit the Pre-Commit to the network
	unsealedCid := tm.unsealedCids[sectorNumber]
	r, err := tm.submitMessage(ctx, &miner14.PreCommitSectorBatchParams2{
		Sectors: []miner14.SectorPreCommitInfo{{
			Expiration:    2880 * 300,
			SectorNumber:  sectorNumber,
			SealProof:     TestSpt,
			SealedCID:     tm.sealedCids[sectorNumber],
			SealRandEpoch: preCommitSealRand,
			UnsealedCid:   &unsealedCid,
		}},
	}, 1, builtin.MethodsMiner.PreCommitSectorBatch2)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())

	// Step 5: Generate a ProveCommit for the CC sector
	waitSeedRandomness := tm.proveCommitWaitSeed(ctx, sectorNumber)

	proveCommit := tm.generateProveCommit(ctx, sectorNumber, proofType, waitSeedRandomness, pieces)

	// Step 6: Submit the ProveCommit to the network
	tm.t.Log("Submitting ProveCommitSector ...")

	var manifest []miner14.PieceActivationManifest
	for _, piece := range pieces {
		manifest = append(manifest, miner14.PieceActivationManifest{
			CID:  piece.PieceCID,
			Size: piece.Size,
		})
	}

	r, err = tm.submitMessage(ctx, &miner14.ProveCommitSectors3Params{
		SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: sectorNumber, Pieces: manifest}},
		SectorProofs:             [][]byte{proveCommit},
		RequireActivationSuccess: true,
	}, 1, builtin.MethodsMiner.ProveCommitSectors3)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())

	tm.proofType[sectorNumber] = proofType

	respCh := make(chan WindowPostResp, 1)

	go tm.wdPostLoop(ctx, sectorNumber, respCh)

	return sectorNumber, respCh
}

func (tm *TestUnmanagedMiner) OnboardCCSectorWithRealProofs(ctx context.Context, proofType abi.RegisteredSealProof) (abi.SectorNumber, chan WindowPostResp) {
	req := require.New(tm.t)
	sectorNumber := tm.currentSectorNum
	tm.currentSectorNum++

	// --------------------Create pre-commit for the CC sector -> we'll just pre-commit `sector size` worth of 0s for this CC sector

	// Step 1: Wait for the pre-commitseal randomness to be available (we can only draw seal randomness from tipsets that have already achieved finality)
	preCommitSealRand := tm.waitPreCommitSealRandomness(ctx, sectorNumber)

	// Step 2: Write empty bytes that we want to seal i.e. create our CC sector
	tm.makeAndSaveCCSector(ctx, sectorNumber)

	// Step 3: Generate a Pre-Commit for the CC sector -> this persists the proof on the `TestUnmanagedMiner` Miner State
	tm.generatePreCommit(ctx, sectorNumber, preCommitSealRand, proofType, []abi.PieceInfo{})

	// Step 4 : Submit the Pre-Commit to the network
	r, err := tm.submitMessage(ctx, &miner14.PreCommitSectorBatchParams2{
		Sectors: []miner14.SectorPreCommitInfo{{
			Expiration:    2880 * 300,
			SectorNumber:  sectorNumber,
			SealProof:     TestSpt,
			SealedCID:     tm.sealedCids[sectorNumber],
			SealRandEpoch: preCommitSealRand,
		}},
	}, 1, builtin.MethodsMiner.PreCommitSectorBatch2)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())

	// Step 5: Generate a ProveCommit for the CC sector
	waitSeedRandomness := tm.proveCommitWaitSeed(ctx, sectorNumber)

	proveCommit := tm.generateProveCommit(ctx, sectorNumber, proofType, waitSeedRandomness, []abi.PieceInfo{})

	// Step 6: Submit the ProveCommit to the network
	tm.t.Log("Submitting ProveCommitSector ...")

	r, err = tm.submitMessage(ctx, &miner14.ProveCommitSectors3Params{
		SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: sectorNumber}},
		SectorProofs:             [][]byte{proveCommit},
		RequireActivationSuccess: true,
	}, 0, builtin.MethodsMiner.ProveCommitSectors3)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())

	tm.proofType[sectorNumber] = proofType

	respCh := make(chan WindowPostResp, 1)

	go tm.wdPostLoop(ctx, sectorNumber, respCh)

	return sectorNumber, respCh
}

func (tm *TestUnmanagedMiner) wdPostLoop(ctx context.Context, sectorNumber abi.SectorNumber, respCh chan WindowPostResp) {
	go func() {
		writeRespF := func(respErr error) {
			select {
			case respCh <- WindowPostResp{SectorNumber: sectorNumber, Error: respErr}:
			case <-ctx.Done():
			default:
			}
		}

		currentEpoch, nextPost, err := tm.calculateNextPostEpoch(ctx, sectorNumber)
		tm.t.Logf("Activating sector %d, next post %d, current epoch %d", sectorNumber, nextPost, currentEpoch)
		if err != nil {
			writeRespF(err)
			return
		}

		if _, err := tm.FullNode.WaitTillChainOrError(ctx, HeightAtLeast(nextPost)); err != nil {
			writeRespF(err)
			return
		}

		err = tm.submitWindowPost(ctx, sectorNumber)
		writeRespF(err)
		if ctx.Err() != nil {
			return
		}
	}()
}

func (tm *TestUnmanagedMiner) SubmitPostDispute(ctx context.Context, sectorNumber abi.SectorNumber) error {
	tm.t.Logf("Miner %s: Starting dispute submission for sector %d", tm.ActorAddr, sectorNumber)

	head, err := tm.FullNode.ChainHead(ctx)
	if err != nil {
		return fmt.Errorf("MinerB(%s): failed to get chain head: %w", tm.ActorAddr, err)
	}

	sp, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return fmt.Errorf("MinerB(%s): failed to get sector partition for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}

	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("MinerB(%s): failed to get proving deadline for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}

	disputeEpoch := di.Close + 5
	tm.t.Logf("Miner %s: Sector %d - Waiting %d epochs until epoch %d to submit dispute", tm.ActorAddr, sectorNumber, disputeEpoch-head.Height(), disputeEpoch)

	tm.FullNode.WaitTillChain(ctx, HeightAtLeast(disputeEpoch))

	tm.t.Logf("Miner %s: Sector %d - Disputing WindowedPoSt to confirm validity at epoch %d", tm.ActorAddr, sectorNumber, disputeEpoch)

	_, err = tm.submitMessage(ctx, &miner14.DisputeWindowedPoStParams{
		Deadline:  sp.Deadline,
		PoStIndex: 0,
	}, 1, builtin.MethodsMiner.DisputeWindowedPoSt)
	return err
}

func (tm *TestUnmanagedMiner) submitWindowPost(ctx context.Context, sectorNumber abi.SectorNumber) error {
	tm.t.Logf("Miner(%s): WindowPoST(%d): Running WindowPoSt ...\n", tm.ActorAddr, sectorNumber)

	head, err := tm.FullNode.ChainHead(ctx)
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to get chain head: %w", tm.ActorAddr, err)
	}

	sp, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to get sector partition for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}

	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to get proving deadline for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}
	tm.t.Logf("Miner(%s): WindowPoST(%d): SectorPartition: %+v, ProvingDeadline: %+v\n", tm.ActorAddr, sectorNumber, sp, di)
	if di.Index != sp.Deadline {
		return fmt.Errorf("Miner(%s): sector %d is not in the deadline %d, but %d", tm.ActorAddr, sectorNumber, sp.Deadline, di.Index)
	}

	var proofBytes []byte
	proofBytes, err = tm.generateWindowPost(ctx, sectorNumber)
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to generate window post for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}

	tm.t.Logf("Miner(%s): WindowedPoSt(%d) Submitting ...\n", tm.ActorAddr, sectorNumber)

	chainRandomnessEpoch := di.Challenge
	chainRandomness, err := tm.FullNode.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, chainRandomnessEpoch,
		nil, head.Key())
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to get chain randomness for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}

	minerInfo, err := tm.FullNode.StateMinerInfo(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to get miner info for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}

	r, err := tm.submitMessage(ctx, &miner14.SubmitWindowedPoStParams{
		ChainCommitEpoch: chainRandomnessEpoch,
		ChainCommitRand:  chainRandomness,
		Deadline:         sp.Deadline,
		Partitions:       []miner14.PoStPartition{{Index: sp.Partition}},
		Proofs:           []proof.PoStProof{{PoStProof: minerInfo.WindowPoStProofType, ProofBytes: proofBytes}},
	}, 0, builtin.MethodsMiner.SubmitWindowedPoSt)
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to submit window post for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}

	if !r.Receipt.ExitCode.IsSuccess() {
		return fmt.Errorf("Miner(%s): submitting PoSt for sector %d failed: %s", tm.ActorAddr, sectorNumber, r.Receipt.ExitCode)
	}

	tm.t.Logf("Miner(%s): WindowedPoSt(%d) Submitted ...\n", tm.ActorAddr, sectorNumber)

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
			SealedCID:    tm.sealedCids[sectorNumber],
		},
		CacheDirPath:     tm.cacheDirPaths[sectorNumber],
		PoStProofType:    minerInfo.WindowPoStProofType,
		SealedSectorPath: tm.sealedSectorPaths[sectorNumber],
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
		ChallengedSectors: []proof.SectorInfo{{SealProof: tm.proofType[sectorNumber], SectorNumber: sectorNumber, SealedCID: tm.sealedCids[sectorNumber]}},
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
func (tm *TestUnmanagedMiner) waitPreCommitSealRandomness(ctx context.Context, sectorNumber abi.SectorNumber) abi.ChainEpoch {
	// We want to draw seal randomness from a tipset that has already achieved finality as PreCommits are expensive to re-generate.
	// Check if we already have an epoch that is already final and wait for such an epoch if we don't have one.
	head, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	var sealRandEpoch abi.ChainEpoch
	if head.Height() > policy.SealRandomnessLookback {
		sealRandEpoch = head.Height() - policy.SealRandomnessLookback
	} else {
		sealRandEpoch = policy.SealRandomnessLookback
		tm.t.Logf("Miner %s waiting for at least epoch %d for seal randomness for sector %d (current epoch %d)...", tm.ActorAddr, sealRandEpoch+5,
			sectorNumber, head.Height())
		tm.FullNode.WaitTillChain(ctx, HeightAtLeast(sealRandEpoch+5))
	}

	tm.t.Logf("Miner %s using seal randomness from epoch %d for head %d for sector %d", tm.ActorAddr, sealRandEpoch, head.Height(), sectorNumber)

	return sealRandEpoch
}

// calculateNextPostEpoch calculates the first epoch of the deadline proving window
// that is desired for the given sector for the specified miner.
// This function returns the current epoch and the calculated proving epoch.
func (tm *TestUnmanagedMiner) calculateNextPostEpoch(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
) (abi.ChainEpoch, abi.ChainEpoch, error) {
	// Retrieve the current blockchain head
	head, err := tm.FullNode.ChainHead(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get chain head: %w", err)
	}

	// Fetch the sector partition for the given sector number
	sp, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get sector partition: %w", err)
	}

	tm.t.Logf("Miner %s: WindowPoST(%d): SectorPartition: %+v", tm.ActorAddr, sectorNumber, sp)

	// Obtain the proving deadline information for the miner
	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get proving deadline: %w", err)
	}

	tm.t.Logf("Miner %s: WindowPoST(%d): ProvingDeadline: %+v", tm.ActorAddr, sectorNumber, di)

	// Calculate the start of the period, adjusting if the current deadline has passed
	periodStart := di.PeriodStart
	if di.PeriodStart < di.CurrentEpoch && sp.Deadline <= di.Index {
		// If the deadline has passed in the current proving period, calculate for the next period
		periodStart += di.WPoStProvingPeriod
	}

	// Calculate the exact epoch when proving should occur
	provingEpoch := periodStart + (di.WPoStProvingPeriod/abi.ChainEpoch(di.WPoStPeriodDeadlines))*abi.ChainEpoch(sp.Deadline)

	tm.t.Logf("Miner %s: WindowPoST(%d): next ProvingEpoch: %d", tm.ActorAddr, sectorNumber, provingEpoch)

	return di.CurrentEpoch, provingEpoch, nil
}

func (tm *TestUnmanagedMiner) generatePreCommit(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
	sealRandEpoch abi.ChainEpoch,
	proofType abi.RegisteredSealProof,
	pieceInfo []abi.PieceInfo,
) {
	req := require.New(tm.t)
	tm.t.Logf("Miner %s: Generating proof type %d PreCommit for sector %d...", tm.ActorAddr, proofType, sectorNumber)

	head, err := tm.FullNode.ChainHead(ctx)
	req.NoError(err, "Miner %s: Failed to get chain head for sector %d", tm.ActorAddr, sectorNumber)

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes), "Miner %s: Failed to marshal address for sector %d", tm.ActorAddr, sectorNumber)

	rand, err := tm.FullNode.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_SealRandomness, sealRandEpoch, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err, "Miner %s: Failed to get randomness for sector %d", tm.ActorAddr, sectorNumber)
	sealTickets := abi.SealRandomness(rand)

	tm.t.Logf("Miner %s: Running proof type %d SealPreCommitPhase1 for sector %d...", tm.ActorAddr, proofType, sectorNumber)

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	req.NoError(err, "Miner %s: Failed to get actor ID for sector %d", tm.ActorAddr, sectorNumber)
	actorId := abi.ActorID(actorIdNum)

	pc1, err := ffi.SealPreCommitPhase1(
		proofType,
		tm.cacheDirPaths[sectorNumber],
		tm.unsealedSectorPaths[sectorNumber],
		tm.sealedSectorPaths[sectorNumber],
		sectorNumber,
		actorId,
		sealTickets,
		pieceInfo,
	)
	req.NoError(err, "Miner %s: SealPreCommitPhase1 failed for sector %d", tm.ActorAddr, sectorNumber)
	req.NotNil(pc1, "Miner %s: SealPreCommitPhase1 returned nil for sector %d", tm.ActorAddr, sectorNumber)

	tm.t.Logf("Miner %s: Running proof type %d SealPreCommitPhase2 for sector %d...", tm.ActorAddr, proofType, sectorNumber)

	sealedCid, unsealedCid, err := ffi.SealPreCommitPhase2(
		pc1,
		tm.cacheDirPaths[sectorNumber],
		tm.sealedSectorPaths[sectorNumber],
	)
	req.NoError(err, "Miner %s: SealPreCommitPhase2 failed for sector %d", tm.ActorAddr, sectorNumber)

	tm.t.Logf("Miner %s: Unsealed CID for sector %d: %s", tm.ActorAddr, sectorNumber, unsealedCid)
	tm.t.Logf("Miner %s: Sealed CID for sector %d: %s", tm.ActorAddr, sectorNumber, sealedCid)

	tm.sealTickets[sectorNumber] = sealTickets
	tm.sealedCids[sectorNumber] = sealedCid
	tm.unsealedCids[sectorNumber] = unsealedCid
}

func (tm *TestUnmanagedMiner) proveCommitWaitSeed(ctx context.Context, sectorNumber abi.SectorNumber) abi.InteractiveSealRandomness {
	req := require.New(tm.t)
	head, err := tm.FullNode.ChainHead(ctx)
	req.NoError(err)

	tm.t.Logf("Miner %s: Fetching pre-commit info for sector %d...", tm.ActorAddr, sectorNumber)
	preCommitInfo, err := tm.FullNode.StateSectorPreCommitInfo(ctx, tm.ActorAddr, sectorNumber, head.Key())
	req.NoError(err)
	seedRandomnessHeight := preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

	tm.t.Logf("Miner %s: Waiting %d epochs for seed randomness at epoch %d (current epoch %d) for sector %d...", tm.ActorAddr, seedRandomnessHeight-head.Height(), seedRandomnessHeight, head.Height(), sectorNumber)
	tm.FullNode.WaitTillChain(ctx, HeightAtLeast(seedRandomnessHeight+5))

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes))

	head, err = tm.FullNode.ChainHead(ctx)
	req.NoError(err)

	tm.t.Logf("Miner %s: Fetching seed randomness for sector %d...", tm.ActorAddr, sectorNumber)
	rand, err := tm.FullNode.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, seedRandomnessHeight, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	seedRandomness := abi.InteractiveSealRandomness(rand)

	tm.t.Logf("Miner %s: Obtained seed randomness for sector %d: %x", tm.ActorAddr, sectorNumber, seedRandomness)
	return seedRandomness
}

func (tm *TestUnmanagedMiner) generateProveCommit(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
	proofType abi.RegisteredSealProof,
	seedRandomness abi.InteractiveSealRandomness,
	pieces []abi.PieceInfo,
) []byte {
	tm.t.Logf("Miner %s: Generating proof type %d Sector Proof for sector %d...", tm.ActorAddr, proofType, sectorNumber)
	req := require.New(tm.t)

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	tm.t.Logf("Miner %s: Running proof type %d SealCommitPhase1 for sector %d...", tm.ActorAddr, proofType, sectorNumber)

	scp1, err := ffi.SealCommitPhase1(
		proofType,
		tm.sealedCids[sectorNumber],
		tm.unsealedCids[sectorNumber],
		tm.cacheDirPaths[sectorNumber],
		tm.sealedSectorPaths[sectorNumber],
		sectorNumber,
		actorId,
		tm.sealTickets[sectorNumber],
		seedRandomness,
		pieces,
	)
	req.NoError(err)

	tm.t.Logf("Miner %s: Running proof type %d SealCommitPhase2 for sector %d...", tm.ActorAddr, proofType, sectorNumber)

	sectorProof, err := ffi.SealCommitPhase2(scp1, sectorNumber, actorId)
	req.NoError(err)

	tm.t.Logf("Miner %s: Got proof type %d sector proof of length %d for sector %d", tm.ActorAddr, proofType, len(sectorProof), sectorNumber)

	return sectorProof
}

func (tm *TestUnmanagedMiner) submitMessage(
	ctx context.Context,
	params cbg.CBORMarshaler,
	value uint64,
	method abi.MethodNum,
) (*api.MsgLookup, error) {
	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return nil, aerr
	}

	tm.t.Logf("Submitting message for miner %s with method number %d", tm.ActorAddr, method)

	m, err := tm.FullNode.MpoolPushMessage(ctx, &types.Message{
		To:     tm.ActorAddr,
		From:   tm.OwnerKey.Address,
		Value:  types.FromFil(value),
		Method: method,
		Params: enc,
	}, nil)
	if err != nil {
		return nil, err
	}

	tm.t.Logf("Pushed message with CID: %s for miner %s", m.Cid(), tm.ActorAddr)

	msg, err := tm.FullNode.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	if err != nil {
		return nil, err
	}

	tm.t.Logf("Message with CID: %s has been confirmed on-chain for miner %s", m.Cid(), tm.ActorAddr)

	return msg, nil
}

func requireTempFile(t *testing.T, fileContentsReader io.Reader, size uint64) *os.File {
	// Create a temporary file
	tempFile, err := os.CreateTemp(t.TempDir(), "")
	require.NoError(t, err)

	// Copy contents from the reader to the temporary file
	bytesCopied, err := io.Copy(tempFile, fileContentsReader)
	require.NoError(t, err)

	// Ensure the expected size matches the copied size
	require.EqualValues(t, size, bytesCopied)

	// Synchronize the file's content to disk
	require.NoError(t, tempFile.Sync())

	// Reset the file pointer to the beginning of the file
	_, err = tempFile.Seek(0, io.SeekStart)
	require.NoError(t, err)

	return tempFile
}
