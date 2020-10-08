package storageadapter

// this file implements storagemarket.StorageProviderNode

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

var addPieceRetryWait = 5 * time.Minute
var addPieceRetryTimeout = 6 * time.Hour
var log = logging.Logger("storageadapter")

type ProviderNodeAdapter struct {
	api.FullNode

	// this goes away with the data transfer module
	dag dtypes.StagingDAG

	secb *sectorblocks.SectorBlocks
	ev   *events.Events

	publishSpec, addBalanceSpec *api.MessageSendSpec
}

func NewProviderNodeAdapter(fc *config.MinerFeeConfig) func(dag dtypes.StagingDAG, secb *sectorblocks.SectorBlocks, full api.FullNode) storagemarket.StorageProviderNode {
	return func(dag dtypes.StagingDAG, secb *sectorblocks.SectorBlocks, full api.FullNode) storagemarket.StorageProviderNode {
		na := &ProviderNodeAdapter{
			FullNode: full,

			dag:  dag,
			secb: secb,
			ev:   events.NewEvents(context.TODO(), full),
		}
		if fc != nil {
			na.publishSpec = &api.MessageSendSpec{MaxFee: abi.TokenAmount(fc.MaxPublishDealsFee)}
			na.addBalanceSpec = &api.MessageSendSpec{MaxFee: abi.TokenAmount(fc.MaxMarketBalanceAddFee)}
		}
		return na
	}
}

func (n *ProviderNodeAdapter) PublishDeals(ctx context.Context, deal storagemarket.MinerDeal) (cid.Cid, error) {
	log.Info("publishing deal")

	mi, err := n.StateMinerInfo(ctx, deal.Proposal.Provider, types.EmptyTSK)
	if err != nil {
		return cid.Undef, err
	}

	params, err := actors.SerializeParams(&market0.PublishStorageDealsParams{
		Deals: []market0.ClientDealProposal{deal.ClientDealProposal},
	})

	if err != nil {
		return cid.Undef, xerrors.Errorf("serializing PublishStorageDeals params failed: %w", err)
	}

	// TODO: We may want this to happen after fetching data
	smsg, err := n.MpoolPushMessage(ctx, &types.Message{
		To:     market.Address,
		From:   mi.Worker,
		Value:  types.NewInt(0),
		Method: builtin0.MethodsMarket.PublishStorageDeals,
		Params: params,
	}, n.publishSpec)
	if err != nil {
		return cid.Undef, err
	}
	return smsg.Cid(), nil
}

func (n *ProviderNodeAdapter) OnDealComplete(ctx context.Context, deal storagemarket.MinerDeal, pieceSize abi.UnpaddedPieceSize, pieceData io.Reader) (*storagemarket.PackingResult, error) {
	if deal.PublishCid == nil {
		return nil, xerrors.Errorf("deal.PublishCid can't be nil")
	}

	sdInfo := sealing.DealInfo{
		DealID:     deal.DealID,
		PublishCid: deal.PublishCid,
		DealSchedule: sealing.DealSchedule{
			StartEpoch: deal.ClientDealProposal.Proposal.StartEpoch,
			EndEpoch:   deal.ClientDealProposal.Proposal.EndEpoch,
		},
		KeepUnsealed: deal.FastRetrieval,
	}

	p, offset, err := n.secb.AddPiece(ctx, pieceSize, pieceData, sdInfo)
	curTime := time.Now()
	for time.Since(curTime) < addPieceRetryTimeout {
		if !xerrors.Is(err, sealing.ErrTooManySectorsSealing) {
			if err != nil {
				log.Errorf("failed to addPiece for deal %d, err: %w", deal.DealID, err)
			}
			break
		}
		select {
		case <-time.After(addPieceRetryWait):
			p, offset, err = n.secb.AddPiece(ctx, pieceSize, pieceData, sdInfo)
		case <-ctx.Done():
			return nil, xerrors.New("context expired while waiting to retry AddPiece")
		}
	}

	if err != nil {
		return nil, xerrors.Errorf("AddPiece failed: %s", err)
	}
	log.Warnf("New Deal: deal %d", deal.DealID)

	return &storagemarket.PackingResult{
		SectorNumber: p,
		Offset:       offset,
		Size:         pieceSize.Padded(),
	}, nil
}

func (n *ProviderNodeAdapter) VerifySignature(ctx context.Context, sig crypto.Signature, addr address.Address, input []byte, encodedTs shared.TipSetToken) (bool, error) {
	addr, err := n.StateAccountKey(ctx, addr, types.EmptyTSK)
	if err != nil {
		return false, err
	}

	err = sigs.Verify(&sig, addr, input)
	return err == nil, err
}

func (n *ProviderNodeAdapter) GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return address.Undef, err
	}

	mi, err := n.StateMinerInfo(ctx, miner, tsk)
	if err != nil {
		return address.Address{}, err
	}
	return mi.Worker, nil
}

func (n *ProviderNodeAdapter) SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error) {
	signer, err := n.StateAccountKey(ctx, signer, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	localSignature, err := n.WalletSign(ctx, signer, b)
	if err != nil {
		return nil, err
	}
	return localSignature, nil
}

func (n *ProviderNodeAdapter) EnsureFunds(ctx context.Context, addr, wallet address.Address, amt abi.TokenAmount, encodedTs shared.TipSetToken) (cid.Cid, error) {
	return n.MarketEnsureAvailable(ctx, addr, wallet, amt)
}

// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
func (n *ProviderNodeAdapter) AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) (cid.Cid, error) {
	// (Provider Node API)
	smsg, err := n.MpoolPushMessage(ctx, &types.Message{
		To:     market.Address,
		From:   addr,
		Value:  amount,
		Method: builtin0.MethodsMarket.AddBalance,
	}, n.addBalanceSpec)
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}

func (n *ProviderNodeAdapter) GetBalance(ctx context.Context, addr address.Address, encodedTs shared.TipSetToken) (storagemarket.Balance, error) {
	tsk, err := types.TipSetKeyFromBytes(encodedTs)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	bal, err := n.StateMarketBalance(ctx, addr, tsk)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	return utils.ToSharedBalance(bal), nil
}

// TODO: why doesnt this method take in a sector ID?
func (n *ProviderNodeAdapter) LocatePieceForDealWithinSector(ctx context.Context, dealID abi.DealID, encodedTs shared.TipSetToken) (sectorID abi.SectorNumber, offset abi.PaddedPieceSize, length abi.PaddedPieceSize, err error) {
	refs, err := n.secb.GetRefs(dealID)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(refs) == 0 {
		return 0, 0, 0, xerrors.New("no sector information for deal ID")
	}

	// TODO: better strategy (e.g. look for already unsealed)
	var best api.SealedRef
	var bestSi sealing.SectorInfo
	for _, r := range refs {
		si, err := n.secb.Miner.GetSectorInfo(r.SectorID)
		if err != nil {
			return 0, 0, 0, xerrors.Errorf("getting sector info: %w", err)
		}
		if si.State == sealing.Proving {
			best = r
			bestSi = si
			break
		}
	}
	if bestSi.State == sealing.UndefinedSectorState {
		return 0, 0, 0, xerrors.New("no sealed sector found")
	}
	return best.SectorID, best.Offset, best.Size.Padded(), nil
}

func (n *ProviderNodeAdapter) DealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, isVerified bool) (abi.TokenAmount, abi.TokenAmount, error) {
	bounds, err := n.StateDealProviderCollateralBounds(ctx, size, isVerified, types.EmptyTSK)
	if err != nil {
		return abi.TokenAmount{}, abi.TokenAmount{}, err
	}

	return bounds.Min, bounds.Max, nil
}

func (n *ProviderNodeAdapter) OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID abi.DealID, cb storagemarket.DealSectorCommittedCallback) error {
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		sd, err := n.StateMarketStorageDeal(ctx, dealID, ts.Key())

		if err != nil {
			// TODO: This may be fine for some errors
			return false, false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		if sd.State.SectorStartEpoch > 0 {
			cb(nil)
			return true, false, nil
		}

		return false, true, nil
	}

	called := func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error) {
		defer func() {
			if err != nil {
				cb(xerrors.Errorf("handling applied event: %w", err))
			}
		}()

		if msg == nil {
			log.Error("timed out waiting for deal activation... what now?")
			return false, nil
		}

		sd, err := n.StateMarketStorageDeal(ctx, dealID, ts.Key())
		if err != nil {
			return false, xerrors.Errorf("failed to look up deal on chain: %w", err)
		}

		if sd.State.SectorStartEpoch < 1 {
			return false, xerrors.Errorf("deal wasn't active: deal=%d, parentState=%s, h=%d", dealID, ts.ParentState(), ts.Height())
		}

		log.Infof("Storage deal %d activated at epoch %d", dealID, sd.State.SectorStartEpoch)

		cb(nil)

		return false, nil
	}

	revert := func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("deal activation reverted; TODO: actually handle this!")
		// TODO: Just go back to DealSealing?
		return nil
	}

	var sectorNumber abi.SectorNumber
	var sectorFound bool

	matchEvent := func(msg *types.Message) (matchOnce bool, matched bool, err error) {
		if msg.To != provider {
			return true, false, nil
		}

		switch msg.Method {
		case builtin0.MethodsMiner.PreCommitSector:
			var params miner.SectorPreCommitInfo
			if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
				return true, false, xerrors.Errorf("unmarshal pre commit: %w", err)
			}

			for _, did := range params.DealIDs {
				if did == dealID {
					sectorNumber = params.SectorNumber
					sectorFound = true
					return true, false, nil
				}
			}

			return true, false, nil
		case builtin0.MethodsMiner.ProveCommitSector:
			var params miner.ProveCommitSectorParams
			if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
				return true, false, xerrors.Errorf("failed to unmarshal prove commit sector params: %w", err)
			}

			if !sectorFound {
				return true, false, nil
			}

			if params.SectorNumber != sectorNumber {
				return true, false, nil
			}

			return false, true, nil
		default:
			return true, false, nil
		}

	}

	if err := n.ev.Called(checkFunc, called, revert, int(build.MessageConfidence+1), events.NoTimeout, matchEvent); err != nil {
		return xerrors.Errorf("failed to set up called handler: %w", err)
	}

	return nil
}

func (n *ProviderNodeAdapter) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	head, err := n.ChainHead(ctx)
	if err != nil {
		return nil, 0, err
	}

	return head.Key().Bytes(), head.Height(), nil
}

func (n *ProviderNodeAdapter) WaitForMessage(ctx context.Context, mcid cid.Cid, cb func(code exitcode.ExitCode, bytes []byte, finalCid cid.Cid, err error) error) error {
	receipt, err := n.StateWaitMsg(ctx, mcid, 2*build.MessageConfidence)
	if err != nil {
		return cb(0, nil, cid.Undef, err)
	}
	return cb(receipt.Receipt.ExitCode, receipt.Receipt.Return, receipt.Message, nil)
}

func (n *ProviderNodeAdapter) GetDataCap(ctx context.Context, addr address.Address, encodedTs shared.TipSetToken) (*abi.StoragePower, error) {
	tsk, err := types.TipSetKeyFromBytes(encodedTs)
	if err != nil {
		return nil, err
	}

	sp, err := n.StateVerifiedClientStatus(ctx, addr, tsk)
	return sp, err
}

func (n *ProviderNodeAdapter) OnDealExpiredOrSlashed(ctx context.Context, dealID abi.DealID, onDealExpired storagemarket.DealExpiredCallback, onDealSlashed storagemarket.DealSlashedCallback) error {
	head, err := n.ChainHead(ctx)
	if err != nil {
		return xerrors.Errorf("client: failed to get chain head: %w", err)
	}

	sd, err := n.StateMarketStorageDeal(ctx, dealID, head.Key())
	if err != nil {
		return xerrors.Errorf("client: failed to look up deal %d on chain: %w", dealID, err)
	}

	// Called immediately to check if the deal has already expired or been slashed
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
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
		if sd.Proposal.EndEpoch <= ts2.Height() {
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
	preds := state.NewStatePredicates(n)
	dealDiff := preds.OnStorageMarketActorChanged(
		preds.OnDealStateChanged(
			preds.DealStateChangedForIDs([]abi.DealID{dealID})))
	match := func(oldTs, newTs *types.TipSet) (bool, events.StateChange, error) {
		return dealDiff(ctx, oldTs.Key(), newTs.Key())
	}

	// Wait until after the end epoch for the deal and then timeout
	timeout := (sd.Proposal.EndEpoch - head.Height()) + 1
	if err := n.ev.StateChanged(checkFunc, stateChanged, revert, int(build.MessageConfidence)+1, timeout, match); err != nil {
		return xerrors.Errorf("failed to set up state changed handler: %w", err)
	}

	return nil
}

var _ storagemarket.StorageProviderNode = &ProviderNodeAdapter{}
