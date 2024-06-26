package kit

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math"
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
	t          *testing.T
	options    nodeOpts
	mockProofs bool

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
	Posted bool
	Error  error
}

func NewTestUnmanagedMiner(t *testing.T, full *TestFullNode, actorAddr address.Address, mockProofs bool, opts ...NodeOpt) *TestUnmanagedMiner {
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
		mockProofs:        mockProofs,
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
	tm.t.Logf("Miner %s RBP: %v, QaP: %v", tm.ActorAddr, p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
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

func (tm *TestUnmanagedMiner) mkStagedFileWithPieces(pt abi.RegisteredSealProof) ([]abi.PieceInfo, string) {
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

	return publicPieces, unsealedSectorFile.Name()
}

func (tm *TestUnmanagedMiner) SnapDeal(ctx context.Context, proofType abi.RegisteredSealProof, sectorNumber abi.SectorNumber) []abi.PieceInfo {
	updateProofType := abi.SealProofInfos[proofType].UpdateProof
	var pieces []abi.PieceInfo
	var snapProof []byte
	var newSealedCid cid.Cid

	if !tm.mockProofs {
		// generate sector key
		var unsealedPath string
		pieces, unsealedPath = tm.mkStagedFileWithPieces(proofType)

		s, err := os.Stat(tm.sealedSectorPaths[sectorNumber])
		require.NoError(tm.t, err)

		randomBytes := make([]byte, s.Size())
		_, err = io.ReadFull(rand.Reader, randomBytes)
		require.NoError(tm.t, err)

		updatePath := requireTempFile(tm.t, bytes.NewReader(randomBytes), uint64(s.Size()))
		require.NoError(tm.t, updatePath.Close())
		updateDir := filepath.Join(tm.t.TempDir(), fmt.Sprintf("update-%d", sectorNumber))
		require.NoError(tm.t, os.MkdirAll(updateDir, 0700))

		var newUnsealedCid cid.Cid
		newSealedCid, newUnsealedCid, err = ffi.SectorUpdate.EncodeInto(updateProofType, updatePath.Name(), updateDir,
			tm.sealedSectorPaths[sectorNumber], tm.cacheDirPaths[sectorNumber], unsealedPath, pieces)
		require.NoError(tm.t, err)

		vp, err := ffi.SectorUpdate.GenerateUpdateVanillaProofs(updateProofType, tm.sealedCids[sectorNumber],
			newSealedCid, newUnsealedCid, updatePath.Name(), updateDir, tm.sealedSectorPaths[sectorNumber],
			tm.cacheDirPaths[sectorNumber])
		require.NoError(tm.t, err)

		snapProof, err = ffi.SectorUpdate.GenerateUpdateProofWithVanilla(updateProofType, tm.sealedCids[sectorNumber],
			newSealedCid, newUnsealedCid, vp)
		require.NoError(tm.t, err)
	} else {
		pieces = []abi.PieceInfo{{
			Size:     abi.PaddedPieceSize(tm.options.sectorSize),
			PieceCID: cid.MustParse("baga6ea4seaqlhznlutptgfwhffupyer6txswamerq5fc2jlwf2lys2mm5jtiaeq"),
		}}
		snapProof = []byte{0xde, 0xad, 0xbe, 0xef}
		newSealedCid = cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkieka")
	}

	tm.waitForMutableDeadline(ctx, sectorNumber)

	// submit proof
	var manifest []miner14.PieceActivationManifest
	for _, piece := range pieces {
		manifest = append(manifest, miner14.PieceActivationManifest{
			CID:  piece.PieceCID,
			Size: piece.Size,
		})
	}

	head, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	sl, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, head.Key())
	require.NoError(tm.t, err)

	params := &miner14.ProveReplicaUpdates3Params{
		SectorUpdates: []miner14.SectorUpdateManifest{
			{
				Sector:       sectorNumber,
				Deadline:     sl.Deadline,
				Partition:    sl.Partition,
				NewSealedCID: newSealedCid,
				Pieces:       manifest,
			},
		},
		SectorProofs:               [][]byte{snapProof},
		UpdateProofsType:           updateProofType,
		RequireActivationSuccess:   true,
		RequireNotificationSuccess: false,
	}
	r, err := tm.SubmitMessage(ctx, params, 1, builtin.MethodsMiner.ProveReplicaUpdates3)
	require.NoError(tm.t, err)
	require.True(tm.t, r.Receipt.ExitCode.IsSuccess())

	return pieces
}

func (tm *TestUnmanagedMiner) waitForMutableDeadline(ctx context.Context, sectorNum abi.SectorNumber) {
	ts, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	sl, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNum, ts.Key())
	require.NoError(tm.t, err)

	dlinfo, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, ts.Key())
	require.NoError(tm.t, err)

	sectorDeadlineOpen := sl.Deadline == dlinfo.Index
	sectorDeadlineNext := (dlinfo.Index+1)%dlinfo.WPoStPeriodDeadlines == sl.Deadline
	immutable := sectorDeadlineOpen || sectorDeadlineNext

	// Sleep for immutable epochs
	if immutable {
		dlineEpochsRemaining := dlinfo.NextOpen() - ts.Height()
		var targetEpoch abi.ChainEpoch
		if sectorDeadlineOpen {
			// sleep for remainder of deadline
			targetEpoch = ts.Height() + dlineEpochsRemaining
		} else {
			// sleep for remainder of deadline and next one
			targetEpoch = ts.Height() + dlineEpochsRemaining + dlinfo.WPoStChallengeWindow
		}
		_, err := tm.FullNode.WaitTillChainOrError(ctx, HeightAtLeast(targetEpoch+5))
		require.NoError(tm.t, err)
	}
}

func (tm *TestUnmanagedMiner) NextSectorNumber() abi.SectorNumber {
	sectorNumber := tm.currentSectorNum
	tm.currentSectorNum++
	return sectorNumber
}

func (tm *TestUnmanagedMiner) PrepareSectorForProveCommit(
	ctx context.Context,
	proofType abi.RegisteredSealProof,
	sectorNumber abi.SectorNumber,
	pieces []abi.PieceInfo,
) (seedEpoch abi.ChainEpoch, proveCommit []byte) {

	req := require.New(tm.t)

	// Wait for the pre-commitseal randomness to be available (we can only draw seal randomness from tipsets that have already achieved finality)
	preCommitSealRandEpoch := tm.waitPreCommitSealRandomness(ctx, sectorNumber, proofType)

	// Generate a Pre-Commit for the CC sector -> this persists the proof on the `TestUnmanagedMiner` Miner State
	tm.generatePreCommit(ctx, sectorNumber, preCommitSealRandEpoch, proofType, pieces)

	// --------------------Create pre-commit for the CC sector -> we'll just pre-commit `sector size` worth of 0s for this CC sector

	if !proofType.IsNonInteractive() {
		// Submit the Pre-Commit to the network
		var uc *cid.Cid
		if len(pieces) > 0 {
			unsealedCid := tm.unsealedCids[sectorNumber]
			uc = &unsealedCid
		}
		r, err := tm.SubmitMessage(ctx, &miner14.PreCommitSectorBatchParams2{
			Sectors: []miner14.SectorPreCommitInfo{{
				Expiration:    2880 * 300,
				SectorNumber:  sectorNumber,
				SealProof:     proofType,
				SealedCID:     tm.sealedCids[sectorNumber],
				SealRandEpoch: preCommitSealRandEpoch,
				UnsealedCid:   uc,
			}},
		}, 1, builtin.MethodsMiner.PreCommitSectorBatch2)
		req.NoError(err)
		req.True(r.Receipt.ExitCode.IsSuccess())
	}

	// Generate a ProveCommit for the CC sector
	var seedRandomness abi.InteractiveSealRandomness
	seedEpoch, seedRandomness = tm.proveCommitWaitSeed(ctx, sectorNumber, proofType)

	proveCommit = []byte{0xde, 0xad, 0xbe, 0xef} // mock prove commit
	if !tm.mockProofs {
		proveCommit = tm.generateProveCommit(ctx, sectorNumber, proofType, seedRandomness, pieces)
	}

	return seedEpoch, proveCommit
}

func (tm *TestUnmanagedMiner) SubmitProveCommit(
	ctx context.Context,
	proofType abi.RegisteredSealProof,
	sectorNumber abi.SectorNumber,
	seedEpoch abi.ChainEpoch,
	proveCommit []byte,
	pieceManifest []miner14.PieceActivationManifest,
) {

	req := require.New(tm.t)

	if proofType.IsNonInteractive() {
		req.Nil(pieceManifest, "piece manifest should be nil for Non-interactive PoRep")
	}

	// Step 6: Submit the ProveCommit to the network
	if proofType.IsNonInteractive() {
		tm.t.Log("Submitting ProveCommitSector ...")

		var provingDeadline uint64 = 7
		if tm.IsImmutableDeadline(ctx, provingDeadline) {
			// avoid immutable deadlines
			provingDeadline = 5
		}

		actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
		req.NoError(err)
		actorId := abi.ActorID(actorIdNum)

		r, err := tm.SubmitMessage(ctx, &miner14.ProveCommitSectorsNIParams{
			Sectors: []miner14.SectorNIActivationInfo{{
				SealingNumber: sectorNumber,
				SealerID:      actorId,
				SealedCID:     tm.sealedCids[sectorNumber],
				SectorNumber:  sectorNumber,
				SealRandEpoch: seedEpoch,
				Expiration:    2880 * 300,
			}},
			AggregateProof:           proveCommit,
			SealProofType:            proofType,
			AggregateProofType:       abi.RegisteredAggregationProof_SnarkPackV2,
			ProvingDeadline:          provingDeadline,
			RequireActivationSuccess: true,
		}, 1, builtin.MethodsMiner.ProveCommitSectorsNI)
		req.NoError(err)
		req.True(r.Receipt.ExitCode.IsSuccess())

		// NI-PoRep lets us determine the deadline, so we can check that it's set correctly
		sp, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, r.TipSet)
		req.NoError(err)
		req.Equal(provingDeadline, sp.Deadline)
	} else {
		tm.t.Log("Submitting ProveCommitSector ...")

		r, err := tm.SubmitMessage(ctx, &miner14.ProveCommitSectors3Params{
			SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: sectorNumber, Pieces: pieceManifest}},
			SectorProofs:             [][]byte{proveCommit},
			RequireActivationSuccess: true,
		}, 0, builtin.MethodsMiner.ProveCommitSectors3)
		req.NoError(err)
		req.True(r.Receipt.ExitCode.IsSuccess())
	}
}

func (tm *TestUnmanagedMiner) OnboardCCSector(ctx context.Context, proofType abi.RegisteredSealProof) (abi.SectorNumber, chan WindowPostResp, context.CancelFunc) {
	sectorNumber := tm.NextSectorNumber()

	if !tm.mockProofs {
		// Write empty bytes that we want to seal i.e. create our CC sector
		tm.makeAndSaveCCSector(ctx, sectorNumber)
	}

	seedEpoch, proveCommit := tm.PrepareSectorForProveCommit(ctx, proofType, sectorNumber, []abi.PieceInfo{})

	tm.SubmitProveCommit(ctx, proofType, sectorNumber, seedEpoch, proveCommit, nil)

	tm.proofType[sectorNumber] = proofType
	respCh, cancelFn := tm.wdPostLoop(ctx, sectorNumber, tm.sealedCids[sectorNumber], tm.sealedSectorPaths[sectorNumber], tm.cacheDirPaths[sectorNumber])

	return sectorNumber, respCh, cancelFn
}

func (tm *TestUnmanagedMiner) OnboardSectorWithPieces(ctx context.Context, proofType abi.RegisteredSealProof) (abi.SectorNumber, chan WindowPostResp, context.CancelFunc) {
	sectorNumber := tm.NextSectorNumber()

	// Build a sector with non 0 Pieces that we want to onboard
	var pieces []abi.PieceInfo
	if !tm.mockProofs {
		pieces = tm.mkAndSavePiecesToOnboard(ctx, sectorNumber, proofType)
	} else {
		pieces = []abi.PieceInfo{{
			Size:     abi.PaddedPieceSize(tm.options.sectorSize),
			PieceCID: cid.MustParse("baga6ea4seaqjtovkwk4myyzj56eztkh5pzsk5upksan6f5outesy62bsvl4dsha"),
		}}
	}

	_, proveCommit := tm.PrepareSectorForProveCommit(ctx, proofType, sectorNumber, pieces)

	// Submit the ProveCommit to the network
	tm.t.Log("Submitting ProveCommitSector ...")

	var manifest []miner14.PieceActivationManifest
	for _, piece := range pieces {
		manifest = append(manifest, miner14.PieceActivationManifest{
			CID:  piece.PieceCID,
			Size: piece.Size,
		})
	}

	tm.SubmitProveCommit(ctx, proofType, sectorNumber, 0, proveCommit, manifest)

	tm.proofType[sectorNumber] = proofType
	respCh, cancelFn := tm.wdPostLoop(ctx, sectorNumber, tm.sealedCids[sectorNumber], tm.sealedSectorPaths[sectorNumber], tm.cacheDirPaths[sectorNumber])

	return sectorNumber, respCh, cancelFn
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

	// Obtain the proving deadline information for the miner
	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get proving deadline: %w", err)
	}

	tm.t.Logf("Miner %s: WindowPoST(%d): ProvingDeadline: %+v", tm.ActorAddr, sectorNumber, di)

	// Fetch the sector partition for the given sector number
	sp, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get sector partition: %w", err)
	}

	tm.t.Logf("Miner %s: WindowPoST(%d): SectorPartition: %+v", tm.ActorAddr, sectorNumber, sp)

	// Calculate the start of the period, adjusting if the current deadline has passed
	periodStart := di.PeriodStart
	// calculate current deadline index because it won't be reliable from state until the first
	// challenge window cron tick after first sector onboarded
	currentDeadlineIdx := uint64(math.Abs(float64((di.CurrentEpoch - di.PeriodStart) / di.WPoStChallengeWindow)))
	if di.PeriodStart < di.CurrentEpoch && sp.Deadline <= currentDeadlineIdx {
		// If the deadline has passed in the current proving period, calculate for the next period
		// Note that di.Open may be > di.CurrentEpoch if the miner has just been enrolled in cron so
		// their deadlines haven't started rolling yet
		periodStart += di.WPoStProvingPeriod
	}

	// Calculate the exact epoch when proving should occur
	provingEpoch := periodStart + di.WPoStChallengeWindow*abi.ChainEpoch(sp.Deadline)

	tm.t.Logf("Miner %s: WindowPoST(%d): next ProvingEpoch: %d", tm.ActorAddr, sectorNumber, provingEpoch)

	return di.CurrentEpoch, provingEpoch, nil
}

func (tm *TestUnmanagedMiner) wdPostLoop(
	pctx context.Context,
	sectorNumber abi.SectorNumber,
	sealedCid cid.Cid,
	sealedPath,
	cacheDir string,
) (chan WindowPostResp, context.CancelFunc) {

	ctx, cancelFn := context.WithCancel(pctx)
	respCh := make(chan WindowPostResp, 1)

	head, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	// wait one challenge window for cron to do its thing with deadlines, just to be sure so we get
	// an accurate dline.Info whenever we ask for it
	_ = tm.FullNode.WaitTillChain(ctx, HeightAtLeast(head.Height()+miner14.WPoStChallengeWindow+5))

	go func() {
		var firstPost bool

		writeRespF := func(respErr error) {
			var send WindowPostResp
			if respErr == nil {
				if firstPost {
					return // already reported on our first post, no error to report, don't send anything
				}
				send.Posted = true
				firstPost = true
			} else {
				if ctx.Err() == nil {
					tm.t.Logf("Sector %d: WindowPoSt submission failed: %s", sectorNumber, respErr)
				}
				send.Error = respErr
			}
			select {
			case respCh <- send:
			case <-ctx.Done():
			default:
			}
		}

		var postCount int
		for ctx.Err() == nil {
			currentEpoch, nextPost, err := tm.calculateNextPostEpoch(ctx, sectorNumber)
			tm.t.Logf("Activating sector %d, next post %d, current epoch %d", sectorNumber, nextPost, currentEpoch)
			if err != nil {
				writeRespF(err)
				return
			}

			nextPost += 5 // add some padding so we're properly into the window

			if nextPost > currentEpoch {
				if _, err := tm.FullNode.WaitTillChainOrError(ctx, HeightAtLeast(nextPost)); err != nil {
					writeRespF(err)
					return
				}
			}

			err = tm.submitWindowPost(ctx, sectorNumber, sealedCid, sealedPath, cacheDir)
			writeRespF(err) // send an error, or first post, or nothing if no error and this isn't the first post
			if err != nil {
				return
			}
			postCount++
			tm.t.Logf("Sector %d: WindowPoSt #%d submitted", sectorNumber, postCount)
		}
	}()

	return respCh, cancelFn
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

	_, err = tm.SubmitMessage(ctx, &miner14.DisputeWindowedPoStParams{
		Deadline:  sp.Deadline,
		PoStIndex: 0,
	}, 1, builtin.MethodsMiner.DisputeWindowedPoSt)
	return err
}

func (tm *TestUnmanagedMiner) submitWindowPost(ctx context.Context, sectorNumber abi.SectorNumber, sealedCid cid.Cid, sealedPath, cacheDir string) error {
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
	if tm.mockProofs {
		proofBytes = []byte{0xde, 0xad, 0xbe, 0xef}
	} else {
		proofBytes, err = tm.generateWindowPost(ctx, sectorNumber, sealedCid, sealedPath, cacheDir)
		if err != nil {
			return fmt.Errorf("Miner(%s): failed to generate window post for sector %d: %w", tm.ActorAddr, sectorNumber, err)
		}
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

	r, err := tm.SubmitMessage(ctx, &miner14.SubmitWindowedPoStParams{
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
	sealedCid cid.Cid,
	sealedPath string,
	cacheDir string,
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
			SealedCID:    sealedCid,
		},
		CacheDirPath:     cacheDir,
		PoStProofType:    minerInfo.WindowPoStProofType,
		SealedSectorPath: sealedPath,
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
		ChallengedSectors: []proof.SectorInfo{{SealProof: tm.proofType[sectorNumber], SectorNumber: sectorNumber, SealedCID: sealedCid}},
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
func (tm *TestUnmanagedMiner) waitPreCommitSealRandomness(ctx context.Context, sectorNumber abi.SectorNumber, proofType abi.RegisteredSealProof) abi.ChainEpoch {
	// We want to draw seal randomness from a tipset that has already achieved finality as PreCommits are expensive to re-generate.
	// Check if we already have an epoch that is already final and wait for such an epoch if we don't have one.
	head, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	if proofType.IsNonInteractive() {
		return head.Height() - 1 // no need to wait
	}

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

func (tm *TestUnmanagedMiner) generatePreCommit(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
	sealRandEpoch abi.ChainEpoch,
	proofType abi.RegisteredSealProof,
	pieceInfo []abi.PieceInfo,
) {

	if tm.mockProofs {
		tm.sealedCids[sectorNumber] = cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz")
		if len(pieceInfo) > 0 {
			tm.unsealedCids[sectorNumber] = cid.MustParse("baga6ea4seaqjtovkwk4myyzj56eztkh5pzsk5upksan6f5outesy62bsvl4dsha")
		}
		return
	}

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

func (tm *TestUnmanagedMiner) proveCommitWaitSeed(ctx context.Context, sectorNumber abi.SectorNumber, proofType abi.RegisteredSealProof) (abi.ChainEpoch, abi.InteractiveSealRandomness) {
	req := require.New(tm.t)
	head, err := tm.FullNode.ChainHead(ctx)
	req.NoError(err)

	var seedRandomnessHeight abi.ChainEpoch

	if proofType.IsNonInteractive() {
		seedRandomnessHeight = head.Height() - 1 // no need to wait, it just can't be current epoch
	} else {
		tm.t.Logf("Miner %s: Fetching pre-commit info for sector %d...", tm.ActorAddr, sectorNumber)
		preCommitInfo, err := tm.FullNode.StateSectorPreCommitInfo(ctx, tm.ActorAddr, sectorNumber, head.Key())
		req.NoError(err)
		seedRandomnessHeight = preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

		tm.t.Logf("Miner %s: Waiting %d epochs for seed randomness at epoch %d (current epoch %d) for sector %d...", tm.ActorAddr, seedRandomnessHeight-head.Height(), seedRandomnessHeight, head.Height(), sectorNumber)
		tm.FullNode.WaitTillChain(ctx, HeightAtLeast(seedRandomnessHeight+5))

		head, err = tm.FullNode.ChainHead(ctx)
		req.NoError(err)
	}

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes))

	tm.t.Logf("Miner %s: Fetching seed randomness for sector %d...", tm.ActorAddr, sectorNumber)
	rand, err := tm.FullNode.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, seedRandomnessHeight, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	seedRandomness := abi.InteractiveSealRandomness(rand)

	tm.t.Logf("Miner %s: Obtained seed randomness for sector %d: %x", tm.ActorAddr, sectorNumber, seedRandomness)
	return seedRandomnessHeight, seedRandomness
}

func (tm *TestUnmanagedMiner) generateProveCommit(
	_ context.Context,
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

	var sectorProof []byte

	if proofType.IsNonInteractive() {
		circuitProofs, err := ffi.SealCommitPhase2CircuitProofs(scp1, sectorNumber)
		req.NoError(err)
		asvpai := proof.AggregateSealVerifyProofAndInfos{
			Miner:          actorId,
			SealProof:      proofType,
			AggregateProof: abi.RegisteredAggregationProof_SnarkPackV2,
			Infos: []proof.AggregateSealVerifyInfo{{
				Number:                sectorNumber,
				Randomness:            tm.sealTickets[sectorNumber],
				InteractiveRandomness: make([]byte, 32),
				SealedCID:             tm.sealedCids[sectorNumber],
				UnsealedCID:           tm.unsealedCids[sectorNumber],
			}},
		}
		tm.t.Logf("Miner %s: Aggregating circuit proofs for sector %d: %+v", tm.ActorAddr, sectorNumber, asvpai)
		sectorProof, err = ffi.AggregateSealProofs(asvpai, [][]byte{circuitProofs})
		req.NoError(err)
	} else {
		sectorProof, err = ffi.SealCommitPhase2(scp1, sectorNumber, actorId)
		req.NoError(err)
	}

	tm.t.Logf("Miner %s: Got proof type %d sector proof of length %d for sector %d", tm.ActorAddr, proofType, len(sectorProof), sectorNumber)

	return sectorProof
}

func (tm *TestUnmanagedMiner) SubmitMessage(
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

func (tm *TestUnmanagedMiner) WaitTillActivatedAndAssertPower(
	ctx context.Context,
	respCh chan WindowPostResp,
	sector abi.SectorNumber,
) {

	// wait till sector is activated
	select {
	case resp := <-respCh:
		require.NoError(tm.t, resp.Error)
		require.True(tm.t, resp.Posted)
	case <-ctx.Done():
		tm.t.Fatal("timed out waiting for sector activation")
	}

	// Fetch on-chain sector properties
	head, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	soi, err := tm.FullNode.StateSectorGetInfo(ctx, tm.ActorAddr, sector, head.Key())
	require.NoError(tm.t, err)
	tm.t.Logf("Miner %s SectorOnChainInfo %d: %+v", tm.ActorAddr.String(), sector, soi)

	_ = tm.FullNode.WaitTillChain(ctx, HeightAtLeast(head.Height()+5))

	tm.t.Log("Checking power after PoSt ...")

	// Miner B should now have power
	tm.AssertPower(ctx, uint64(tm.options.sectorSize), uint64(tm.options.sectorSize))

	if !tm.mockProofs {
		// WindowPost Dispute should fail
		tm.AssertDisputeFails(ctx, sector)
	} // else it would pass, which we don't want
}

func (tm *TestUnmanagedMiner) AssertDisputeFails(ctx context.Context, sector abi.SectorNumber) {
	err := tm.SubmitPostDispute(ctx, sector)
	require.Error(tm.t, err)
	require.Contains(tm.t, err.Error(), "failed to dispute valid post")
	require.Contains(tm.t, err.Error(), "(RetCode=16)")
}

func (tm *TestUnmanagedMiner) IsImmutableDeadline(ctx context.Context, deadlineIndex uint64) bool {
	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, types.EmptyTSK)
	require.NoError(tm.t, err)
	// don't rely on di.Index because if we haven't enrolled in cron it won't be ticking
	currentDeadlineIdx := uint64(math.Abs(float64((di.CurrentEpoch - di.PeriodStart) / di.WPoStChallengeWindow)))
	return currentDeadlineIdx == deadlineIndex || currentDeadlineIdx == deadlineIndex-1
}
