package providerstates

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	carv2 "github.com/ipld/go-car/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"

	// add by lin
	"os"
	// end
)

var log = logging.Logger("providerstates")

// add by lin
type ReadSeekStarter struct {
	io.Reader
}

func (r *ReadSeekStarter) SeekStart() error {
	return nil
}
// end

// TODO: These are copied from spec-actors master, use spec-actors exports when we update
const DealMaxLabelSize = 256

// ProviderDealEnvironment are the dependencies needed for processing deals
// with a ProviderStateEntryFunc
type ProviderDealEnvironment interface {
	ReadCAR(path string) (*carv2.Reader, error)

	RegisterShard(ctx context.Context, pieceCid cid.Cid, path string, eagerInit bool) error
	AnnounceIndex(ctx context.Context, deal storagemarket.MinerDeal) (cid.Cid, error)
	RemoveIndex(ctx context.Context, proposalCid cid.Cid) error

	FinalizeBlockstore(proposalCid cid.Cid) error
	TerminateBlockstore(proposalCid cid.Cid, path string) error

	GeneratePieceCommitment(proposalCid cid.Cid, path string, dealSize abi.PaddedPieceSize) (cid.Cid, filestore.Path, error)

	Address() address.Address
	Node() storagemarket.StorageProviderNode
	Ask() storagemarket.StorageAsk
	SendSignedResponse(ctx context.Context, response *network.Response) error
	Disconnect(proposalCid cid.Cid) error
	FileStore() filestore.FileStore
	PieceStore() piecestore.PieceStore
	RunCustomDecisionLogic(context.Context, storagemarket.MinerDeal) (bool, string, error)
	AwaitRestartTimeout() <-chan time.Time
	network.PeerTagger
}

// ProviderStateEntryFunc is the signature for a StateEntryFunc in the provider FSM
type ProviderStateEntryFunc func(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error

// ValidateDealProposal validates a proposed deal against the provider criteria
func ValidateDealProposal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	environment.TagPeer(deal.Client, deal.ProposalCid.String())

	tok, curEpoch, err := environment.Node().GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("node error getting most recent state id: %w", err))
	}

	if err := providerutils.VerifyProposal(ctx.Context(), deal.ClientDealProposal, tok, environment.Node().VerifySignature); err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("verifying StorageDealProposal: %w", err))
	}

	proposal := deal.Proposal

	if proposal.Provider != environment.Address() {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("incorrect provider for deal"))
	}

	if proposal.Label.Length() > DealMaxLabelSize {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("deal label can be at most %d bytes, is %d", DealMaxLabelSize, proposal.Label.Length()))
	}

	if err := proposal.PieceSize.Validate(); err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("proposal piece size is invalid: %w", err))
	}

	if !proposal.PieceCID.Defined() {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("proposal PieceCID undefined"))
	}

	if proposal.PieceCID.Prefix() != market.PieceCIDPrefix {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("proposal PieceCID had wrong prefix"))
	}

	if proposal.EndEpoch <= proposal.StartEpoch {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("proposal end before proposal start"))
	}

	if curEpoch > proposal.StartEpoch {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("deal start epoch has already elapsed"))
	}

	// Check that the delta between the start and end epochs (the deal
	// duration) is within acceptable bounds
	minDuration, maxDuration := market.DealDurationBounds(proposal.PieceSize)
	if proposal.Duration() < minDuration || proposal.Duration() > maxDuration {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("deal duration out of bounds (min, max, provided): %d, %d, %d", minDuration, maxDuration, proposal.Duration()))
	}

	// Check that the proposed end epoch isn't too far beyond the current epoch
	maxEndEpoch := curEpoch + minertypes.MaxSectorExpirationExtension
	if proposal.EndEpoch > maxEndEpoch {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("invalid deal end epoch %d: cannot be more than %d past current epoch %d", proposal.EndEpoch, minertypes.MaxSectorExpirationExtension, curEpoch))
	}

	pcMin, pcMax, err := environment.Node().DealProviderCollateralBounds(ctx.Context(), proposal.PieceSize, proposal.VerifiedDeal)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("node error getting collateral bounds: %w", err))
	}

	if proposal.ProviderCollateral.LessThan(pcMin) {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("proposed provider collateral below minimum: %s < %s", proposal.ProviderCollateral, pcMin))
	}

	if proposal.ProviderCollateral.GreaterThan(pcMax) {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("proposed provider collateral above maximum: %s > %s", proposal.ProviderCollateral, pcMax))
	}

	askPrice := environment.Ask().Price
	if deal.Proposal.VerifiedDeal {
		askPrice = environment.Ask().VerifiedPrice
	}

	minPrice := big.Div(big.Mul(askPrice, abi.NewTokenAmount(int64(proposal.PieceSize))), abi.NewTokenAmount(1<<30))
	if proposal.StoragePricePerEpoch.LessThan(minPrice) {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected,
			xerrors.Errorf("storage price per epoch less than asking price: %s < %s", proposal.StoragePricePerEpoch, minPrice))
	}

	if proposal.PieceSize < environment.Ask().MinPieceSize {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected,
			xerrors.Errorf("piece size less than minimum required size: %d < %d", proposal.PieceSize, environment.Ask().MinPieceSize))
	}

	if proposal.PieceSize > environment.Ask().MaxPieceSize {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected,
			xerrors.Errorf("piece size more than maximum allowed size: %d > %d", proposal.PieceSize, environment.Ask().MaxPieceSize))
	}

	// check market funds
	clientMarketBalance, err := environment.Node().GetBalance(ctx.Context(), proposal.Client, tok)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("node error getting client market balance failed: %w", err))
	}

	// This doesn't guarantee that the client won't withdraw / lock those funds
	// but it's a decent first filter
	if clientMarketBalance.Available.LessThan(proposal.ClientBalanceRequirement()) {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("clientMarketBalance.Available too small: %d < %d", clientMarketBalance.Available, proposal.ClientBalanceRequirement()))
	}

	// Verified deal checks
	if proposal.VerifiedDeal {
		dataCap, err := environment.Node().GetDataCap(ctx.Context(), proposal.Client, tok)
		if err != nil {
			return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("node error fetching verified data cap: %w", err))
		}
		if dataCap == nil {
			return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("node error fetching verified data cap: data cap missing -- client not verified"))
		}
		pieceSize := big.NewIntUnsigned(uint64(proposal.PieceSize))
		if dataCap.LessThan(pieceSize) {
			return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("verified deal DataCap too small for proposed piece size"))
		}
	}

	return ctx.Trigger(storagemarket.ProviderEventDealDeciding)
}

// DecideOnProposal allows custom decision logic to run before accepting a deal, such as allowing a manual
// operator to decide whether or not to accept the deal
func DecideOnProposal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	accept, reason, err := environment.RunCustomDecisionLogic(ctx.Context(), deal)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, xerrors.Errorf("custom deal decision logic failed: %w", err))
	}

	if !accept {
		return ctx.Trigger(storagemarket.ProviderEventDealRejected, fmt.Errorf(reason))
	}

	// Send intent to accept
	err = environment.SendSignedResponse(ctx.Context(), &network.Response{
		State:    storagemarket.StorageDealWaitingForData,
		Proposal: deal.ProposalCid,
	})

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventSendResponseFailed, err)
	}

	if err := environment.Disconnect(deal.ProposalCid); err != nil {
		log.Warnf("closing client connection: %+v", err)
	}

	return ctx.Trigger(storagemarket.ProviderEventDataRequested)
}

// WaitForTransferRestart fires a timeout after a set amount of time. If the restart hasn't started at this point,
// the transfer fails
func WaitForTransferRestart(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {

	timeout := environment.AwaitRestartTimeout()
	go func() {
		select {
		case <-ctx.Context().Done():
		case <-timeout:
			ctx.Trigger(storagemarket.ProviderEventAwaitTransferRestartTimeout)
		}
	}()
	return nil
}

// VerifyData verifies that data received for a deal matches the pieceCID
// in the proposal
func VerifyData(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	// finalize the blockstore as we're done writing deal data to it.
	if err := environment.FinalizeBlockstore(deal.ProposalCid); err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDataVerificationFailed, xerrors.Errorf("failed to finalize read/write blockstore: %w", err), filestore.Path(""), filestore.Path(""))
	}

	pieceCid, metadataPath, err := environment.GeneratePieceCommitment(deal.ProposalCid, deal.InboundCAR, deal.Proposal.PieceSize)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDataVerificationFailed, xerrors.Errorf("error generating CommP: %w", err), filestore.Path(""), filestore.Path(""))
	}

	// Verify CommP matches
	if pieceCid != deal.Proposal.PieceCID {
		return ctx.Trigger(storagemarket.ProviderEventDataVerificationFailed, xerrors.Errorf("proposal CommP doesn't match calculated CommP"), filestore.Path(""), metadataPath)
	}

	return ctx.Trigger(storagemarket.ProviderEventVerifiedData, filestore.Path(""), metadataPath)
}

// ReserveProviderFunds adds funds, as needed to the StorageMarketActor, so the miner has adequate collateral for the deal
func ReserveProviderFunds(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	node := environment.Node()

	tok, _, err := node.GetChainHead(ctx.Context())
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("acquiring chain head: %w", err))
	}

	waddr, err := node.GetMinerWorkerAddress(ctx.Context(), deal.Proposal.Provider, tok)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("looking up miner worker: %w", err))
	}

	mcid, err := node.ReserveFunds(ctx.Context(), waddr, deal.Proposal.Provider, deal.Proposal.ProviderCollateral)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("reserving funds: %w", err))
	}

	_ = ctx.Trigger(storagemarket.ProviderEventFundsReserved, deal.Proposal.ProviderCollateral)

	// if no message was sent, and there was no error, funds were already available
	if mcid == cid.Undef {
		return ctx.Trigger(storagemarket.ProviderEventFunded)
	}

	return ctx.Trigger(storagemarket.ProviderEventFundingInitiated, mcid)
}

// WaitForFunding waits for a message posted to add funds to the StorageMarketActor to appear on chain
func WaitForFunding(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	node := environment.Node()

	return node.WaitForMessage(ctx.Context(), *deal.AddFundsCid, func(code exitcode.ExitCode, bytes []byte, finalCid cid.Cid, err error) error {
		if err != nil {
			return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("AddFunds errored: %w", err))
		}
		if code != exitcode.Ok {
			return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("AddFunds exit code: %s", code.String()))
		}
		return ctx.Trigger(storagemarket.ProviderEventFunded)
	})
}

// PublishDeal sends a message to publish a deal on chain
func PublishDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	smDeal := storagemarket.MinerDeal{
		Client:             deal.Client,
		ClientDealProposal: deal.ClientDealProposal,
		ProposalCid:        deal.ProposalCid,
		State:              deal.State,
		Ref:                deal.Ref,
	}

	mcid, err := environment.Node().PublishDeals(ctx.Context(), smDeal)
	if err != nil {
		if strings.Contains(err.Error(), "not enough funds") {
			log.Warnf("publishing deal failed due to lack of funds: %s", err)

			return nil
		}
		return ctx.Trigger(storagemarket.ProviderEventNodeErrored, xerrors.Errorf("publishing deal: %w", err))
	}

	return ctx.Trigger(storagemarket.ProviderEventDealPublishInitiated, mcid)
}

// WaitForPublish waits for the publish message on chain and saves the deal id
// so it can be sent back to the client
func WaitForPublish(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	// add by lin
	if deal.PublishCid == nil {
		log.Errorw("zlin: WaitForPublish deal %+v don't have PublishCid", deal.ProposalCid)
		return ctx.Trigger(storagemarket.ProviderEventDealPublishError, xerrors.Errorf("PublishStorageDeals errored:  deal.PublishCid is nil"))
	}
	// end
	res, err := environment.Node().WaitForPublishDeals(ctx.Context(), *deal.PublishCid, deal.Proposal)
	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealPublishError, xerrors.Errorf("PublishStorageDeals errored: %w", err))
	}

	// Once the deal has been published, release funds that were reserved
	// for deal publishing
	releaseReservedFunds(ctx, environment, deal)

	// add by lin
	if os.Getenv("LOTUS_OF_SXX") == "1" && strings.HasPrefix(string(deal.PiecePath), "/"){
		return ctx.Trigger(storagemarket.ProviderEventDealPublishedOfSxx, res.DealID, res.FinalCid)
	}
	log.Errorw("zlin: unuse SXX publish")
	// end

	return ctx.Trigger(storagemarket.ProviderEventDealPublished, res.DealID, res.FinalCid)
}

// add by lin
func HandoffDealOfSxx(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	carFilePath := string(deal.PiecePath)
	if deal.PiecePath == "" {
		err := xerrors.Errorf("our path of deal car is nil")
		return ctx.Trigger(storagemarket.ProviderEventDealHandoffFailed, err)
	}
	// Data for offline deals is stored on disk, so if PiecePath is set,
	// create a Reader from the file path
	var reader shared.ReadSeekStarter
	reader = &ReadSeekStarter{io.LimitReader(nil, 0)}

	packingInfo, err := environment.Node().OnDealCompleteOfSxx(
			ctx.Context(),
			storagemarket.MinerDeal{
				Client:             deal.Client,
				ClientDealProposal: deal.ClientDealProposal,
				ProposalCid:        deal.ProposalCid,
				State:              deal.State,
				Ref:                deal.Ref,
				PublishCid:         deal.PublishCid,
				DealID:             deal.DealID,
				FastRetrieval:      deal.FastRetrieval,
				RemoteFilepath:     carFilePath,
			},
			deal.Proposal.PieceSize.Unpadded(),
			reader,
		)

	if err != nil {
		err = xerrors.Errorf("packing piece at path %s: %w", deal.PiecePath, err)
		return ctx.Trigger(storagemarket.ProviderEventDealHandoffFailed, err)
	}

	if err := recordPiece(environment, deal, packingInfo.SectorNumber, packingInfo.Offset, packingInfo.Size); err != nil {
		err = xerrors.Errorf("failed to register deal data for piece %s for retrieval: %w", deal.Ref.PieceCid, err)
		log.Error(err.Error())
		_ = ctx.Trigger(storagemarket.ProviderEventPieceStoreErrored, err)
	}

	// Register the deal data as a "shard" with the DAG store. Later it can be
	// fetched from the DAG store during retrieval.
	if err := environment.RegisterShard(ctx.Context(), deal.Proposal.PieceCID, carFilePath, true); err != nil {
		err = xerrors.Errorf("failed to activate shard: %w", err)
		log.Error(err)
	}

	// announce the deal to the network indexer
	annCid, err := environment.AnnounceIndex(ctx.Context(), deal)
	if err != nil {
		log.Errorw("failed to announce index via reference provider", "proposalCid", deal.ProposalCid, "err", err)
	} else {
		log.Infow("deal announcement sent to index provider", "advertisementCid", annCid, "shard-key", deal.Proposal.PieceCID,
			"proposalCid", deal.ProposalCid)
	}

	log.Infow("successfully handed off deal to sealing subsystem", "pieceCid", deal.Proposal.PieceCID, "proposalCid", deal.ProposalCid)
	return ctx.Trigger(storagemarket.ProviderEventDealHandedOff)
}
// end

// HandoffDeal hands off a published deal for sealing and commitment in a sector
func HandoffDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	var packingInfo *storagemarket.PackingResult
	var carFilePath string
	if deal.PiecePath != "" {
		// Data for offline deals is stored on disk, so if PiecePath is set,
		// create a Reader from the file path
		file, err := environment.FileStore().Open(deal.PiecePath)
		if err != nil {
			return ctx.Trigger(storagemarket.ProviderEventFileStoreErrored,
				xerrors.Errorf("reading piece at path %s: %w", deal.PiecePath, err))
		}
		carFilePath = string(file.OsPath())

		// Hand the deal off to the process that adds it to a sector
		log.Infow("handing off deal to sealing subsystem", "pieceCid", deal.Proposal.PieceCID, "proposalCid", deal.ProposalCid)
		packingInfo, err = handoffDeal(ctx.Context(), environment, deal, file, uint64(file.Size()))
		if err := file.Close(); err != nil {
			log.Errorw("failed to close imported CAR file", "pieceCid", deal.Proposal.PieceCID, "proposalCid", deal.ProposalCid, "err", err)
		}

		if err != nil {
			err = xerrors.Errorf("packing piece at path %s: %w", deal.PiecePath, err)
			return ctx.Trigger(storagemarket.ProviderEventDealHandoffFailed, err)
		}
	} else {
		carFilePath = deal.InboundCAR

		v2r, err := environment.ReadCAR(deal.InboundCAR)
		if err != nil {
			return ctx.Trigger(storagemarket.ProviderEventDealHandoffFailed, xerrors.Errorf("failed to open CARv2 file, proposalCid=%s: %w",
				deal.ProposalCid, err))
		}

		// Hand the deal off to the process that adds it to a sector
		var packingErr error
		log.Infow("handing off deal to sealing subsystem", "pieceCid", deal.Proposal.PieceCID, "proposalCid", deal.ProposalCid)
		r, err := v2r.DataReader()
		if err != nil {
			return ctx.Trigger(storagemarket.ProviderEventDealHandoffFailed, fmt.Errorf("failed to get reader over file data, proposalCid=%s: %w",
				deal.ProposalCid, err))
		}
		packingInfo, packingErr = handoffDeal(ctx.Context(), environment, deal, r, v2r.Header.DataSize)
		// Close the reader as we're done reading from it.
		if err := v2r.Close(); err != nil {
			return ctx.Trigger(storagemarket.ProviderEventDealHandoffFailed, xerrors.Errorf("failed to close CARv2 reader: %w", err))
		}
		log.Infow("closed car datareader after handing off deal to sealing subsystem", "pieceCid", deal.Proposal.PieceCID, "proposalCid", deal.ProposalCid)
		if packingErr != nil {
			err = xerrors.Errorf("packing piece %s: %w", deal.Ref.PieceCid, packingErr)
			return ctx.Trigger(storagemarket.ProviderEventDealHandoffFailed, err)
		}
	}

	if err := recordPiece(environment, deal, packingInfo.SectorNumber, packingInfo.Offset, packingInfo.Size); err != nil {
		err = xerrors.Errorf("failed to register deal data for piece %s for retrieval: %w", deal.Ref.PieceCid, err)
		log.Error(err.Error())
		_ = ctx.Trigger(storagemarket.ProviderEventPieceStoreErrored, err)
	}

	// Register the deal data as a "shard" with the DAG store. Later it can be
	// fetched from the DAG store during retrieval.
	if err := environment.RegisterShard(ctx.Context(), deal.Proposal.PieceCID, carFilePath, true); err != nil {
		err = xerrors.Errorf("failed to activate shard: %w", err)
		log.Error(err)
	}

	// announce the deal to the network indexer
	annCid, err := environment.AnnounceIndex(ctx.Context(), deal)
	if err != nil {
		log.Errorw("failed to announce index via reference provider", "proposalCid", deal.ProposalCid, "err", err)
	} else {
		log.Infow("deal announcement sent to index provider", "advertisementCid", annCid, "shard-key", deal.Proposal.PieceCID,
			"proposalCid", deal.ProposalCid)
	}

	log.Infow("successfully handed off deal to sealing subsystem", "pieceCid", deal.Proposal.PieceCID, "proposalCid", deal.ProposalCid)
	return ctx.Trigger(storagemarket.ProviderEventDealHandedOff)
}

func handoffDeal(ctx context.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal, reader io.ReadSeeker, payloadSize uint64) (*storagemarket.PackingResult, error) {
	// because we use the PadReader directly during Add Piece we need to produce the
	// correct amount of zeroes
	// (alternative would be to keep precise track of sector offsets for each
	// piece which is just too much work for a seldom used feature)
	paddedReader, err := shared.NewInflatorReader(reader, payloadSize, deal.Proposal.PieceSize.Unpadded())
	if err != nil {
		return nil, err
	}

	return environment.Node().OnDealComplete(
		ctx,
		storagemarket.MinerDeal{
			Client:             deal.Client,
			ClientDealProposal: deal.ClientDealProposal,
			ProposalCid:        deal.ProposalCid,
			State:              deal.State,
			Ref:                deal.Ref,
			PublishCid:         deal.PublishCid,
			DealID:             deal.DealID,
			FastRetrieval:      deal.FastRetrieval,
		},
		deal.Proposal.PieceSize.Unpadded(),
		paddedReader,
	)
}

func recordPiece(environment ProviderDealEnvironment, deal storagemarket.MinerDeal, sectorID abi.SectorNumber, offset, length abi.PaddedPieceSize) error {

	var blockLocations map[cid.Cid]piecestore.BlockLocation
	if deal.MetadataPath != filestore.Path("") {
		var err error
		blockLocations, err = providerutils.LoadBlockLocations(environment.FileStore(), deal.MetadataPath)
		if err != nil {
			return xerrors.Errorf("failed to load block locations: %w", err)
		}
	} else {
		blockLocations = map[cid.Cid]piecestore.BlockLocation{
			deal.Ref.Root: {},
		}
	}

	if err := environment.PieceStore().AddPieceBlockLocations(deal.Proposal.PieceCID, blockLocations); err != nil {
		return xerrors.Errorf("failed to add piece block locations: %s", err)
	}

	err := environment.PieceStore().AddDealForPiece(deal.Proposal.PieceCID, deal.ProposalCid, piecestore.DealInfo{
		DealID:   deal.DealID,
		SectorID: sectorID,
		Offset:   offset,
		Length:   length,
	})
	if err != nil {
		return xerrors.Errorf("failed to add deal for piece: %s", err)
	}

	return nil
}

// CleanupDeal clears the filestore once we know the mining component has read the data and it is in a sealed sector
func CleanupDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	if deal.PiecePath != "" {
		err := environment.FileStore().Delete(deal.PiecePath)
		if err != nil {
			log.Warnf("deleting piece at path %s: %w", deal.PiecePath, err)
		}
	}
	if deal.MetadataPath != "" {
		err := environment.FileStore().Delete(deal.MetadataPath)
		if err != nil {
			log.Warnf("deleting piece at path %s: %w", deal.MetadataPath, err)
		}
	}

	if deal.InboundCAR != "" {
		if err := environment.TerminateBlockstore(deal.ProposalCid, deal.InboundCAR); err != nil {
			log.Warnf("failed to cleanup blockstore, car_path=%s: %s", deal.InboundCAR, err)
		}
	}

	return ctx.Trigger(storagemarket.ProviderEventFinalized)
}

// VerifyDealPreCommitted verifies that a deal has been pre-committed
func VerifyDealPreCommitted(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	cb := func(sectorNumber abi.SectorNumber, isActive bool, err error) {
		// It's possible that
		// - we miss the pre-commit message and have to wait for prove-commit
		// - the deal is already active (for example if the node is restarted
		//   while waiting for pre-commit)
		// In either of these two cases, isActive will be true.
		switch {
		case err != nil:
			_ = ctx.Trigger(storagemarket.ProviderEventDealPrecommitFailed, err)
		case isActive:
			_ = ctx.Trigger(storagemarket.ProviderEventDealActivated)
		default:
			_ = ctx.Trigger(storagemarket.ProviderEventDealPrecommitted, sectorNumber)
		}
	}

	err := environment.Node().OnDealSectorPreCommitted(ctx.Context(), deal.Proposal.Provider, deal.DealID, deal.Proposal, deal.PublishCid, cb)

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealPrecommitFailed, err)
	}
	return nil
}

// VerifyDealActivated verifies that a deal has been committed to a sector and activated
func VerifyDealActivated(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	// TODO: consider waiting for seal to happen
	cb := func(err error) {
		if err != nil {
			_ = ctx.Trigger(storagemarket.ProviderEventDealActivationFailed, err)
		} else {
			_ = ctx.Trigger(storagemarket.ProviderEventDealActivated)
		}
	}

	err := environment.Node().OnDealSectorCommitted(ctx.Context(), deal.Proposal.Provider, deal.DealID, deal.SectorNumber, deal.Proposal, deal.PublishCid, cb)

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealActivationFailed, err)
	}
	return nil
}

// WaitForDealCompletion waits for the deal to be slashed or to expire
func WaitForDealCompletion(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	// At this point we have all the data so we can unprotect the connection
	environment.UntagPeer(deal.Client, deal.ProposalCid.String())

	node := environment.Node()

	// Called when the deal expires
	expiredCb := func(err error) {
		// Ask the indexer to remove this deal
		environment.RemoveIndex(ctx.Context(), deal.ProposalCid)

		if err != nil {
			_ = ctx.Trigger(storagemarket.ProviderEventDealCompletionFailed, xerrors.Errorf("deal expiration err: %w", err))
		} else {
			_ = ctx.Trigger(storagemarket.ProviderEventDealExpired)
		}
	}

	// Called when the deal is slashed
	slashedCb := func(slashEpoch abi.ChainEpoch, err error) {
		// Ask the indexer to remove this deal
		environment.RemoveIndex(ctx.Context(), deal.ProposalCid)

		if err != nil {
			_ = ctx.Trigger(storagemarket.ProviderEventDealCompletionFailed, xerrors.Errorf("deal slashing err: %w", err))
		} else {
			_ = ctx.Trigger(storagemarket.ProviderEventDealSlashed, slashEpoch)
		}
	}

	if err := node.OnDealExpiredOrSlashed(ctx.Context(), deal.DealID, expiredCb, slashedCb); err != nil {
		return ctx.Trigger(storagemarket.ProviderEventDealCompletionFailed, err)
	}

	return nil
}

// RejectDeal sends a failure response before terminating a deal
func RejectDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	err := environment.SendSignedResponse(ctx.Context(), &network.Response{
		State:    storagemarket.StorageDealFailing,
		Message:  deal.Message,
		Proposal: deal.ProposalCid,
	})

	if err != nil {
		return ctx.Trigger(storagemarket.ProviderEventSendResponseFailed, err)
	}

	if err := environment.Disconnect(deal.ProposalCid); err != nil {
		log.Warnf("closing client connection: %+v", err)
	}

	return ctx.Trigger(storagemarket.ProviderEventRejectionSent)
}

// FailDeal cleans up before terminating a deal
func FailDeal(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) error {
	log.Warnf("deal %s failed: %s", deal.ProposalCid, deal.Message)

	environment.UntagPeer(deal.Client, deal.ProposalCid.String())

	if deal.PiecePath != filestore.Path("") {
		err := environment.FileStore().Delete(deal.PiecePath)
		if err != nil {
			log.Warnf("deleting piece at path %s: %w", deal.PiecePath, err)
		}
	}
	if deal.MetadataPath != filestore.Path("") {
		err := environment.FileStore().Delete(deal.MetadataPath)
		if err != nil {
			log.Warnf("deleting piece at path %s: %w", deal.MetadataPath, err)
		}
	}

	if deal.InboundCAR != "" {
		if err := environment.FinalizeBlockstore(deal.ProposalCid); err != nil {
			log.Warnf("error finalizing read-write store, car_path=%s: %s", deal.InboundCAR, err)
		}

		if err := environment.TerminateBlockstore(deal.ProposalCid, deal.InboundCAR); err != nil {
			log.Warnf("error deleting store, car_path=%s: %s", deal.InboundCAR, err)
		}
	}

	releaseReservedFunds(ctx, environment, deal)

	return ctx.Trigger(storagemarket.ProviderEventFailed)
}

func releaseReservedFunds(ctx fsm.Context, environment ProviderDealEnvironment, deal storagemarket.MinerDeal) {
	if !deal.FundsReserved.Nil() && !deal.FundsReserved.IsZero() {
		err := environment.Node().ReleaseFunds(ctx.Context(), deal.Proposal.Provider, deal.FundsReserved)
		if err != nil {
			// nonfatal error
			log.Warnf("failed to release funds: %s", err)
		}
		_ = ctx.Trigger(storagemarket.ProviderEventFundsReleased, deal.FundsReserved)
	}
}
