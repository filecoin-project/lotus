package storageadapter

// this file implements storagemarket.StorageProviderNode

import (
	"bytes"
	"context"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	unixfile "github.com/ipfs/go-unixfs/file"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	sharedtypes "github.com/filecoin-project/go-fil-markets/shared/types"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/padreader"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

var log = logging.Logger("provideradapter")

type ProviderNodeAdapter struct {
	api.FullNode

	// this goes away with the data transfer module
	dag dtypes.StagingDAG

	secb *sectorblocks.SectorBlocks
}

func NewProviderNodeAdapter(dag dtypes.StagingDAG, secb *sectorblocks.SectorBlocks, full api.FullNode) storagemarket.StorageProviderNode {
	return &ProviderNodeAdapter{
		FullNode: full,
		dag:      dag,
		secb:     secb,
	}
}

func (n *ProviderNodeAdapter) PublishDeals(ctx context.Context, deal storagemarket.MinerDeal) (storagemarket.DealID, cid.Cid, error) {
	log.Info("publishing deal")

	worker, err := n.StateMinerWorker(ctx, deal.Proposal.Provider, types.EmptyTSK)
	if err != nil {
		return 0, cid.Undef, err
	}

	localProposal, err := utils.FromSharedStorageDealProposal(&deal.Proposal)
	if err != nil {
		return 0, cid.Undef, err
	}
	params, err := actors.SerializeParams(&actors.PublishStorageDealsParams{
		Deals: []actors.StorageDealProposal{*localProposal},
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
		Method:   actors.SMAMethods.PublishStorageDeals,
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
	var resp actors.PublishStorageDealResponse
	if err := resp.UnmarshalCBOR(bytes.NewReader(r.Receipt.Return)); err != nil {
		return 0, cid.Undef, err
	}
	if len(resp.DealIDs) != 1 {
		return 0, cid.Undef, xerrors.Errorf("got unexpected number of DealIDs from")
	}

	return storagemarket.DealID(resp.DealIDs[0]), smsg.Cid(), nil
}

func (n *ProviderNodeAdapter) OnDealComplete(ctx context.Context, deal storagemarket.MinerDeal, piecePath string) (uint64, error) {
	root, err := n.dag.Get(ctx, deal.Ref)
	if err != nil {
		return 0, xerrors.Errorf("failed to get file root for deal: %s", err)
	}

	// TODO: abstract this away into ReadSizeCloser + implement different modes
	node, err := unixfile.NewUnixfsFile(ctx, n.dag, root)
	if err != nil {
		return 0, xerrors.Errorf("cannot open unixfs file: %s", err)
	}

	uf, ok := node.(sectorblocks.UnixfsReader)
	if !ok {
		// we probably got directory, unsupported for now
		return 0, xerrors.Errorf("unsupported unixfs file type")
	}

	// TODO: uf.Size() is user input, not trusted
	// This won't be useful / here after we migrate to putting CARs into sectors
	size, err := uf.Size()
	if err != nil {
		return 0, xerrors.Errorf("getting unixfs file size: %w", err)
	}
	if padreader.PaddedSize(uint64(size)) != deal.Proposal.PieceSize {
		return 0, xerrors.Errorf("deal.Proposal.PieceSize didn't match padded unixfs file size")
	}

	sectorID, err := n.secb.AddUnixfsPiece(ctx, uf, deal.DealID)
	if err != nil {
		return 0, xerrors.Errorf("AddPiece failed: %s", err)
	}
	log.Warnf("New Sector: %d (deal %d)", sectorID, deal.DealID)

	return sectorID, nil
}

func (n *ProviderNodeAdapter) ListProviderDeals(ctx context.Context, addr address.Address) ([]storagemarket.StorageDeal, error) {
	allDeals, err := n.StateMarketDeals(ctx, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	var out []storagemarket.StorageDeal

	for _, deal := range allDeals {
		sharedDeal := utils.FromOnChainDeal(deal)
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

func (n *ProviderNodeAdapter) SignBytes(ctx context.Context, signer address.Address, b []byte) (*sharedtypes.Signature, error) {
	localSignature, err := n.WalletSign(ctx, signer, b)
	if err != nil {
		return nil, err
	}
	return utils.ToSharedSignature(localSignature)
}

func (n *ProviderNodeAdapter) EnsureFunds(ctx context.Context, addr address.Address, amt tokenamount.TokenAmount) error {
	return n.MarketEnsureAvailable(ctx, addr, utils.FromSharedTokenAmount(amt))
}

func (n *ProviderNodeAdapter) MostRecentStateId(ctx context.Context) (storagemarket.StateKey, error) {
	return n.ChainHead(ctx)
}

// Adds funds with the StorageMinerActor for a storage participant.  Used by both providers and clients.
func (n *ProviderNodeAdapter) AddFunds(ctx context.Context, addr address.Address, amount tokenamount.TokenAmount) error {
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

func (n *ProviderNodeAdapter) GetBalance(ctx context.Context, addr address.Address) (storagemarket.Balance, error) {
	bal, err := n.StateMarketBalance(ctx, addr, types.EmptyTSK)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	return utils.ToSharedBalance(bal), nil
}

var _ storagemarket.StorageProviderNode = &ProviderNodeAdapter{}
