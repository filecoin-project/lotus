package storage

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	proof0 "github.com/filecoin-project/specs-actors/actors/runtime/proof"
	tutils "github.com/filecoin-project/specs-actors/support/testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/journal"
)

type mockStorageMinerAPI struct {
	partitions     []api.Partition
	pushedMessages chan *types.Message
}

func newMockStorageMinerAPI() *mockStorageMinerAPI {
	return &mockStorageMinerAPI{
		pushedMessages: make(chan *types.Message),
	}
}

func (m *mockStorageMinerAPI) StateMinerInfo(ctx context.Context, a address.Address, key types.TipSetKey) (miner.MinerInfo, error) {
	return miner.MinerInfo{
		Worker: tutils.NewIDAddr(nil, 101),
		Owner:  tutils.NewIDAddr(nil, 101),
	}, nil
}

func (m *mockStorageMinerAPI) StateNetworkVersion(ctx context.Context, key types.TipSetKey) (network.Version, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) ChainGetRandomnessFromTickets(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	return abi.Randomness("ticket rand"), nil
}

func (m *mockStorageMinerAPI) ChainGetRandomnessFromBeacon(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	return abi.Randomness("beacon rand"), nil
}

func (m *mockStorageMinerAPI) setPartitions(ps []api.Partition) {
	m.partitions = append(m.partitions, ps...)
}

func (m *mockStorageMinerAPI) StateMinerPartitions(ctx context.Context, a address.Address, dlIdx uint64, tsk types.TipSetKey) ([]api.Partition, error) {
	return m.partitions, nil
}

func (m *mockStorageMinerAPI) StateMinerSectors(ctx context.Context, address address.Address, snos *bitfield.BitField, key types.TipSetKey) ([]*miner.SectorOnChainInfo, error) {
	var sis []*miner.SectorOnChainInfo
	if snos == nil {
		panic("unsupported")
	}
	_ = snos.ForEach(func(i uint64) error {
		sis = append(sis, &miner.SectorOnChainInfo{
			SectorNumber: abi.SectorNumber(i),
		})
		return nil
	})
	return sis, nil
}

func (m *mockStorageMinerAPI) MpoolPushMessage(ctx context.Context, message *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	m.pushedMessages <- message
	return &types.SignedMessage{
		Message: *message,
	}, nil
}

func (m *mockStorageMinerAPI) StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	return &api.MsgLookup{
		Receipt: types.MessageReceipt{
			ExitCode: 0,
		},
	}, nil
}

type mockProver struct {
}

func (m *mockProver) GenerateWinningPoSt(context.Context, abi.ActorID, []proof0.SectorInfo, abi.PoStRandomness) ([]proof0.PoStProof, error) {
	panic("implement me")
}

func (m *mockProver) GenerateWindowPoSt(ctx context.Context, aid abi.ActorID, sis []proof0.SectorInfo, pr abi.PoStRandomness) ([]proof0.PoStProof, []abi.SectorID, error) {
	return []proof0.PoStProof{
		{
			PoStProof:  abi.RegisteredPoStProof_StackedDrgWindow2KiBV1,
			ProofBytes: []byte("post-proof"),
		},
	}, nil, nil
}

type mockFaultTracker struct {
}

func (m mockFaultTracker) CheckProvable(ctx context.Context, spt abi.RegisteredSealProof, sectors []abi.SectorID) ([]abi.SectorID, error) {
	// Returns "bad" sectors so just return nil meaning all sectors are good
	return nil, nil
}

// TestWDPostDoPost verifies that doPost will send the correct number of window
// PoST messages for a given number of partitions
func TestWDPostDoPost(t *testing.T) {
	ctx := context.Background()
	expectedMsgCount := 5

	proofType := abi.RegisteredPoStProof_StackedDrgWindow2KiBV1
	postAct := tutils.NewIDAddr(t, 100)
	workerAct := tutils.NewIDAddr(t, 101)

	mockStgMinerAPI := newMockStorageMinerAPI()

	// Get the number of sectors allowed in a partition for this proof type
	sectorsPerPartition, err := builtin0.PoStProofWindowPoStPartitionSectors(proofType)
	require.NoError(t, err)
	// Work out the number of partitions that can be included in a message
	// without exceeding the message sector limit

	require.NoError(t, err)
	partitionsPerMsg := int(miner0.AddressedSectorsMax / sectorsPerPartition)

	// Enough partitions to fill expectedMsgCount-1 messages
	partitionCount := (expectedMsgCount - 1) * partitionsPerMsg
	// Add an extra partition that should be included in the last message
	partitionCount++

	var partitions []api.Partition
	for p := 0; p < partitionCount; p++ {
		sectors := bitfield.New()
		for s := uint64(0); s < sectorsPerPartition; s++ {
			sectors.Set(s)
		}
		partitions = append(partitions, api.Partition{
			AllSectors:        sectors,
			FaultySectors:     bitfield.New(),
			RecoveringSectors: bitfield.New(),
			LiveSectors:       sectors,
			ActiveSectors:     sectors,
		})
	}
	mockStgMinerAPI.setPartitions(partitions)

	// Run window PoST
	scheduler := &WindowPoStScheduler{
		api:          mockStgMinerAPI,
		prover:       &mockProver{},
		faultTracker: &mockFaultTracker{},
		proofType:    proofType,
		actor:        postAct,
		worker:       workerAct,
		journal:      journal.NilJournal(),
	}

	di := &dline.Info{
		WPoStPeriodDeadlines:   miner0.WPoStPeriodDeadlines,
		WPoStProvingPeriod:     miner0.WPoStProvingPeriod,
		WPoStChallengeWindow:   miner0.WPoStChallengeWindow,
		WPoStChallengeLookback: miner0.WPoStChallengeLookback,
		FaultDeclarationCutoff: miner0.FaultDeclarationCutoff,
	}
	ts := mockTipSet(t)

	scheduler.startGeneratePoST(ctx, ts, di, func(posts []miner.SubmitWindowedPoStParams, err error) {
		scheduler.startSubmitPoST(ctx, ts, di, posts, func(err error) {})
	})

	// Read the window PoST messages
	for i := 0; i < expectedMsgCount; i++ {
		msg := <-mockStgMinerAPI.pushedMessages
		require.Equal(t, builtin0.MethodsMiner.SubmitWindowedPoSt, msg.Method)
		var params miner.SubmitWindowedPoStParams
		err := params.UnmarshalCBOR(bytes.NewReader(msg.Params))
		require.NoError(t, err)

		if i == expectedMsgCount-1 {
			// In the last message we only included a single partition (see above)
			require.Len(t, params.Partitions, 1)
		} else {
			// All previous messages should include the full number of partitions
			require.Len(t, params.Partitions, partitionsPerMsg)
		}
	}
}

func mockTipSet(t *testing.T) *types.TipSet {
	minerAct := tutils.NewActorAddr(t, "miner")
	c, err := cid.Decode("QmbFMke1KXqnYyBBWxB74N4c5SBnJMVAiMNRcGu6x1AwQH")
	require.NoError(t, err)
	blks := []*types.BlockHeader{
		{
			Miner:                 minerAct,
			Height:                abi.ChainEpoch(1),
			ParentStateRoot:       c,
			ParentMessageReceipts: c,
			Messages:              c,
		},
	}
	ts, err := types.NewTipSet(blks)
	require.NoError(t, err)
	return ts
}

//
// All the mock methods below here are unused
//

func (m *mockStorageMinerAPI) StateCall(ctx context.Context, message *types.Message, key types.TipSetKey) (*api.InvocResult, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateMinerDeadlines(ctx context.Context, maddr address.Address, tok types.TipSetKey) ([]api.Deadline, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateSectorPreCommitInfo(ctx context.Context, address address.Address, number abi.SectorNumber, key types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateSectorGetInfo(ctx context.Context, address address.Address, number abi.SectorNumber, key types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok types.TipSetKey) (*miner.SectorLocation, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateMinerProvingDeadline(ctx context.Context, address address.Address, key types.TipSetKey) (*dline.Info, error) {
	return &dline.Info{
		CurrentEpoch:           0,
		PeriodStart:            0,
		Index:                  0,
		Open:                   0,
		Close:                  0,
		Challenge:              0,
		FaultCutoff:            0,
		WPoStPeriodDeadlines:   miner0.WPoStPeriodDeadlines,
		WPoStProvingPeriod:     miner0.WPoStProvingPeriod,
		WPoStChallengeWindow:   miner0.WPoStChallengeWindow,
		WPoStChallengeLookback: miner0.WPoStChallengeLookback,
		FaultDeclarationCutoff: miner0.FaultDeclarationCutoff,
	}, nil
}

func (m *mockStorageMinerAPI) StateMinerPreCommitDepositForPower(ctx context.Context, address address.Address, info miner.SectorPreCommitInfo, key types.TipSetKey) (types.BigInt, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateMinerInitialPledgeCollateral(ctx context.Context, address address.Address, info miner.SectorPreCommitInfo, key types.TipSetKey) (types.BigInt, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateSearchMsg(ctx context.Context, cid cid.Cid) (*api.MsgLookup, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error) {
	return &types.Actor{
		Code: builtin0.StorageMinerActorCodeID,
	}, nil
}

func (m *mockStorageMinerAPI) StateGetReceipt(ctx context.Context, cid cid.Cid, key types.TipSetKey) (*types.MessageReceipt, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateMarketStorageDeal(ctx context.Context, id abi.DealID, key types.TipSetKey) (*api.MarketDeal, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateMinerFaults(ctx context.Context, address address.Address, key types.TipSetKey) (bitfield.BitField, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateMinerRecoveries(ctx context.Context, address address.Address, key types.TipSetKey) (bitfield.BitField, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) StateAccountKey(ctx context.Context, address address.Address, key types.TipSetKey) (address.Address, error) {
	return address, nil
}

func (m *mockStorageMinerAPI) GasEstimateMessageGas(ctx context.Context, message *types.Message, spec *api.MessageSendSpec, key types.TipSetKey) (*types.Message, error) {
	msg := *message
	msg.GasFeeCap = big.NewInt(1)
	msg.GasPremium = big.NewInt(1)
	msg.GasLimit = 2
	return &msg, nil
}

func (m *mockStorageMinerAPI) ChainHead(ctx context.Context) (*types.TipSet, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) ChainGetTipSetByHeight(ctx context.Context, epoch abi.ChainEpoch, key types.TipSetKey) (*types.TipSet, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) ChainGetBlockMessages(ctx context.Context, cid cid.Cid) (*api.BlockMessages, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) ChainReadObj(ctx context.Context, cid cid.Cid) ([]byte, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) ChainHasObj(ctx context.Context, cid cid.Cid) (bool, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) ChainGetTipSet(ctx context.Context, key types.TipSetKey) (*types.TipSet, error) {
	panic("implement me")
}

func (m *mockStorageMinerAPI) WalletSign(ctx context.Context, address address.Address, bytes []byte) (*crypto.Signature, error) {
	return nil, nil
}

func (m *mockStorageMinerAPI) WalletBalance(ctx context.Context, address address.Address) (types.BigInt, error) {
	return big.NewInt(333), nil
}

func (m *mockStorageMinerAPI) WalletHas(ctx context.Context, address address.Address) (bool, error) {
	return true, nil
}

var _ storageMinerApi = &mockStorageMinerAPI{}
