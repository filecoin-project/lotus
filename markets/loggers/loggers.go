package marketevents

import (
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("markets")

// StorageClientLogger logs events from the storage client
func StorageClientLogger(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
	log.Infof("Storage Event: %s, Proposal CID: %s, State: %s, Message: %s", storagemarket.ClientEvents[event], deal.ProposalCid, storagemarket.DealStates[deal.State], deal.Message)
}

// StorageProviderLogger logs events from the storage provider
func StorageProviderLogger(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
	log.Infof("Storage Event: %s, Proposal CID: %s, State: %s, Message: %s", storagemarket.ProviderEvents[event], deal.ProposalCid, storagemarket.DealStates[deal.State], deal.Message)
}

// RetrievalClientLogger logs events from the retrieval client
func RetrievalClientLogger(event retrievalmarket.ClientEvent, deal retrievalmarket.ClientDealState) {
	log.Infof("Retrieval Event: %s, Deal ID: %d, State: %s, Message: %s", retrievalmarket.ClientEvents[event], deal.ID, retrievalmarket.DealStatuses[deal.Status], deal.Message)
}

// RetrievalProviderLogger logs events from the retrieval provider
func RetrievalProviderLogger(event retrievalmarket.ProviderEvent, deal retrievalmarket.ProviderDealState) {
	log.Infof("Retrieval Event: %s, Deal ID: %s, State: %s, Message: %s", retrievalmarket.ProviderEvents[event], deal.Identifier().String(), retrievalmarket.DealStatuses[deal.Status], deal.Message)
}
