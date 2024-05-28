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

// TestUnmanagedMiner is a miner that's not managed by the storage/
// infrastructure, all tasks must be manually executed, managed and scheduled by
// the test or test kit.
type TestUnmanagedMiner struct {
	t                 *testing.T
	options           nodeOpts
	cacheDir          string
	unsealedSectorDir string
	sealedSectorDir   string
	sectorSize        abi.SectorSize

	CacheDirPaths       map[abi.SectorNumber]string
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

	currentSectorNum abi.SectorNumber
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
		cacheDir:          cacheDir,
		unsealedSectorDir: unsealedSectorDir,
		sealedSectorDir:   sealedSectorDir,

		CacheDirPaths:       make(map[abi.SectorNumber]string),
		UnsealedSectorPaths: make(map[abi.SectorNumber]string),
		SealedSectorPaths:   make(map[abi.SectorNumber]string),
		SealedCids:          make(map[abi.SectorNumber]cid.Cid),
		UnsealedCids:        make(map[abi.SectorNumber]cid.Cid),
		SealTickets:         make(map[abi.SectorNumber]abi.SealRandomness),

		ActorAddr:        actorAddr,
		OwnerKey:         options.ownerKey,
		FullNode:         full,
		currentSectorNum: 101,
	}
	tm.Libp2p.PeerID = peerId
	tm.Libp2p.PrivKey = privkey

	return &tm
}

func (tm *TestUnmanagedMiner) CurrentPower(ctx context.Context) *api.MinerPower {
	head, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	p, err := tm.FullNode.StateMinerPower(ctx, tm.ActorAddr, head.Key())
	require.NoError(tm.t, err)

	return p
}

func (tm *TestUnmanagedMiner) AssertNoPower(ctx context.Context) {
	p := tm.CurrentPower(ctx)
	tm.t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
	require.True(tm.t, p.MinerPower.RawBytePower.IsZero())
}

func (tm *TestUnmanagedMiner) AssertPower(ctx context.Context, raw uint64, qa uint64) {
	req := require.New(tm.t)
	p := tm.CurrentPower(ctx)
	tm.t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
	req.Equal(raw, p.MinerPower.RawBytePower.Uint64())
	req.Equal(qa, p.MinerPower.QualityAdjPower.Uint64())

}

func (tm *TestUnmanagedMiner) OnboardCCSectorWithMockProofs(ctx context.Context, proofType abi.RegisteredSealProof) abi.SectorNumber {
	req := require.New(tm.t)
	sectorNumber := tm.currentSectorNum
	tm.currentSectorNum++

	tm.SealedCids[sectorNumber] = cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz")

	var sealRandEpoch abi.ChainEpoch

	head, err := tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)

	if head.Height() > policy.SealRandomnessLookback {
		sealRandEpoch = head.Height() - policy.SealRandomnessLookback
	} else {
		sealRandEpoch = policy.SealRandomnessLookback
		tm.t.Logf("Waiting for at least epoch %d for seal randomness (current epoch %d) ...", sealRandEpoch+5, head.Height())
		tm.FullNode.WaitTillChain(ctx, HeightAtLeast(sealRandEpoch+5))
	}

	// Step 4 : Submit the Pre-Commit to the network
	r := tm.manualOnboardingSubmitMessage(ctx, &miner14.PreCommitSectorBatchParams2{
		Sectors: []miner14.SectorPreCommitInfo{{
			Expiration:    2880 * 300,
			SectorNumber:  sectorNumber,
			SealProof:     TestSpt,
			SealedCID:     tm.SealedCids[sectorNumber],
			SealRandEpoch: sealRandEpoch,
		}},
	}, 1, builtin.MethodsMiner.PreCommitSectorBatch2)
	req.True(r.Receipt.ExitCode.IsSuccess())

	_, err = tm.FullNode.StateSectorPreCommitInfo(ctx, tm.ActorAddr, sectorNumber, r.TipSet)
	req.NoError(err)

	sectorProof := []byte{0xde, 0xad, 0xbe, 0xef}

	var seedRandomnessHeight abi.ChainEpoch

	head, err = tm.FullNode.ChainHead(ctx)
	require.NoError(tm.t, err)
	preCommitInfo, err := tm.FullNode.StateSectorPreCommitInfo(ctx, tm.ActorAddr, sectorNumber, head.Key())
	req.NoError(err)
	seedRandomnessHeight = preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

	tm.t.Logf("Waiting %d epochs for seed randomness at epoch %d (current epoch %d)...", seedRandomnessHeight-head.Height(), seedRandomnessHeight, head.Height())
	tm.FullNode.WaitTillChain(ctx, HeightAtLeast(seedRandomnessHeight+5))

	r = tm.manualOnboardingSubmitMessage(ctx, &miner14.ProveCommitSectors3Params{
		SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: sectorNumber}},
		SectorProofs:             [][]byte{sectorProof},
		RequireActivationSuccess: true,
	}, 0, builtin.MethodsMiner.ProveCommitSectors3)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())

	return sectorNumber
}

func (tm *TestUnmanagedMiner) OnboardCCSectorWithRealProofs(ctx context.Context, proofType abi.RegisteredSealProof) abi.SectorNumber {
	req := require.New(tm.t)
	sectorNumber := tm.currentSectorNum
	tm.currentSectorNum++

	// --------------------Create pre-commit for the CC sector -> we'll just pre-commit `sector size` worth of 0s for this CC sector

	// Step 1: Wait for the seal randomness to be available -> we want to draw seal randomess from a tipset that has achieved finality as PoReps are expensive to generate
	// See if we already have such a epoch and wait if not
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

	// Step 2: Write empty 32 bytes that we want to seal i.e. create our CC sector
	unsealedSectorPath := filepath.Join(tm.unsealedSectorDir, fmt.Sprintf("%d", sectorNumber))
	sealedSectorPath := filepath.Join(tm.sealedSectorDir, fmt.Sprintf("%d", sectorNumber))

	unsealedSize := abi.PaddedPieceSize(tm.sectorSize).Unpadded()
	req.NoError(os.WriteFile(unsealedSectorPath, make([]byte, unsealedSize), 0644))
	req.NoError(os.WriteFile(sealedSectorPath, make([]byte, tm.sectorSize), 0644))

	tm.t.Logf("Wrote unsealed sector to %s", unsealedSectorPath)
	tm.t.Logf("Wrote sealed sector to %s", sealedSectorPath)

	// Step 3: Generate a Pre-Commit for the CC sector -> this persists the proof on the Miner State
	tm.manualOnboardingGeneratePreCommit(ctx, tm.cacheDir, unsealedSectorPath, sealedSectorPath, sectorNumber, sealRandEpoch, proofType)

	// Step 4 : Submit the Pre-Commit to the network
	r := tm.manualOnboardingSubmitMessage(ctx, &miner14.PreCommitSectorBatchParams2{
		Sectors: []miner14.SectorPreCommitInfo{{
			Expiration:    2880 * 300,
			SectorNumber:  sectorNumber,
			SealProof:     TestSpt,
			SealedCID:     tm.SealedCids[sectorNumber],
			SealRandEpoch: sealRandEpoch,
		}},
	}, 1, builtin.MethodsMiner.PreCommitSectorBatch2)
	req.True(r.Receipt.ExitCode.IsSuccess())

	_, err = tm.FullNode.StateSectorPreCommitInfo(ctx, tm.ActorAddr, sectorNumber, r.TipSet)
	req.NoError(err)

	// Step 5: Generate a ProveCommit for the CC sector
	proveCommit := tm.manualOnboardingGenerateProveCommit(ctx, sectorNumber, proofType)

	// Step 6: Submit the ProveCommit to the network
	tm.t.Log("Submitting MinerB ProveCommitSector ...")

	r = tm.manualOnboardingSubmitMessage(ctx, &miner14.ProveCommitSectors3Params{
		SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: sectorNumber}},
		SectorProofs:             [][]byte{proveCommit},
		RequireActivationSuccess: true,
	}, 0, builtin.MethodsMiner.ProveCommitSectors3)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())

	return sectorNumber
}

func (tm *TestUnmanagedMiner) manualOnboardingGeneratePreCommit(
	ctx context.Context,
	cacheDirPath string,
	unsealedSectorPath string,
	sealedSectorPath string,
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

	tm.t.Logf("Running proof type %d SealPreCommitPhase2 for sector %d...", proofType, sectorNumber)

	sealedCid, unsealedCid, err := ffi.SealPreCommitPhase2(
		pc1,
		cacheDirPath,
		sealedSectorPath,
	)
	req.NoError(err)

	tm.t.Logf("Unsealed CID: %s", unsealedCid)
	tm.t.Logf("Sealed CID: %s", sealedCid)

	tm.SealTickets[sectorNumber] = sealTickets
	tm.SealedCids[sectorNumber] = sealedCid
	tm.UnsealedCids[sectorNumber] = unsealedCid
	tm.CacheDirPaths[sectorNumber] = cacheDirPath
	tm.UnsealedSectorPaths[sectorNumber] = unsealedSectorPath
	tm.SealedSectorPaths[sectorNumber] = sealedSectorPath
}

func (tm *TestUnmanagedMiner) manualOnboardingGenerateProveCommit(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
	proofType abi.RegisteredSealProof,
) []byte {
	req := require.New(tm.t)

	tm.t.Logf("Generating proof type %d Sector Proof ...", proofType)

	head, err := tm.FullNode.ChainHead(ctx)
	req.NoError(err)

	var seedRandomnessHeight abi.ChainEpoch

	preCommitInfo, err := tm.FullNode.StateSectorPreCommitInfo(ctx, tm.ActorAddr, sectorNumber, head.Key())
	req.NoError(err)
	seedRandomnessHeight = preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

	tm.t.Logf("Waiting %d epochs for seed randomness at epoch %d (current epoch %d)...", seedRandomnessHeight-head.Height(), seedRandomnessHeight, head.Height())
	tm.FullNode.WaitTillChain(ctx, HeightAtLeast(seedRandomnessHeight+5))

	head, err = tm.FullNode.ChainHead(ctx)
	req.NoError(err)

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes))

	tm.t.Logf("Getting seed randomness from beacon at epoch %d", seedRandomnessHeight)
	tm.t.Logf("Getting seed randomness from tickets at epoch %d", head.Height())

	rand, err := tm.FullNode.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, seedRandomnessHeight, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	seedRandomness := abi.InteractiveSealRandomness(rand)

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	tm.t.Logf("Running proof type %d SealCommitPhase1 for sector %d...", proofType, sectorNumber)

	scp1, err := ffi.SealCommitPhase1(
		proofType,
		tm.SealedCids[sectorNumber],
		tm.UnsealedCids[sectorNumber],
		tm.CacheDirPaths[sectorNumber],
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

func (tm *TestUnmanagedMiner) manualOnboardingSubmitMessage(
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
