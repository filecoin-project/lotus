package storageadapter

// this file implements storagemarket.StorageClientNode

import (
	"bytes"
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	sharedtypes "github.com/filecoin-project/go-fil-markets/shared/types"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/impl/full"
)

type ClientNodeAdapter struct {
	full.StateAPI
	full.ChainAPI
	full.MpoolAPI

	sm *stmgr.StateManager
	cs *store.ChainStore
	fm *market.FundMgr
	ev *events.Events
}

type clientApi struct {
	full.ChainAPI
	full.StateAPI
}

func NewClientNodeAdapter(state full.StateAPI, chain full.ChainAPI, mpool full.MpoolAPI, sm *stmgr.StateManager, cs *store.ChainStore, fm *market.FundMgr) storagemarket.StorageClientNode {
	return &ClientNodeAdapter{
		StateAPI: state,
		ChainAPI: chain,
		MpoolAPI: mpool,

		sm: sm,
		cs: cs,
		fm: fm,
		ev: events.NewEvents(context.TODO(), &clientApi{chain, state}),
	}
}

func (n *ClientNodeAdapter) ListStorageProviders(ctx context.Context) ([]*storagemarket.StorageProviderInfo, error) {
	ts, err := n.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	addresses, err := n.StateListMiners(ctx, ts.Key())
	if err != nil {
		return nil, err
	}

	var out []*storagemarket.StorageProviderInfo

	for _, addr := range addresses {
		workerAddr, err := n.StateMinerWorker(ctx, addr, ts.Key())
		if err != nil {
			return nil, err
		}

		sectorSize, err := n.StateMinerSectorSize(ctx, addr, ts.Key())
		if err != nil {
			return nil, err
		}

		peerId, err := n.StateMinerPeerID(ctx, addr, ts.Key())
		if err != nil {
			return nil, err
		}
		storageProviderInfo := utils.NewStorageProviderInfo(addr, workerAddr, sectorSize, peerId)
		out = append(out, &storageProviderInfo)
	}

	return out, nil
}

func (n *ClientNodeAdapter) ListClientDeals(ctx context.Context, addr address.Address) ([]storagemarket.StorageDeal, error) {
	allDeals, err := n.StateMarketDeals(ctx, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	var out []storagemarket.StorageDeal

	for _, deal := range allDeals {
		storageDeal := utils.FromOnChainDeal(deal)
		if storageDeal.Client == addr {
			out = append(out, storageDeal)
		}
	}

	return out, nil
}

func (n *ClientNodeAdapter) MostRecentStateId(ctx context.Context) (storagemarket.StateKey, error) {
	return n.ChainHead(ctx)
}

// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
func (n *ClientNodeAdapter) AddFunds(ctx context.Context, addr address.Address, amount tokenamount.TokenAmount) error {
	// (Provider Node API)
	smsg, err := n.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     addr,
		Value:    utils.FromSharedTokenAmount(amount),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
		Method:   actors.SMAMethods.AddBalance,
	})
	if err != nil {
		return err
	}

	r, err := n.StateWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return err
	}

	if r.Receipt.ExitCode != 0 {
		return xerrors.Errorf("adding funds to storage miner market actor failed: exit %d", r.Receipt.ExitCode)
	}

	return nil
}

func (n *ClientNodeAdapter) EnsureFunds(ctx context.Context, addr address.Address, amount tokenamount.TokenAmount) error {
	return n.fm.EnsureAvailable(ctx, addr, utils.FromSharedTokenAmount(amount))
}

func (n *ClientNodeAdapter) GetBalance(ctx context.Context, addr address.Address) (storagemarket.Balance, error) {
	bal, err := n.StateMarketBalance(ctx, addr, types.EmptyTSK)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	return utils.ToSharedBalance(bal), nil
}

// ValidatePublishedDeal validates that the provided deal has appeared on chain and references the same ClientDeal
// returns the Deal id if there is no error
func (c *ClientNodeAdapter) ValidatePublishedDeal(ctx context.Context, deal storagemarket.ClientDeal) (uint64, error) {
	log.Infow("DEAL ACCEPTED!")

	pubmsg, err := c.cs.GetMessage(*deal.PublishMessage)
	if err != nil {
		return 0, xerrors.Errorf("getting deal pubsish message: %w", err)
	}

	pw, err := stmgr.GetMinerWorker(ctx, c.sm, nil, deal.Proposal.Provider)
	if err != nil {
		return 0, xerrors.Errorf("getting miner worker failed: %w", err)
	}

	if pubmsg.From != pw {
		return 0, xerrors.Errorf("deal wasn't published by storage provider: from=%s, provider=%s", pubmsg.From, deal.Proposal.Provider)
	}

	if pubmsg.To != actors.StorageMarketAddress {
		return 0, xerrors.Errorf("deal publish message wasn't set to StorageMarket actor (to=%s)", pubmsg.To)
	}

	if pubmsg.Method != actors.SMAMethods.PublishStorageDeals {
		return 0, xerrors.Errorf("deal publish message called incorrect method (method=%s)", pubmsg.Method)
	}

	var params actors.PublishStorageDealsParams
	if err := params.UnmarshalCBOR(bytes.NewReader(pubmsg.Params)); err != nil {
		return 0, err
	}

	dealIdx := -1
	for i, storageDeal := range params.Deals {
		// TODO: make it less hacky
		sd := storageDeal
		eq, err := cborutil.Equals(&deal.Proposal, &sd)
		if err != nil {
			return 0, err
		}
		if eq {
			dealIdx = i
			break
		}
	}

	if dealIdx == -1 {
		return 0, xerrors.Errorf("deal publish didn't contain our deal (message cid: %s)", deal.PublishMessage)
	}

	// TODO: timeout
	_, ret, err := c.sm.WaitForMessage(ctx, *deal.PublishMessage)
	if err != nil {
		return 0, xerrors.Errorf("waiting for deal publish message: %w", err)
	}
	if ret.ExitCode != 0 {
		return 0, xerrors.Errorf("deal publish failed: exit=%d", ret.ExitCode)
	}

	var res actors.PublishStorageDealResponse
	if err := res.UnmarshalCBOR(bytes.NewReader(ret.Return)); err != nil {
		return 0, err
	}

	return res.DealIDs[dealIdx], nil
}

func (c *ClientNodeAdapter) OnDealSectorCommitted(ctx context.Context, provider address.Address, dealId uint64, cb storagemarket.DealSectorCommittedCallback) error {
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		sd, err := stmgr.GetStorageDeal(ctx, c.StateManager, dealId, ts)
		if err != nil {
			// TODO: This may be fine for some errors
			return false, false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		if sd.ActivationEpoch > 0 {
			cb(nil)
			return true, false, nil
		}

		return false, true, nil
	}

	called := func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH uint64) (more bool, err error) {
		defer func() {
			if err != nil {
				cb(xerrors.Errorf("handling applied event: %w", err))
			}
		}()

		if msg == nil {
			log.Error("timed out waiting for deal activation... what now?")
			return false, nil
		}

		sd, err := stmgr.GetStorageDeal(ctx, c.StateManager, dealId, ts)
		if err != nil {
			return false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		if sd.ActivationEpoch == 0 {
			return false, xerrors.Errorf("deal wasn't active: deal=%d, parentState=%s, h=%d", dealId, ts.ParentState(), ts.Height())
		}

		log.Infof("Storage deal %d activated at epoch %d", dealId, sd.ActivationEpoch)

		cb(nil)

		return false, nil
	}

	revert := func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("deal activation reverted; TODO: actually handle this!")
		// TODO: Just go back to DealSealing?
		return nil
	}

	matchEvent := func(msg *types.Message) (bool, error) {
		if msg.To != provider {
			return false, nil
		}

		if msg.Method != actors.MAMethods.ProveCommitSector {
			return false, nil
		}

		var params actors.SectorProveCommitInfo
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return false, err
		}

		var found bool
		for _, dealID := range params.DealIDs {
			if dealID == dealId {
				found = true
				break
			}
		}

		return found, nil
	}

	if err := c.ev.Called(checkFunc, called, revert, 3, build.SealRandomnessLookbackLimit, matchEvent); err != nil {
		return xerrors.Errorf("failed to set up called handler")
	}

	return nil
}

func (n *ClientNodeAdapter) SignProposal(ctx context.Context, signer address.Address, proposal *storagemarket.StorageDealProposal) error {
	localProposal, err := utils.FromSharedStorageDealProposal(proposal)
	if err != nil {
		return err
	}
	err = api.SignWith(ctx, n.Wallet.Sign, signer, localProposal)
	if err != nil {
		return err
	}
	signature, err := utils.ToSharedSignature(localProposal.ProposerSignature)
	if err != nil {
		return err
	}
	proposal.ProposerSignature = signature
	return nil
}

func (n *ClientNodeAdapter) GetDefaultWalletAddress(ctx context.Context) (address.Address, error) {
	addr, err := n.Wallet.GetDefault()
	return addr, err
}

func (n *ClientNodeAdapter) ValidateAskSignature(ask *sharedtypes.SignedStorageAsk) error {
	tss := n.cs.GetHeaviestTipSet().ParentState()

	w, err := stmgr.GetMinerWorkerRaw(context.TODO(), n.StateManager, tss, ask.Ask.Miner)
	if err != nil {
		return xerrors.Errorf("failed to get worker for miner in ask", err)
	}

	sigb, err := cborutil.Dump(ask.Ask)
	if err != nil {
		return xerrors.Errorf("failed to re-serialize ask")
	}

	return ask.Signature.Verify(w, sigb)
}

var _ storagemarket.StorageClientNode = &ClientNodeAdapter{}
