package markets

import (
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"

	"github.com/filecoin-project/lotus/journal"
)

type StorageClientEvt struct {
	Event string
	Deal  storagemarket.ClientDeal
}

type StorageProviderEvt struct {
	Event string
	Deal  storagemarket.MinerDeal
}

type RetrievalClientEvt struct {
	Event string
	Deal  retrievalmarket.ClientDealState
}

type RetrievalProviderEvt struct {
	Event string
	Deal  retrievalmarket.ProviderDealState
}

// StorageClientJournaler records journal events from the storage client.
func StorageClientJournaler(evtType journal.EventType) func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
	return func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		journal.J.RecordEvent(evtType, func() interface{} {
			return StorageClientEvt{
				Event: storagemarket.ClientEvents[event],
				Deal:  deal,
			}
		})
	}
}

// StorageProviderJournaler records journal events from the storage provider.
func StorageProviderJournaler(evtType journal.EventType) func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
	return func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		journal.J.RecordEvent(evtType, func() interface{} {
			return StorageProviderEvt{
				Event: storagemarket.ProviderEvents[event],
				Deal:  deal,
			}
		})
	}
}

// RetrievalClientJournaler records journal events from the retrieval client.
func RetrievalClientJournaler(evtType journal.EventType) func(event retrievalmarket.ClientEvent, deal retrievalmarket.ClientDealState) {
	return func(event retrievalmarket.ClientEvent, deal retrievalmarket.ClientDealState) {
		journal.J.RecordEvent(evtType, func() interface{} {
			return RetrievalClientEvt{
				Event: retrievalmarket.ClientEvents[event],
				Deal:  deal,
			}
		})
	}
}

// RetrievalProviderJournaler records journal events from the retrieval provider.
func RetrievalProviderJournaler(evtType journal.EventType) func(event retrievalmarket.ProviderEvent, deal retrievalmarket.ProviderDealState) {
	return func(event retrievalmarket.ProviderEvent, deal retrievalmarket.ProviderDealState) {
		journal.J.RecordEvent(evtType, func() interface{} {
			return RetrievalProviderEvt{
				Event: retrievalmarket.ProviderEvents[event],
				Deal:  deal,
			}
		})
	}
}
