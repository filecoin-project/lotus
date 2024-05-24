package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/proof"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	minerparams "github.com/filecoin-project/lotus/chain/actors/builtin/miner"

	"github.com/google/uuid"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/miner"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type MinerSubsystem int

const (
	SMining MinerSubsystem = 1 << iota
	SSealing
	SSectorStorage

	SHarmony

	MinerSubsystems = iota
)

func (ms MinerSubsystem) Add(single MinerSubsystem) MinerSubsystem {
	return ms | single
}

func (ms MinerSubsystem) Has(single MinerSubsystem) bool {
	return ms&single == single
}

func (ms MinerSubsystem) All() [MinerSubsystems]bool {
	var out [MinerSubsystems]bool

	for i := range out {
		out[i] = ms&(1<<i) > 0
	}

	return out
}

// TestMiner represents a miner enrolled in an Ensemble.
type TestMiner struct {
	api.StorageMiner

	BaseAPI api.StorageMiner

	t *testing.T

	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node
	ListenAddr multiaddr.Multiaddr

	ActorAddr address.Address
	OwnerKey  *key.Key
	MineOne   func(context.Context, miner.MineReq) error
	Stop      func(context.Context) error

	FullNode       *TestFullNode
	PresealDir     string
	PresealSectors int

	Libp2p struct {
		PeerID  peer.ID
		PrivKey libp2pcrypto.PrivKey
	}

	RemoteListener net.Listener

	cacheDirPath       string
	unsealedSectorPath string
	sealedSectorPath   string

	options nodeOpts
}

func (tm *TestMiner) SubmitProveCommit(ctx context.Context, t *testing.T, sectorNumber abi.SectorNumber, sectorProof []byte) *api.MsgLookup {
	req := require.New(t)

	proveCommitParams := miner14.ProveCommitSectors3Params{
		SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: sectorNumber}},
		SectorProofs:             [][]byte{sectorProof},
		RequireActivationSuccess: true,
	}

	enc := new(bytes.Buffer)
	req.NoError(proveCommitParams.MarshalCBOR(enc))

	m, err := tm.FullNode.MpoolPushMessage(ctx, &types.Message{
		To:     tm.ActorAddr,
		From:   tm.OwnerKey.Address,
		Value:  types.FromFil(0),
		Method: builtin.MethodsMiner.ProveCommitSectors3,
		Params: enc.Bytes(),
	}, nil)
	req.NoError(err)

	r, err := tm.FullNode.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())
	return r
}

func (tm *TestMiner) GenerateWindowPost(
	ctx context.Context,
	t *testing.T,
	sectorNumber abi.SectorNumber,
	sealedCid cid.Cid,
) []byte {
	req := require.New(t)

	head, err := tm.FullNode.ChainHead(ctx)
	req.NoError(err)

	minerInfo, err := tm.FullNode.StateMinerInfo(ctx, tm.ActorAddr, head.Key())
	req.NoError(err)

	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, types.EmptyTSK)
	req.NoError(err)

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes))

	rand, err := tm.FullNode.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_WindowedPoStChallengeSeed, di.Challenge, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	postRand := abi.PoStRandomness(rand)
	postRand[31] &= 0x3f // make fr32 compatible

	privateSectorInfo := ffi.PrivateSectorInfo{
		SectorInfo: proof.SectorInfo{
			SealProof:    TestSpt,
			SectorNumber: sectorNumber,
			SealedCID:    sealedCid,
		},
		CacheDirPath:     tm.cacheDirPath,
		PoStProofType:    minerInfo.WindowPoStProofType,
		SealedSectorPath: tm.sealedSectorPath,
	}

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
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
		ChallengedSectors: []proof.SectorInfo{{SealProof: TestSpt, SectorNumber: sectorNumber, SealedCID: sealedCid}},
		Prover:            actorId,
	}

	verified, err := ffi.VerifyWindowPoSt(info)
	req.NoError(err)
	req.True(verified, "window post verification failed")

	return proofBytes
}

func (tm *TestMiner) GenerateValidProveCommit(
	ctx context.Context,
	t *testing.T,
	sectorNumber abi.SectorNumber,
	sealedCid, unsealedCid cid.Cid,
	sealTickets abi.SealRandomness,
) []byte {
	req := require.New(t)

	t.Log("Generating Sector Proof ...")

	head := tm.FullNode.GetChainHead(ctx, t)
	preCommitInfo, err := tm.FullNode.StateSectorPreCommitInfo(ctx, tm.ActorAddr, sectorNumber, head.Key())
	req.NoError(err)

	seedRandomnessHeight := preCommitInfo.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes))

	rand, err := tm.FullNode.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, seedRandomnessHeight,
		minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	seedRandomness := abi.InteractiveSealRandomness(rand)

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	t.Logf("Running SealCommitPhase1 for sector %d...", sectorNumber)

	scp1, err := ffi.SealCommitPhase1(
		TestSpt,
		sealedCid,
		unsealedCid,
		tm.cacheDirPath,
		tm.sealedSectorPath,
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

func (tm *TestMiner) SubmitPrecommit(ctx context.Context, t *testing.T, sealedCid cid.Cid, sealRandEpoch abi.ChainEpoch) *api.MsgLookup {
	req := require.New(t)
	preCommitParams := &minerparams.PreCommitSectorBatchParams2{
		Sectors: []minerparams.SectorPreCommitInfo{{
			Expiration:    2880 * 300,
			SectorNumber:  22,
			SealProof:     TestSpt,
			SealedCID:     sealedCid,
			SealRandEpoch: sealRandEpoch,
		}},
	}

	enc := new(bytes.Buffer)
	req.NoError(preCommitParams.MarshalCBOR(enc))

	m, err := tm.FullNode.MpoolPushMessage(ctx, &types.Message{
		To:     tm.ActorAddr,
		From:   tm.OwnerKey.Address,
		Value:  types.FromFil(1),
		Method: builtin.MethodsMiner.PreCommitSectorBatch2,
		Params: enc.Bytes(),
	}, nil)
	req.NoError(err)

	r, err := tm.FullNode.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	req.NoError(err)
	req.True(r.Receipt.ExitCode.IsSuccess())
	return r
}

func (tm *TestMiner) GenerateValidPreCommit(
	ctx context.Context,
	t *testing.T,
	sectorNumber abi.SectorNumber,
	sealRandEpoch abi.ChainEpoch,
) (abi.SealRandomness, cid.Cid, cid.Cid) {
	req := require.New(t)
	t.Log("Generating PreCommit ...")

	sectorSize := abi.SectorSize(2 << 10)
	unsealedSize := abi.PaddedPieceSize(sectorSize).Unpadded()
	req.NoError(os.WriteFile(tm.unsealedSectorPath, make([]byte, unsealedSize), 0644))
	req.NoError(os.WriteFile(tm.sealedSectorPath, make([]byte, sectorSize), 0644))

	head := tm.FullNode.GetChainHead(ctx, t)

	minerAddrBytes := new(bytes.Buffer)
	req.NoError(tm.ActorAddr.MarshalCBOR(minerAddrBytes))

	rand, err := tm.FullNode.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_SealRandomness, sealRandEpoch, minerAddrBytes.Bytes(), head.Key())
	req.NoError(err)
	sealTickets := abi.SealRandomness(rand)

	t.Logf("Running SealPreCommitPhase1 for sector %d...", sectorNumber)

	actorIdNum, err := address.IDFromAddress(tm.ActorAddr)
	req.NoError(err)
	actorId := abi.ActorID(actorIdNum)

	pc1, err := ffi.SealPreCommitPhase1(
		TestSpt,
		tm.cacheDirPath,
		tm.unsealedSectorPath,
		tm.sealedSectorPath,
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
		tm.cacheDirPath,
		tm.sealedSectorPath,
	)
	req.NoError(err)

	t.Logf("Unsealed CID: %s", unsealedCid)
	t.Logf("Sealed CID: %s", sealedCid)

	return sealTickets, sealedCid, unsealedCid
}

func (tm *TestMiner) PledgeSectors(ctx context.Context, n, existing int, blockNotif <-chan struct{}) {
	toCheck := tm.StartPledge(ctx, n, existing, blockNotif)
	tm.WaitSectorsProving(ctx, toCheck)
}

func (tm *TestMiner) WaitSectorsProving(ctx context.Context, toCheck map[abi.SectorNumber]struct{}) {
	tm.WaitSectorsProvingAllowFails(ctx, toCheck, map[api.SectorState]struct{}{})
}

func (tm *TestMiner) WaitSectorsProvingAllowFails(ctx context.Context, toCheck map[abi.SectorNumber]struct{}, okFails map[api.SectorState]struct{}) {
	for len(toCheck) > 0 {
		tm.FlushSealingBatches(ctx)

		states := map[api.SectorState]int{}
		for n := range toCheck {
			st, err := tm.StorageMiner.SectorsStatus(ctx, n, false)
			require.NoError(tm.t, err)
			states[st.State]++
			if st.State == api.SectorState(sealing.Proving) || st.State == api.SectorState(sealing.Available) || st.State == api.SectorState(sealing.Removed) {
				delete(toCheck, n)
			}
			if strings.Contains(string(st.State), "Fail") {
				if _, ok := okFails[st.State]; !ok {
					tm.t.Fatal("sector in a failed state", st.State)
				}
			}
		}

		build.Clock.Sleep(100 * time.Millisecond)
		fmt.Printf("WaitSectorsProving: %d %+v\n", len(toCheck), states)
	}
}

func (tm *TestMiner) StartPledge(ctx context.Context, n, existing int, blockNotif <-chan struct{}) map[abi.SectorNumber]struct{} {
	for i := 0; i < n; i++ {
		if i%3 == 0 && blockNotif != nil {
			<-blockNotif
			tm.t.Log("WAIT")
		}
		tm.t.Logf("PLEDGING %d", i)
		_, err := tm.StorageMiner.PledgeSector(ctx)
		require.NoError(tm.t, err)
	}

	for {
		s, err := tm.SectorsListNonGenesis(ctx)
		require.NoError(tm.t, err)
		fmt.Printf("Sectors: %d (n %d, ex %d)\n", len(s), n, existing)
		if len(s) >= n+existing {
			break
		}

		build.Clock.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("All sectors is fsm\n")

	s, err := tm.SectorsListNonGenesis(ctx)
	require.NoError(tm.t, err)

	toCheck := map[abi.SectorNumber]struct{}{}
	for _, number := range s {
		toCheck[number] = struct{}{}
	}

	return toCheck
}

func (tm *TestMiner) FlushSealingBatches(ctx context.Context) {
	pcb, err := tm.StorageMiner.SectorPreCommitFlush(ctx)
	require.NoError(tm.t, err)
	if pcb != nil {
		fmt.Printf("PRECOMMIT BATCH: %+v\n", pcb)
	}

	cb, err := tm.StorageMiner.SectorCommitFlush(ctx)
	require.NoError(tm.t, err)
	if cb != nil {
		fmt.Printf("COMMIT BATCH: %+v\n", cb)
	}
}

const metaFile = "sectorstore.json"

func (tm *TestMiner) AddStorage(ctx context.Context, t *testing.T, conf func(*storiface.LocalStorageMeta)) storiface.ID {
	p := t.TempDir()

	if err := os.MkdirAll(p, 0755); err != nil {
		if !os.IsExist(err) {
			require.NoError(t, err)
		}
	}

	_, err := os.Stat(filepath.Join(p, metaFile))
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}

	cfg := &storiface.LocalStorageMeta{
		ID:       storiface.ID(uuid.New().String()),
		Weight:   10,
		CanSeal:  false,
		CanStore: false,
	}

	conf(cfg)

	if !(cfg.CanStore || cfg.CanSeal) {
		t.Fatal("must specify at least one of CanStore or cfg.CanSeal")
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(p, metaFile), b, 0644)
	require.NoError(t, err)

	err = tm.StorageAddLocal(ctx, p)
	require.NoError(t, err)

	return cfg.ID
}
func (tm *TestMiner) SectorsListNonGenesis(ctx context.Context) ([]abi.SectorNumber, error) {
	l, err := tm.SectorsList(ctx)
	if err != nil {
		return nil, err
	}
	// sort just in case
	sort.Slice(l, func(i, j int) bool {
		return l[i] < l[j]
	})

	return l[tm.PresealSectors:], nil
}

// comes from https://github.com/filecoin-project/lotus/blob/8ba4355cabd25e5f65261aaa561ff676321ffbd8/storage/sealer/manager.go#L1226
// todo: have this defined in one place
type SchedInfo struct {
	CallToWork   struct{}
	EarlyRet     interface{}
	ReturnedWork interface{}
	SchedInfo    struct {
		OpenWindows []string
		Requests    []struct {
			Priority int
			SchedId  uuid.UUID
			Sector   struct {
				Miner  int
				Number int
			}
			TaskType string
		}
	}
	Waiting interface{}
}

func (tm *TestMiner) SchedInfo(ctx context.Context) SchedInfo {
	schedb, err := tm.SealingSchedDiag(ctx, false)
	require.NoError(tm.t, err)

	j, err := json.MarshalIndent(&schedb, "", "  ")
	require.NoError(tm.t, err)

	var b SchedInfo
	err = json.Unmarshal(j, &b)
	require.NoError(tm.t, err)

	return b
}
