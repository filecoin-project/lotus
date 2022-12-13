package clientstates_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-statemachine/fsm"
	fsmtest "github.com/filecoin-project/go-statemachine/fsm/testutil"

	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientstates"
	smnet "github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

var clientDealProposal = tut.MakeTestClientDealProposal()

func TestReserveClientFunds(t *testing.T) {
	t.Run("immediately succeeds", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealReserveClientFunds, clientstates.ReserveClientFunds, testCase{
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFundsReserved, deal.State)
				assert.Equal(t, env.node.DealFunds.ReserveCalls[0], deal.Proposal.ClientBalanceRequirement())
				assert.Len(t, env.node.DealFunds.ReleaseCalls, 0)
				assert.Equal(t, deal.Proposal.ClientBalanceRequirement(), deal.FundsReserved)
			},
		})
	})
	t.Run("succeeds by sending an AddFunds message", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealReserveClientFunds, clientstates.ReserveClientFunds, testCase{
			nodeParams: nodeParams{AddFundsCid: tut.GenerateCids(1)[0]},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealClientFunding, deal.State)
				assert.Equal(t, env.node.DealFunds.ReserveCalls[0], deal.Proposal.ClientBalanceRequirement())
				assert.Len(t, env.node.DealFunds.ReleaseCalls, 0)
				assert.Equal(t, deal.Proposal.ClientBalanceRequirement(), deal.FundsReserved)
			},
		})
	})
	t.Run("Reserve fails", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealReserveClientFunds, clientstates.ReserveClientFunds, testCase{
			nodeParams: nodeParams{
				ReserveFundsError: errors.New("Something went wrong"),
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				assert.Equal(t, "adding market funds failed: Something went wrong", deal.Message)
				assert.Len(t, env.node.DealFunds.ReserveCalls, 0)
				assert.Len(t, env.node.DealFunds.ReleaseCalls, 0)
				assert.True(t, deal.FundsReserved.Nil())
			},
		})
	})
}

func TestWaitForFunding(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealClientFunding, clientstates.WaitForFunding, testCase{
			nodeParams: nodeParams{WaitForMessageExitCode: exitcode.Ok},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFundsReserved, deal.State)
			},
		})
	})
	t.Run("ReserveFunds fails", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealClientFunding, clientstates.WaitForFunding, testCase{
			nodeParams: nodeParams{WaitForMessageExitCode: exitcode.ErrInsufficientFunds},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				assert.Equal(t, "adding market funds failed: AddFunds exit code: 19", deal.Message)
			},
		})
	})
}

func TestProposeDeal(t *testing.T) {
	t.Run("succeeds, closes stream, and tags connection", func(t *testing.T) {
		ds := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ResponseReader: testResponseReader(t, responseParams{
				state:    storagemarket.StorageDealWaitingForData,
				proposal: clientDealProposal,
			}),
		})
		runAndInspect(t, storagemarket.StorageDealFundsReserved, clientstates.ProposeDeal, testCase{
			envParams:  envParams{dealStream: ds},
			nodeParams: nodeParams{WaitForMessageExitCode: exitcode.Ok},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealStartDataTransfer, deal.State)
				assert.Equal(t, 1, env.dealStream.CloseCount)
				assert.Len(t, env.peerTagger.TagCalls, 1)
				assert.Equal(t, deal.Miner, env.peerTagger.TagCalls[0])
			},
		})
	})
	t.Run("sends a fast retrieval flag", func(t *testing.T) {
		var sentProposal *smnet.Proposal

		ds := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ResponseReader: testResponseReader(t, responseParams{
				state:    storagemarket.StorageDealWaitingForData,
				proposal: clientDealProposal,
			}),
			ProposalWriter: func(proposal smnet.Proposal) error {
				sentProposal = &proposal
				return nil
			},
		})

		runAndInspect(t, storagemarket.StorageDealFundsReserved, clientstates.ProposeDeal, testCase{
			envParams:   envParams{dealStream: ds},
			stateParams: dealStateParams{fastRetrieval: true},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealStartDataTransfer, deal.State)
				assert.Equal(t, true, sentProposal.FastRetrieval)
			},
		})
	})
	t.Run("write proposal fails fails", func(t *testing.T) {
		ds := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ProposalWriter: tut.FailStorageProposalWriter,
		})
		runAndInspect(t, storagemarket.StorageDealFundsReserved, clientstates.ProposeDeal, testCase{
			envParams:  envParams{dealStream: ds},
			nodeParams: nodeParams{WaitForMessageExitCode: exitcode.Ok},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "sending proposal to storage provider failed: write proposal failed", deal.Message)
			},
		})
	})
	t.Run("read response fails", func(t *testing.T) {
		ds := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ResponseReader: tut.FailStorageResponseReader,
		})
		runAndInspect(t, storagemarket.StorageDealFundsReserved, clientstates.ProposeDeal, testCase{
			envParams: envParams{
				dealStream: ds,
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				assert.Equal(t, "error reading Response message from provider: read response failed", deal.Message)
			},
		})
	})
	t.Run("closing the stream fails", func(t *testing.T) {
		ds := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{})
		ds.CloseError = xerrors.Errorf("failed to close stream")
		runAndInspect(t, storagemarket.StorageDealFundsReserved, clientstates.ProposeDeal, testCase{
			envParams: envParams{dealStream: ds},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "error attempting to close stream: failed to close stream", deal.Message)
			},
		})
	})
	t.Run("getting chain head fails", func(t *testing.T) {
		ds := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{})
		runAndInspect(t, storagemarket.StorageDealFundsReserved, clientstates.ProposeDeal, testCase{
			envParams: envParams{
				dealStream: ds,
			},
			nodeParams: nodeParams{
				GetChainHeadError: xerrors.Errorf("failed getting chain head"),
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				assert.Equal(t, "unable to verify signature on deal response", deal.Message)
			},
		})
	})
	t.Run("verify signature fails", func(t *testing.T) {
		ds := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{})
		runAndInspect(t, storagemarket.StorageDealFundsReserved, clientstates.ProposeDeal, testCase{
			envParams: envParams{
				dealStream: ds,
			},
			nodeParams: nodeParams{
				VerifySignatureFails: true,
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				assert.Equal(t, "unable to verify signature on deal response", deal.Message)
			},
		})
	})
	t.Run("response contains unexpected state", func(t *testing.T) {
		ds := tut.NewTestStorageDealStream(tut.TestStorageDealStreamParams{
			ResponseReader: testResponseReader(t, responseParams{
				proposal: clientDealProposal,
				state:    storagemarket.StorageDealProposalNotFound,
				message:  "couldn't find deal in store",
			}),
		})
		runAndInspect(t, storagemarket.StorageDealFundsReserved, clientstates.ProposeDeal, testCase{
			envParams: envParams{
				dealStream: ds,
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				assert.Equal(t, "unexpected deal status while waiting for data request: 1 (StorageDealProposalNotFound). Provider message: couldn't find deal in store", deal.Message)
			},
		})
	})
}

func TestInitiateDataTransfer(t *testing.T) {
	t.Run("succeeds and starts the data transfer", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealStartDataTransfer, clientstates.InitiateDataTransfer, testCase{
			envParams: envParams{
				dataTransferChannelId: datatransfer.ChannelID{Initiator: peer.ID("1"), Responder: peer.ID("2"), ID: datatransfer.TransferID(1)},
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				assert.Len(t, env.startDataTransferCalls, 1)
				assert.Equal(t, env.startDataTransferCalls[0].to, deal.Miner)
				assert.Equal(t, env.startDataTransferCalls[0].baseCid, deal.DataRef.Root)
				assert.Equal(t, storagemarket.StorageDealStartDataTransfer, deal.State)
			},
		})
	})

	t.Run("starts polling for acceptance with manual transfers", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealStartDataTransfer, clientstates.InitiateDataTransfer, testCase{
			envParams: envParams{
				manualTransfer: true,
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealCheckForAcceptance, deal.State)
				assert.Len(t, env.startDataTransferCalls, 0)
			},
		})
	})

	t.Run("fails if it can't initiate data transfer", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealStartDataTransfer, clientstates.InitiateDataTransfer, testCase{
			envParams: envParams{
				startDataTransferError: xerrors.Errorf("failed to start data transfer"),
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
			},
		})
	})
}

func TestRestartDataTransfer(t *testing.T) {
	t.Run("fails if can't restart data transfer", func(t *testing.T) {
		err := xerrors.New("error")

		runAndInspect(t, storagemarket.StorageDealClientTransferRestart, clientstates.RestartDataTransfer, testCase{
			envParams: envParams{
				restartDataTransferError: err,
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				assert.Len(t, env.restartDataTransferCalls, 1)
				assert.Equal(t, datatransfer.ChannelID{}, env.restartDataTransferCalls[0].channelId)
				assert.Equal(t, xerrors.Errorf("failed to restart data transfer: %w", err).Error(), deal.Message)
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
			},
		})
	})
}

func TestCheckForDealAcceptance(t *testing.T) {
	testCids := tut.GenerateCids(4)
	proposalCid := tut.GenerateCid(t, clientDealProposal)

	makeProviderDealState := func(status storagemarket.StorageDealStatus) *storagemarket.ProviderDealState {
		return &storagemarket.ProviderDealState{
			State:       status,
			Message:     "",
			Proposal:    &clientDealProposal.Proposal,
			ProposalCid: &proposalCid,
			AddFundsCid: &testCids[1],
			PublishCid:  &testCids[2],
			DealID:      123,
		}
	}

	t.Run("succeeds when provider indicates a successful deal", func(t *testing.T) {
		successStates := []storagemarket.StorageDealStatus{
			storagemarket.StorageDealActive,
			storagemarket.StorageDealAwaitingPreCommit,
			storagemarket.StorageDealSealing,
			storagemarket.StorageDealStaged,
			storagemarket.StorageDealSlashed,
			storagemarket.StorageDealExpired,
		}

		for _, s := range successStates {
			runAndInspect(t, storagemarket.StorageDealCheckForAcceptance, clientstates.CheckForDealAcceptance, testCase{
				envParams: envParams{
					providerDealState: makeProviderDealState(s),
				},
				inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
					tut.AssertDealState(t, storagemarket.StorageDealProposalAccepted, deal.State)
				},
			})
		}
	})

	t.Run("fails when provider indicates a failed deal", func(t *testing.T) {
		failureStates := []storagemarket.StorageDealStatus{
			storagemarket.StorageDealFailing,
			storagemarket.StorageDealError,
		}

		for _, s := range failureStates {
			runAndInspect(t, storagemarket.StorageDealCheckForAcceptance, clientstates.CheckForDealAcceptance, testCase{
				envParams: envParams{
					providerDealState: makeProviderDealState(s),
				},
				inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
					tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				},
			})
		}
	})

	t.Run("continues polling if there is an error querying provider deal state", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealCheckForAcceptance, clientstates.CheckForDealAcceptance, testCase{
			envParams: envParams{
				getDealStatusErr: xerrors.Errorf("network error"),
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealCheckForAcceptance, deal.State)
				assert.Equal(t, uint64(1), deal.PollRetryCount)
				assert.Equal(t, uint64(1), deal.PollErrorCount)
				assert.Equal(t, "Provider state: StorageDealUnknown", deal.Message)
			},
		})
	})

	t.Run("stops polling if (start epoch + grace period) has elapsed", func(t *testing.T) {
		startEpoch := abi.ChainEpoch(1)
		maxEpoch := startEpoch + clientstates.MaxGraceEpochsForDealAcceptance

		runAndInspect(t, storagemarket.StorageDealCheckForAcceptance, clientstates.CheckForDealAcceptance, testCase{
			envParams: envParams{
				providerDealState: makeProviderDealState(storagemarket.StorageDealVerifyData),
			},
			nodeParams: nodeParams{
				CurrentEpoch: maxEpoch + 1,
			},
			stateParams: dealStateParams{
				startEpoch: startEpoch,
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				assert.Contains(t, deal.Message, "start epoch already elapsed")
			},
		})
	})

	t.Run("continues polling with an indeterminate deal state", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealCheckForAcceptance, clientstates.CheckForDealAcceptance, testCase{
			envParams: envParams{
				providerDealState: makeProviderDealState(storagemarket.StorageDealVerifyData),
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealCheckForAcceptance, deal.State)
				assert.Equal(t, "Provider state: StorageDealVerifyData", deal.Message)
			},
		})
	})

	t.Run("fails if the wrong proposal comes back", func(t *testing.T) {
		pds := makeProviderDealState(storagemarket.StorageDealActive)
		pds.ProposalCid = &tut.GenerateCids(1)[0]

		runAndInspect(t, storagemarket.StorageDealCheckForAcceptance, clientstates.CheckForDealAcceptance, testCase{
			envParams: envParams{providerDealState: pds},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealFailing, deal.State)
				assert.Regexp(t, "miner responded to a wrong proposal", deal.Message)
			},
		})
	})
}

func TestValidateDealPublished(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealProposalAccepted, clientstates.ValidateDealPublished, testCase{
			nodeParams: nodeParams{ValidatePublishedDealID: abi.DealID(5)},
			stateParams: dealStateParams{
				reserveFunds: true,
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAwaitingPreCommit, deal.State)
				assert.Equal(t, abi.DealID(5), deal.DealID)
				assert.Equal(t, env.node.DealFunds.ReleaseCalls[0], deal.Proposal.ClientBalanceRequirement())
				assert.True(t, deal.FundsReserved.Nil() || deal.FundsReserved.IsZero())
				assert.Len(t, env.peerTagger.UntagCalls, 1)
				assert.Equal(t, deal.Miner, env.peerTagger.UntagCalls[0])
			},
		})
	})
	t.Run("succeeds, funds already released", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealProposalAccepted, clientstates.ValidateDealPublished, testCase{
			nodeParams: nodeParams{ValidatePublishedDealID: abi.DealID(5)},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealAwaitingPreCommit, deal.State)
				assert.Equal(t, abi.DealID(5), deal.DealID)
				assert.Len(t, env.node.DealFunds.ReleaseCalls, 0)
				assert.Len(t, env.peerTagger.UntagCalls, 1)
				assert.Equal(t, deal.Miner, env.peerTagger.UntagCalls[0])
			},
		})
	})
	t.Run("fails", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealProposalAccepted, clientstates.ValidateDealPublished, testCase{
			nodeParams: nodeParams{
				ValidatePublishedDealID: abi.DealID(5),
				ValidatePublishedError:  errors.New("Something went wrong"),
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "error validating deal published: Something went wrong", deal.Message)
			},
		})
	})
}

func TestVerifyDealPreCommitted(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealAwaitingPreCommit, clientstates.VerifyDealPreCommitted, testCase{
			nodeParams: nodeParams{PreCommittedSectorNumber: abi.SectorNumber(10)},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealSealing, deal.State)
				assert.Equal(t, abi.SectorNumber(10), deal.SectorNumber)
			},
		})
	})
	t.Run("succeeds, active", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealAwaitingPreCommit, clientstates.VerifyDealPreCommitted, testCase{
			nodeParams: nodeParams{PreCommittedIsActive: true},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealActive, deal.State)
			},
		})
	})
	t.Run("fails synchronously", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealAwaitingPreCommit, clientstates.VerifyDealPreCommitted, testCase{
			nodeParams: nodeParams{DealPreCommittedSyncError: errors.New("Something went wrong")},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "error waiting for deal pre-commit message to appear on chain: Something went wrong", deal.Message)
			},
		})
	})
	t.Run("fails asynchronously", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealAwaitingPreCommit, clientstates.VerifyDealPreCommitted, testCase{
			nodeParams: nodeParams{DealPreCommittedAsyncError: errors.New("Something went wrong later")},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "error waiting for deal pre-commit message to appear on chain: Something went wrong later", deal.Message)
			},
		})
	})
}

func TestVerifyDealActivated(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealSealing, clientstates.VerifyDealActivated, testCase{
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealActive, deal.State)
			},
		})
	})
	t.Run("fails synchronously", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealSealing, clientstates.VerifyDealActivated, testCase{
			nodeParams: nodeParams{DealCommittedSyncError: errors.New("Something went wrong")},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "error in deal activation: Something went wrong", deal.Message)
			},
		})
	})
	t.Run("fails asynchronously", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealSealing, clientstates.VerifyDealActivated, testCase{
			nodeParams: nodeParams{DealCommittedAsyncError: errors.New("Something went wrong later")},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "error in deal activation: Something went wrong later", deal.Message)
			},
		})
	})
}

func TestWaitForDealCompletion(t *testing.T) {
	t.Run("slashing succeeds", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealActive, clientstates.WaitForDealCompletion, testCase{
			nodeParams: nodeParams{OnDealSlashedEpoch: abi.ChainEpoch(5)},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealSlashed, deal.State)
				assert.Equal(t, abi.ChainEpoch(5), deal.SlashEpoch)
			},
		})
	})
	t.Run("expiration succeeds", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealActive, clientstates.WaitForDealCompletion, testCase{
			// OnDealSlashedEpoch of zero signals to test node to call onDealExpired()
			nodeParams: nodeParams{OnDealSlashedEpoch: abi.ChainEpoch(0)},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealExpired, deal.State)
			},
		})
	})
	t.Run("slashing fails", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealActive, clientstates.WaitForDealCompletion, testCase{
			nodeParams: nodeParams{OnDealSlashedError: errors.New("an err")},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "error waiting for deal completion: deal slashing err: an err", deal.Message)
			},
		})
	})
	t.Run("expiration fails", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealActive, clientstates.WaitForDealCompletion, testCase{
			nodeParams: nodeParams{OnDealExpiredError: errors.New("an err")},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "error waiting for deal completion: deal expiration err: an err", deal.Message)
			},
		})
	})
	t.Run("fails synchronously", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealActive, clientstates.WaitForDealCompletion, testCase{
			nodeParams: nodeParams{WaitForDealCompletionError: errors.New("an err")},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, "error waiting for deal completion: an err", deal.Message)
			},
		})
	})
}

func TestFailDeal(t *testing.T) {
	t.Run("releases funds", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealFailing, clientstates.FailDeal, testCase{
			stateParams: dealStateParams{
				reserveFunds: true,
			},
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Equal(t, env.node.DealFunds.ReleaseCalls[0], deal.Proposal.ClientBalanceRequirement())
				assert.True(t, deal.FundsReserved.Nil() || deal.FundsReserved.IsZero())
			},
		})
	})
	t.Run("funds already released", func(t *testing.T) {
		runAndInspect(t, storagemarket.StorageDealFailing, clientstates.FailDeal, testCase{
			inspector: func(deal storagemarket.ClientDeal, env *fakeEnvironment) {
				tut.AssertDealState(t, storagemarket.StorageDealError, deal.State)
				assert.Len(t, env.node.DealFunds.ReleaseCalls, 0)
				assert.True(t, deal.FundsReserved.Nil() || deal.FundsReserved.IsZero())
			},
		})
	})
}

type envParams struct {
	dealStream               *tut.TestStorageDealStream
	startDataTransferError   error
	restartDataTransferError error
	dataTransferChannelId    datatransfer.ChannelID
	manualTransfer           bool
	providerDealState        *storagemarket.ProviderDealState
	getDealStatusErr         error
	pollingInterval          time.Duration
}

type dealStateParams struct {
	addFundsCid   *cid.Cid
	reserveFunds  bool
	fastRetrieval bool
	startEpoch    abi.ChainEpoch
}

type executor func(t *testing.T,
	nodeParams nodeParams,
	envParams envParams,
	dealInspector func(deal storagemarket.ClientDeal, env *fakeEnvironment))

func makeExecutor(ctx context.Context,
	eventProcessor fsm.EventProcessor,
	initialState storagemarket.StorageDealStatus,
	stateEntryFunc clientstates.ClientStateEntryFunc,
	dealParams dealStateParams,
	clientDealProposal *market.ClientDealProposal) executor {
	return func(t *testing.T,
		nodeParams nodeParams,
		envParams envParams,
		dealInspector func(deal storagemarket.ClientDeal, env *fakeEnvironment)) {
		node := makeNode(nodeParams)
		dealState, err := tut.MakeTestClientDeal(initialState, clientDealProposal, envParams.manualTransfer)
		assert.NoError(t, err)
		dealState.AddFundsCid = &tut.GenerateCids(1)[0]
		dealState.FastRetrieval = dealParams.fastRetrieval
		dealState.TransferChannelID = &datatransfer.ChannelID{}

		if dealParams.addFundsCid != nil {
			dealState.AddFundsCid = dealParams.addFundsCid
		}
		if dealParams.reserveFunds {
			dealState.FundsReserved = clientDealProposal.Proposal.ClientBalanceRequirement()
		}
		if dealParams.startEpoch != 0 {
			dealState.Proposal.StartEpoch = dealParams.startEpoch
		}

		environment := &fakeEnvironment{
			node:                       node,
			dealStream:                 envParams.dealStream,
			startDataTransferError:     envParams.startDataTransferError,
			startDataTransferChannelId: envParams.dataTransferChannelId,
			restartDataTransferError:   envParams.restartDataTransferError,
			providerDealState:          envParams.providerDealState,
			getDealStatusErr:           envParams.getDealStatusErr,
			pollingInterval:            envParams.pollingInterval,
			peerTagger:                 tut.NewTestPeerTagger(),
		}

		if environment.pollingInterval == 0 {
			environment.pollingInterval = 0
		}

		fsmCtx := fsmtest.NewTestContext(ctx, eventProcessor)
		err = stateEntryFunc(fsmCtx, environment, *dealState)
		assert.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
		fsmCtx.ReplayEvents(t, dealState)
		dealInspector(*dealState, environment)
	}
}

type nodeParams struct {
	CurrentEpoch               abi.ChainEpoch
	AddFundsCid                cid.Cid
	ReserveFundsError          error
	VerifySignatureFails       bool
	GetBalanceError            error
	GetChainHeadError          error
	WaitForMessageBlocks       bool
	WaitForMessageError        error
	WaitForMessageExitCode     exitcode.ExitCode
	WaitForMessageRetBytes     []byte
	ClientAddr                 address.Address
	ValidationError            error
	ValidatePublishedDealID    abi.DealID
	ValidatePublishedError     error
	PreCommittedSectorNumber   abi.SectorNumber
	PreCommittedIsActive       bool
	DealPreCommittedSyncError  error
	DealPreCommittedAsyncError error
	DealCommittedSyncError     error
	DealCommittedAsyncError    error
	WaitForDealCompletionError error
	OnDealExpiredError         error
	OnDealSlashedError         error
	OnDealSlashedEpoch         abi.ChainEpoch
}

func makeNode(params nodeParams) *testnodes.FakeClientNode {
	var out testnodes.FakeClientNode
	out.SMState = testnodes.NewStorageMarketState()
	if params.CurrentEpoch != 0 {
		out.SMState.Epoch = params.CurrentEpoch
	}

	out.DealFunds = tut.NewTestDealFunds()
	out.AddFundsCid = params.AddFundsCid
	out.ReserveFundsError = params.ReserveFundsError
	out.VerifySignatureFails = params.VerifySignatureFails
	out.GetBalanceError = params.GetBalanceError
	out.GetChainHeadError = params.GetChainHeadError
	out.WaitForMessageBlocks = params.WaitForMessageBlocks
	out.WaitForMessageError = params.WaitForMessageError
	out.WaitForMessageExitCode = params.WaitForMessageExitCode
	out.WaitForMessageRetBytes = params.WaitForMessageRetBytes
	out.ClientAddr = params.ClientAddr
	out.ValidationError = params.ValidationError
	out.ValidatePublishedDealID = params.ValidatePublishedDealID
	out.ValidatePublishedError = params.ValidatePublishedError
	out.PreCommittedSectorNumber = params.PreCommittedSectorNumber
	out.PreCommittedIsActive = params.PreCommittedIsActive
	out.DealPreCommittedSyncError = params.DealPreCommittedSyncError
	out.DealPreCommittedAsyncError = params.DealPreCommittedAsyncError
	out.DealCommittedSyncError = params.DealCommittedSyncError
	out.DealCommittedAsyncError = params.DealCommittedAsyncError
	out.WaitForDealCompletionError = params.WaitForDealCompletionError
	out.OnDealExpiredError = params.OnDealExpiredError
	out.OnDealSlashedError = params.OnDealSlashedError
	out.OnDealSlashedEpoch = params.OnDealSlashedEpoch
	return &out
}

type fakeEnvironment struct {
	node       *testnodes.FakeClientNode
	dealStream *tut.TestStorageDealStream

	startDataTransferChannelId datatransfer.ChannelID
	startDataTransferError     error

	startDataTransferCalls []dataTransferParams

	restartDataTransferError error
	restartDataTransferCalls []restartDataTransferParams

	providerDealState *storagemarket.ProviderDealState
	getDealStatusErr  error
	pollingInterval   time.Duration
	peerTagger        *tut.TestPeerTagger
}

type dataTransferParams struct {
	to       peer.ID
	voucher  datatransfer.Voucher
	baseCid  cid.Cid
	selector ipld.Node
}

type restartDataTransferParams struct {
	channelId datatransfer.ChannelID
}

func (fe *fakeEnvironment) StartDataTransfer(_ context.Context, to peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.ChannelID, error) {
	fe.startDataTransferCalls = append(fe.startDataTransferCalls, dataTransferParams{
		to:       to,
		voucher:  voucher,
		baseCid:  baseCid,
		selector: selector,
	})
	return fe.startDataTransferChannelId, fe.startDataTransferError
}

func (fe *fakeEnvironment) RestartDataTransfer(_ context.Context, channelId datatransfer.ChannelID) error {
	fe.restartDataTransferCalls = append(fe.restartDataTransferCalls, restartDataTransferParams{channelId: channelId})

	return fe.restartDataTransferError
}

func (fe *fakeEnvironment) Node() storagemarket.StorageClientNode {
	return fe.node
}

func (fe *fakeEnvironment) WriteDealProposal(_ peer.ID, _ cid.Cid, proposal smnet.Proposal) error {
	return fe.dealStream.WriteDealProposal(proposal)
}

func (fe *fakeEnvironment) NewDealStream(_ context.Context, _ peer.ID) (smnet.StorageDealStream, error) {
	return fe.dealStream, nil
}

func (fe *fakeEnvironment) GetProviderDealState(_ context.Context, _ cid.Cid) (*storagemarket.ProviderDealState, error) {
	if fe.getDealStatusErr != nil {
		return nil, fe.getDealStatusErr
	}
	return fe.providerDealState, nil
}

func (fe *fakeEnvironment) PollingInterval() time.Duration {
	return fe.pollingInterval
}

func (fe *fakeEnvironment) TagPeer(id peer.ID, ident string) {
	fe.peerTagger.TagPeer(id, ident)
}

func (fe *fakeEnvironment) UntagPeer(id peer.ID, ident string) {
	fe.peerTagger.UntagPeer(id, ident)
}

func (fe *fakeEnvironment) CleanBlockstore(proposalCid cid.Cid) error {
	return nil
}

var _ clientstates.ClientDealEnvironment = &fakeEnvironment{}

type responseParams struct {
	proposal       *market.ClientDealProposal
	state          storagemarket.StorageDealStatus
	message        string
	publishMessage *cid.Cid
	proposalCid    cid.Cid
}

func testResponseReader(t *testing.T, params responseParams) tut.StorageDealResponseReader {
	response := smnet.Response{
		State:          params.state,
		Proposal:       params.proposalCid,
		Message:        params.message,
		PublishMessage: params.publishMessage,
	}

	if response.Proposal == cid.Undef {
		proposalNd, err := cborutil.AsIpld(params.proposal)
		assert.NoError(t, err)
		response.Proposal = proposalNd.Cid()
	}

	return tut.StubbedStorageResponseReader(smnet.SignedResponse{
		Response:  response,
		Signature: tut.MakeTestSignature(),
	})
}

type testCase struct {
	envParams   envParams
	nodeParams  nodeParams
	stateParams dealStateParams
	inspector   func(deal storagemarket.ClientDeal, env *fakeEnvironment)
}

func runAndInspect(t *testing.T, initialState storagemarket.StorageDealStatus, stateFunc clientstates.ClientStateEntryFunc, tc testCase) {
	ctx := context.Background()
	eventProcessor, err := fsm.NewEventProcessor(storagemarket.ClientDeal{}, "State", clientstates.ClientEvents)
	assert.NoError(t, err)
	executor := makeExecutor(ctx, eventProcessor, initialState, stateFunc, tc.stateParams, clientDealProposal)
	executor(t, tc.nodeParams, tc.envParams, tc.inspector)
}
