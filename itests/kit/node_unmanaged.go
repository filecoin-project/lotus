package kit

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/sync/errgroup"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-commp-utils/v2"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/batch"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
)

// TODO: make a randomiser for this
var (
	BogusPieceCid1 = cid.MustParse("baga6ea4seaqjtovkwk4myyzj56eztkh5pzsk5upksan6f5outesy62bsvl4dsha")
	BogusPieceCid2 = cid.MustParse("baga6ea4seaqlhznlutptgfwhffupyer6txswamerq5fc2jlwf2lys2mm5jtiaeq")
)

// 32 bytes of 1's: this value essentially ignored in NI-PoRep proofs, but all zeros is not recommended.
// Regardless of what we submit to the chain, actors will replace it with 32 1's anyway but we are
// also doing proof verification call directly when we prepare an aggregate NI proof.
var niPorepInteractiveRandomness = abi.InteractiveSealRandomness([]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1})

// TestUnmanagedMiner is a miner that's not managed by the storage/infrastructure, all tasks must be manually executed, managed and scheduled by the test or test kit.
// Note: `TestUnmanagedMiner` is not thread safe and assumes linear access of it's methods
type TestUnmanagedMiner struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	t          *testing.T
	options    nodeOpts
	mockProofs bool

	cacheDir          string
	unsealedSectorDir string
	sealedSectorDir   string
	currentSectorNum  abi.SectorNumber

	committedSectorsLk sync.Mutex
	committedSectors   map[abi.SectorNumber]sectorInfo

	runningWdPostLoop bool
	postsLk           sync.Mutex
	posts             []windowPost

	ActorAddr address.Address
	OwnerKey  *key.Key
	FullNode  *TestFullNode
	Libp2p    struct {
		PeerID  peer.ID
		PrivKey libp2pcrypto.PrivKey
	}
}

// sectorInfo contains all of the info we need to manage the lifecycle of the sector. These
// properties are private, but could be moved to OnboardedSector if they are useful to calling tests
// or need to be modified by WithModifyNIActivationsBeforeSubmit.
type sectorInfo struct {
	sectorNumber        abi.SectorNumber
	proofType           abi.RegisteredSealProof
	cacheDirPath        string
	unsealedSectorPath  string
	sealedSectorPath    string
	sealedCid           cid.Cid
	unsealedCid         cid.Cid
	sectorProof         []byte
	pieces              []miner14.PieceActivationManifest
	sealTickets         abi.SealRandomness
	sealRandomnessEpoch abi.ChainEpoch
	duration            abi.ChainEpoch
}

func (si sectorInfo) piecesToPieceInfos() []abi.PieceInfo {
	pcPieces := make([]abi.PieceInfo, len(si.pieces))
	for i, piece := range si.pieces {
		pcPieces[i] = abi.PieceInfo{
			Size:     piece.Size,
			PieceCID: piece.CID,
		}
	}
	return pcPieces
}

type windowPost struct {
	Posted []abi.SectorNumber
	Epoch  abi.ChainEpoch
	Error  error
}

func NewTestUnmanagedMiner(ctx context.Context, t *testing.T, full *TestFullNode, actorAddr address.Address, mockProofs bool, opts ...NodeOpt) *TestUnmanagedMiner {
	req := require.New(t)

	req.NotNil(full, "full node required when instantiating miner")

	options := DefaultNodeOpts
	for _, o := range opts {
		err := o(&options)
		req.NoError(err)
	}

	actorIdNum, err := address.IDFromAddress(actorAddr)
	req.NoError(err)

	privkey, _, err := libp2pcrypto.GenerateEd25519Key(rand.Reader)
	req.NoError(err)

	req.NotNil(options.ownerKey, "owner key is required for initializing a miner")

	peerId, err := peer.IDFromPrivateKey(privkey)
	req.NoError(err)
	tmpDir := t.TempDir()

	cacheDir := filepath.Join(tmpDir, fmt.Sprintf("cache-%s", actorAddr))
	unsealedSectorDir := filepath.Join(tmpDir, fmt.Sprintf("unsealed-%s", actorAddr))
	sealedSectorDir := filepath.Join(tmpDir, fmt.Sprintf("sealed-%s", actorAddr))

	_ = os.Mkdir(cacheDir, 0755)
	_ = os.Mkdir(unsealedSectorDir, 0755)
	_ = os.Mkdir(sealedSectorDir, 0755)

	ctx, cancel := context.WithCancel(ctx)

	tm := TestUnmanagedMiner{
		ctx:               ctx,
		cancelFunc:        cancel,
		t:                 t,
		options:           options,
		mockProofs:        mockProofs,
		cacheDir:          cacheDir,
		unsealedSectorDir: unsealedSectorDir,
		sealedSectorDir:   sealedSectorDir,

		committedSectors: make(map[abi.SectorNumber]sectorInfo),

		ActorAddr:        actorAddr,
		OwnerKey:         options.ownerKey,
		FullNode:         full,
		currentSectorNum: abi.SectorNumber(actorIdNum * 100), // include the actor id to make them unique across miners and easier to identify in logs
	}
	tm.Libp2p.PeerID = peerId
	tm.Libp2p.PrivKey = privkey

	return &tm
}

func (tm *TestUnmanagedMiner) Stop() {
	tm.cancelFunc()
	tm.AssertNoWindowPostError()
}

type OnboardOpt func(opts *onboardOpt) error
type onboardOpt struct {
	modifyNIActivationsBeforeSubmit func([]miner14.SectorNIActivationInfo) []miner14.SectorNIActivationInfo
	requireActivationSuccess        bool
	expectedExitCodes               []exitcode.ExitCode
}

func WithModifyNIActivationsBeforeSubmit(f func([]miner14.SectorNIActivationInfo) []miner14.SectorNIActivationInfo) OnboardOpt {
	return func(opts *onboardOpt) error {
		opts.modifyNIActivationsBeforeSubmit = f
		return nil
	}
}

func WithRequireActivationSuccess(requireActivationSuccess bool) OnboardOpt {
	return func(opts *onboardOpt) error {
		opts.requireActivationSuccess = requireActivationSuccess
		return nil
	}
}

func WithExpectedExitCodes(exitCodes []exitcode.ExitCode) OnboardOpt {
	return func(opts *onboardOpt) error {
		opts.expectedExitCodes = exitCodes
		return nil
	}
}

// SectorManifest is (currently) a simplified SectorAllocationManifest, allowing zero or one pieces
// to be built into a sector, where that one piece may be verified. Undefined Piece and nil Verified
// onboards a CC sector. Duration changes the default sector expiration of 300 days past the minimum.
type SectorManifest struct {
	Piece    cid.Cid
	Verified *miner14.VerifiedAllocationKey
	Duration abi.ChainEpoch
}

// SectorBatch is a builder for creating sector manifests
type SectorBatch struct {
	manifests []SectorManifest
}

// NewSectorBatch creates an empty sector batch
func NewSectorBatch() *SectorBatch {
	return &SectorBatch{manifests: []SectorManifest{}}
}

// AddEmptySectors adds the specified number of CC sectors to the batch
func (sb *SectorBatch) AddEmptySectors(count int) *SectorBatch {
	for i := 0; i < count; i++ {
		sb.manifests = append(sb.manifests, EmptySector())
	}
	return sb
}

// AddSectorsWithRandomPieces adds sectors with random pieces
func (sb *SectorBatch) AddSectorsWithRandomPieces(count int) *SectorBatch {
	for i := 0; i < count; i++ {
		sb.manifests = append(sb.manifests, SectorWithPiece(BogusPieceCid1))
	}
	return sb
}

// AddSector adds a custom sector manifest
func (sb *SectorBatch) AddSector(manifest SectorManifest) *SectorBatch {
	sb.manifests = append(sb.manifests, manifest)
	return sb
}

// EmptySector creates an empty (CC) sector with no pieces
func EmptySector() SectorManifest {
	return SectorManifest{}
}

// SectorWithPiece creates a sector with the specified piece CID
func SectorWithPiece(piece cid.Cid) SectorManifest {
	return SectorManifest{
		Piece: piece,
	}
}

// SectorWithVerifiedPiece creates a sector with a verified allocation
func SectorWithVerifiedPiece(piece cid.Cid, key *miner14.VerifiedAllocationKey) SectorManifest {
	return SectorManifest{
		Piece:    piece,
		Verified: key,
	}
}

// OnboardSectors onboards the specified number of sectors to the miner using the specified proof
// type. If `withPieces` is true and the proof type supports it, the sectors will be onboarded with
// pieces, otherwise they will be CC.
//
// This method is synchronous but each sector is prepared in a separate goroutine to speed up the
// process with real proofs.
func (tm *TestUnmanagedMiner) OnboardSectors(
	proofType abi.RegisteredSealProof,
	sectorBatch *SectorBatch,
	opts ...OnboardOpt,
) ([]abi.SectorNumber, types.TipSetKey) {

	req := require.New(tm.t)

	options := onboardOpt{}
	for _, o := range opts {
		req.NoError(o(&options))
	}

	sectors := make([]sectorInfo, len(sectorBatch.manifests))

	// Wait for the seal randomness to be available (we can only draw seal randomness from
	// tipsets that have already achieved finality)
	sealRandEpoch, err := tm.waitPreCommitSealRandomness(proofType)
	req.NoError(err)

	var eg errgroup.Group

	// For each sector, run PC1, PC2, C1 an C2, preparing for ProveCommit. If the proof needs it we
	// will also submit a precommit for the sector.
	for idx, sm := range sectorBatch.manifests {

		// We hold on to `sector`, adding new properties to it as we go along until we're finished with
		// this phase, then add it to `sectors`
		sector := tm.nextSector(proofType)
		sector.sealRandomnessEpoch = sealRandEpoch
		sector.duration = sm.Duration
		if sector.duration == 0 {
			sector.duration = builtin.EpochsInDay * 300
		}

		eg.Go(func() error {
			if sm.Piece.Defined() {
				// Build a sector with non-zero pieces to onboard
				if tm.mockProofs {
					sector.pieces = []miner14.PieceActivationManifest{{
						Size:                  abi.PaddedPieceSize(tm.options.sectorSize),
						CID:                   sm.Piece,
						VerifiedAllocationKey: sm.Verified,
					}}
				} else {
					var err error
					sector, err = tm.mkAndSavePiecesToOnboard(sector)
					if err != nil {
						return fmt.Errorf("failed to create sector with pieces: %w", err)
					}
				}
			} else {
				// Build a sector with no pieces (CC) to onboard
				if !tm.mockProofs {
					sector, err = tm.makeAndSaveCCSector(sector)
					if err != nil {
						return fmt.Errorf("failed to create CC sector: %w", err)
					}
				}
			}

			// Generate a PreCommit for the CC sector if required
			sector, err = tm.generatePreCommit(sector, sealRandEpoch)
			if err != nil {
				return fmt.Errorf("failed to generate PreCommit for sector: %w", err)
			}

			// Submit the PreCommit to the network if required
			//TODO: should do this for all sectors in the batch in one go
			err = tm.preCommitSectors(sealRandEpoch, proofType, sector)
			if err != nil {
				return fmt.Errorf("failed to submit PreCommit for sector: %w", err)
			}

			// Generate a ProveCommit for the CC sector
			sectorProof, err := tm.generateSectorProof(sector)
			if err != nil {
				return fmt.Errorf("failed to generate ProveCommit for sector: %w", err)
			}
			sector.sectorProof = sectorProof

			sectors[idx] = sector
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		tm.t.Fatal(err)
	}

	// Submit ProveCommit for all sectors
	exitCodes, tsk := tm.submitProveCommit(proofType, sectors, options.requireActivationSuccess, options.modifyNIActivationsBeforeSubmit)

	// ProveCommit may have succeeded overall, but some sectors may have failed if RequireActivationSuccess
	// was set to false. We need to return the exit codes for each sector so the caller can determine
	// which sectors failed.
	onboarded := make([]abi.SectorNumber, 0)
	for i, sector := range sectors {
		// expect them all to pass unless the caller specified otherwise
		if options.expectedExitCodes == nil {
			req.Equal(exitCodes[i], exitcode.Ok, "sector %d failed with exit code %s", sector.sectorNumber, exitCodes[i])
		} else {
			req.Equal(options.expectedExitCodes[i], exitCodes[i], "sector %d failed with exit code %s", sector.sectorNumber, exitCodes[i])
		}

		if exitCodes[i].IsSuccess() {
			// only save the sector if it was successfully committed
			tm.setCommittedSector(sectors[i])
			onboarded = append(onboarded, sector.sectorNumber)
		}
	}

	tm.wdPostLoop()

	return onboarded, tsk
}

// SnapDeal snaps a deal into a sector, generating a new sealed sector and updating the sector's state.
// WindowPoSt should continue to operate after this operation if required.
// The SectorManifest argument (currently) only impacts mock proofs, and is ignored otherwise.
func (tm *TestUnmanagedMiner) SnapDeal(sectorNumber abi.SectorNumber, sm SectorManifest) ([]abi.PieceInfo, types.TipSetKey) {
	req := require.New(tm.t)

	tm.log("Snapping a deal into sector %d ...", sectorNumber)

	si, err := tm.getCommittedSector(sectorNumber)
	req.NoError(err)

	updateProofType := abi.SealProofInfos[si.proofType].UpdateProof
	var pieces []abi.PieceInfo
	var snapProof []byte
	var newSealedCid, newUnsealedCid cid.Cid

	if !tm.mockProofs {
		var unsealedPath string
		var err error
		pieces, unsealedPath, err = tm.mkStagedFileWithPieces(si.proofType)
		req.NoError(err)

		s, err := os.Stat(si.sealedSectorPath)
		req.NoError(err)

		randomBytes := make([]byte, s.Size())
		_, err = io.ReadFull(rand.Reader, randomBytes)
		req.NoError(err)

		updatePath, err := mkTempFile(tm.t, bytes.NewReader(randomBytes), uint64(s.Size()))
		req.NoError(err)
		req.NoError(updatePath.Close())
		updateDir := filepath.Join(tm.t.TempDir(), fmt.Sprintf("update-%d", sectorNumber))
		req.NoError(os.MkdirAll(updateDir, 0700))

		newSealedCid, newUnsealedCid, err = ffi.SectorUpdate.EncodeInto(updateProofType, updatePath.Name(), updateDir,
			si.sealedSectorPath, si.cacheDirPath, unsealedPath, pieces)
		req.NoError(err)

		vp, err := ffi.SectorUpdate.GenerateUpdateVanillaProofs(updateProofType, si.sealedCid,
			newSealedCid, newUnsealedCid, updatePath.Name(), updateDir, si.sealedSectorPath, si.cacheDirPath)
		req.NoError(err)

		snapProof, err = ffi.SectorUpdate.GenerateUpdateProofWithVanilla(updateProofType, si.sealedCid, newSealedCid, newUnsealedCid, vp)
		req.NoError(err)
	} else {
		pieces = []abi.PieceInfo{{
			Size:     abi.PaddedPieceSize(tm.options.sectorSize),
			PieceCID: sm.Piece,
		}}
		snapProof = []byte{0xde, 0xad, 0xbe, 0xef}
		newSealedCid = cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkieka")
	}

	tm.waitForMutableDeadline(sectorNumber)

	var manifest []miner14.PieceActivationManifest
	for _, piece := range pieces {
		pm := miner14.PieceActivationManifest{
			CID:  piece.PieceCID,
			Size: piece.Size,
		}
		if tm.mockProofs {
			pm.VerifiedAllocationKey = sm.Verified
		}
		manifest = append(manifest, pm)
	}

	head, err := tm.FullNode.ChainHead(tm.ctx)
	req.NoError(err)

	sl, err := tm.FullNode.StateSectorPartition(tm.ctx, tm.ActorAddr, sectorNumber, head.Key())
	req.NoError(err)

	tm.log("Submitting ProveReplicaUpdates3 for sector %d ...", sectorNumber)

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
		SectorProofs:     [][]byte{snapProof},
		UpdateProofsType: updateProofType,
		// Do not require activation to succeed synchronously; avoid immutable deadline races in CI.
		RequireActivationSuccess:   false,
		RequireNotificationSuccess: false,
	}
	r, err := tm.SubmitMessage(params, 1, builtin.MethodsMiner.ProveReplicaUpdates3)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())

	si.pieces = manifest
	si.sealedCid = newSealedCid
	si.unsealedCid = newUnsealedCid
	tm.setCommittedSector(si)

	return pieces, r.TipSet
}

func (tm *TestUnmanagedMiner) ExtendSectorExpiration(sectorNumber abi.SectorNumber, expiration abi.ChainEpoch) types.TipSetKey {
	req := require.New(tm.t)

	sl, err := tm.FullNode.StateSectorPartition(tm.ctx, tm.ActorAddr, sectorNumber, types.EmptyTSK)
	req.NoError(err)

	params := &miner14.ExtendSectorExpiration2Params{
		Extensions: []miner14.ExpirationExtension2{
			{
				Deadline:      sl.Deadline,
				Partition:     sl.Partition,
				Sectors:       bitfield.NewFromSet([]uint64{uint64(sectorNumber)}),
				NewExpiration: expiration,
			},
		},
	}
	r, err := tm.SubmitMessage(params, 1, builtin.MethodsMiner.ExtendSectorExpiration2)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())

	return r.TipSet
}

func (tm *TestUnmanagedMiner) log(msg string, args ...interface{}) {
	tm.t.Logf(fmt.Sprintf("Miner %s: %s", tm.ActorAddr, msg), args...)
}

func (tm *TestUnmanagedMiner) getCommittedSector(sectorNumber abi.SectorNumber) (sectorInfo, error) {
	tm.committedSectorsLk.Lock()
	defer tm.committedSectorsLk.Unlock()
	si, ok := tm.committedSectors[sectorNumber]
	if !ok {
		return sectorInfo{}, fmt.Errorf("sector %d not found", sectorNumber)
	}
	return si, nil
}

func (tm *TestUnmanagedMiner) setCommittedSector(sectorInfo sectorInfo) {
	tm.committedSectorsLk.Lock()
	tm.committedSectors[sectorInfo.sectorNumber] = sectorInfo
	tm.committedSectorsLk.Unlock()
}

// nextSector creates a new sectorInfo{} with a new unique sector number for this miner,
// but it doesn't persist it to the miner state so we don't accidentally try and wdpost
// a sector that's either not ready or doesn't get committed.
func (tm *TestUnmanagedMiner) nextSector(proofType abi.RegisteredSealProof) sectorInfo {
	si := sectorInfo{
		sectorNumber: tm.currentSectorNum,
		proofType:    proofType,
	}
	tm.currentSectorNum++
	return si
}

func (tm *TestUnmanagedMiner) mkAndSavePiecesToOnboard(sector sectorInfo) (sectorInfo, error) {
	paddedPieceSize := abi.PaddedPieceSize(tm.options.sectorSize)
	unpaddedPieceSize := paddedPieceSize.Unpadded()

	// Generate random bytes for the piece
	randomBytes := make([]byte, unpaddedPieceSize)
	if _, err := io.ReadFull(rand.Reader, randomBytes); err != nil {
		return sectorInfo{}, err
	}

	// Create a temporary file for the first piece
	pieceFileA, err := mkTempFile(tm.t, bytes.NewReader(randomBytes), uint64(unpaddedPieceSize))
	if err != nil {
		return sectorInfo{}, err
	}

	// Generate the piece CID from the file
	pieceCIDA, err := commp.GeneratePieceCIDFromFile(sector.proofType, pieceFileA, unpaddedPieceSize)
	if err != nil {
		return sectorInfo{}, err
	}

	// Reset file offset to the beginning after CID generation
	if _, err = pieceFileA.Seek(0, io.SeekStart); err != nil {
		return sectorInfo{}, err
	}

	unsealedSectorFile, err := mkTempFile(tm.t, bytes.NewReader([]byte{}), 0)
	if err != nil {
		return sectorInfo{}, err
	}

	defer func() {
		_ = unsealedSectorFile.Close()
	}()

	// Write the piece to the staged sector file without alignment
	writtenBytes, pieceCID, err := ffi.WriteWithoutAlignment(sector.proofType, pieceFileA, unpaddedPieceSize, unsealedSectorFile)
	if err != nil {
		return sectorInfo{}, err
	}
	if unpaddedPieceSize != writtenBytes {
		return sectorInfo{}, fmt.Errorf("expected to write %d bytes, wrote %d", unpaddedPieceSize, writtenBytes)
	}
	if !pieceCID.Equals(pieceCIDA) {
		return sectorInfo{}, fmt.Errorf("expected piece CID %s, got %s", pieceCIDA, pieceCID)
	}

	// Create a struct for the piece info
	sector.pieces = []miner14.PieceActivationManifest{{
		Size: paddedPieceSize,
		CID:  pieceCIDA,
	}}

	// Create a temporary file for the sealed sector
	sealedSectorFile, err := mkTempFile(tm.t, bytes.NewReader([]byte{}), 0)
	if err != nil {
		return sectorInfo{}, err
	}

	defer func() {
		_ = sealedSectorFile.Close()
	}()

	// Update paths for the sector
	sector.sealedSectorPath = sealedSectorFile.Name()
	sector.unsealedSectorPath = unsealedSectorFile.Name()
	sector.cacheDirPath = filepath.Join(tm.cacheDir, fmt.Sprintf("%d", sector.sectorNumber))

	// Ensure the cache directory exists
	if err = os.Mkdir(sector.cacheDirPath, 0755); err != nil {
		return sectorInfo{}, err
	}

	return sector, nil
}

func (tm *TestUnmanagedMiner) makeAndSaveCCSector(sector sectorInfo) (sectorInfo, error) {
	// Create cache directory
	sector.cacheDirPath = filepath.Join(tm.cacheDir, fmt.Sprintf("%d", sector.sectorNumber))
	if err := os.Mkdir(sector.cacheDirPath, 0755); err != nil {
		return sectorInfo{}, err
	}
	tm.log("Sector %d: created cache directory at %s", sector.sectorNumber, sector.cacheDirPath)

	// Define paths for unsealed and sealed sectors
	sector.unsealedSectorPath = filepath.Join(tm.unsealedSectorDir, fmt.Sprintf("%d", sector.sectorNumber))
	sector.sealedSectorPath = filepath.Join(tm.sealedSectorDir, fmt.Sprintf("%d", sector.sectorNumber))
	unsealedSize := abi.PaddedPieceSize(tm.options.sectorSize).Unpadded()

	// Write unsealed sector file
	if err := os.WriteFile(sector.unsealedSectorPath, make([]byte, unsealedSize), 0644); err != nil {
		return sectorInfo{}, err
	}
	tm.log("Sector %d: wrote unsealed CC sector to %s", sector.sectorNumber, sector.unsealedSectorPath)

	// Write sealed sector file
	if err := os.WriteFile(sector.sealedSectorPath, make([]byte, tm.options.sectorSize), 0644); err != nil {
		return sectorInfo{}, err
	}
	tm.log("Sector %d: wrote sealed CC sector to %s", sector.sectorNumber, sector.sealedSectorPath)

	return sector, nil
}

func (tm *TestUnmanagedMiner) mkStagedFileWithPieces(pt abi.RegisteredSealProof) ([]abi.PieceInfo, string, error) {
	paddedPieceSize := abi.PaddedPieceSize(tm.options.sectorSize)
	unpaddedPieceSize := paddedPieceSize.Unpadded()

	// Generate random bytes for the piece
	randomBytes := make([]byte, unpaddedPieceSize)
	if _, err := io.ReadFull(rand.Reader, randomBytes); err != nil {
		return nil, "", err
	}

	// Create a temporary file for the first piece
	pieceFileA, err := mkTempFile(tm.t, bytes.NewReader(randomBytes), uint64(unpaddedPieceSize))
	if err != nil {
		return nil, "", err
	}

	// Generate the piece CID from the file
	pieceCIDA, err := commp.GeneratePieceCIDFromFile(pt, pieceFileA, unpaddedPieceSize)
	if err != nil {
		return nil, "", err
	}

	// Reset file offset to the beginning after CID generation
	if _, err = pieceFileA.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}

	unsealedSectorFile, err := mkTempFile(tm.t, bytes.NewReader([]byte{}), 0)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = unsealedSectorFile.Close()
	}()

	// Write the piece to the staged sector file without alignment
	writtenBytes, pieceCID, err := ffi.WriteWithoutAlignment(pt, pieceFileA, unpaddedPieceSize, unsealedSectorFile)
	if err != nil {
		return nil, "", err
	}
	if unpaddedPieceSize != writtenBytes {
		return nil, "", fmt.Errorf("expected to write %d bytes, wrote %d", unpaddedPieceSize, writtenBytes)
	}
	if !pieceCID.Equals(pieceCIDA) {
		return nil, "", fmt.Errorf("expected piece CID %s, got %s", pieceCIDA, pieceCID)
	}

	// Create a struct for the piece info
	publicPieces := []abi.PieceInfo{{
		Size:     paddedPieceSize,
		PieceCID: pieceCIDA,
	}}

	return publicPieces, unsealedSectorFile.Name(), nil
}

// waitForMutableDeadline will wait until we are not in the proving deadline for the given
// sector, or the deadline after the proving deadline.
// For safety, to avoid possible races with the window post loop, we will also avoid the
// deadline before the proving deadline.
func (tm *TestUnmanagedMiner) waitForMutableDeadline(sectorNum abi.SectorNumber) {
	req := require.New(tm.t)

	ts, err := tm.FullNode.ChainHead(tm.ctx)
	req.NoError(err)

	sl, err := tm.FullNode.StateSectorPartition(tm.ctx, tm.ActorAddr, sectorNum, ts.Key())
	req.NoError(err)

	dlinfo, err := tm.FullNode.StateMinerProvingDeadline(tm.ctx, tm.ActorAddr, ts.Key())
	req.NoError(err)

	sectorDeadlineCurrent := sl.Deadline == dlinfo.Index                                                          // we are in the proving deadline
	sectorDeadlineNext := (dlinfo.Index+1)%dlinfo.WPoStPeriodDeadlines == sl.Deadline                             // we are in the deadline after the proving deadline
	sectorDeadlinePrev := (dlinfo.Index-1+dlinfo.WPoStPeriodDeadlines)%dlinfo.WPoStPeriodDeadlines == sl.Deadline // we are in the deadline before the proving deadline

	if sectorDeadlineCurrent || sectorDeadlineNext || sectorDeadlinePrev {
		// We are in a sensitive, or immutable deadline, we need to wait
		targetEpoch := dlinfo.NextOpen() // end of current deadline
		if sectorDeadlineCurrent {
			// we are in the proving deadline, wait until the end of the next one
			targetEpoch += dlinfo.WPoStChallengeWindow
		} else if sectorDeadlinePrev {
			// we are in the deadline before the proving deadline, wait an additional window
			targetEpoch += dlinfo.WPoStChallengeWindow * 2
		}
		_, err := tm.FullNode.WaitTillChainOrError(tm.ctx, HeightAtLeast(targetEpoch+5))
		req.NoError(err)
	}
}

func (tm *TestUnmanagedMiner) preCommitSectors(
	sealRandEpoch abi.ChainEpoch,
	proofType abi.RegisteredSealProof,
	sector sectorInfo,
) error {

	if proofType.IsNonInteractive() {
		return nil
	}

	// Submit the PreCommit to the network
	if sector.proofType != proofType {
		return fmt.Errorf("sector proof type does not match PreCommit proof type")
	}
	sealedCid := sector.sealedCid
	var uc *cid.Cid
	if len(sector.pieces) > 0 {
		unsealedCid := sector.unsealedCid
		uc = &unsealedCid
	}
	head, err := tm.FullNode.ChainHead(tm.ctx)
	require.NoError(tm.t, err)
	spci := []miner14.SectorPreCommitInfo{{
		Expiration:    head.Height() + (30*builtin.EpochsInDay + 10 /* pre_commit_challenge_delay - short */) + sector.duration,
		SectorNumber:  sector.sectorNumber,
		SealProof:     proofType,
		SealedCID:     sealedCid,
		SealRandEpoch: sealRandEpoch,
		UnsealedCid:   uc,
	}}
	r, err := tm.SubmitMessage(&miner14.PreCommitSectorBatchParams2{Sectors: spci}, 1, builtin.MethodsMiner.PreCommitSectorBatch2)
	if err != nil {
		return err
	}
	tm.log("PreCommitSectorBatch2 submitted at epoch %d: %+v", r.Height, spci[0])
	if !r.Receipt.ExitCode.IsSuccess() {
		return fmt.Errorf("PreCommit failed with exit code: %s", r.Receipt.ExitCode)
	}

	return nil
}

func (tm *TestUnmanagedMiner) submitProveCommit(
	proofType abi.RegisteredSealProof,
	sectors []sectorInfo,
	requireActivationSuccess bool,
	modifyNIActivationsBeforeSubmit func([]miner14.SectorNIActivationInfo) []miner14.SectorNIActivationInfo,
) ([]exitcode.ExitCode, types.TipSetKey) {

	req := require.New(tm.t)

	sectorProofs := make([][]byte, len(sectors))
	for i, sector := range sectors {
		sectorProofs[i] = sector.sectorProof
	}

	var msgReturn *api.MsgLookup
	var provingDeadline uint64 = 7 // for niporep

	// Step 6: Submit the ProveCommit to the network
	if proofType.IsNonInteractive() {
		if tm.MaybeImmutableDeadline(provingDeadline) {
			// avoid immutable deadlines
			provingDeadline = 5
		}

		actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
		req.NoError(err)
		actorId := abi.ActorID(actorIdNum)

		head, err := tm.FullNode.ChainHead(tm.ctx)
		req.NoError(err)

		infos := make([]proof.AggregateSealVerifyInfo, len(sectors))
		activations := make([]miner14.SectorNIActivationInfo, len(sectors))
		for i, sector := range sectors {
			req.Nil(sector.pieces, "pieces should be nil for Non-interactive PoRep")

			infos[i] = proof.AggregateSealVerifyInfo{
				Number:                sector.sectorNumber,
				Randomness:            sector.sealTickets,
				InteractiveRandomness: niPorepInteractiveRandomness,
				SealedCID:             sector.sealedCid,
				UnsealedCID:           sector.unsealedCid,
			}

			activations[i] = miner14.SectorNIActivationInfo{
				SealingNumber: sector.sectorNumber,
				SealerID:      actorId,
				SealedCID:     sector.sealedCid,
				SectorNumber:  sector.sectorNumber,
				SealRandEpoch: sector.sealRandomnessEpoch,
				Expiration:    head.Height() + sector.duration,
			}
		}

		var sectorProof []byte
		if tm.mockProofs {
			sectorProof = []byte{0xde, 0xad, 0xbe, 0xef}
		} else {
			asvpai := proof.AggregateSealVerifyProofAndInfos{
				Miner:          actorId,
				SealProof:      proofType,
				AggregateProof: abi.RegisteredAggregationProof_SnarkPackV2,
				Infos:          infos,
			}
			tm.log("Aggregating circuit proofs: %+v", asvpai)
			sectorProof, err = ffi.AggregateSealProofs(asvpai, sectorProofs)
			req.NoError(err)

			asvpai.Proof = sectorProof

			verified, err := ffi.VerifyAggregateSeals(asvpai)
			req.NoError(err)
			req.True(verified, "failed to verify aggregated circuit proof")
		}

		if modifyNIActivationsBeforeSubmit != nil {
			activations = modifyNIActivationsBeforeSubmit(activations)
		}

		tm.log("Submitting ProveCommitSectorsNI ...")
		msgReturn, err = tm.SubmitMessage(&miner14.ProveCommitSectorsNIParams{
			Sectors:                  activations,
			AggregateProof:           sectorProof,
			SealProofType:            proofType,
			AggregateProofType:       abi.RegisteredAggregationProof_SnarkPackV2,
			ProvingDeadline:          provingDeadline,
			RequireActivationSuccess: requireActivationSuccess,
		}, 1, builtin.MethodsMiner.ProveCommitSectorsNI)
		req.NoError(err)
		req.True(msgReturn.Receipt.ExitCode.IsSuccess())
	} else {
		// else standard porep		activations := make([]miner14.SectorActivationManifest, len(sectors))
		activations := make([]miner14.SectorActivationManifest, len(sectors))
		for i, sector := range sectors {
			activations[i] = miner14.SectorActivationManifest{SectorNumber: sector.sectorNumber}
			if len(sector.pieces) > 0 {
				activations[i].Pieces = sector.pieces
			}
		}

		tm.log("Submitting ProveCommitSectors3 with activations: %+v", activations)
		var err error
		msgReturn, err = tm.SubmitMessage(&miner14.ProveCommitSectors3Params{
			SectorActivations:        activations,
			SectorProofs:             sectorProofs,
			RequireActivationSuccess: requireActivationSuccess,
		}, 0, builtin.MethodsMiner.ProveCommitSectors3)
		req.NoError(err)
		req.True(msgReturn.Receipt.ExitCode.IsSuccess())
	}

	var returnValue batch.BatchReturn
	req.NoError(returnValue.UnmarshalCBOR(bytes.NewReader(msgReturn.Receipt.Return)))
	exitCodes := returnValue.Codes()

	for i, sector := range sectors {
		si, err := tm.FullNode.StateSectorGetInfo(tm.ctx, tm.ActorAddr, sector.sectorNumber, msgReturn.TipSet)
		tm.log("SectorOnChainInfo for sector %d w/ exit code %s: %+v", sector.sectorNumber, exitCodes[i], si)
		if exitCodes[i].IsSuccess() {
			req.NoError(err)
			req.Equal(si.SectorNumber, sector.sectorNumber)
			// To check the activation epoch, we use Parents() rather than Height-1 to account for null rounds
			ts, err := tm.FullNode.ChainGetTipSet(tm.ctx, msgReturn.TipSet)
			req.NoError(err)
			parent, err := tm.FullNode.ChainGetTipSet(tm.ctx, ts.Parents())
			req.NoError(err)
			req.Equal(si.Activation, parent.Height())
		} else {
			req.Nil(si, "sector should not be on chain")
		}
		if proofType.IsNonInteractive() {
			// NI-PoRep lets us determine the deadline, so we can check that it's set correctly
			sp, err := tm.FullNode.StateSectorPartition(tm.ctx, tm.ActorAddr, sector.sectorNumber, msgReturn.TipSet)
			if exitCodes[i].IsSuccess() {
				req.NoError(err)
				req.Equal(provingDeadline, sp.Deadline)
			} else {
				req.ErrorContains(err, fmt.Sprintf("sector %d not due at any deadline", sector.sectorNumber))
			}
		}
	}

	return exitCodes, msgReturn.TipSet
}

func (tm *TestUnmanagedMiner) wdPostLoop() {
	if tm.runningWdPostLoop {
		return
	}
	tm.runningWdPostLoop = true

	head, err := tm.FullNode.ChainHead(tm.ctx)
	require.NoError(tm.t, err)

	// wait one challenge window for cron to do its thing with deadlines, just to be sure so we get
	// an accurate dline.Info whenever we ask for it
	_ = tm.FullNode.WaitTillChain(tm.ctx, HeightAtLeast(head.Height()+miner14.WPoStChallengeWindow+5))

	go func() {
		var postCount int

		recordPostOrError := func(post windowPost) {
			head, err := tm.FullNode.ChainHead(tm.ctx)
			if err != nil {
				tm.log("WindowPoSt submission failed to get chain head: %s", err)
			} else {
				post.Epoch = head.Height()
			}
			if post.Error != nil && tm.ctx.Err() == nil {
				tm.log("WindowPoSt submission failed for sectors %v at epoch %d: %s", post.Posted, post.Epoch, post.Error)
			} else if tm.ctx.Err() == nil {
				postCount++
				tm.log("WindowPoSt loop completed post %d at epoch %d for sectors %v", postCount, post.Epoch, post.Posted)
			}
			if tm.ctx.Err() == nil {
				tm.postsLk.Lock()
				tm.posts = append(tm.posts, post)
				tm.postsLk.Unlock()
			}
		}

		for tm.ctx.Err() == nil {
			postSectors, err := tm.sectorsToPost()
			if err != nil {
				recordPostOrError(windowPost{Error: err})
				return
			}
			tm.log("WindowPoST sectors to post in this challenge window: %v", postSectors)
			if len(postSectors) > 0 {
				// something to post now
				if err = tm.submitWindowPost(postSectors); err != nil {
					recordPostOrError(windowPost{Error: err})
					return
				}
				recordPostOrError(windowPost{Posted: postSectors})
			}

			// skip to next challenge window
			if err := tm.waitForNextPostDeadline(); err != nil {
				recordPostOrError(windowPost{Error: err})
				return
			}
		}
	}()
}

// waitForNextPostDeadline waits until we are within the next challenge window for this miner
func (tm *TestUnmanagedMiner) waitForNextPostDeadline() error {
	ctx, cancel := context.WithCancel(tm.ctx)
	defer cancel()

	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, types.EmptyTSK)
	if err != nil {
		return fmt.Errorf("waitForNextPostDeadline: failed to get proving deadline: %w", err)
	}
	currentDeadlineIdx := CurrentDeadlineIndex(di)
	nextDeadlineEpoch := di.PeriodStart + di.WPoStChallengeWindow*abi.ChainEpoch(currentDeadlineIdx+1)

	tm.log("Window PoST waiting until next challenge window, currentDeadlineIdx: %d, nextDeadlineEpoch: %d", currentDeadlineIdx, nextDeadlineEpoch)

	heads, err := tm.FullNode.ChainNotify(ctx)
	if err != nil {
		return err
	}

	for chg := range heads {
		for _, c := range chg {
			if c.Type != "apply" {
				continue
			}
			if ts := c.Val; ts.Height() >= nextDeadlineEpoch+5 { // add some buffer
				return nil
			}
		}
	}

	return fmt.Errorf("waitForNextPostDeadline: failed to wait for nextDeadlineEpoch %d", nextDeadlineEpoch)
}

// sectorsToPost returns the sectors that are due to be posted in the current challenge window
func (tm *TestUnmanagedMiner) sectorsToPost() ([]abi.SectorNumber, error) {
	di, err := tm.FullNode.StateMinerProvingDeadline(tm.ctx, tm.ActorAddr, types.EmptyTSK)
	if err != nil {
		return nil, fmt.Errorf("failed to get proving deadline: %w", err)
	}

	currentDeadlineIdx := CurrentDeadlineIndex(di)

	var allSectors []abi.SectorNumber
	var sectorsToPost []abi.SectorNumber

	tm.committedSectorsLk.Lock()
	for _, sector := range tm.committedSectors {
		allSectors = append(allSectors, sector.sectorNumber)
	}
	tm.committedSectorsLk.Unlock()

	for _, sectorNumber := range allSectors {
		sp, err := tm.FullNode.StateSectorPartition(tm.ctx, tm.ActorAddr, sectorNumber, types.EmptyTSK)
		if err != nil {
			return nil, fmt.Errorf("failed to get sector partition: %w", err)
		}
		if sp.Deadline == currentDeadlineIdx {
			sectorsToPost = append(sectorsToPost, sectorNumber)
		}
	}

	return sectorsToPost, nil
}

func (tm *TestUnmanagedMiner) submitPostDispute(sectorNumber abi.SectorNumber) error {
	tm.log("Starting dispute submission for sector %d", sectorNumber)

	head, err := tm.FullNode.ChainHead(tm.ctx)
	if err != nil {
		return fmt.Errorf("MinerB(%s): failed to get chain head: %w", tm.ActorAddr, err)
	}

	sp, err := tm.FullNode.StateSectorPartition(tm.ctx, tm.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return fmt.Errorf("MinerB(%s): failed to get sector partition for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}

	di, err := tm.FullNode.StateMinerProvingDeadline(tm.ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("MinerB(%s): failed to get proving deadline for sector %d: %w", tm.ActorAddr, sectorNumber, err)
	}

	disputeEpoch := di.Close + 5
	tm.log("Sector %d - Waiting %d epochs until epoch %d to submit dispute", sectorNumber, disputeEpoch-head.Height(), disputeEpoch)

	tm.FullNode.WaitTillChain(tm.ctx, HeightAtLeast(disputeEpoch))

	tm.log("Sector %d - Disputing WindowedPoSt to confirm validity at epoch %d", sectorNumber, disputeEpoch)

	_, err = tm.SubmitMessage(&miner14.DisputeWindowedPoStParams{
		Deadline:  sp.Deadline,
		PoStIndex: 0,
	}, 1, builtin.MethodsMiner.DisputeWindowedPoSt)
	return err
}

func (tm *TestUnmanagedMiner) submitWindowPost(sectorNumbers []abi.SectorNumber) error {
	if len(sectorNumbers) == 0 {
		return fmt.Errorf("no sectors to submit window post for")
	}

	// We are limited to PoSting PoStedPartitionsMax (3) partitions at a time, so we need to group the
	// sectors by partition and submit the PoSts in batches of PoStedPartitionsMax.

	partitionMap := make(map[uint64][]sectorInfo)

	head, err := tm.FullNode.ChainHead(tm.ctx)
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to get chain head: %w", tm.ActorAddr, err)
	}

	di, err := tm.FullNode.StateMinerProvingDeadline(tm.ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to get proving deadline: %w", tm.ActorAddr, err)
	}
	chainRandomnessEpoch := di.Challenge

	for _, sectorNumber := range sectorNumbers {
		sector, err := tm.getCommittedSector(sectorNumber)
		if err != nil {
			return fmt.Errorf("Miner(%s): failed to get committed sector %d: %w", tm.ActorAddr, sectorNumber, err)
		}

		sp, err := tm.FullNode.StateSectorPartition(tm.ctx, tm.ActorAddr, sectorNumber, head.Key())
		if err != nil {
			return fmt.Errorf("Miner(%s): failed to get sector partition for sector %d: %w", tm.ActorAddr, sectorNumber, err)
		}

		if di.Index != sp.Deadline {
			return fmt.Errorf("Miner(%s): sector %d is not in the expected deadline %d, but %d", tm.ActorAddr, sectorNumber, sp.Deadline, di.Index)
		}

		if _, ok := partitionMap[sp.Partition]; !ok {
			partitionMap[sp.Partition] = make([]sectorInfo, 0)
		}
		partitionMap[sp.Partition] = append(partitionMap[sp.Partition], sector)
	}

	chainRandomness, err := tm.FullNode.StateGetRandomnessFromTickets(tm.ctx, crypto.DomainSeparationTag_PoStChainCommit, chainRandomnessEpoch,
		nil, head.Key())
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to get chain randomness for deadline %d: %w", tm.ActorAddr, di.Index, err)
	}

	minerInfo, err := tm.FullNode.StateMinerInfo(tm.ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("Miner(%s): failed to get miner info: %w", tm.ActorAddr, err)
	}

	postMessages := make([]cid.Cid, 0)

	submit := func(partitions []uint64, sectors []sectorInfo) error {
		var proofBytes []byte
		if tm.mockProofs {
			proofBytes = []byte{0xde, 0xad, 0xbe, 0xef}
		} else {
			proofBytes, err = tm.generateWindowPost(sectors)
			if err != nil {
				return fmt.Errorf("Miner(%s): failed to generate window post for deadline %d, partitions %v: %w", tm.ActorAddr, di.Index, partitions, err)
			}
		}

		tm.log("WindowPoST submitting %d sectors for deadline %d, partitions %v", len(sectors), di.Index, partitions)

		pp := make([]miner14.PoStPartition, len(partitions))
		for i, p := range partitions {
			pp[i] = miner14.PoStPartition{Index: p}
		}

		// Push all our post messages to the mpool and wait for them later. The blockminer might have
		// ceased mining while it waits to see our posts in the mpool and if we wait for the first
		// message to land but it's waiting for a later sector then we'll deadlock.
		mCid, err := tm.mpoolPushMessage(&miner14.SubmitWindowedPoStParams{
			ChainCommitEpoch: chainRandomnessEpoch,
			ChainCommitRand:  chainRandomness,
			Deadline:         di.Index,
			Partitions:       pp,
			Proofs:           []proof.PoStProof{{PoStProof: minerInfo.WindowPoStProofType, ProofBytes: proofBytes}}, // can only have 1
		}, 0, builtin.MethodsMiner.SubmitWindowedPoSt)
		if err != nil {
			return fmt.Errorf("Miner(%s): failed to submit PoSt for deadline %d, partitions %v: %w", tm.ActorAddr, di.Index, partitions, err)
		}

		postMessages = append(postMessages, mCid)

		return nil
	}

	toSubmitPartitions := make([]uint64, 0)
	toSubmitSectors := make([]sectorInfo, 0)
	for partition, sectors := range partitionMap {
		if len(sectors) == 0 {
			continue
		}
		toSubmitPartitions = append(toSubmitPartitions, partition)
		toSubmitSectors = append(toSubmitSectors, sectors...)
		if len(toSubmitPartitions) == miner14.PoStedPartitionsMax {
			if err := submit(toSubmitPartitions, toSubmitSectors); err != nil {
				return err
			}
			toSubmitPartitions = make([]uint64, 0)
			toSubmitSectors = make([]sectorInfo, 0)
		}
	}
	if len(toSubmitPartitions) > 0 {
		if err := submit(toSubmitPartitions, toSubmitSectors); err != nil {
			return err
		}
	}

	tm.log("%d WindowPoST messages submitted for %d sectors, waiting for them to land on chain ...", len(postMessages), len(sectorNumbers))
	for _, mCid := range postMessages {
		r, err := tm.waitMessage(mCid)
		if err != nil {
			return fmt.Errorf("Miner(%s): failed to wait for PoSt message %s: %w", tm.ActorAddr, mCid, err)
		}
		if !r.Receipt.ExitCode.IsSuccess() {
			return fmt.Errorf("Miner(%s): PoSt submission failed for deadline %d: %s", tm.ActorAddr, di.Index, r.Receipt.ExitCode)
		}
	}

	tm.log("WindowPoST(%v) submitted for deadline %d", sectorNumbers, di.Index)

	return nil
}

func (tm *TestUnmanagedMiner) generateWindowPost(sectorInfos []sectorInfo) ([]byte, error) {
	head, err := tm.FullNode.ChainHead(tm.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain head: %w", err)
	}

	minerInfo, err := tm.FullNode.StateMinerInfo(tm.ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return nil, fmt.Errorf("failed to get miner info: %w", err)
	}

	di, err := tm.FullNode.StateMinerProvingDeadline(tm.ctx, tm.ActorAddr, types.EmptyTSK)
	if err != nil {
		return nil, fmt.Errorf("failed to get proving deadline: %w", err)
	}

	minerAddrBytes := new(bytes.Buffer)
	if err := tm.ActorAddr.MarshalCBOR(minerAddrBytes); err != nil {
		return nil, fmt.Errorf("failed to marshal miner address: %w", err)
	}

	rand, err := tm.FullNode.StateGetRandomnessFromBeacon(tm.ctx, crypto.DomainSeparationTag_WindowedPoStChallengeSeed, di.Challenge, minerAddrBytes.Bytes(), head.Key())
	if err != nil {
		return nil, fmt.Errorf("failed to get randomness: %w", err)
	}
	postRand := abi.PoStRandomness(rand)
	postRand[31] &= 0x3f // make fr32 compatible

	privateSectorInfo := make([]ffi.PrivateSectorInfo, len(sectorInfos))
	proofSectorInfo := make([]proof.SectorInfo, len(sectorInfos))
	for i, sector := range sectorInfos {
		privateSectorInfo[i] = ffi.PrivateSectorInfo{
			SectorInfo: proof.SectorInfo{
				SealProof:    sector.proofType,
				SectorNumber: sector.sectorNumber,
				SealedCID:    sector.sealedCid,
			},
			CacheDirPath:     sector.cacheDirPath,
			PoStProofType:    minerInfo.WindowPoStProofType,
			SealedSectorPath: sector.sealedSectorPath,
		}
		proofSectorInfo[i] = proof.SectorInfo{
			SealProof:    sector.proofType,
			SectorNumber: sector.sectorNumber,
			SealedCID:    sector.sealedCid,
		}
	}

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor ID: %w", err)
	}
	actorId := abi.ActorID(actorIdNum)

	windowProofs, faultySectors, err := ffi.GenerateWindowPoSt(actorId, ffi.NewSortedPrivateSectorInfo(privateSectorInfo...), postRand)
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
		ChallengedSectors: proofSectorInfo,
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

func (tm *TestUnmanagedMiner) waitPreCommitSealRandomness(proofType abi.RegisteredSealProof) (abi.ChainEpoch, error) {
	// We want to draw seal randomness from a tipset that has already achieved finality as PreCommits are expensive to re-generate.
	// Check if we already have an epoch that is already final and wait for such an epoch if we don't have one.
	head, err := tm.FullNode.ChainHead(tm.ctx)
	if err != nil {
		return 0, err
	}

	sealLookback := policy.SealRandomnessLookback

	if proofType.IsNonInteractive() {
		// For NI-PoRep there isn't a strict wait, but we should wait enough epochs to be sure to avoid reorgs
		sealLookback = 150
	}

	var sealRandEpoch abi.ChainEpoch
	if head.Height() > sealLookback {
		sealRandEpoch = head.Height() - sealLookback
	} else {
		sealRandEpoch = sealLookback
		tm.log("Waiting for at least epoch %d for seal randomness (current epoch %d)...", sealRandEpoch+5, head.Height())
		head = tm.FullNode.WaitTillChain(tm.ctx, HeightAtLeast(sealRandEpoch+5))
	}

	tm.log("Using seal randomness from epoch %d (now at head %d)", sealRandEpoch, head.Height())

	return sealRandEpoch, nil
}

// generatePreCommit is goroutine safe, only using assertions, no FailNow or other panic inducing methods.
func (tm *TestUnmanagedMiner) generatePreCommit(sector sectorInfo, sealRandEpoch abi.ChainEpoch) (sectorInfo, error) {
	if tm.mockProofs {
		sector.sealedCid = cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz")
		switch len(sector.pieces) {
		case 0:
		case 1:
			sector.unsealedCid = sector.pieces[0].CID
		default:
			require.FailNow(tm.t, "generatePreCommit: multiple pieces not supported") // yet
		}
		return sector, nil
	}

	tm.log("Generating proof type %d PreCommit for sector %d...", sector.proofType, sector.sectorNumber)

	head, err := tm.FullNode.ChainHead(tm.ctx)
	if err != nil {
		return sectorInfo{}, fmt.Errorf("failed to get chain head for sector %d: %w", sector.sectorNumber, err)
	}

	minerAddrBytes := new(bytes.Buffer)
	if err := tm.ActorAddr.MarshalCBOR(minerAddrBytes); err != nil {
		return sectorInfo{}, fmt.Errorf("failed to marshal address for sector %d: %w", sector.sectorNumber, err)
	}

	rand, err := tm.FullNode.StateGetRandomnessFromTickets(tm.ctx, crypto.DomainSeparationTag_SealRandomness, sealRandEpoch, minerAddrBytes.Bytes(), head.Key())
	if err != nil {
		return sectorInfo{}, fmt.Errorf("failed to get randomness for sector %d: %w", sector.sectorNumber, err)
	}
	sealTickets := abi.SealRandomness(rand)

	tm.log("Running proof type %d SealPreCommitPhase1 for sector %d...", sector.proofType, sector.sectorNumber)

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	if err != nil {
		return sectorInfo{}, fmt.Errorf("failed to get actor ID for sector %d: %w", sector.sectorNumber, err)
	}
	actorId := abi.ActorID(actorIdNum)

	pc1, err := ffi.SealPreCommitPhase1(
		sector.proofType,
		sector.cacheDirPath,
		sector.unsealedSectorPath,
		sector.sealedSectorPath,
		sector.sectorNumber,
		actorId,
		sealTickets,
		sector.piecesToPieceInfos(),
	)
	if err != nil {
		return sectorInfo{}, fmt.Errorf("failed to run SealPreCommitPhase1 for sector %d: %w", sector.sectorNumber, err)
	}
	if pc1 == nil {
		return sectorInfo{}, fmt.Errorf("SealPreCommitPhase1 returned nil for sector %d", sector.sectorNumber)
	}

	tm.log("Running proof type %d SealPreCommitPhase2 for sector %d...", sector.proofType, sector.sectorNumber)

	sealedCid, unsealedCid, err := ffi.SealPreCommitPhase2(
		pc1,
		sector.cacheDirPath,
		sector.sealedSectorPath,
	)
	if err != nil {
		return sectorInfo{}, fmt.Errorf("failed to run SealPreCommitPhase2 for sector %d: %w", sector.sectorNumber, err)
	}

	tm.log("Unsealed CID for sector %d: %s", sector.sectorNumber, unsealedCid)
	tm.log("Sealed CID for sector %d: %s", sector.sectorNumber, sealedCid)

	sector.sealTickets = sealTickets
	sector.sealedCid = sealedCid
	sector.unsealedCid = unsealedCid

	return sector, nil
}

func (tm *TestUnmanagedMiner) proveCommitInteractiveRandomness(
	sectorNumber abi.SectorNumber,
	proofType abi.RegisteredSealProof,
) abi.InteractiveSealRandomness {

	req := require.New(tm.t)

	head, err := tm.FullNode.ChainHead(tm.ctx)
	req.NoError(err)

	var interactiveRandomnessHeight abi.ChainEpoch

	if proofType.IsNonInteractive() {
		// For NI-PoRep this isn't used, so we don't need to wait
		return niPorepInteractiveRandomness
	}

	tm.log("Fetching PreCommit info for sector %d...", sectorNumber)
	preCommitInfo, err := tm.FullNode.StateSectorPreCommitInfo(tm.ctx, tm.ActorAddr, sectorNumber, head.Key())
	req.NoError(err)
	interactiveRandomnessHeight = preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()
	tm.log("Waiting %d (+5) epochs for interactive randomness at epoch %d (current epoch %d) for sector %d...", interactiveRandomnessHeight-head.Height(), interactiveRandomnessHeight, head.Height(), sectorNumber)

	head = tm.FullNode.WaitTillChain(tm.ctx, HeightAtLeast(interactiveRandomnessHeight+5))

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes))

	tm.log("Fetching interactive randomness for sector %d...", sectorNumber)
	rand, err := tm.FullNode.StateGetRandomnessFromBeacon(tm.ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, interactiveRandomnessHeight, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	interactiveRandomness := abi.InteractiveSealRandomness(rand)

	return interactiveRandomness
}

func (tm *TestUnmanagedMiner) generateSectorProof(sector sectorInfo) ([]byte, error) {
	interactiveRandomness := tm.proveCommitInteractiveRandomness(sector.sectorNumber, sector.proofType)

	if tm.mockProofs {
		return []byte{0xde, 0xad, 0xbe, 0xef}, nil // mock prove commit
	}

	tm.log("Generating proof type %d Sector Proof for sector %d...", sector.proofType, sector.sectorNumber)

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor ID for sector %d: %w", sector.sectorNumber, err)
	}
	actorId := abi.ActorID(actorIdNum)

	tm.log("Running proof type %d SealCommitPhase1 for sector %d...", sector.proofType, sector.sectorNumber)

	scp1, err := ffi.SealCommitPhase1(
		sector.proofType,
		sector.sealedCid,
		sector.unsealedCid,
		sector.cacheDirPath,
		sector.sealedSectorPath,
		sector.sectorNumber,
		actorId,
		sector.sealTickets,
		interactiveRandomness,
		sector.piecesToPieceInfos(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to run SealCommitPhase1 for sector %d: %w", sector.sectorNumber, err)
	}

	tm.log("Running proof type %d SealCommitPhase2 for sector %d...", sector.proofType, sector.sectorNumber)

	var sectorProof []byte

	if sector.proofType.IsNonInteractive() {
		sectorProof, err = ffi.SealCommitPhase2CircuitProofs(scp1, sector.sectorNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to run SealCommitPhase2CircuitProofs for sector %d: %w", sector.sectorNumber, err)
		}
	} else {
		sectorProof, err = ffi.SealCommitPhase2(scp1, sector.sectorNumber, actorId)
		if err != nil {
			return nil, fmt.Errorf("failed to run SealCommitPhase2 for sector %d: %w", sector.sectorNumber, err)
		}
	}

	tm.log("Got proof type %d sector proof of length %d for sector %d", sector.proofType, len(sectorProof), sector.sectorNumber)

	return sectorProof, nil
}

func (tm *TestUnmanagedMiner) SubmitMessage(
	params cbg.CBORMarshaler,
	value uint64,
	method abi.MethodNum,
) (*api.MsgLookup, error) {

	mCid, err := tm.mpoolPushMessage(params, value, method)
	if err != nil {
		return nil, err
	}

	return tm.waitMessage(mCid)
}

func (tm *TestUnmanagedMiner) waitMessage(mCid cid.Cid) (*api.MsgLookup, error) {
	msg, err := tm.FullNode.StateWaitMsg(tm.ctx, mCid, 2, api.LookbackNoLimit, true)
	if err != nil {
		return nil, err
	}

	tm.log("Message with CID: %s has been confirmed on-chain", mCid)

	return msg, nil
}

func (tm *TestUnmanagedMiner) mpoolPushMessage(
	params cbg.CBORMarshaler,
	value uint64,
	method abi.MethodNum,
) (cid.Cid, error) {

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return cid.Undef, aerr
	}

	tm.log("Submitting message for miner with method number %d", method)

	m, err := tm.FullNode.MpoolPushMessage(tm.ctx, &types.Message{
		To:     tm.ActorAddr,
		From:   tm.OwnerKey.Address,
		Value:  types.FromFil(value),
		Method: method,
		Params: enc,
	}, nil)
	if err != nil {
		return cid.Undef, err
	}

	tm.log("Pushed message with CID: %s", m.Cid())

	return m.Cid(), nil
}

func mkTempFile(t *testing.T, fileContentsReader io.Reader, size uint64) (*os.File, error) {
	// Create a temporary file
	tempFile, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		return nil, err
	}

	// Copy contents from the reader to the temporary file
	bytesCopied, err := io.Copy(tempFile, fileContentsReader)
	if err != nil {
		return nil, err
	}

	// Ensure the expected size matches the copied size
	if int64(size) != bytesCopied {
		return nil, fmt.Errorf("expected file size %d, got %d", size, bytesCopied)
	}

	// Synchronize the file's content to disk
	if err := tempFile.Sync(); err != nil {
		return nil, err
	}

	// Reset the file pointer to the beginning of the file
	if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	return tempFile, nil
}

func (tm *TestUnmanagedMiner) WaitTillActivatedAndAssertPower(sectors []abi.SectorNumber, raw uint64, qa uint64) {
	req := require.New(tm.t)

	di, err := tm.FullNode.StateMinerProvingDeadline(tm.ctx, tm.ActorAddr, types.EmptyTSK)
	req.NoError(err)

	// wait till sectors are activated
	for _, sectorNumber := range sectors {
		tm.WaitTillPostCount(sectorNumber, 1)

		if !tm.mockProofs { // else it would pass, which we don't want
			// if the sector is in the current or previous deadline, we can't dispute the PoSt
			sl, err := tm.FullNode.StateSectorPartition(tm.ctx, tm.ActorAddr, sectorNumber, types.EmptyTSK)
			req.NoError(err)
			if sl.Deadline == CurrentDeadlineIndex(di) {
				tm.log("In current proving deadline for sector %d, waiting for the next deadline to dispute PoSt", sectorNumber)
				// wait until we're past the proving deadline for this sector
				tm.FullNode.WaitTillChain(tm.ctx, HeightAtLeast(di.Close+5))
			}
			// WindowPost Dispute should fail
			tm.AssertDisputeFails(sectorNumber)
		}
	}

	tm.t.Log("Checking power after PoSt ...")

	// Miner B should now have power
	tm.AssertPower(raw, qa)
}

func (tm *TestUnmanagedMiner) AssertNoWindowPostError() {
	tm.postsLk.Lock()
	defer tm.postsLk.Unlock()
	for _, post := range tm.posts {
		require.NoError(tm.t, post.Error, "expected no error in window post but found one at epoch %d", post.Epoch)
	}
}

func (tm *TestUnmanagedMiner) GetPostCount(sectorNumber abi.SectorNumber) int {
	return tm.GetPostCountSince(0, sectorNumber)
}

func (tm *TestUnmanagedMiner) GetPostCountSince(epoch abi.ChainEpoch, sectorNumber abi.SectorNumber) int {
	tm.postsLk.Lock()
	defer tm.postsLk.Unlock()

	var postCount int
	for _, post := range tm.posts {
		require.NoError(tm.t, post.Error, "expected no error in window post but found one at epoch %d", post.Epoch)
		if post.Error == nil {
			for _, sn := range post.Posted {
				if post.Epoch >= epoch && sn == sectorNumber {
					postCount++
				}
			}
		}
	}
	return postCount
}

func (tm *TestUnmanagedMiner) WaitTillPostCount(sectorNumber abi.SectorNumber, count int) {
	for i := 0; tm.ctx.Err() == nil; i++ {
		if i%10 == 0 {
			tm.log("Waiting for sector %d to be posted", sectorNumber)
		}
		if tm.GetPostCount(sectorNumber) >= count {
			return
		}
		select {
		case <-tm.ctx.Done():
			return
		case <-time.After(time.Millisecond * 100):
		}
	}
}

func (tm *TestUnmanagedMiner) AssertNoPower() {
	req := require.New(tm.t)

	p := tm.CurrentPower()
	tm.log("RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
	req.True(p.MinerPower.RawBytePower.IsZero())
}

func (tm *TestUnmanagedMiner) CurrentPower() *api.MinerPower {
	req := require.New(tm.t)

	head, err := tm.FullNode.ChainHead(tm.ctx)
	req.NoError(err)

	p, err := tm.FullNode.StateMinerPower(tm.ctx, tm.ActorAddr, head.Key())
	req.NoError(err)

	return p
}

func (tm *TestUnmanagedMiner) AssertPower(raw uint64, qa uint64) {
	req := require.New(tm.t)

	p := tm.CurrentPower()
	tm.log("RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
	req.Equal(raw, p.MinerPower.RawBytePower.Uint64())
	req.Equal(qa, p.MinerPower.QualityAdjPower.Uint64())
}

func (tm *TestUnmanagedMiner) AssertDisputeFails(sector abi.SectorNumber) {
	req := require.New(tm.t)

	tm.log("Disputing WindowedPoSt for sector %d ...", sector)

	err := tm.submitPostDispute(sector)
	req.Error(err)
	req.Contains(err.Error(), "failed to dispute valid post")
	req.Contains(err.Error(), "(RetCode=16)")
}

// MaybeImmutableDeadline checks if the deadline index is immutable. Immutable deadlines are defined
// as being the current deadline for the miner or their next deadline. But we also include the
// deadline after the next deadline in our check here to be conservative to avoid race conditions of
// the chain progressing before messages land.
func (tm *TestUnmanagedMiner) MaybeImmutableDeadline(deadlineIndex uint64) bool {
	req := require.New(tm.t)
	di, err := tm.FullNode.StateMinerProvingDeadline(tm.ctx, tm.ActorAddr, types.EmptyTSK)
	req.NoError(err)
	currentDeadlineIdx := CurrentDeadlineIndex(di)
	// Check if deadlineIndex is the current, next, or the one after next.
	return deadlineIndex >= currentDeadlineIdx && deadlineIndex <= currentDeadlineIdx+2
}

// CurrentDeadlineIndex manually calculates the current deadline index. This may be useful in
// situations where the miner hasn't been enrolled in cron and the deadline index isn't ticking
// so dline.Info.Index may not be accurate.
func CurrentDeadlineIndex(di *dline.Info) uint64 {
	didx := int64((di.CurrentEpoch - di.PeriodStart) / di.WPoStChallengeWindow)
	if didx < 0 { // before the first deadline
		return uint64(int64(di.WPoStPeriodDeadlines) + didx)
	}
	return uint64(didx)
}
