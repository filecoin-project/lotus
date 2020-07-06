package storageadapter

// this file implements storagemarket.StorageProviderNode

import (
	"bytes"
	"context"
	"io"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
	sealing "github.com/filecoin-project/storage-fsm"
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

func (n *ProviderNodeAdapter) PublishDeals(ctx context.Context, deal storagemarket.MinerDeal) (cid.Cid, error) {
	log.Info("publishing deal")

	mi, err := n.StateMinerInfo(ctx, deal.Proposal.Provider, types.EmptyTSK)
	if err != nil {
		return cid.Undef, err
	}

	params, err := actors.SerializeParams(&market.PublishStorageDealsParams{
		Deals: []market.ClientDealProposal{deal.ClientDealProposal},
	})

	if err != nil {
		return cid.Undef, xerrors.Errorf("serializing PublishStorageDeals params failed: ", err)
	}

	// TODO: We may want this to happen after fetching data
	smsg, err := n.MpoolPushMessage(ctx, &types.Message{
		To:       builtin.StorageMarketActorAddr,
		From:     mi.Worker,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: 1000000,
		Method:   builtin.MethodsMarket.PublishStorageDeals,
		Params:   params,
	})
	if err != nil {
		return cid.Undef, err
	}
	return smsg.Cid(), nil
}

func (n *ProviderNodeAdapter) OnDealComplete(ctx context.Context, deal storagemarket.MinerDeal, pieceSize abi.UnpaddedPieceSize, pieceData io.Reader) error {
	_, err := n.secb.AddPiece(ctx, pieceSize, pieceData, sealing.DealInfo{
		DealID: deal.DealID,
		DealSchedule: sealing.DealSchedule{
			StartEpoch: deal.ClientDealProposal.Proposal.StartEpoch,
			EndEpoch:   deal.ClientDealProposal.Proposal.EndEpoch,
		},
	})
	if err != nil {
		return xerrors.Errorf("AddPiece failed: %s", err)
	}
	log.Warnf("New Deal: deal %d", deal.DealID)

	return nil
}

func (n *ProviderNodeAdapter) VerifySignature(ctx context.Context, sig crypto.Signature, addr address.Address, input []byte, encodedTs shared.TipSetToken) (bool, error) {
	addr, err := n.StateAccountKey(ctx, addr, types.EmptyTSK)
	if err != nil {
		return false, err
	}

	err = sigs.Verify(&sig, addr, input)
	return err == nil, err
}

func (n *ProviderNodeAdapter) ListProviderDeals(ctx context.Context, addr address.Address, encodedTs shared.TipSetToken) ([]storagemarket.StorageDeal, error) {
	tsk, err := types.TipSetKeyFromBytes(encodedTs)
	if err != nil {
		return nil, err
	}
	allDeals, err := n.StateMarketDeals(ctx, tsk)
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
		To:       builtin.StorageMarketActorAddr,
		From:     addr,
		Value:    amount,
		GasPrice: types.NewInt(0),
		GasLimit: 1000000,
		Method:   builtin.MethodsMarket.AddBalance,
	})
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

func (n *ProviderNodeAdapter) LocatePieceForDealWithinSector(ctx context.Context, dealID abi.DealID, encodedTs shared.TipSetToken) (sectorID uint64, offset uint64, length uint64, err error) {
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
	return uint64(best.SectorID), best.Offset, uint64(best.Size), nil
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

	var sectorNumber abi.SectorNumber
	var sectorFound bool

	matchEvent := func(msg *types.Message) (bool, error) {
		if msg.To != provider {
			return false, nil
		}

		switch msg.Method {
		case builtin.MethodsMiner.PreCommitSector:
			var params miner.SectorPreCommitInfo
			if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
				return false, xerrors.Errorf("unmarshal pre commit: %w", err)
			}

			for _, did := range params.DealIDs {
				if did == abi.DealID(dealID) {
					sectorNumber = params.SectorNumber
					sectorFound = true
					return false, nil
				}
			}

			return false, nil
		case builtin.MethodsMiner.ProveCommitSector:
			var params miner.ProveCommitSectorParams
			if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
				return false, xerrors.Errorf("failed to unmarshal prove commit sector params: %w", err)
			}

			if !sectorFound {
				return false, nil
			}

			if params.SectorNumber != sectorNumber {
				return false, nil
			}

			return true, nil
		default:
			return false, nil
		}

	}

	if err := n.ev.Called(checkFunc, called, revert, int(build.MessageConfidence+1), build.SealRandomnessLookbackLimit, matchEvent); err != nil {
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

func (n *ProviderNodeAdapter) WaitForMessage(ctx context.Context, mcid cid.Cid, cb func(code exitcode.ExitCode, bytes []byte, err error) error) error {
	receipt, err := n.StateWaitMsg(ctx, mcid, build.MessageConfidence)
	if err != nil {
		return cb(0, nil, err)
	}
	return cb(receipt.Receipt.ExitCode, receipt.Receipt.Return, nil)
}

var _ storagemarket.StorageProviderNode = &ProviderNodeAdapter{}
