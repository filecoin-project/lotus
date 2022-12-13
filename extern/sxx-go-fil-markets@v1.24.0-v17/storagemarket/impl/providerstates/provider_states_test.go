package providerstates_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-statemachine/fsm"
	fsmtest "github.com/filecoin-project/go-statemachine/fsm/testutil"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/shared"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/blockrecorder"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestValidateDealProposal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runValidateDealProposal := makeExecutor(ctx, eventProcessor, providerstates.ValidateDealProposal, storagemarket.StorageDealValidating)
	otherAddr, err := address.NewActorAddress([]byte("applesauce"))
	require.NoError(t, err)
	bigDataCap := big.NewIntUnsigned(uint64(defaultPieceSize))
	smallDataCap := big.NewIntUnsigned(uint64(defaultPieceSize - 1))

	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAcceptWait, deal.State)
				require.Len(t, env.peerTagger.TagCalls, 1)
				require.Equal(t, deal.Client, env.peerTagger.TagCalls[0])
			},
		},
		"verify signature fails": {
			nodeParams: nodeParams{
				VerifySignatureFails: true,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: verifying StorageDealProposal: could not verify signature", deal.Message)
			},
		},
		"provider address does not match": {
			environmentParams: environmentParams{
				Address: otherAddr,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: incorrect provider for deal", deal.Message)
			},
		},
		"MostRecentStateID errors": {
			nodeParams: nodeParams{
				MostRecentStateIDError: errors.New("couldn't get id"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: node error getting most recent state id: couldn't get id", deal.Message)
			},
		},
		"PricePerEpoch too low": {
			dealParams: dealParams{
				StoragePricePerEpoch: abi.NewTokenAmount(5000),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: storage price per epoch less than asking price: 5000 < 9765", deal.Message)
			},
		},
		"PieceSize < MinPieceSize": {
			dealParams: dealParams{
				PieceSize: abi.PaddedPieceSize(128),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: piece size less than minimum required size: 128 < 256", deal.Message)
			},
		},
		"Get balance error": {
			nodeParams: nodeParams{
				ClientMarketBalanceError: errors.New("could not get balance"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: node error getting client market balance failed: could not get balance", deal.Message)
			},
		},
		"Not enough funds": {
			nodeParams: nodeParams{
				ClientMarketBalance: big.NewInt(200*10000 - 1),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.True(t, strings.Contains(deal.Message, "deal rejected: clientMarketBalance.Available too small"))
			},
		},
		"Not enough funds due to client collateral": {
			nodeParams: nodeParams{
				ClientMarketBalance: big.NewInt(200*10000 + 99),
			},
			dealParams: dealParams{
				ClientCollateral: big.NewInt(100),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.True(t, strings.Contains(deal.Message, "deal rejected: clientMarketBalance.Available too small"))
			},
		},
		"verified deal succeeds": {
			dealParams: dealParams{
				VerifiedDeal: true,
			},
			nodeParams: nodeParams{
				DataCap: &bigDataCap,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				require.True(t, deal.Proposal.VerifiedDeal)
				tut.AssertDealState(t, storagemarket.StorageDealAcceptWait, deal.State)
			},
		},
		"verified deal fails getting client data cap": {
			dealParams: dealParams{
				VerifiedDeal: true,
			},
			nodeParams: nodeParams{
				GetDataCapError: xerrors.Errorf("failure getting data cap"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				require.True(t, deal.Proposal.VerifiedDeal)
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: node error fetching verified data cap: failure getting data cap", deal.Message)
			},
		},
		"verified deal fails data cap not found": {
			dealParams: dealParams{
				VerifiedDeal: true,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				require.True(t, deal.Proposal.VerifiedDeal)
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: node error fetching verified data cap: data cap missing -- client not verified", deal.Message)
			},
		},
		"verified deal fails with insufficient data cap": {
			dealParams: dealParams{
				VerifiedDeal: true,
			},
			nodeParams: nodeParams{
				DataCap: &smallDataCap,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				require.True(t, deal.Proposal.VerifiedDeal)
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: verified deal DataCap too small for proposed piece size", deal.Message)
			},
		},
		"invalid piece size": {
			dealParams: dealParams{
				PieceSize: 129,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: proposal piece size is invalid: padded piece size must be a power of 2", deal.Message)
			},
		},
		"invalid piece cid prefix": {
			dealParams: dealParams{
				PieceCid: &tut.GenerateCids(1)[0],
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: proposal PieceCID had wrong prefix", deal.Message)
			},
		},
		"end epoch before start": {
			dealParams: dealParams{
				StartEpoch: 1000,
				EndEpoch:   900,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: proposal end before proposal start", deal.Message)
			},
		},
		"start epoch has already passed": {
			dealParams: dealParams{
				StartEpoch: defaultHeight - 1,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: deal start epoch has already elapsed", deal.Message)
			},
		},
		"deal duration too short (less than 180 days)": {
			dealParams: dealParams{
				StartEpoch: defaultHeight,
				EndEpoch:   defaultHeight + builtin.EpochsInDay*180 - 1,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.True(t, strings.Contains(deal.Message, "deal rejected: deal duration out of bounds"))
			},
		},
		"deal duration too long (more than 540 days)": {
			nodeParams: nodeParams{
				ClientMarketBalance: big.Mul(abi.NewTokenAmount(builtin.EpochsInDay*54+1), defaultStoragePricePerEpoch),
			},
			dealParams: dealParams{
				StartEpoch: defaultHeight,
				EndEpoch:   defaultHeight + builtin.EpochsInDay*540 + 1,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.True(t, strings.Contains(deal.Message, "deal rejected: deal duration out of bounds"))
			},
		},
		"end epoch too long after current epoch": {
			nodeParams: nodeParams{
				Height: defaultHeight - 10,
			},
			dealParams: dealParams{
				StartEpoch: defaultHeight,
				EndEpoch:   defaultHeight + builtin.EpochsInDay*540,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.True(t, strings.Contains(deal.Message, "invalid deal end epoch"))
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runValidateDealProposal(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestDecideOnProposal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runDecideOndeal := makeExecutor(ctx, eventProcessor, providerstates.DecideOnProposal, storagemarket.StorageDealAcceptWait)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealWaitingForData, deal.State)
			},
		},
		"Custom Decision Rejects Deal": {
			environmentParams: environmentParams{
				RejectDeal:   true,
				RejectReason: "I just don't like it",
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: I just don't like it", deal.Message)
			},
		},
		"Custom Decision Errors": {
			environmentParams: environmentParams{
				DecisionError: errors.New("I can't make up my mind"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealRejecting, deal.State)
				require.Equal(t, "deal rejected: custom deal decision logic failed: I can't make up my mind", deal.Message)
			},
		},
		"SendSignedResponse errors": {
			environmentParams: environmentParams{
				SendSignedResponseError: errors.New("could not send"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "sending response to deal: could not send", deal.Message)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runDecideOndeal(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestWaitForTransferRestart(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	awaitRestartTimeout := make(chan time.Time)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		state             storagemarket.StorageDealStatus
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"no timeout fired": {
			environmentParams: environmentParams{},
			state:             storagemarket.StorageDealProviderTransferAwaitRestart,
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealProviderTransferAwaitRestart, deal.State)
			},
		},

		"fires after state change": {
			environmentParams: environmentParams{
				AwaitRestartTimeout: awaitRestartTimeout,
			},
			state: storagemarket.StorageDealTransferring,
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealTransferring, deal.State)
			},
		},

		"firsts without state change": {
			environmentParams: environmentParams{
				AwaitRestartTimeout: awaitRestartTimeout,
			},
			state: storagemarket.StorageDealProviderTransferAwaitRestart,
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "timed out waiting for client to restart transfer", deal.Message)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runWaitForTransferRestart := makeExecutor(ctx, eventProcessor, providerstates.WaitForTransferRestart, data.state)
			runWaitForTransferRestart(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}
func TestVerifyData(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	expMetaPath := filestore.Path("somemetadata.txt")
	runVerifyData := makeExecutor(ctx, eventProcessor, providerstates.VerifyData, storagemarket.StorageDealVerifyData)

	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			environmentParams: environmentParams{
				MetadataPath: expMetaPath,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealReserveProviderFunds, deal.State)
				require.Equal(t, filestore.Path(""), deal.PiecePath)
				require.Equal(t, expMetaPath, deal.MetadataPath)
			},
		},

		"finalize blockstore fails": {
			environmentParams: environmentParams{
				FinalizeBlockstoreError: errors.New("finalize error"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				//require.Contains(t, deal.Message, "finalize error")
			},
		},

		"generate piece CID fails": {
			environmentParams: environmentParams{
				GenerateCommPError: errors.New("could not generate CommP"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "deal data verification failed: error generating CommP: could not generate CommP", deal.Message)
			},
		},
		"piece CIDs do not match": {
			environmentParams: environmentParams{
				MetadataPath: expMetaPath,
				PieceCid:     tut.GenerateCids(1)[0],
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "deal data verification failed: proposal CommP doesn't match calculated CommP", deal.Message)
				require.Equal(t, filestore.Path(""), deal.PiecePath)
				require.Equal(t, expMetaPath, deal.MetadataPath)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runVerifyData(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestWaitForFunding(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runWaitForFunding := makeExecutor(ctx, eventProcessor, providerstates.WaitForFunding, storagemarket.StorageDealProviderFunding)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			nodeParams: nodeParams{
				WaitForMessageExitCode: exitcode.Ok,
				WaitForMessageRetBytes: []byte{},
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealPublish, deal.State)
			},
		},
		"AddFunds returns non-ok exit code": {
			nodeParams: nodeParams{
				WaitForMessageExitCode: exitcode.ErrInsufficientFunds,
				WaitForMessageRetBytes: []byte{},
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, fmt.Sprintf("error calling node: AddFunds exit code: %s", exitcode.ErrInsufficientFunds), deal.Message)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runWaitForFunding(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestReserveProviderFunds(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runReserveProviderFunds := makeExecutor(ctx, eventProcessor, providerstates.ReserveProviderFunds, storagemarket.StorageDealReserveProviderFunds)
	cids := tut.GenerateCids(1)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds immediately": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealPublish, deal.State)
				require.Equal(t, env.node.DealFunds.ReserveCalls[0], deal.Proposal.ProviderBalanceRequirement())
				require.Len(t, env.node.DealFunds.ReleaseCalls, 0)
				require.Equal(t, deal.Proposal.ProviderBalanceRequirement(), deal.FundsReserved)
			},
		},
		"succeeds by sending an AddBalance message": {
			dealParams: dealParams{
				ProviderCollateral: abi.NewTokenAmount(1),
			},
			nodeParams: nodeParams{
				AddFundsCid: cids[0],
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealProviderFunding, deal.State)
				require.Equal(t, &cids[0], deal.AddFundsCid)
				require.Equal(t, env.node.DealFunds.ReserveCalls[0], deal.Proposal.ProviderBalanceRequirement())
				require.Len(t, env.node.DealFunds.ReleaseCalls, 0)
				require.Equal(t, deal.Proposal.ProviderBalanceRequirement(), deal.FundsReserved)
			},
		},
		"get miner worker fails": {
			nodeParams: nodeParams{
				MinerWorkerError: errors.New("could not get worker"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Len(t, env.node.DealFunds.ReserveCalls, 0)
				require.Len(t, env.node.DealFunds.ReleaseCalls, 0)
				require.True(t, deal.FundsReserved.Nil())
				require.Equal(t, "error calling node: looking up miner worker: could not get worker", deal.Message)
			},
		},
		"reserveFunds errors": {
			nodeParams: nodeParams{
				ReserveFundsError: errors.New("not enough funds"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "error calling node: reserving funds: not enough funds", deal.Message)
				require.Len(t, env.node.DealFunds.ReserveCalls, 0)
				require.Len(t, env.node.DealFunds.ReleaseCalls, 0)
				require.True(t, deal.FundsReserved.Nil())
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runReserveProviderFunds(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestPublishDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runPublishDeal := makeExecutor(ctx, eventProcessor, providerstates.PublishDeal, storagemarket.StorageDealPublish)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealPublishing, deal.State)
			},
		},
		"PublishDealsErrors returns not enough funds error": {
			nodeParams: nodeParams{
				PublishDealsError: errors.New("not enough funds"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealPublish, deal.State)
				require.Equal(t, "", deal.Message)
			},
		},
		"PublishDealsErrors errors": {
			nodeParams: nodeParams{
				PublishDealsError: errors.New("could not post to chain"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "error calling node: publishing deal: could not post to chain", deal.Message)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runPublishDeal(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestWaitForPublish(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runWaitForPublish := makeExecutor(ctx, eventProcessor, providerstates.WaitForPublish, storagemarket.StorageDealPublishing)
	expDealID := abi.DealID(10)
	finalCid := tut.GenerateCids(10)[9]

	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			dealParams: dealParams{
				ReserveFunds: true,
			},
			nodeParams: nodeParams{
				PublishDealID:            expDealID,
				WaitForMessagePublishCid: finalCid,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealStaged, deal.State)
				require.Equal(t, expDealID, deal.DealID)
				assert.Equal(t, env.node.DealFunds.ReleaseCalls[0], deal.Proposal.ProviderBalanceRequirement())
				assert.True(t, deal.FundsReserved.Nil() || deal.FundsReserved.IsZero())
				assert.Equal(t, deal.PublishCid, &finalCid)
			},
		},
		"succeeds, funds already released": {
			nodeParams: nodeParams{
				PublishDealID:            expDealID,
				WaitForMessagePublishCid: finalCid,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealStaged, deal.State)
				require.Equal(t, expDealID, deal.DealID)
				assert.Len(t, env.node.DealFunds.ReleaseCalls, 0)
			},
		},
		"PublishStorageDeal errors": {
			nodeParams: nodeParams{
				WaitForPublishDealsError: errors.New("wait publish err"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "PublishStorageDeal error: PublishStorageDeals errored: wait publish err", deal.Message)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runWaitForPublish(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestHandoffDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runHandoffDeal := makeExecutor(ctx, eventProcessor, providerstates.HandoffDeal, storagemarket.StorageDealStaged)
	carv2Reader := &carv2.Reader{}

	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds for offline deal": {
			dealParams: dealParams{
				PiecePath:     defaultPath,
				FastRetrieval: true,
			},
			fileStoreParams: tut.TestFileStoreParams{
				Files:         []filestore.File{defaultDataFile},
				ExpectedOpens: []filestore.Path{defaultPath},
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAwaitingPreCommit, deal.State)
				require.Len(t, env.node.OnDealCompleteCalls, 1)
				require.True(t, env.node.OnDealCompleteCalls[0].FastRetrieval)
				require.True(t, deal.AvailableForRetrieval)
			},
		},

		"succeed, assemble piece on demand": {
			dealParams: dealParams{
				FastRetrieval: true,
			},
			environmentParams: environmentParams{
				Carv2Reader: carv2Reader,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAwaitingPreCommit, deal.State)
				require.Len(t, env.node.OnDealCompleteCalls, 1)
				require.True(t, env.node.OnDealCompleteCalls[0].FastRetrieval)
				require.True(t, deal.AvailableForRetrieval)
			},
		},

		"fails when can't get a CARv2 reader": {
			dealParams: dealParams{
				FastRetrieval: true,
			},
			environmentParams: environmentParams{
				Carv2Reader: carv2Reader,
				Carv2Error:  errors.New("reader error"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Len(t, env.node.OnDealCompleteCalls, 0)
				require.Empty(t, env.node.OnDealCompleteCalls)
				require.Contains(t, deal.Message, "reader error")
			},
		},

		"succeeds w metadata": {
			dealParams: dealParams{
				PiecePath:     defaultPath,
				MetadataPath:  defaultMetadataPath,
				FastRetrieval: true,
			},
			fileStoreParams: tut.TestFileStoreParams{
				Files:         []filestore.File{defaultDataFile, defaultMetadataFile},
				ExpectedOpens: []filestore.Path{defaultPath, defaultMetadataPath},
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAwaitingPreCommit, deal.State)
				require.Len(t, env.node.OnDealCompleteCalls, 1)
				require.True(t, env.node.OnDealCompleteCalls[0].FastRetrieval)
				require.True(t, deal.AvailableForRetrieval)
			},
		},

		"reading metadata fails": {
			dealParams: dealParams{
				PiecePath:     defaultPath,
				MetadataPath:  filestore.Path("Missing.txt"),
				FastRetrieval: true,
			},
			fileStoreParams: tut.TestFileStoreParams{
				Files:         []filestore.File{defaultDataFile},
				ExpectedOpens: []filestore.Path{defaultPath},
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAwaitingPreCommit, deal.State)
				require.Equal(t, fmt.Sprintf("recording piece for retrieval: failed to register deal data for piece %s for retrieval: failed to load block locations: file not found", deal.Ref.PieceCid), deal.Message)
			},
		},

		"add piece block locations errors": {
			dealParams: dealParams{
				PiecePath:     defaultPath,
				FastRetrieval: true,
			},
			fileStoreParams: tut.TestFileStoreParams{
				Files:         []filestore.File{defaultDataFile},
				ExpectedOpens: []filestore.Path{defaultPath},
			},
			pieceStoreParams: tut.TestPieceStoreParams{
				AddPieceBlockLocationsError: errors.New("could not add block locations"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAwaitingPreCommit, deal.State)
				require.Equal(t, fmt.Sprintf("recording piece for retrieval: failed to register deal data for piece %s for retrieval: failed to add piece block locations: could not add block locations", deal.Ref.PieceCid), deal.Message)
			},
		},

		"add deal for piece errors": {
			dealParams: dealParams{
				PiecePath:     defaultPath,
				FastRetrieval: true,
			},
			fileStoreParams: tut.TestFileStoreParams{
				Files:         []filestore.File{defaultDataFile},
				ExpectedOpens: []filestore.Path{defaultPath},
			},
			pieceStoreParams: tut.TestPieceStoreParams{
				AddDealForPieceError: errors.New("could not add deal info"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAwaitingPreCommit, deal.State)
				require.Equal(t, fmt.Sprintf("recording piece for retrieval: failed to register deal data for piece %s for retrieval: failed to add deal for piece: could not add deal info", deal.Ref.PieceCid), deal.Message)
			},
		},
		"opening file errors": {
			dealParams: dealParams{
				PiecePath: filestore.Path("missing.txt"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, fmt.Sprintf("accessing file store: reading piece at path missing.txt: %s", tut.TestErrNotFound.Error()), deal.Message)
			},
		},

		"OnDealComplete errors": {
			dealParams: dealParams{
				PiecePath: defaultPath,
			},
			fileStoreParams: tut.TestFileStoreParams{
				Files:         []filestore.File{defaultDataFile},
				ExpectedOpens: []filestore.Path{defaultPath},
			},
			nodeParams: nodeParams{
				OnDealCompleteError: errors.New("failed building sector"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "handing off deal to node: packing piece at path file.txt: failed building sector", deal.Message)
			},
		},

		"assemble piece on demand fails because OnComplete fails": {
			environmentParams: environmentParams{
				Carv2Reader: carv2Reader,
			},
			dealParams: dealParams{
				FastRetrieval: true,
			},
			nodeParams: nodeParams{
				OnDealCompleteError: errors.New("failed building sector"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Contains(t, deal.Message, "failed building sector")
			},
		},

		"succeeds even if shard activation fails": {
			dealParams: dealParams{
				FastRetrieval: true,
			},
			environmentParams: environmentParams{
				Carv2Reader:          carv2Reader,
				ShardActivationError: errors.New("some error"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAwaitingPreCommit, deal.State)
				require.Len(t, env.node.OnDealCompleteCalls, 1)
				require.True(t, env.node.OnDealCompleteCalls[0].FastRetrieval)
				require.True(t, deal.AvailableForRetrieval)
			},
		},
	}

	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runHandoffDeal(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestVerifyDealPrecommitted(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runVerifyDealActivated := makeExecutor(ctx, eventProcessor, providerstates.VerifyDealPreCommitted, storagemarket.StorageDealAwaitingPreCommit)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			nodeParams: nodeParams{
				PreCommittedSectorNumber: abi.SectorNumber(10),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealSealing, deal.State)
				require.Equal(t, abi.SectorNumber(10), deal.SectorNumber)
			},
		},
		"succeeds, active": {
			nodeParams: nodeParams{
				PreCommittedIsActive: true,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFinalizing, deal.State)
			},
		},
		"sync error": {
			nodeParams: nodeParams{
				DealPreCommittedSyncError: errors.New("couldn't check deal pre-commit"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "error awaiting deal pre-commit: couldn't check deal pre-commit", deal.Message)
			},
		},
		"async error": {
			nodeParams: nodeParams{
				DealPreCommittedAsyncError: errors.New("deal did not appear on chain"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "error awaiting deal pre-commit: deal did not appear on chain", deal.Message)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runVerifyDealActivated(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestVerifyDealActivated(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runVerifyDealActivated := makeExecutor(ctx, eventProcessor, providerstates.VerifyDealActivated, storagemarket.StorageDealSealing)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFinalizing, deal.State)
			},
		},
		"sync error": {
			nodeParams: nodeParams{
				DealCommittedSyncError: errors.New("couldn't check deal commitment"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "error activating deal: couldn't check deal commitment", deal.Message)
			},
		},
		"async error": {
			nodeParams: nodeParams{
				DealCommittedAsyncError: errors.New("deal did not appear on chain"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, "error activating deal: deal did not appear on chain", deal.Message)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runVerifyDealActivated(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestCleanupDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runCleanupDeal := makeExecutor(ctx, eventProcessor, providerstates.CleanupDeal, storagemarket.StorageDealFinalizing)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			dealParams: dealParams{
				PiecePath: defaultPath,
			},
			fileStoreParams: tut.TestFileStoreParams{
				Files:             []filestore.File{defaultDataFile},
				ExpectedDeletions: []filestore.Path{defaultPath},
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealActive, deal.State)
			},
		},
		"succeeds w metadata": {
			dealParams: dealParams{
				PiecePath:    defaultPath,
				MetadataPath: defaultMetadataPath,
			},
			fileStoreParams: tut.TestFileStoreParams{
				Files:             []filestore.File{defaultDataFile, defaultMetadataFile},
				ExpectedDeletions: []filestore.Path{defaultMetadataPath, defaultPath},
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealActive, deal.State)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runCleanupDeal(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestWaitForDealCompletion(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runWaitForDealCompletion := makeExecutor(ctx, eventProcessor, providerstates.WaitForDealCompletion, storagemarket.StorageDealActive)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"slashing succeeds": {
			nodeParams: nodeParams{OnDealSlashedEpoch: abi.ChainEpoch(5)},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealSlashed, deal.State)
				require.Equal(t, abi.ChainEpoch(5), deal.SlashEpoch)
				require.Len(t, env.peerTagger.UntagCalls, 1)
				require.Equal(t, deal.Client, env.peerTagger.UntagCalls[0])
			},
		},
		"expiration succeeds": {
			// OnDealSlashedEpoch of zero signals to test node to call onDealExpired()
			nodeParams: nodeParams{OnDealSlashedEpoch: abi.ChainEpoch(0)},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealExpired, deal.State)
				require.Len(t, env.peerTagger.UntagCalls, 1)
				require.Equal(t, deal.Client, env.peerTagger.UntagCalls[0])
			},
		},
		"slashing fails": {
			nodeParams: nodeParams{OnDealSlashedError: errors.New("an err")},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				require.Equal(t, "error waiting for deal completion: deal slashing err: an err", deal.Message)
			},
		},
		"expiration fails": {
			nodeParams: nodeParams{OnDealExpiredError: errors.New("an err")},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				require.Equal(t, "error waiting for deal completion: deal expiration err: an err", deal.Message)
			},
		},
		"fails synchronously": {
			nodeParams: nodeParams{WaitForDealCompletionError: errors.New("an err")},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				require.Equal(t, "error waiting for deal completion: an err", deal.Message)
			},
		},
	}

	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runWaitForDealCompletion(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestRejectDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runRejectDeal := makeExecutor(ctx, eventProcessor, providerstates.RejectDeal, storagemarket.StorageDealRejecting)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, 1, env.disconnectCalls)
			},
		},
		"fails if it cannot send a response": {
			environmentParams: environmentParams{
				SendSignedResponseError: xerrors.New("error sending response"),
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				require.Equal(t, deal.Message, "sending response to deal: error sending response")
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runRejectDeal(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

func TestFailDeal(t *testing.T) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.MinerDeal{}, "State", providerstates.ProviderEvents)
	require.NoError(t, err)
	runFailDeal := makeExecutor(ctx, eventProcessor, providerstates.FailDeal, storagemarket.StorageDealFailing)
	tests := map[string]struct {
		nodeParams        nodeParams
		dealParams        dealParams
		environmentParams environmentParams
		fileStoreParams   tut.TestFileStoreParams
		pieceStoreParams  tut.TestPieceStoreParams
		dealInspector     func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)
	}{
		"succeeds": {
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
			},
		},
		"succeeds, funds released": {
			dealParams: dealParams{
				ReserveFunds: true,
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, env.node.DealFunds.ReleaseCalls[0], deal.Proposal.ProviderBalanceRequirement())
				assert.True(t, deal.FundsReserved.Nil() || deal.FundsReserved.IsZero())
			},
		},
		"succeeds, file deletions": {
			dealParams: dealParams{
				PiecePath:    defaultPath,
				MetadataPath: defaultMetadataPath,
			},
			fileStoreParams: tut.TestFileStoreParams{
				Files:             []filestore.File{defaultDataFile, defaultMetadataFile},
				ExpectedDeletions: []filestore.Path{defaultPath, defaultMetadataPath},
			},
			dealInspector: func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			runFailDeal(t, data.nodeParams, data.environmentParams, data.dealParams, data.fileStoreParams, data.pieceStoreParams, data.dealInspector)
		})
	}
}

// all of these default parameters are setup to allow a deal to complete each handler with no errors
var defaultHeight = abi.ChainEpoch(50)
var defaultTipSetToken = []byte{1, 2, 3}
var defaultStoragePricePerEpoch = abi.NewTokenAmount(10000)
var defaultPieceSize = abi.PaddedPieceSize(1048576)
var defaultStartEpoch = abi.ChainEpoch(200)
var defaultEndEpoch = defaultStartEpoch + ((24*3600)/30)*200 // 200 days

var defaultPieceCid = mkPieceCid("piece cid")
var defaultPath = filestore.Path("file.txt")
var defaultMetadataPath = filestore.Path("metadataPath.txt")
var defaultClientAddress = address.TestAddress
var defaultProviderAddress = address.TestAddress2
var defaultMinerAddr, _ = address.NewActorAddress([]byte("miner"))
var defaultClientCollateral = abi.NewTokenAmount(0)
var defaultProviderCollateral = abi.NewTokenAmount(10000)
var defaultDataRef = storagemarket.DataRef{
	Root:         tut.GenerateCids(1)[0],
	TransferType: storagemarket.TTGraphsync,
}
var defaultClientMarketBalance = big.Mul(big.NewInt(int64(defaultEndEpoch-defaultStartEpoch)), defaultStoragePricePerEpoch)

var defaultAsk = storagemarket.StorageAsk{
	Price:         abi.NewTokenAmount(10000000),
	VerifiedPrice: abi.NewTokenAmount(1000000),
	MinPieceSize:  abi.PaddedPieceSize(256),
	MaxPieceSize:  1 << 20,
}

var testData = tut.NewTestIPLDTree()
var dataBuf = new(bytes.Buffer)
var blockLocationBuf = new(bytes.Buffer)
var _ error = testData.DumpToCar(dataBuf, blockrecorder.RecordEachBlockTo(blockLocationBuf))
var defaultDataFile = tut.NewTestFile(tut.TestFileParams{
	Buffer: dataBuf,
	Path:   defaultPath,
	Size:   400,
})
var defaultMetadataFile = tut.NewTestFile(tut.TestFileParams{
	Buffer: blockLocationBuf,
	Path:   defaultMetadataPath,
	Size:   400,
})

func mkPieceCid(input string) cid.Cid {
	var prefix = cid.Prefix{
		Version:  1,
		Codec:    cid.FilCommitmentUnsealed,
		MhType:   mh.SHA2_256_TRUNC254_PADDED,
		MhLength: 32,
	}

	data := []byte(input)

	c, err := prefix.Sum(data)
	switch err {
	case mh.ErrSumNotSupported:
		// multihash library doesn't support this hash function.
		// just fake it.
	case nil:
		return c
	default:
		//panic(err)
	}

	sum := sha256.Sum256(data)
	hash, err := mh.Encode(sum[:], prefix.MhType)
	if err != nil {
		panic(err)
	}
	return cid.NewCidV1(prefix.Codec, hash)
}

func generatePublishDealsReturn(t *testing.T) (abi.DealID, []byte) {
	dealId := abi.DealID(rand.Uint64())

	psdReturn := market.PublishStorageDealsReturn{IDs: []abi.DealID{dealId}}
	psdReturnBytes := bytes.NewBuffer([]byte{})
	err := psdReturn.MarshalCBOR(psdReturnBytes)
	require.NoError(t, err)

	return dealId, psdReturnBytes.Bytes()
}

type nodeParams struct {
	MinerAddr                  address.Address
	MinerWorkerError           error
	ReserveFundsError          error
	Height                     abi.ChainEpoch
	TipSetToken                shared.TipSetToken
	ClientMarketBalance        abi.TokenAmount
	ClientMarketBalanceError   error
	AddFundsCid                cid.Cid
	VerifySignatureFails       bool
	MostRecentStateIDError     error
	PieceLength                uint64
	PieceSectorID              uint64
	PublishDealsError          error
	PublishDealID              abi.DealID
	WaitForPublishDealsError   error
	OnDealCompleteError        error
	PreCommittedSectorNumber   abi.SectorNumber
	PreCommittedIsActive       bool
	DealPreCommittedSyncError  error
	DealPreCommittedAsyncError error
	DealCommittedSyncError     error
	DealCommittedAsyncError    error
	WaitForMessageBlocks       bool
	WaitForMessagePublishCid   cid.Cid
	WaitForMessageError        error
	WaitForMessageExitCode     exitcode.ExitCode
	WaitForMessageRetBytes     []byte
	WaitForDealCompletionError error
	OnDealExpiredError         error
	OnDealSlashedError         error
	OnDealSlashedEpoch         abi.ChainEpoch
	DataCap                    *verifreg.DataCap
	GetDataCapError            error
}

type dealParams struct {
	PieceCid             *cid.Cid
	PiecePath            filestore.Path
	MetadataPath         filestore.Path
	DealID               abi.DealID
	DataRef              *storagemarket.DataRef
	StoragePricePerEpoch abi.TokenAmount
	ProviderCollateral   abi.TokenAmount
	ClientCollateral     abi.TokenAmount
	PieceSize            abi.PaddedPieceSize
	StartEpoch           abi.ChainEpoch
	EndEpoch             abi.ChainEpoch
	FastRetrieval        bool
	VerifiedDeal         bool
	ReserveFunds         bool
	TransferChannelId    *datatransfer.ChannelID
	Label                market.DealLabel
}

type environmentParams struct {
	Address                  address.Address
	Ask                      storagemarket.StorageAsk
	DataTransferError        error
	PieceCid                 cid.Cid
	MetadataPath             filestore.Path
	GenerateCommPError       error
	PieceReader              io.ReadCloser
	PieceSize                uint64
	SendSignedResponseError  error
	DisconnectError          error
	TagsProposal             bool
	RejectDeal               bool
	RejectReason             string
	DecisionError            error
	RestartDataTransferError error
	AwaitRestartTimeout      chan time.Time
	FinalizeBlockstoreError  error

	Carv2Reader *carv2.Reader
	Carv2Error  error

	ShardActivationError error
}

type executor func(t *testing.T,
	node nodeParams,
	params environmentParams,
	dealParams dealParams,
	fileStoreParams tut.TestFileStoreParams,
	pieceStoreParams tut.TestPieceStoreParams,
	dealInspector func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment))

func makeExecutor(ctx context.Context,
	eventProcessor fsm.EventProcessor,
	stateEntryFunc providerstates.ProviderStateEntryFunc,
	initialState storagemarket.StorageDealStatus) executor {
	return func(t *testing.T,
		nodeParams nodeParams,
		params environmentParams,
		dealParams dealParams,
		fileStoreParams tut.TestFileStoreParams,
		pieceStoreParams tut.TestPieceStoreParams,
		dealInspector func(t *testing.T, deal storagemarket.MinerDeal, env *fakeEnvironment)) {

		smstate := testnodes.NewStorageMarketState()
		if nodeParams.Height != abi.ChainEpoch(0) {
			smstate.Epoch = nodeParams.Height
			smstate.TipSetToken = nodeParams.TipSetToken
		} else {
			smstate.Epoch = defaultHeight
			smstate.TipSetToken = defaultTipSetToken
		}
		if !nodeParams.ClientMarketBalance.Nil() {
			smstate.AddFunds(defaultClientAddress, nodeParams.ClientMarketBalance)
		} else {
			smstate.AddFunds(defaultClientAddress, defaultClientMarketBalance)
		}

		common := testnodes.FakeCommonNode{
			SMState:                    smstate,
			DealFunds:                  tut.NewTestDealFunds(),
			GetChainHeadError:          nodeParams.MostRecentStateIDError,
			GetBalanceError:            nodeParams.ClientMarketBalanceError,
			VerifySignatureFails:       nodeParams.VerifySignatureFails,
			ReserveFundsError:          nodeParams.ReserveFundsError,
			PreCommittedIsActive:       nodeParams.PreCommittedIsActive,
			PreCommittedSectorNumber:   nodeParams.PreCommittedSectorNumber,
			DealPreCommittedSyncError:  nodeParams.DealPreCommittedSyncError,
			DealPreCommittedAsyncError: nodeParams.DealPreCommittedAsyncError,
			DealCommittedSyncError:     nodeParams.DealCommittedSyncError,
			DealCommittedAsyncError:    nodeParams.DealCommittedAsyncError,
			AddFundsCid:                nodeParams.AddFundsCid,
			WaitForMessageBlocks:       nodeParams.WaitForMessageBlocks,
			WaitForMessageError:        nodeParams.WaitForMessageError,
			WaitForMessageFinalCid:     nodeParams.WaitForMessagePublishCid,
			WaitForMessageExitCode:     nodeParams.WaitForMessageExitCode,
			WaitForMessageRetBytes:     nodeParams.WaitForMessageRetBytes,
			WaitForDealCompletionError: nodeParams.WaitForDealCompletionError,
			OnDealExpiredError:         nodeParams.OnDealExpiredError,
			OnDealSlashedError:         nodeParams.OnDealSlashedError,
			OnDealSlashedEpoch:         nodeParams.OnDealSlashedEpoch,
		}

		node := &testnodes.FakeProviderNode{
			FakeCommonNode:           common,
			MinerAddr:                nodeParams.MinerAddr,
			MinerWorkerError:         nodeParams.MinerWorkerError,
			PieceLength:              nodeParams.PieceLength,
			PieceSectorID:            nodeParams.PieceSectorID,
			PublishDealsError:        nodeParams.PublishDealsError,
			PublishDealID:            nodeParams.PublishDealID,
			WaitForPublishDealsError: nodeParams.WaitForPublishDealsError,
			OnDealCompleteError:      nodeParams.OnDealCompleteError,
			OnDealCompleteSkipCommP:  true,
			DataCap:                  nodeParams.DataCap,
			GetDataCapErr:            nodeParams.GetDataCapError,
		}

		if nodeParams.MinerAddr == address.Undef {
			node.MinerAddr = defaultMinerAddr
		}

		proposal := market.DealProposal{
			PieceCID:             defaultPieceCid,
			PieceSize:            defaultPieceSize,
			Client:               defaultClientAddress,
			Provider:             defaultProviderAddress,
			StartEpoch:           defaultStartEpoch,
			EndEpoch:             defaultEndEpoch,
			StoragePricePerEpoch: defaultStoragePricePerEpoch,
			ProviderCollateral:   defaultProviderCollateral,
			ClientCollateral:     defaultClientCollateral,
			Label:                dealParams.Label,
		}
		if dealParams.PieceCid != nil {
			proposal.PieceCID = *dealParams.PieceCid
		}
		if !dealParams.StoragePricePerEpoch.Nil() {
			proposal.StoragePricePerEpoch = dealParams.StoragePricePerEpoch
		}
		if !dealParams.ProviderCollateral.Nil() {
			proposal.ProviderCollateral = dealParams.ProviderCollateral
		}
		if !dealParams.ClientCollateral.Nil() {
			proposal.ClientCollateral = dealParams.ClientCollateral
		}
		if dealParams.StartEpoch != abi.ChainEpoch(0) {
			proposal.StartEpoch = dealParams.StartEpoch
		}
		if dealParams.EndEpoch != abi.ChainEpoch(0) {
			proposal.EndEpoch = dealParams.EndEpoch
		}
		if dealParams.PieceSize != abi.PaddedPieceSize(0) {
			proposal.PieceSize = dealParams.PieceSize
		}
		proposal.VerifiedDeal = dealParams.VerifiedDeal
		signedProposal := &market.ClientDealProposal{
			Proposal:        proposal,
			ClientSignature: *tut.MakeTestSignature(),
		}
		dataRef := &defaultDataRef
		if dealParams.DataRef != nil {
			dataRef = dealParams.DataRef
		}
		dealState, err := tut.MakeTestMinerDeal(initialState,
			signedProposal, dataRef)
		require.NoError(t, err)
		dealState.AddFundsCid = &tut.GenerateCids(1)[0]
		dealState.PublishCid = &tut.GenerateCids(1)[0]
		if dealParams.PiecePath != filestore.Path("") {
			dealState.PiecePath = dealParams.PiecePath
		}
		if dealParams.MetadataPath != filestore.Path("") {
			dealState.MetadataPath = dealParams.MetadataPath
		}
		if dealParams.DealID != abi.DealID(0) {
			dealState.DealID = dealParams.DealID
		}
		dealState.FastRetrieval = dealParams.FastRetrieval
		if dealParams.ReserveFunds {
			dealState.FundsReserved = proposal.ProviderCollateral
		}
		if dealParams.TransferChannelId != nil {
			dealState.TransferChannelId = dealParams.TransferChannelId
		}

		fs := tut.NewTestFileStore(fileStoreParams)
		pieceStore := tut.NewTestPieceStoreWithParams(pieceStoreParams)
		expectedTags := make(map[string]struct{})
		if params.TagsProposal {
			expectedTags[dealState.ProposalCid.String()] = struct{}{}
		}
		environment := &fakeEnvironment{
			expectedTags:            expectedTags,
			receivedTags:            make(map[string]struct{}),
			address:                 params.Address,
			node:                    node,
			ask:                     params.Ask,
			dataTransferError:       params.DataTransferError,
			pieceCid:                params.PieceCid,
			metadataPath:            params.MetadataPath,
			generateCommPError:      params.GenerateCommPError,
			pieceReader:             params.PieceReader,
			pieceSize:               params.PieceSize,
			sendSignedResponseError: params.SendSignedResponseError,
			disconnectError:         params.DisconnectError,
			rejectDeal:              params.RejectDeal,
			rejectReason:            params.RejectReason,
			decisionError:           params.DecisionError,
			fs:                      fs,
			pieceStore:              pieceStore,
			peerTagger:              tut.NewTestPeerTagger(),

			restartDataTransferError: params.RestartDataTransferError,

			finalizeBlockstoreErr: params.FinalizeBlockstoreError,

			carV2Reader:          params.Carv2Reader,
			carV2Error:           params.Carv2Error,
			shardActivationError: params.ShardActivationError,
			awaitRestartTimeout:  params.AwaitRestartTimeout,
		}
		if environment.pieceCid == cid.Undef {
			environment.pieceCid = defaultPieceCid
		}
		if environment.metadataPath == filestore.Path("") {
			environment.metadataPath = defaultMetadataPath
		}
		if environment.address == address.Undef {
			environment.address = defaultProviderAddress
		}
		if environment.ask == storagemarket.StorageAskUndefined {
			environment.ask = defaultAsk
		}
		if environment.pieceSize == 0 {
			environment.pieceSize = uint64(defaultPieceSize.Unpadded())
		}
		if environment.pieceReader == nil {
			environment.pieceReader = newStubbedReadCloser(nil)
		}

		fsmCtx := fsmtest.NewTestContext(ctx, eventProcessor)
		err = stateEntryFunc(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		if environment.awaitRestartTimeout != nil {
			environment.awaitRestartTimeout <- time.Now()
			time.Sleep(10 * time.Millisecond)
		}
		fsmCtx.ReplayEvents(t, dealState)
		dealInspector(t, *dealState, environment)

		fs.VerifyExpectations(t)
		pieceStore.VerifyExpectations(t)
		environment.VerifyExpectations(t)
	}
}

type restartDataTransferCall struct {
	chId datatransfer.ChannelID
}

type fakeEnvironment struct {
	address                 address.Address
	node                    *testnodes.FakeProviderNode
	ask                     storagemarket.StorageAsk
	dataTransferError       error
	pieceCid                cid.Cid
	metadataPath            filestore.Path
	generateCommPError      error
	pieceReader             io.ReadCloser
	pieceSize               uint64
	sendSignedResponseError error
	disconnectCalls         int
	disconnectError         error
	rejectDeal              bool
	rejectReason            string
	decisionError           error
	fs                      filestore.FileStore
	pieceStore              piecestore.PieceStore
	expectedTags            map[string]struct{}
	receivedTags            map[string]struct{}
	peerTagger              *tut.TestPeerTagger

	finalizeBlockstoreErr error

	restartDataTransferCalls []restartDataTransferCall
	restartDataTransferError error

	carV2Reader          *carv2.Reader
	carV2Error           error
	awaitRestartTimeout  chan time.Time
	shardActivationError error
}

func (fe *fakeEnvironment) RemoveIndex(ctx context.Context, proposalCid cid.Cid) error {
	return nil
}

func (fe *fakeEnvironment) RestartDataTransfer(_ context.Context, chId datatransfer.ChannelID) error {
	fe.restartDataTransferCalls = append(fe.restartDataTransferCalls, restartDataTransferCall{chId})
	return fe.restartDataTransferError
}

func (fe *fakeEnvironment) Address() address.Address {
	return fe.address
}

func (fe *fakeEnvironment) Node() storagemarket.StorageProviderNode {
	return fe.node
}

func (fe *fakeEnvironment) Ask() storagemarket.StorageAsk {
	return fe.ask
}

func (fe *fakeEnvironment) SendSignedResponse(ctx context.Context, response *network.Response) error {
	return fe.sendSignedResponseError
}

func (fe *fakeEnvironment) VerifyExpectations(t *testing.T) {
	require.Equal(t, fe.expectedTags, fe.receivedTags)
}

func (fe *fakeEnvironment) Disconnect(proposalCid cid.Cid) error {
	fe.disconnectCalls += 1
	return fe.disconnectError
}

func (fe *fakeEnvironment) FileStore() filestore.FileStore {
	return fe.fs
}

func (fe *fakeEnvironment) PieceStore() piecestore.PieceStore {
	return fe.pieceStore
}

func (fe *fakeEnvironment) RunCustomDecisionLogic(context.Context, storagemarket.MinerDeal) (bool, string, error) {
	return !fe.rejectDeal, fe.rejectReason, fe.decisionError
}

func (fe *fakeEnvironment) TagPeer(id peer.ID, s string) {
	fe.peerTagger.TagPeer(id, s)
}

func (fe *fakeEnvironment) UntagPeer(id peer.ID, s string) {
	fe.peerTagger.UntagPeer(id, s)
}

func (fe *fakeEnvironment) RegisterShard(ctx context.Context, pieceCid cid.Cid, path string, eagerInit bool) error {
	return fe.shardActivationError
}

func (fe *fakeEnvironment) TerminateBlockstore(proposalCid cid.Cid, carFilePath string) error {
	return nil
}

func (fe *fakeEnvironment) GeneratePieceCommitment(proposalCid cid.Cid, _ string, dealSize abi.PaddedPieceSize) (cid.Cid, filestore.Path, error) {
	return fe.pieceCid, fe.metadataPath, fe.generateCommPError
}

func (fe *fakeEnvironment) FinalizeBlockstore(proposalCid cid.Cid) error {
	return fe.finalizeBlockstoreErr
}

func (fe *fakeEnvironment) ReadCAR(_ string) (*carv2.Reader, error) {
	return fe.carV2Reader, fe.carV2Error
}

func (fe *fakeEnvironment) AwaitRestartTimeout() <-chan time.Time {
	return fe.awaitRestartTimeout
}

func (fe *fakeEnvironment) AnnounceIndex(ctx context.Context, deal storagemarket.MinerDeal) (cid.Cid, error) {
	return cid.Undef, nil
}

var _ providerstates.ProviderDealEnvironment = &fakeEnvironment{}

type stubbedReadCloser struct {
	err error
}

func (src *stubbedReadCloser) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

func (src *stubbedReadCloser) Close() error {
	return src.err
}

func newStubbedReadCloser(err error) io.ReadCloser {
	return &stubbedReadCloser{err}
}
