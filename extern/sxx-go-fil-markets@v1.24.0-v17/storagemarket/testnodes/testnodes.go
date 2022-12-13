// Package testnodes contains stubbed implementations of the StorageProviderNode
// and StorageClientNode interface to simulate communications with a filecoin node
package testnodes

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/go-fil-markets/commp"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

// Below fake node implementations

// StorageMarketState represents a state for the storage market that can be inspected
// - methods on the provider nodes will affect this state
type StorageMarketState struct {
	TipSetToken shared.TipSetToken
	Epoch       abi.ChainEpoch
	DealID      abi.DealID
	Balances    map[address.Address]abi.TokenAmount
	Providers   map[address.Address]*storagemarket.StorageProviderInfo
}

// NewStorageMarketState returns a new empty state for the storage market
func NewStorageMarketState() *StorageMarketState {
	return &StorageMarketState{
		Epoch:     0,
		DealID:    0,
		Balances:  map[address.Address]abi.TokenAmount{},
		Providers: map[address.Address]*storagemarket.StorageProviderInfo{},
	}
}

// AddFunds adds funds for a given address in the storage market
func (sma *StorageMarketState) AddFunds(addr address.Address, amount abi.TokenAmount) {
	if existing, ok := sma.Balances[addr]; ok {
		sma.Balances[addr] = big.Add(existing, amount)
	} else {
		sma.Balances[addr] = amount
	}
}

// Balance returns the balance of a given address in the market
func (sma *StorageMarketState) Balance(addr address.Address) storagemarket.Balance {
	if existing, ok := sma.Balances[addr]; ok {
		return storagemarket.Balance{Locked: big.NewInt(0), Available: existing}
	}
	return storagemarket.Balance{Locked: big.NewInt(0), Available: big.NewInt(0)}
}

// StateKey returns a state key with the storage market states set Epoch
func (sma *StorageMarketState) StateKey() (shared.TipSetToken, abi.ChainEpoch) {
	return sma.TipSetToken, sma.Epoch
}

// FakeCommonNode implements common methods for the storage & client node adapters
// where responses are stubbed
type FakeCommonNode struct {
	SMState                    *StorageMarketState
	DealFunds                  *shared_testutil.TestDealFunds
	AddFundsCid                cid.Cid
	ReserveFundsError          error
	VerifySignatureFails       bool
	GetBalanceError            error
	GetChainHeadError          error
	SignBytesError             error
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

	WaitForMessageBlocks    bool
	WaitForMessageError     error
	WaitForMessageExitCode  exitcode.ExitCode
	WaitForMessageRetBytes  []byte
	WaitForMessageFinalCid  cid.Cid
	WaitForMessageNodeError error
	WaitForMessageCalls     []cid.Cid

	DelayFakeCommonNode DelayFakeCommonNode
}

// DelayFakeCommonNode allows configuring delay in the FakeCommonNode functions
type DelayFakeCommonNode struct {
	OnDealSectorPreCommitted     bool
	OnDealSectorPreCommittedChan chan struct{}

	OnDealSectorCommitted     bool
	OnDealSectorCommittedChan chan struct{}

	OnDealExpiredOrSlashed     bool
	OnDealExpiredOrSlashedChan chan struct{}

	ValidatePublishedDeal     bool
	ValidatePublishedDealChan chan struct{}
}

// GetChainHead returns the state id in the storage market state
func (n *FakeCommonNode) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	if n.GetChainHeadError == nil {
		key, epoch := n.SMState.StateKey()
		return key, epoch, nil
	}

	return []byte{}, 0, n.GetChainHeadError
}

// AddFunds adds funds to the given actor in the storage market state
func (n *FakeCommonNode) AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) (cid.Cid, error) {
	n.SMState.AddFunds(addr, amount)
	return n.AddFundsCid, nil
}

// ReserveFunds reserves funds required for a deal with the storage market actor
func (n *FakeCommonNode) ReserveFunds(ctx context.Context, wallet, addr address.Address, amt abi.TokenAmount) (cid.Cid, error) {
	if n.ReserveFundsError == nil {
		_, _ = n.DealFunds.Reserve(amt)
		balance := n.SMState.Balance(addr)
		if balance.Available.LessThan(amt) {
			return n.AddFunds(ctx, addr, big.Sub(amt, balance.Available))
		}
	}

	return cid.Undef, n.ReserveFundsError
}

// ReleaseFunds releases funds reserved with ReserveFunds
func (n *FakeCommonNode) ReleaseFunds(ctx context.Context, addr address.Address, amt abi.TokenAmount) error {
	n.DealFunds.Release(amt)
	return nil
}

// WaitForMessage simulates waiting for a message to appear on chain
func (n *FakeCommonNode) WaitForMessage(ctx context.Context, mcid cid.Cid, onCompletion func(exitcode.ExitCode, []byte, cid.Cid, error) error) error {
	n.WaitForMessageCalls = append(n.WaitForMessageCalls, mcid)

	if n.WaitForMessageError != nil {
		return n.WaitForMessageError
	}

	if n.WaitForMessageBlocks {
		// just leave the test node in this state to simulate a long operation
		return nil
	}

	finalCid := n.WaitForMessageFinalCid
	if finalCid.Equals(cid.Undef) {
		finalCid = mcid
	}

	return onCompletion(n.WaitForMessageExitCode, n.WaitForMessageRetBytes, finalCid, n.WaitForMessageNodeError)
}

// GetBalance returns the funds in the storage market state
func (n *FakeCommonNode) GetBalance(ctx context.Context, addr address.Address, tok shared.TipSetToken) (storagemarket.Balance, error) {
	if n.GetBalanceError == nil {
		return n.SMState.Balance(addr), nil
	}
	return storagemarket.Balance{}, n.GetBalanceError
}

// VerifySignature just always returns true, for now
func (n *FakeCommonNode) VerifySignature(ctx context.Context, signature crypto.Signature, addr address.Address, data []byte, tok shared.TipSetToken) (bool, error) {
	return !n.VerifySignatureFails, nil
}

// SignBytes simulates signing data by returning a test signature
func (n *FakeCommonNode) SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error) {
	if n.SignBytesError == nil {
		return shared_testutil.MakeTestSignature(), nil
	}
	return nil, n.SignBytesError
}

func (n *FakeCommonNode) DealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, isVerified bool) (abi.TokenAmount, abi.TokenAmount, error) {
	return abi.NewTokenAmount(5000), builtin.TotalFilecoin, nil
}

// OnDealSectorPreCommitted returns immediately, and returns stubbed errors
func (n *FakeCommonNode) OnDealSectorPreCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, proposal market.DealProposal, publishCid *cid.Cid, cb storagemarket.DealSectorPreCommittedCallback) error {
	if n.DelayFakeCommonNode.OnDealSectorPreCommitted {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-n.DelayFakeCommonNode.OnDealSectorPreCommittedChan:
		}
	}
	if n.DealPreCommittedSyncError == nil {
		cb(n.PreCommittedSectorNumber, n.PreCommittedIsActive, n.DealPreCommittedAsyncError)
	}
	return n.DealPreCommittedSyncError
}

// OnDealSectorCommitted returns immediately, and returns stubbed errors
func (n *FakeCommonNode) OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, sectorNumber abi.SectorNumber, proposal market.DealProposal, publishCid *cid.Cid, cb storagemarket.DealSectorCommittedCallback) error {
	if n.DelayFakeCommonNode.OnDealSectorCommitted {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-n.DelayFakeCommonNode.OnDealSectorCommittedChan:
		}
	}
	if n.DealCommittedSyncError == nil {
		cb(n.DealCommittedAsyncError)
	}
	return n.DealCommittedSyncError
}

// OnDealExpiredOrSlashed simulates waiting for a deal to be expired or slashed, but provides stubbed behavior
func (n *FakeCommonNode) OnDealExpiredOrSlashed(ctx context.Context, dealID abi.DealID, onDealExpired storagemarket.DealExpiredCallback, onDealSlashed storagemarket.DealSlashedCallback) error {
	if n.DelayFakeCommonNode.OnDealExpiredOrSlashed {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-n.DelayFakeCommonNode.OnDealExpiredOrSlashedChan:
		}
	}

	if n.WaitForDealCompletionError != nil {
		return n.WaitForDealCompletionError
	}

	if n.OnDealSlashedError != nil {
		onDealSlashed(abi.ChainEpoch(0), n.OnDealSlashedError)
		return nil
	}

	if n.OnDealExpiredError != nil {
		onDealExpired(n.OnDealExpiredError)
		return nil
	}

	if n.OnDealSlashedEpoch == 0 {
		onDealExpired(nil)
		return nil
	}

	onDealSlashed(n.OnDealSlashedEpoch, nil)
	return nil
}

var _ storagemarket.StorageCommon = (*FakeCommonNode)(nil)

// FakeClientNode is a node adapter for a storage client whose responses
// are stubbed
type FakeClientNode struct {
	FakeCommonNode
	ClientAddr              address.Address
	MinerAddr               address.Address
	WorkerAddr              address.Address
	ValidationError         error
	ValidatePublishedDealID abi.DealID
	ValidatePublishedError  error
	ExpectedMinerInfos      []address.Address
	receivedMinerInfos      []address.Address
}

// ListStorageProviders lists the providers in the storage market state
func (n *FakeClientNode) ListStorageProviders(ctx context.Context, tok shared.TipSetToken) ([]*storagemarket.StorageProviderInfo, error) {
	providers := make([]*storagemarket.StorageProviderInfo, 0, len(n.SMState.Providers))
	for _, provider := range n.SMState.Providers {
		providers = append(providers, provider)
	}
	return providers, nil
}

// ValidatePublishedDeal always succeeds
func (n *FakeClientNode) ValidatePublishedDeal(ctx context.Context, deal storagemarket.ClientDeal) (abi.DealID, error) {
	if n.DelayFakeCommonNode.ValidatePublishedDeal {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-n.DelayFakeCommonNode.ValidatePublishedDealChan:
		}
	}

	return n.ValidatePublishedDealID, n.ValidatePublishedError
}

// SignProposal signs a deal with a dummy signature
func (n *FakeClientNode) SignProposal(ctx context.Context, signer address.Address, proposal market.DealProposal) (*market.ClientDealProposal, error) {
	return &market.ClientDealProposal{
		Proposal:        proposal,
		ClientSignature: *shared_testutil.MakeTestSignature(),
	}, nil
}

// GetDefaultWalletAddress returns a stubbed ClientAddr
func (n *FakeClientNode) GetDefaultWalletAddress(ctx context.Context) (address.Address, error) {
	return n.ClientAddr, nil
}

// GetMinerInfo returns stubbed information for the first miner in storage market state
func (n *FakeClientNode) GetMinerInfo(ctx context.Context, maddr address.Address, tok shared.TipSetToken) (*storagemarket.StorageProviderInfo, error) {
	n.receivedMinerInfos = append(n.receivedMinerInfos, maddr)
	info, ok := n.SMState.Providers[maddr]
	if !ok {
		return nil, errors.New("Provider not found")
	}
	return info, nil
}

func (n *FakeClientNode) VerifyExpectations(t *testing.T) {
	require.Equal(t, n.ExpectedMinerInfos, n.receivedMinerInfos)
}

var _ storagemarket.StorageClientNode = (*FakeClientNode)(nil)

// FakeProviderNode implements functions specific to the StorageProviderNode
type FakeProviderNode struct {
	FakeCommonNode
	MinerAddr                           address.Address
	MinerWorkerError                    error
	PieceLength                         uint64
	PieceSectorID                       uint64
	PublishDealID                       abi.DealID
	PublishDealsError                   error
	WaitForPublishDealsError            error
	OnDealCompleteError                 error
	OnDealCompleteSkipCommP             bool
	LastOnDealCompleteBytes             []byte
	OnDealCompleteCalls                 []storagemarket.MinerDeal
	LocatePieceForDealWithinSectorError error
	DataCap                             *verifreg.DataCap
	GetDataCapErr                       error

	lk     sync.Mutex
	Sealed map[abi.SectorNumber]bool
}

// PublishDeals simulates publishing a deal by adding it to the storage market state
func (n *FakeProviderNode) PublishDeals(ctx context.Context, deal storagemarket.MinerDeal) (cid.Cid, error) {
	if n.PublishDealsError == nil {
		return shared_testutil.GenerateCids(1)[0], nil
	}
	return cid.Undef, n.PublishDealsError
}

// WaitForPublishDeals simulates waiting for the deal to be published and
// calling the callback with the results
func (n *FakeProviderNode) WaitForPublishDeals(ctx context.Context, mcid cid.Cid, proposal market.DealProposal) (*storagemarket.PublishDealsWaitResult, error) {
	if n.WaitForPublishDealsError != nil {
		return nil, n.WaitForPublishDealsError
	}

	finalCid := n.WaitForMessageFinalCid
	if finalCid.Equals(cid.Undef) {
		finalCid = mcid
	}

	return &storagemarket.PublishDealsWaitResult{
		DealID:   n.PublishDealID,
		FinalCid: finalCid,
	}, nil
}

// OnDealComplete simulates passing of the deal to the storage miner, and does nothing
func (n *FakeProviderNode) OnDealComplete(ctx context.Context, deal storagemarket.MinerDeal, pieceSize abi.UnpaddedPieceSize, pieceReader shared.ReadSeekStarter) (*storagemarket.PackingResult, error) {
	n.OnDealCompleteCalls = append(n.OnDealCompleteCalls, deal)
	n.LastOnDealCompleteBytes, _ = ioutil.ReadAll(pieceReader)

	if n.OnDealCompleteError != nil || n.OnDealCompleteSkipCommP {
		return &storagemarket.PackingResult{}, n.OnDealCompleteError
	}

	// We read in all the bytes from the reader above, so seek back to the start
	err := pieceReader.SeekStart()
	if err != nil {
		return nil, fmt.Errorf("on deal complete: seeking to start of piece data: %w", err)
	}

	// Generate commP
	pieceCID, err := commp.GenerateCommp(pieceReader, uint64(pieceSize), uint64(pieceSize))
	if err != nil {
		return nil, fmt.Errorf("on deal complete: generating commp: %w", err)
	}

	// Check that commP of the data matches the proposal piece CID
	if pieceCID != deal.Proposal.PieceCID {
		return nil, fmt.Errorf("on deal complete: proposal piece CID %s does not match calculated commP %s", deal.Proposal.PieceCID, pieceCID)
	}

	return &storagemarket.PackingResult{}, n.OnDealCompleteError
}

// GetMinerWorkerAddress returns the address specified by MinerAddr
func (n *FakeProviderNode) GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error) {
	if n.MinerWorkerError == nil {
		return n.MinerAddr, nil
	}
	return address.Undef, n.MinerWorkerError
}

// GetDataCap gets the current data cap for addr
func (n *FakeProviderNode) GetDataCap(ctx context.Context, addr address.Address, tok shared.TipSetToken) (*verifreg.DataCap, error) {
	return n.DataCap, n.GetDataCapErr
}

// GetProofType returns the miner's proof type.
func (n *FakeProviderNode) GetProofType(ctx context.Context, addr address.Address, tok shared.TipSetToken) (abi.RegisteredSealProof, error) {
	return abi.RegisteredSealProof_StackedDrg2KiBV1, nil
}

var _ storagemarket.StorageProviderNode = (*FakeProviderNode)(nil)
