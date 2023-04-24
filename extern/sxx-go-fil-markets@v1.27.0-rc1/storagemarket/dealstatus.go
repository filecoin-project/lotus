package storagemarket

// StorageDealStatus is the local status of a StorageDeal.
// Note: this status has meaning in the context of this module only - it is not
// recorded on chain
type StorageDealStatus = uint64

const (
	// StorageDealUnknown means the current status of a deal is undefined
	StorageDealUnknown = StorageDealStatus(iota)

	// StorageDealProposalNotFound is a status returned in responses when the deal itself cannot
	// be located
	StorageDealProposalNotFound

	// StorageDealProposalRejected is returned by a StorageProvider when it chooses not to accept
	// a DealProposal
	StorageDealProposalRejected

	// StorageDealProposalAccepted indicates an intent to accept a storage deal proposal
	StorageDealProposalAccepted

	// StorageDealStaged means a deal has been published and data is ready to be put into a sector
	StorageDealStaged

	// StorageDealSealing means a deal is in a sector that is being sealed
	StorageDealSealing

	// StorageDealFinalizing means a deal is in a sealed sector and we're doing final
	// housekeeping before marking it active
	StorageDealFinalizing

	// StorageDealActive means a deal is in a sealed sector and the miner is proving the data
	// for the deal
	StorageDealActive

	// StorageDealExpired means a deal has passed its final epoch and is expired
	StorageDealExpired

	// StorageDealSlashed means the deal was in a sector that got slashed from failing to prove
	StorageDealSlashed

	// StorageDealRejecting means the Provider has rejected the deal, and will send a rejection response
	StorageDealRejecting

	// StorageDealFailing means something has gone wrong in a deal. Once data is cleaned up the deal will finalize on
	// StorageDealError
	StorageDealFailing

	// StorageDealFundsReserved means we've deposited funds as necessary to create a deal, ready to move forward
	StorageDealFundsReserved

	// StorageDealCheckForAcceptance means the client is waiting for a provider to seal and publish a deal
	StorageDealCheckForAcceptance

	// StorageDealValidating means the provider is validating that deal parameters are good for a proposal
	StorageDealValidating

	// StorageDealAcceptWait means the provider is running any custom decision logic to decide whether or not to accept the deal
	StorageDealAcceptWait

	// StorageDealStartDataTransfer means data transfer is beginning
	StorageDealStartDataTransfer

	// StorageDealTransferring means data is being sent from the client to the provider via the data transfer module
	StorageDealTransferring

	// StorageDealWaitingForData indicates either a manual transfer
	// or that the provider has not received a data transfer request from the client
	StorageDealWaitingForData

	// StorageDealVerifyData means data has been transferred and we are attempting to verify it against the PieceCID
	StorageDealVerifyData

	// StorageDealReserveProviderFunds means that provider is making sure it has adequate funds for the deal in the StorageMarketActor
	StorageDealReserveProviderFunds

	// StorageDealReserveClientFunds means that client is making sure it has adequate funds for the deal in the StorageMarketActor
	StorageDealReserveClientFunds

	// StorageDealProviderFunding means that the provider has deposited funds in the StorageMarketActor and it is waiting
	// to see the funds appear in its balance
	StorageDealProviderFunding

	// StorageDealClientFunding means that the client has deposited funds in the StorageMarketActor and it is waiting
	// to see the funds appear in its balance
	StorageDealClientFunding

	// StorageDealPublish means the deal is ready to be published on chain
	StorageDealPublish

	// StorageDealPublishing means the deal has been published but we are waiting for it to appear on chain
	StorageDealPublishing

	// StorageDealError means the deal has failed due to an error, and no further updates will occur
	StorageDealError

	// StorageDealProviderTransferAwaitRestart means the provider has restarted while data
	// was being transferred from client to provider, and will wait for the client to
	// resume the transfer
	StorageDealProviderTransferAwaitRestart

	// StorageDealClientTransferRestart means a storage deal data transfer from client to provider will be restarted
	// by the client
	StorageDealClientTransferRestart

	// StorageDealAwaitingPreCommit means a deal is ready and must be pre-committed
	StorageDealAwaitingPreCommit

	// StorageDealTransferQueued means the data transfer request has been queued and will be executed soon.
	StorageDealTransferQueued

	// add by lin
	StorageDealStagedOfSxx
	// end
)

// DealStates maps StorageDealStatus codes to string names
var DealStates = map[StorageDealStatus]string{
	StorageDealUnknown:                      "StorageDealUnknown",
	StorageDealProposalNotFound:             "StorageDealProposalNotFound",
	StorageDealProposalRejected:             "StorageDealProposalRejected",
	StorageDealProposalAccepted:             "StorageDealProposalAccepted",
	StorageDealAcceptWait:                   "StorageDealAcceptWait",
	StorageDealStartDataTransfer:            "StorageDealStartDataTransfer",
	StorageDealStaged:                       "StorageDealStaged",
	// add by lin
	StorageDealStagedOfSxx:                  "StorageDealStagedOfSxx",
	// end
	StorageDealAwaitingPreCommit:            "StorageDealAwaitingPreCommit",
	StorageDealSealing:                      "StorageDealSealing",
	StorageDealActive:                       "StorageDealActive",
	StorageDealExpired:                      "StorageDealExpired",
	StorageDealSlashed:                      "StorageDealSlashed",
	StorageDealRejecting:                    "StorageDealRejecting",
	StorageDealFailing:                      "StorageDealFailing",
	StorageDealFundsReserved:                "StorageDealFundsReserved",
	StorageDealCheckForAcceptance:           "StorageDealCheckForAcceptance",
	StorageDealValidating:                   "StorageDealValidating",
	StorageDealTransferring:                 "StorageDealTransferring",
	StorageDealWaitingForData:               "StorageDealWaitingForData",
	StorageDealVerifyData:                   "StorageDealVerifyData",
	StorageDealReserveProviderFunds:         "StorageDealReserveProviderFunds",
	StorageDealReserveClientFunds:           "StorageDealReserveClientFunds",
	StorageDealProviderFunding:              "StorageDealProviderFunding",
	StorageDealClientFunding:                "StorageDealClientFunding",
	StorageDealPublish:                      "StorageDealPublish",
	StorageDealPublishing:                   "StorageDealPublishing",
	StorageDealError:                        "StorageDealError",
	StorageDealFinalizing:                   "StorageDealFinalizing",
	StorageDealClientTransferRestart:        "StorageDealClientTransferRestart",
	StorageDealProviderTransferAwaitRestart: "StorageDealProviderTransferAwaitRestart",
	StorageDealTransferQueued:               "StorageDealTransferQueued",
}

// DealStatesDescriptions maps StorageDealStatus codes to string description for better UX
var DealStatesDescriptions = map[StorageDealStatus]string{
	StorageDealUnknown:                      "Unknown",
	StorageDealProposalNotFound:             "Proposal not found",
	StorageDealProposalRejected:             "Proposal rejected",
	StorageDealProposalAccepted:             "Proposal accepted",
	StorageDealAcceptWait:                   "AcceptWait",
	StorageDealStartDataTransfer:            "Starting data transfer",
	StorageDealStaged:                       "Staged",
	// add by lin
	StorageDealStagedOfSxx:                  "Staged",
	// end
	StorageDealAwaitingPreCommit:            "Awaiting a PreCommit message on chain",
	StorageDealSealing:                      "Sealing",
	StorageDealActive:                       "Active",
	StorageDealExpired:                      "Expired",
	StorageDealSlashed:                      "Slashed",
	StorageDealRejecting:                    "Rejecting",
	StorageDealFailing:                      "Failing",
	StorageDealFundsReserved:                "FundsReserved",
	StorageDealCheckForAcceptance:           "Checking for deal acceptance",
	StorageDealValidating:                   "Validating",
	StorageDealTransferring:                 "Transferring",
	StorageDealWaitingForData:               "Waiting for data",
	StorageDealVerifyData:                   "Verifying data",
	StorageDealReserveProviderFunds:         "Reserving provider funds",
	StorageDealReserveClientFunds:           "Reserving client funds",
	StorageDealProviderFunding:              "Provider funding",
	StorageDealClientFunding:                "Client funding",
	StorageDealPublish:                      "Publish",
	StorageDealPublishing:                   "Publishing",
	StorageDealError:                        "Error",
	StorageDealFinalizing:                   "Finalizing",
	StorageDealClientTransferRestart:        "Client transfer restart",
	StorageDealProviderTransferAwaitRestart: "ProviderTransferAwaitRestart",
}

var DealStatesDurations = map[StorageDealStatus]string{
	StorageDealUnknown:                      "",
	StorageDealProposalNotFound:             "",
	StorageDealProposalRejected:             "",
	StorageDealProposalAccepted:             "a few minutes",
	StorageDealAcceptWait:                   "a few minutes",
	StorageDealStartDataTransfer:            "a few minutes",
	StorageDealStaged:                       "a few minutes",
	// add by lin
	StorageDealStagedOfSxx:                  "a few minutes",
	// end
	StorageDealAwaitingPreCommit:            "a few minutes",
	StorageDealSealing:                      "a few hours",
	StorageDealActive:                       "",
	StorageDealExpired:                      "",
	StorageDealSlashed:                      "",
	StorageDealRejecting:                    "",
	StorageDealFailing:                      "",
	StorageDealFundsReserved:                "a few minutes",
	StorageDealCheckForAcceptance:           "a few minutes",
	StorageDealValidating:                   "a few minutes",
	StorageDealTransferring:                 "a few minutes",
	StorageDealWaitingForData:               "a few minutes",
	StorageDealVerifyData:                   "a few minutes",
	StorageDealReserveProviderFunds:         "a few minutes",
	StorageDealReserveClientFunds:           "a few minutes",
	StorageDealProviderFunding:              "a few minutes",
	StorageDealClientFunding:                "a few minutes",
	StorageDealPublish:                      "a few minutes",
	StorageDealPublishing:                   "a few minutes",
	StorageDealError:                        "",
	StorageDealFinalizing:                   "a few minutes",
	StorageDealClientTransferRestart:        "depending on data size, anywhere between a few minutes to a few hours",
	StorageDealProviderTransferAwaitRestart: "a few minutes",
}
