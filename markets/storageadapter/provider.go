package storageadapter

// this file implements storagemarket.StorageProviderNode

import (
	"bytes"
	"context"
	"io"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/sealing"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

var log = logging.Logger("provideradapter")

type ProviderNodeAdapter struct {
	api.FullNode

	// this goes away with the data transfer module
	dag dtypes.StagingDAG

	secb *sectorblocks.SectorBlocks
	ev   *events.Events
}

func NewProviderNodeAdapter(dag dtypes.StagingDAG, secb *sectorblocks.SectorBlocks, full api.FullNode) storagemarket.StorageProviderNode {
	return &ProviderNodeAdapter{
		FullNode: full,
		dag:      dag,
		secb:     secb,
		ev:       events.NewEvents(context.TODO(), full),
	}
}

func (n *ProviderNodeAdapter) PublishDeals(ctx context.Context, deal storagemarket.MinerDeal) (storagemarket.DealID, cid.Cid, error) {
	log.Info("publishing deal")

	worker, err := n.StateMinerWorker(ctx, deal.Proposal.Provider, types.EmptyTSK)
	if err != nil {
		return 0, cid.Undef, err
	}

	params, err := actors.SerializeParams(&market.PublishStorageDealsParams{
		Deals: []market.ClientDealProposal{deal.ClientDealProposal},
	})

	if err != nil {
		return 0, cid.Undef, xerrors.Errorf("serializing PublishStorageDeals params failed: ", err)
	}

	// TODO: We may want this to happen after fetching data
	smsg, err := n.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     worker,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
		Method:   builtin.MethodsMarket.PublishStorageDeals,
		Params:   params,
	})
	if err != nil {
		return 0, cid.Undef, err
	}
	r, err := n.StateWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return 0, cid.Undef, err
	}
	if r.Receipt.ExitCode != 0 {
		return 0, cid.Undef, xerrors.Errorf("publishing deal failed: exit %d", r.Receipt.ExitCode)
	}
	var resp market.PublishStorageDealsReturn
	if err := resp.UnmarshalCBOR(bytes.NewReader(r.Receipt.Return)); err != nil {
		return 0, cid.Undef, err
	}
	if len(resp.IDs) != 1 {
		return 0, cid.Undef, xerrors.Errorf("got unexpected number of DealIDs from")
	}

	// TODO: bad types here
	return storagemarket.DealID(resp.IDs[0]), smsg.Cid(), nil
}

func (n *ProviderNodeAdapter) OnDealComplete(ctx context.Context, deal storagemarket.MinerDeal, pieceSize abi.UnpaddedPieceSize, pieceData io.Reader) error {
	_, err := n.secb.AddPiece(ctx, abi.UnpaddedPieceSize(pieceSize), pieceData, abi.DealID(deal.DealID))
	if err != nil {
		return xerrors.Errorf("AddPiece failed: %s", err)
	}
	log.Warnf("New Deal: deal %d", deal.DealID)

	return nil
}

func (n *ProviderNodeAdapter) VerifySignature(sig crypto.Signature, addr address.Address, input []byte) bool {
	log.Warn("stub VerifySignature")
	return true
}

func (n *ProviderNodeAdapter) ListProviderDeals(ctx context.Context, addr address.Address) ([]storagemarket.StorageDeal, error) {
	allDeals, err := n.StateMarketDeals(ctx, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	var out []storagemarket.StorageDeal

	for _, deal := range allDeals {
		sharedDeal := utils.FromOnChainDeal(deal.Proposal, deal.State)
		if sharedDeal.Provider == addr {
			out = append(out, sharedDeal)
		}
	}

	return out, nil
}

func (n *ProviderNodeAdapter) GetMinerWorker(ctx context.Context, miner address.Address) (address.Address, error) {
	addr, err := n.StateMinerWorker(ctx, miner, types.EmptyTSK)
	return addr, err
}

func (n *ProviderNodeAdapter) SignBytes(ctx context.Context, signer address.Address, b []byte) (*crypto.Signature, error) {
	localSignature, err := n.WalletSign(ctx, signer, b)
	if err != nil {
		return nil, err
	}
	return localSignature, nil
}

func (n *ProviderNodeAdapter) EnsureFunds(ctx context.Context, addr address.Address, amt abi.TokenAmount) error {
	return n.MarketEnsureAvailable(ctx, addr, amt)
}

func (n *ProviderNodeAdapter) MostRecentStateId(ctx context.Context) (storagemarket.StateKey, error) {
	return n.ChainHead(ctx)
}

// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
func (n *ProviderNodeAdapter) AddFunds(ctx context.Context, addr address.Address, amount abi.TokenAmount) error {
	// (Provider Node API)
	smsg, err := n.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     addr,
		Value:    amount,
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
		Method:   builtin.MethodsMarket.AddBalance,
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

func (n *ProviderNodeAdapter) GetBalance(ctx context.Context, addr address.Address) (storagemarket.Balance, error) {
	bal, err := n.StateMarketBalance(ctx, addr, types.EmptyTSK)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	return utils.ToSharedBalance(bal), nil
}

func (n *ProviderNodeAdapter) LocatePieceForDealWithinSector(ctx context.Context, dealID uint64) (sectorID uint64, offset uint64, length uint64, err error) {

	refs, err := n.secb.GetRefs(abi.DealID(dealID))
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
		if si.State == api.Proving {
			best = r
			bestSi = si
			break
		}
	}
	if bestSi.State == api.UndefinedSectorState {
		return 0, 0, 0, xerrors.New("no sealed sector found")
	}
	return uint64(best.SectorID), best.Offset, uint64(best.Size), nil
}

func (n *ProviderNodeAdapter) OnDealSectorCommitted(ctx context.Context, provider address.Address, dealID uint64, cb storagemarket.DealSectorCommittedCallback) error {
	checkFunc := func(ts *types.TipSet) (done bool, more bool, err error) {
		sd, err := n.StateMarketStorageDeal(ctx, abi.DealID(dealID), ts.Key())

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

		sd, err := n.StateMarketStorageDeal(ctx, abi.DealID(dealID), ts.Key())
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

	matchEvent := func(msg *types.Message) (bool, error) {
		if msg.To != provider {
			return false, nil
		}

		if msg.Method != builtin.MethodsMiner.ProveCommitSector {
			return false, nil
		}

		var params miner.SectorPreCommitInfo
		if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
			return false, err
		}

		var found bool
		for _, did := range params.DealIDs {
			if did == abi.DealID(dealID) {
				found = true
				break
			}
		}

		return found, nil
	}

	if err := n.ev.Called(checkFunc, called, revert, 3, build.SealRandomnessLookbackLimit, matchEvent); err != nil {
		return xerrors.Errorf("failed to set up called handler: %w", err)
	}

	return nil
}

var _ storagemarket.StorageProviderNode = &ProviderNodeAdapter{}
