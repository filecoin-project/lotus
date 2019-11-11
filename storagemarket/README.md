storagemarket will eventually be extracted into a standalone module to be consumed by filecoin implementations

```
package storagemarket

type Signature {
	Type string
	Data []byte
}

type StorageDealProposal struct {
  PieceCid           []byte
	PieceSize          uint64

	// Will Go Away -- here for compatibility till we do CAR
	PieceSerialization uint64

	Client             []byte
  Provider           []byte
  ProposalExpiration uint64
  DealExpiration     uint64
  StoragePrice       []byte
  StorageCollateral  []byte
  ProposerSignature  Signature
}
 
type StorageDeal struct {
  StorageDealProposal
 
  // Cid of message which published this deal on-chain
  PublishMessage cid.Cid
}
 
type StorageAsk struct {
  Price      []byte
	MinSize uint64
	Timestamp    uint64
	Expiry       uint64
	SeqNo        uint64
  // TODO: maybe other criteria here: MaxDuration, MaxCollateral
}
 
// Closely follows the MinerInfo struct in the spec
type StorageProviderInfo struct {
  Address    []byte // actor address
  Owner      []byte
  Worker     []byte // signs messages
  SectorSize uint64
  PeerID     peer.PeerID
  // probably more like how much storage power, available collateral etc
}
 
type AskCriteria struct {
  MaxPrice []byte
  MinSize  uint64
 
  // TODO: maybe other criteria here: duration, collateral
}
 
type StorageOffer struct {
  provider StorageProviderInfo
  ask      StorageAsk
}
 
// Balance is the funds associated with an address stored in the StorageMarketActor
type Balance struct {
  Address   string
  Locked    []byte
  Available []byte
}
 
type ProposeStorageDealResult struct {
  Proposal StorageDealProposal
  Result   StorageDealProposalResponse
}
 
// A miner or client's state in the StorageMinerActor
type ParticipantState struct {
  Balance Balance
  Deals   []StorageDeal
}
 
type StorageMarketEventHandler interface {
  OnStorageMarketStateChange(stateId cid.Cid, participant ParticipantState)
}

type StorageClientNode interface {
  MostRecentStateId() (cid.Cid, uint64)

  // Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
  AddFunds(addr []byte, amount []byte) Cid

  // GetBalance returns locked/unlocked for a storage participant.  Used by both providers and clients.
  GetBalance(stateId cid.Cid, addr []byte) Balance

  // ListClientDeals lists all deals associated with a storage client
  // TODO: paging or delta-based return values
  ListClientDeals(stateId cid.Cid, addr []byte) []*StorageDeal

  // GetProviderInfo returns information about a single storage provider
  GetProviderInfo(stateId cid.Cid, addr []byte) *StorageProviderInfo

  // GetStorageProviders returns information about known miners
  // TODO: figure out paging/channel/returning lots of results
  ListStorageProviders(stateId cid.Cid) []*StorageProviderInfo

  // Subscribes to storage market actor state changes for a given address.
  // TODO: Should there be a timeout option for this?  In the case that we are waiting for funds to be deposited and it never happens?
  SubscribeStorageMarketEvents(addr []byte, handler StorageMarketEventHandler) (SubID, error)

  // Cancels a subscription
  UnsubscribeStorageMarketEvents(subId SubID)
}

type StorageClientProofs interface {
  GeneratePieceCommitment(piece io.Reader, pieceSize uint64) ([]byte, error)
}

// The interface provided by the module to the outside world for storage clients.
type StorageClient interface {
  // ListProviders queries chain state and returns active storage providers
  ListProviders() <-chan StorageProviderInfo

  // ListDeals lists on-chain deals associated with this client
  ListDeals() <-chan StorageDeal

  // ListInProgressDeals lists deals that are in progress or rejected
  ListInProgressDeals() []StorageDeal

  // GetAsk returns the current ask for a storage provider
  GetAsk(addr Address) *StorageAsk

  // FindStorageOffers lists providers and queries them to find offers that satisfy some criteria based on price, duration, etc.
  FindStorageOffers(criteria AskCriteria, limit uint) []*StorageOffer

  // ProposeStorageDeal initiates deal negotiation with a Storage Provider
  ProposeStorageDeal(provider Address, payloadCid Cid, size uint64, proposalExpiration Epoch, dealExpiration Epoch, price TokenAmount, collateral TokenAmount) (ProposeStorageDealResult, error)

  // GetPaymentEscrow returns the current funds available for deal payment
  GetPaymentEscrow() Balance

  // AddPaymentEscrow adds payment funds to the StorageMinerActor for this client
  AddPaymentEscrow(TokenAmount) error
}

type StorageProviderEvent string

type ProviderEventsHandler func(event StorageProviderEvent, deal *StorageDeal)

// The interface provided for storage providers
type StorageProvider interface {
  AddAsk(ask StorageAsk)

  // ListAsks lists current asks
  ListAsks() []StorageAsk

  // ListDeals lists on-chain deals associated with this provider
  ListDeals(ctx context.Context) <-chan StorageDeal

  // ListIncompleteDeals lists deals that are in progress or rejected
  ListIncompleteDeals(ctx context.Context) []StorageDeal

  // AddStorageCollateral adds storage collateral
  AddStorageCollateral(amount TokenAmount) error

  // GetStorageCollateral returns the current collateral balance
  GetStorageCollateral() Balance

  // SubscribeDealEvents registers a handler which will receive storage-provider-related events
  SubscribeDealEvents(handler ProviderEventsHandler)

  // TODO: Does this belong here for now?  PeerId seems to be going away
  // UpdatePeerId sets the libp2p address for this provider
  SetPeerId(addr Address)
}

// Node dependencies for a StorageProvider
type StorageProviderNode interface {
  MostRecentStateId() StateID

  // Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
  AddFunds(addr Address, amount TokenAmount) Cid

  // GetBalance returns locked/unlocked for a storage participant.  Used by both providers and clients.
  GetBalance(stateId StateID, addr Address) Balance

  // Publishes deal on chain
  PublishDeals(deals []StorageDeal) Cid

  // ListProviderDeals lists all deals associated with a storage provider
  // TODO: paging or delta-based return values
  ListProviderDeals(stateId StateID, addr Address) []*StorageDeal

  // Subscribes to storage market actor state changes for a given address.
  SubscribeStorageMarketEvents(addr Address, handler StorageMarketEventHandler) (SubID, error)

  // Cancels a subscription
  UnsubscribeStorageMarketEvents(subId SubID)
}
```