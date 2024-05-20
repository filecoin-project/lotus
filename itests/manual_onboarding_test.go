package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// Manually onboard CC sectors, bypassing lotus-miner onboarding pathways
func TestManualCCOnboarding(t *testing.T) {
	req := require.New(t)

	kit.QuietMiningLogs()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		blocktime = 2 * time.Millisecond

		client kit.TestFullNode
		minerA kit.TestMiner // A is a standard genesis miner
		minerB kit.TestMiner // B is a CC miner we will onboard manually
	)

	opts := []kit.NodeOpt{kit.WithAllSubsystems()}
	ens := kit.NewEnsemble(t, kit.MockProofs()).
		FullNode(&client, opts...).
		Miner(&minerA, &client, opts...).
		Start().
		InterconnectAll()
	ens.BeginMining(blocktime)

	opts = append(opts, kit.OwnerAddr(client.DefaultKey))
	ens.Miner(&minerB, &client, opts...).Start()

	maddrA, err := minerA.ActorAddress(ctx)
	req.NoError(err)

	build.Clock.Sleep(time.Second)

	t.Log("Submitting PreCommitSector...")

	maddrB, err := minerB.ActorAddress(ctx)
	req.NoError(err)

	head, err := client.ChainHead(ctx)
	req.NoError(err)

	minerBInfo, err := client.StateMinerInfo(ctx, maddrB, head.Key())
	req.NoError(err)

	preCommitParams := &miner.PreCommitSectorBatchParams2{
		Sectors: []miner.SectorPreCommitInfo{{
			Expiration:    2880 * 300,
			SectorNumber:  22,
			SealProof:     kit.TestSpt,
			SealedCID:     cid.MustParse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz"),
			SealRandEpoch: head.Height() - 200,
		}},
	}

	enc := new(bytes.Buffer)
	req.NoError(preCommitParams.MarshalCBOR(enc))

	m, err := client.MpoolPushMessage(ctx, &types.Message{
		To:     maddrB,
		From:   minerB.OwnerKey.Address,
		Value:  types.FromFil(1),
		Method: builtin.MethodsMiner.PreCommitSectorBatch2,
		Params: enc.Bytes(),
	}, nil)
	req.NoError(err)

	r, err := client.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	req.NoError(err)
	require.True(t, r.Receipt.ExitCode.IsSuccess())

	client.WaitTillChain(ctx, kit.HeightAtLeast(r.Height+miner14.PreCommitChallengeDelay+5))

	t.Log("Checking initial power...")

	p, err := client.StateMinerPower(ctx, maddrA, r.TipSet)
	req.NoError(err)
	t.Logf("MinerA RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())

	p, err = client.StateMinerPower(ctx, maddrB, r.TipSet)
	req.NoError(err)
	t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
	require.True(t, p.MinerPower.RawBytePower.IsZero())

	t.Log("Submitting ProveCommitSector...")

	bSectorNum := preCommitParams.Sectors[0].SectorNumber

	proveCommitParams := miner14.ProveCommitSectors3Params{
		SectorActivations:        []miner14.SectorActivationManifest{{SectorNumber: bSectorNum}},
		SectorProofs:             [][]byte{{0xde, 0xad, 0xbe, 0xef}},
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
	require.True(t, r.Receipt.ExitCode.IsSuccess())

	soi, err := client.StateSectorGetInfo(ctx, maddrB, bSectorNum, r.TipSet)
	req.NoError(err)
	t.Logf("SectorOnChainInfo %d: %+v", bSectorNum, soi)

	sp, err := client.StateSectorPartition(ctx, maddrB, bSectorNum, r.TipSet)
	req.NoError(err)
	t.Logf("SectorPartition %d: %+v", bSectorNum, sp)
	bSectorDeadline := sp.Deadline
	bSectorPartition := sp.Partition

	p, err = client.StateMinerPower(ctx, maddrB, r.TipSet)
	req.NoError(err)
	t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
	require.True(t, p.MinerPower.RawBytePower.IsZero())

	di, err := client.StateMinerProvingDeadline(ctx, maddrB, types.EmptyTSK)
	req.NoError(err)
	t.Logf("MinerB Deadline Info: %+v", di)

	// Use the current deadline to work out when the deadline we care about (bSectorDeadline) is open
	// and ready to receive posts
	deadlineCount := di.WPoStPeriodDeadlines
	epochsPerDeadline := uint64(di.WPoStChallengeWindow)
	currentDeadline := di.Index
	currentDeadlineStart := di.Open
	waitTillEpoch := abi.ChainEpoch((deadlineCount-currentDeadline+bSectorDeadline)*epochsPerDeadline) + currentDeadlineStart - di.WPoStProvingPeriod + 1

	t.Logf("Waiting %d until epoch %d to get to deadline %d", waitTillEpoch-di.CurrentEpoch, waitTillEpoch, bSectorDeadline)
	client.WaitTillChain(ctx, kit.HeightAtLeast(waitTillEpoch))

	// We should be up to the deadline we care about
	di, err = client.StateMinerProvingDeadline(ctx, maddrB, types.EmptyTSK)
	req.NoError(err)
	req.Equal(di.Index, bSectorDeadline, "should be in the deadline of the sector to prove")

	t.Log("Submitting WindowedPoSt...")

	head, err = client.ChainHead(ctx)
	req.NoError(err)
	rand, err := client.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, di.Open, nil, head.Key())
	require.NoError(t, err)

	postParams := miner.SubmitWindowedPoStParams{
		ChainCommitEpoch: di.Open,
		ChainCommitRand:  rand,
		Deadline:         bSectorDeadline,
		Partitions:       []miner.PoStPartition{{Index: bSectorPartition}},
		Proofs: []proof.PoStProof{{
			PoStProof:  minerBInfo.WindowPoStProofType,
			ProofBytes: []byte{0x1, 0x2, 0x3, 0x4},
		}},
	}

	enc = new(bytes.Buffer)
	req.NoError(postParams.MarshalCBOR(enc))

	m, err = client.MpoolPushMessage(ctx, &types.Message{
		To:     maddrB,
		From:   minerB.OwnerKey.Address,
		Value:  types.NewInt(0),
		Method: builtin.MethodsMiner.SubmitWindowedPoSt,
		Params: enc.Bytes(),
	}, nil)
	req.NoError(err)

	r, err = client.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
	req.NoError(err)
	require.True(t, r.Receipt.ExitCode.IsSuccess())

	t.Log("Checking power after PoSt...")

	p, err = client.StateMinerPower(ctx, maddrB, r.TipSet)
	req.NoError(err)
	t.Logf("MinerB RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())
	req.Equal(uint64(2<<10), p.MinerPower.RawBytePower.Uint64())    // 2kiB RBP
	req.Equal(uint64(2<<10), p.MinerPower.QualityAdjPower.Uint64()) // 2kiB QaP
}
