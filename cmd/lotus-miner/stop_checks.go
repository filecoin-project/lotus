package main

import (
	"time"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

type IsStopSafe struct {
	MinerAddress        address.Address     `json:"miner-address"`
	DeadlineFaultCutoff DeadlineFaultCutoff `json:"deadline-fault-cutoff"`
	Safe                bool                `json:"safe"`
	Errors              []string            `json:"errors"`
}

type DeadlineFaultCutoff struct {
	CurrentEpoch         abi.ChainEpoch `json:"current-epoch"`
	CutoffEpoch          abi.ChainEpoch `json:"cutoff-epoch"`
	EstimatedSecondsLeft float32        `json:"estimated-seconds-left"`
}

func convertErrorsToStrings(errs []error) []string {
	strs := []string{}
	for _, err := range errs {
		strs = append(strs, err.Error())
	}
	return strs
}

func CheckIsStopSafe(cctx *cli.Context) (IsStopSafe, error) {
	deadlineErrors, maddr, cd := GetDeadlineErrors(cctx)
	if maddr == nil {
		return IsStopSafe{}, xerrors.Errorf("Failed to get miner address")
	}
	if cd == nil {
		return IsStopSafe{}, xerrors.Errorf("Failed to get deadline info")
	}

	safetyErrors := []error{}
	safetyErrors = append(safetyErrors, deadlineErrors...)
	safetyErrors = append(safetyErrors, GetMarketErrors(cctx)...)

	return IsStopSafe{
		MinerAddress: *maddr,
		DeadlineFaultCutoff: DeadlineFaultCutoff{
			CurrentEpoch:         cd.CurrentEpoch,
			CutoffEpoch:          cd.FaultCutoff,
			EstimatedSecondsLeft: float32(lcli.DurationBetweenEpochs(cd.CurrentEpoch, cd.FaultCutoff) / time.Second),
		},
		Safe:   len(safetyErrors) == 0,
		Errors: convertErrorsToStrings(safetyErrors),
	}, nil
}

func GetDeadlineErrors(cctx *cli.Context) ([]error, *address.Address, *dline.Info) {
	api, acloser, err := lcli.GetFullNodeAPI(cctx)
	if err != nil {
		return []error{err}, nil, nil
	}
	defer acloser()

	ctx := lcli.ReqContext(cctx)

	maddr, err := getActorAddress(ctx, cctx)
	if err != nil {
		return []error{err}, nil, nil
	}

	head, err := api.ChainHead(ctx)
	if err != nil {
		return []error{xerrors.Errorf("getting chain head: %w", err)}, &maddr, nil
	}

	cd, err := api.StateMinerProvingDeadline(ctx, maddr, head.Key())
	if err != nil {
		return []error{xerrors.Errorf("getting miner info: %w", err)}, &maddr, nil
	}

	errs := []error{}
	if cd.Open <= cd.CurrentEpoch {
		errs = append(errs, xerrors.Errorf("Deadline Open (%d) is on or before Current Epoch (%d)", cd.Open, cd.CurrentEpoch))
	}
	return errs, &maddr, cd
}

func GetMarketErrors(cctx *cli.Context) []error {
	api, closer, err := lcli.GetMarketsAPI(cctx)
	if err != nil {
		return []error{err}
	}
	defer closer()

	ctx := lcli.DaemonContext(cctx)

	onlineStorageDealsOk, err := api.DealsConsiderOnlineStorageDeals(ctx)
	if err != nil {
		return []error{err}
	}

	onlineRetrievalDealsOk, err := api.DealsConsiderOnlineRetrievalDeals(ctx)
	if err != nil {
		return []error{err}
	}

	errs := []error{}
	if onlineStorageDealsOk {
		errs = append(errs, xerrors.Errorf("Online storage deals are being accepted"))
	}
	if onlineRetrievalDealsOk {
		errs = append(errs, xerrors.Errorf("Online retrieval deals are being accepted"))
	}

	errs = append(errs, GetStorageDealErrors(cctx, api)...)
	errs = append(errs, GetRetrievalDealErrors(cctx, api)...)
	errs = append(errs, GetDataTransferErrors(cctx, api)...)

	return errs
}

func GetStorageDealErrors(cctx *cli.Context, api api.StorageMiner) []error {
	deals, err := api.MarketListIncompleteDeals(lcli.DaemonContext(cctx))
	if err != nil {
		return []error{err}
	}

	errs := []error{}
	for _, deal := range deals {
		if _, ok := SafeStorageDealStates[deal.State]; !ok {
			errs = append(errs, xerrors.Errorf("Storage deal is not in a safe state: %d = %s", deal.DealID, storagemarket.DealStates[deal.State]))
		}
	}
	return errs
}

func GetRetrievalDealErrors(cctx *cli.Context, api api.StorageMiner) []error {
	deals, err := api.MarketListRetrievalDeals(lcli.DaemonContext(cctx))
	if err != nil {
		return []error{err}
	}

	errs := []error{}
	for _, deal := range deals {
		if _, ok := SafeRetrievalDealStatuses[deal.Status]; !ok {
			errs = append(errs, xerrors.Errorf("Retrieval deal is not in a safe state: %d = %s", deal.ID, retrievalmarket.DealStatuses[deal.Status]))
		}
	}
	return errs
}

func GetDataTransferErrors(cctx *cli.Context, api api.StorageMiner) []error {
	ctx := lcli.ReqContext(cctx)

	channels, err := api.MarketListDataTransfers(ctx)
	if err != nil {
		return []error{err}
	}

	errs := []error{}
	for _, channel := range channels {
		if _, ok := SafeDataTransferStatuses[channel.Status]; !ok {
			errs = append(errs, xerrors.Errorf("Data transfer is not in a safe state: %d = %s", channel.TransferID, datatransfer.Statuses[channel.Status]))
		}
	}
	return errs
}

var SafeStorageDealStates = map[storagemarket.StorageDealStatus]struct{}{
	storagemarket.StorageDealUnknown:          {},
	storagemarket.StorageDealProposalNotFound: {},
	storagemarket.StorageDealProposalRejected: {},
	// storagemarket.StorageDealProposalAccepted:             {},
	// storagemarket.StorageDealAcceptWait:                   {},
	// storagemarket.StorageDealStartDataTransfer:            {},
	// storagemarket.StorageDealStaged:                       {},
	// storagemarket.StorageDealAwaitingPreCommit:            {},
	storagemarket.StorageDealSealing: {},
	storagemarket.StorageDealActive:  {},
	storagemarket.StorageDealExpired: {},
	storagemarket.StorageDealSlashed: {},
	// storagemarket.StorageDealRejecting:                    {},
	// storagemarket.StorageDealFailing:                      {},
	// storagemarket.StorageDealFundsReserved:                {},
	// storagemarket.StorageDealCheckForAcceptance:           {},
	// storagemarket.StorageDealValidating:                   {},
	// storagemarket.StorageDealTransferring:                 {},
	// storagemarket.StorageDealWaitingForData:               {},
	// storagemarket.StorageDealVerifyData:                   {},
	// storagemarket.StorageDealReserveProviderFunds:         {},
	// storagemarket.StorageDealReserveClientFunds:           {},
	// storagemarket.StorageDealProviderFunding:              {},
	// storagemarket.StorageDealClientFunding:                {},
	// storagemarket.StorageDealPublish:                      {},
	// storagemarket.StorageDealPublishing:                   {},
	storagemarket.StorageDealError: {},
	// storagemarket.StorageDealFinalizing:                   {},
	// storagemarket.StorageDealClientTransferRestart:        {},
	// storagemarket.StorageDealProviderTransferAwaitRestart: {},
	// storagemarket.StorageDealTransferQueued:               {},
}

var SafeRetrievalDealStatuses = map[retrievalmarket.DealStatus]struct{}{
	//	retrievalmarket.DealStatusNew:                              {},
	//	retrievalmarket.DealStatusUnsealing:                        {},
	//	retrievalmarket.DealStatusUnsealed:                         {},
	//	retrievalmarket.DealStatusWaitForAcceptance:                {},
	//	retrievalmarket.DealStatusPaymentChannelCreating:           {},
	//	retrievalmarket.DealStatusPaymentChannelAddingFunds:        {},
	//	retrievalmarket.DealStatusAccepted:                         {},
	//	retrievalmarket.DealStatusFundsNeededUnseal:                {},
	//	retrievalmarket.DealStatusFailing:                          {},
	//	retrievalmarket.DealStatusRejected:                         {},
	//	retrievalmarket.DealStatusFundsNeeded:                      {},
	//	retrievalmarket.DealStatusSendFunds:                        {},
	//	retrievalmarket.DealStatusSendFundsLastPayment:             {},
	//	retrievalmarket.DealStatusOngoing:                          {},
	//	retrievalmarket.DealStatusFundsNeededLastPayment:           {},
	retrievalmarket.DealStatusCompleted:    {},
	retrievalmarket.DealStatusDealNotFound: {},
	retrievalmarket.DealStatusErrored:      {},
	//	retrievalmarket.DealStatusBlocksComplete:                   {},
	//	retrievalmarket.DealStatusFinalizing:                       {},
	//	retrievalmarket.DealStatusCompleting:                       {},
	//	retrievalmarket.DealStatusCheckComplete:                    {},
	//	retrievalmarket.DealStatusCheckFunds:                       {},
	retrievalmarket.DealStatusInsufficientFunds: {},
	//	retrievalmarket.DealStatusPaymentChannelAllocatingLane:     {},
	//	retrievalmarket.DealStatusCancelling:                       {},
	retrievalmarket.DealStatusCancelled: {},
	//	retrievalmarket.DealStatusRetryLegacy:                      {},
	//	retrievalmarket.DealStatusWaitForAcceptanceLegacy:          {},
	//	retrievalmarket.DealStatusClientWaitingForLastBlocks:       {},
	//	retrievalmarket.DealStatusPaymentChannelAddingInitialFunds: {},
	//	retrievalmarket.DealStatusErroring:                         {},
	//	retrievalmarket.DealStatusRejecting:                        {},
	// retrievalmarket.DealStatusDealNotFoundCleanup:               {},
	//	retrievalmarket.DealStatusFinalizingBlockstore:             {},
}

var SafeDataTransferStatuses = map[datatransfer.Status]struct{}{
	// datatransfer.Requested:                           {},
	// datatransfer.Ongoing:                             {},
	// datatransfer.TransferFinished:                    {},
	// datatransfer.ResponderCompleted:                  {},
	// datatransfer.Finalizing:                          {},
	// datatransfer.Completing:                          {},
	datatransfer.Completed: {},
	// datatransfer.Failing:                             {},
	datatransfer.Failed: {},
	// datatransfer.Cancelling:                          {},
	datatransfer.Cancelled: {},
	// datatransfer.InitiatorPaused:                     {},
	// datatransfer.ResponderPaused:                     {},
	// datatransfer.BothPaused:                          {},
	// datatransfer.ResponderFinalizing:                 {},
	// datatransfer.ResponderFinalizingTransferFinished: {},
	datatransfer.ChannelNotFoundError: {},
}
