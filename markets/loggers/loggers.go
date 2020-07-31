package marketevents

import (
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("markets")

// StorageClientLogger logs events from the storage client
func StorageClientLogger(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
	log.Infow("Storage Event", "Name", storagemarket.ClientEvents[event], "Proposal CID", deal.ProposalCid, "State", storagemarket.DealStates[deal.State], "Message", deal.Message)
}

// StorageProviderLogger logs events from the storage provider
func StorageProviderLogger(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
	log.Infow("Storage Event", "Name", storagemarket.ProviderEvents[event], "Proposal CID", deal.ProposalCid, "State", storagemarket.DealStates[deal.State], "Message", deal.Message)
}

// RetrievalClientLogger logs events from the retrieval client
func RetrievalClientLogger(event retrievalmarket.ClientEvent, deal retrievalmarket.ClientDealState) {
	log.Infow("Retrieval Event", "Name", retrievalmarket.ClientEvents[event], "Deal ID", deal.ID, "State", retrievalmarket.DealStatuses[deal.Status], "Message", deal.Message)
}

// RetrievalProviderLogger logs events from the retrieval provider
func RetrievalProviderLogger(event retrievalmarket.ProviderEvent, deal retrievalmarket.ProviderDealState) {
	log.Infow("Retrieval Event", "Name", retrievalmarket.ProviderEvents[event], "Deal ID", deal.ID, "Receiver", deal.Receiver, "State", retrievalmarket.DealStatuses[deal.Status], "Message", deal.Message)
}
