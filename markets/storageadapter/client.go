package storageadapter

// this file implements storagemarket.StorageClientNode

import (
	"bytes"
	"context"

	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	marketactor "github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type ClientNodeAdapter struct {
	*clientApi

	fundmgr   *market.FundManager
	ev        *events.Events
	dsMatcher *dealStateMatcher
	scMgr     *SectorCommittedManager
}

type clientApi struct {
	full.ChainAPI
	full.StateAPI
	full.MpoolAPI
}

func NewClientNodeAdapter(mctx helpers.MetricsCtx, lc fx.Lifecycle, stateapi full.StateAPI, chain full.ChainAPI, mpool full.MpoolAPI, fundmgr *market.FundManager) (storagemarket.StorageClientNode, error) {
	capi := &clientApi{chain, stateapi, mpool}
	ctx := helpers.LifecycleCtx(mctx, lc)

	ev, err := events.NewEvents(ctx, capi)
	if err != nil {
		return nil, err
	}
	a := &ClientNodeAdapter{
		clientApi: capi,

		fundmgr:   fundmgr,
		ev:        ev,
		dsMatcher: newDealStateMatcher(state.NewStatePredicates(state.WrapFastAPI(capi))),
	}
	a.scMgr = NewSectorCommittedManager(ev, a, &apiWrapper{api: capi})
	return a, nil
}

func (c *ClientNodeAdapter) ListStorageProviders(ctx context.Context, encodedTs shared.TipSetToken) ([]*storagemarket.StorageProviderInfo, error) {
	tsk, err := types.TipSetKeyFromBytes(encodedTs)
	if err != nil {
		return nil, err
	}

	addresses, err := c.StateListMiners(ctx, tsk)
	if err != nil {
		return nil, err
	}

	var out []*storagemarket.StorageProviderInfo

	for _, addr := range addresses {
		mi, err := c.GetMinerInfo(ctx, addr, encodedTs)
		if err != nil {
			return nil, err
		}

		out = append(out, mi)
	}

	return out, nil
}

func (c *ClientNodeAdapter) VerifySignature(ctx context.Context, sig crypto.Signature, addr address.Address, input []byte, encodedTs shared.TipSetToken) (bool, error) {
	addr, err := c.StateAccountKey(ctx, addr, types.EmptyTSK)
	if err != nil {
		return false, err
	}

	err = sigs.Verify(&sig, addr, input)
	return err == nil, err
}

// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
func (c *ClientNodeAdapter) AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) (cid.Cid, error) {
	// (Provider Node API)
	smsg, err := c.MpoolPushMessage(ctx, &types.Message{
		To:     marketactor.Address,
		From:   addr,
		Value:  amount,
		Method: builtin6.MethodsMarket.AddBalance,
	}, nil)
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}

func (c *ClientNodeAdapter) ReserveFunds(ctx context.Context, wallet, addr address.Address, amt abi.TokenAmount) (cid.Cid, error) {
	return c.fundmgr.Reserve(ctx, wallet, addr, amt)
}

func (c *ClientNodeAdapter) ReleaseFunds(ctx context.Context, addr address.Address, amt abi.TokenAmount) error {
	return c.fundmgr.Release(addr, amt)
}

func (c *ClientNodeAdapter) GetBalance(ctx context.Context, addr address.Address, encodedTs shared.TipSetToken) (storagemarket.Balance, error) {
	tsk, err := types.TipSetKeyFromBytes(encodedTs)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	bal, err := c.StateMarketBalance(ctx, addr, tsk)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	return utils.ToSharedBalance(bal), nil
}

// ValidatePublishedDeal validates that the provided deal has appeared on chain and references the same ClientDeal
// returns the Deal id if there is no error
// TODO: Don't return deal ID
func (c *ClientNodeAdapter) ValidatePublishedDeal(ctx context.Context, deal storagemarket.ClientDeal) (abi.DealID, error) {
	log.Infow("DEAL ACCEPTED!")

	pubmsg, err := c.ChainGetMessage(ctx, *deal.PublishMessage)
	if err != nil {
		return 0, xerrors.Errorf("getting deal publish message: %w", err)
	}

	mi, err := c.StateMinerInfo(ctx, deal.Proposal.Provider, types.EmptyTSK)
	if err != nil {
		return 0, xerrors.Errorf("getting miner worker failed: %w", err)
	}

	fromid, err := c.StateLookupID(ctx, pubmsg.From, types.EmptyTSK)
	if err != nil {
		return 0, xerrors.Errorf("failed to resolve from msg ID addr: %w", err)
	}

	var pubOk bool
	pubAddrs := append([]address.Address{mi.Worker, mi.Owner}, mi.ControlAddresses...)
	for _, a := range pubAddrs {
		if fromid == a {
			pubOk = true
			break
		}
	}

	if !pubOk {
		return 0, xerrors.Errorf("deal wasn't published by storage provider: from=%s, provider=%s,%+v", pubmsg.From, deal.Proposal.Provider, pubAddrs)
	}

	if pubmsg.To != marketactor.Address {
		return 0, xerrors.Errorf("deal publish message wasn't set to StorageMarket actor (to=%s)", pubmsg.To)
	}

	if pubmsg.Method != builtin6.MethodsMarket.PublishStorageDeals {
		return 0, xerrors.Errorf("deal publish message called incorrect method (method=%s)", pubmsg.Method)
	}

	var params marketactor.PublishStorageDealsParams
	if err := params.UnmarshalCBOR(bytes.NewReader(pubmsg.Params)); err != nil {
		return 0, err
	}

	dealIdx := -1
	for i, storageDeal := range params.Deals {
		// TODO: make it less hacky
		sd := storageDeal
		eq, err := cborutil.Equals(&deal.ClientDealProposal, &sd)
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
	ret, err := c.StateWaitMsg(ctx, *deal.PublishMessage, build.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		return 0, xerrors.Errorf("waiting for deal publish message: %w", err)
	}
	if ret.Receipt.ExitCode != 0 {
		return 0, xerrors.Errorf("deal publish failed: exit=%d", ret.Receipt.ExitCode)
	}

	nv, err := c.StateNetworkVersion(ctx, ret.TipSet)
	if err != nil {
		return 0, xerrors.Errorf("getting network version: %w", err)
	}

	res, err := marketactor.DecodePublishStorageDealsReturn(ret.Receipt.Return, nv)
	if err != nil {
		return 0, xerrors.Errorf("decoding deal publish return: %w", err)
	}

	dealIDs, err := res.DealIDs()
	if err != nil {
		return 0, xerrors.Errorf("getting dealIDs: %w", err)
	}

	if dealIdx >= len(dealIDs) {
		return 0, xerrors.Errorf(
			"deal index %d out of bounds of deals (len %d) in publish deals message %s",
			dealIdx, len(dealIDs), pubmsg.Cid())
	}

	valid, err := res.IsDealValid(uint64(dealIdx))
	if err != nil {
		return 0, xerrors.Errorf("determining deal validity: %w", err)
	}

	if !valid {
		return 0, xerrors.New("deal was invalid at publication")
	}

	return dealIDs[dealIdx], nil
}

var clientOverestimation = struct {
	numerator   int64
	denominator int64
}{
	numerator:   12,
	denominator: 10,
}

func (c *ClientNodeAdapter) DealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, isVerified bool) (abi.TokenAmount, abi.TokenAmount, error) {
	bounds, err := c.StateDealProviderCollateralBounds(ctx, size, isVerified, types.EmptyTSK)
	if err != nil {
		return abi.TokenAmount{}, abi.TokenAmount{}, err
	}

	min := big.Mul(bounds.Min, big.NewInt(clientOverestimation.numerator))
	min = big.Div(min, big.NewInt(clientOverestimation.denominator))
	return min, bounds.Max, nil
}

// TODO: Remove dealID parameter, change publishCid to be cid.Cid (instead of pointer)
func (c *ClientNodeAdapter) OnDealSectorPreCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, proposal market0.DealProposal, publishCid *cid.Cid, cb storagemarket.DealSectorPreCommittedCallback) error {
	return c.scMgr.OnDealSectorPreCommitted(ctx, provider, marketactor.DealProposal(proposal), *publishCid, cb)
}

// TODO: Remove dealID parameter, change publishCid to be cid.Cid (instead of pointer)
func (c *ClientNodeAdapter) OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, sectorNumber abi.SectorNumber, proposal market0.DealProposal, publishCid *cid.Cid, cb storagemarket.DealSectorCommittedCallback) error {
	return c.scMgr.OnDealSectorCommitted(ctx, provider, sectorNumber, marketactor.DealProposal(proposal), *publishCid, cb)
}

// TODO: Replace dealID parameter with DealProposal
func (c *ClientNodeAdapter) OnDealExpiredOrSlashed(ctx context.Context, dealID abi.DealID, onDealExpired storagemarket.DealExpiredCallback, onDealSlashed storagemarket.DealSlashedCallback) error {
	head, err := c.ChainHead(ctx)
	if err != nil {
		return xerrors.Errorf("client: failed to get chain head: %w", err)
	}

	sd, err := c.StateMarketStorageDeal(ctx, dealID, head.Key())
	if err != nil {
		return xerrors.Errorf("client: failed to look up deal %d on chain: %w", dealID, err)
	}

	// Called immediately to check if the deal has already expired or been slashed
	checkFunc := func(ctx context.Context, ts *types.TipSet) (done bool, more bool, err error) {
		if ts == nil {
			// keep listening for events
			return false, true, nil
		}

		// Check if the deal has already expired
		if sd.Proposal.EndEpoch <= ts.Height() {
			onDealExpired(nil)
			return true, false, nil
		}

		// If there is no deal assume it's already been slashed
		if sd.State.SectorStartEpoch < 0 {
			onDealSlashed(ts.Height(), nil)
			return true, false, nil
		}

		// No events have occurred yet, so return
		// done: false, more: true (keep listening for events)
		return false, true, nil
	}

	// Called when there was a match against the state change we're looking for
	// and the chain has advanced to the confidence height
	stateChanged := func(ts *types.TipSet, ts2 *types.TipSet, states events.StateChange, h abi.ChainEpoch) (more bool, err error) {
		// Check if the deal has already expired
		if ts2 == nil || sd.Proposal.EndEpoch <= ts2.Height() {
			onDealExpired(nil)
			return false, nil
		}

		// Timeout waiting for state change
		if states == nil {
			log.Error("timed out waiting for deal expiry")
			return false, nil
		}

		changedDeals, ok := states.(state.ChangedDeals)
		if !ok {
			panic("Expected state.ChangedDeals")
		}

		deal, ok := changedDeals[dealID]
		if !ok {
			// No change to deal
			return true, nil
		}

		// Deal was slashed
		if deal.To == nil {
			onDealSlashed(ts2.Height(), nil)
			return false, nil
		}

		return true, nil
	}

	// Called when there was a chain reorg and the state change was reverted
	revert := func(ctx context.Context, ts *types.TipSet) error {
		// TODO: Is it ok to just ignore this?
		log.Warn("deal state reverted; TODO: actually handle this!")
		return nil
	}

	// Watch for state changes to the deal
	match := c.dsMatcher.matcher(ctx, dealID)

	// Wait until after the end epoch for the deal and then timeout
	timeout := (sd.Proposal.EndEpoch - head.Height()) + 1
	if err := c.ev.StateChanged(checkFunc, stateChanged, revert, int(build.MessageConfidence)+1, timeout, match); err != nil {
		return xerrors.Errorf("failed to set up state changed handler: %w", err)
	}

	return nil
}

func (c *ClientNodeAdapter) SignProposal(ctx context.Context, signer address.Address, proposal market0.DealProposal) (*marketactor.ClientDealProposal, error) {
	// TODO: output spec signed proposal
	buf, err := cborutil.Dump(&proposal)
	if err != nil {
		return nil, err
	}

	signer, err = c.StateAccountKey(ctx, signer, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	sig, err := c.Wallet.WalletSign(ctx, signer, buf, api.MsgMeta{
		Type: api.MTDealProposal,
	})
	if err != nil {
		return nil, err
	}

	return &marketactor.ClientDealProposal{
		Proposal:        proposal,
		ClientSignature: *sig,
	}, nil
}

func (c *ClientNodeAdapter) GetDefaultWalletAddress(ctx context.Context) (address.Address, error) {
	addr, err := c.DefWallet.GetDefault()
	return addr, err
}

func (c *ClientNodeAdapter) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	head, err := c.ChainHead(ctx)
	if err != nil {
		return nil, 0, err
	}

	return head.Key().Bytes(), head.Height(), nil
}

func (c *ClientNodeAdapter) WaitForMessage(ctx context.Context, mcid cid.Cid, cb func(code exitcode.ExitCode, bytes []byte, finalCid cid.Cid, err error) error) error {
	receipt, err := c.StateWaitMsg(ctx, mcid, build.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		return cb(0, nil, cid.Undef, err)
	}
	return cb(receipt.Receipt.ExitCode, receipt.Receipt.Return, receipt.Message, nil)
}

func (c *ClientNodeAdapter) GetMinerInfo(ctx context.Context, addr address.Address, encodedTs shared.TipSetToken) (*storagemarket.StorageProviderInfo, error) {
	tsk, err := types.TipSetKeyFromBytes(encodedTs)
	if err != nil {
		return nil, err
	}
	mi, err := c.StateMinerInfo(ctx, addr, tsk)
	if err != nil {
		return nil, err
	}

	out := utils.NewStorageProviderInfo(addr, mi.Worker, mi.SectorSize, *mi.PeerId, mi.Multiaddrs)
	return &out, nil
}

func (c *ClientNodeAdapter) SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error) {
	signer, err := c.StateAccountKey(ctx, signer, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	localSignature, err := c.Wallet.WalletSign(ctx, signer, b, api.MsgMeta{
		Type: api.MTUnknown, // TODO: pass type here
	})
	if err != nil {
		return nil, err
	}
	return localSignature, nil
}

var _ storagemarket.StorageClientNode = &ClientNodeAdapter{}
