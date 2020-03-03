package client

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-car"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	chunker "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

const dealStartBuffer abi.ChainEpoch = 10000 // TODO: allow setting

type API struct {
	fx.In

	full.ChainAPI
	full.StateAPI
	full.WalletAPI
	paych.PaychAPI

	SMDealClient storagemarket.StorageClient
	RetDiscovery retrievalmarket.PeerResolver
	Retrieval    retrievalmarket.RetrievalClient
	Chain        *store.ChainStore

	LocalDAG   dtypes.ClientDAG
	Blockstore dtypes.ClientBlockstore
	Filestore  dtypes.ClientFilestore `optional:"true"`
}

func (a *API) ClientStartDeal(ctx context.Context, params *api.StartDealParams) (*cid.Cid, error) {
	exist, err := a.WalletHas(ctx, params.Wallet)
	if err != nil {
		return nil, xerrors.Errorf("failed getting addr from wallet: %w", params.Wallet)
	}
	if !exist {
		return nil, xerrors.Errorf("provided address doesn't exist in wallet")
	}

	pid, err := a.StateMinerPeerID(ctx, params.Miner, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed getting peer ID: %w", err)
	}

	mw, err := a.StateMinerWorker(ctx, params.Miner, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed getting miner worker: %w", err)
	}

	ssize, err := a.StateMinerSectorSize(ctx, params.Miner, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed checking miners sector size: %w", err)
	}

	rt, _, err := api.ProofTypeFromSectorSize(ssize)
	if err != nil {
		return nil, xerrors.Errorf("bad sector size: %w", err)
	}

	providerInfo := utils.NewStorageProviderInfo(params.Miner, mw, 0, pid)
	ts, err := a.ChainHead(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed getting chain height: %w", err)
	}

	result, err := a.SMDealClient.ProposeStorageDeal(
		ctx,
		params.Wallet,
		&providerInfo,
		params.Data,
		ts.Height()+dealStartBuffer,
		ts.Height()+dealStartBuffer+abi.ChainEpoch(params.BlocksDuration),
		params.EpochPrice,
		big.Zero(),
		rt,
	)

	if err != nil {
		return nil, xerrors.Errorf("failed to start deal: %w", err)
	}

	return &result.ProposalCid, nil
}

func (a *API) ClientListDeals(ctx context.Context) ([]api.DealInfo, error) {
	deals, err := a.SMDealClient.ListInProgressDeals(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]api.DealInfo, len(deals))
	for k, v := range deals {
		out[k] = api.DealInfo{
			ProposalCid: v.ProposalCid,
			State:       v.State,
			Provider:    v.Proposal.Provider,

			PieceRef: v.Proposal.PieceCID.Bytes(),
			Size:     uint64(v.Proposal.PieceSize.Unpadded()),

			PricePerEpoch: v.Proposal.StoragePricePerEpoch,
			Duration:      uint64(v.Proposal.Duration()),
		}
	}

	return out, nil
}

func (a *API) ClientGetDealInfo(ctx context.Context, d cid.Cid) (*api.DealInfo, error) {
	v, err := a.SMDealClient.GetInProgressDeal(ctx, d)
	if err != nil {
		return nil, err
	}

	return &api.DealInfo{
		ProposalCid:   v.ProposalCid,
		State:         v.State,
		Provider:      v.Proposal.Provider,
		PieceRef:      v.Proposal.PieceCID.Bytes(),
		Size:          uint64(v.Proposal.PieceSize.Unpadded()),
		PricePerEpoch: v.Proposal.StoragePricePerEpoch,
		Duration:      uint64(v.Proposal.Duration()),
	}, nil
}

func (a *API) ClientHasLocal(ctx context.Context, root cid.Cid) (bool, error) {
	// TODO: check if we have the ENTIRE dag

	offExch := merkledag.NewDAGService(blockservice.New(a.Blockstore, offline.Exchange(a.Blockstore)))
	_, err := offExch.Get(ctx, root)
	if err == ipld.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *API) ClientFindData(ctx context.Context, root cid.Cid) ([]api.QueryOffer, error) {
	peers, err := a.RetDiscovery.GetPeers(root)
	if err != nil {
		return nil, err
	}

	out := make([]api.QueryOffer, len(peers))
	for k, p := range peers {
		queryResponse, err := a.Retrieval.Query(ctx, p, root, retrievalmarket.QueryParams{})
		if err != nil {
			out[k] = api.QueryOffer{Err: err.Error(), Miner: p.Address, MinerPeerID: p.ID}
		} else {
			out[k] = api.QueryOffer{
				Root:                    root,
				Size:                    queryResponse.Size,
				MinPrice:                queryResponse.PieceRetrievalPrice(),
				PaymentInterval:         queryResponse.MaxPaymentInterval,
				PaymentIntervalIncrease: queryResponse.MaxPaymentIntervalIncrease,
				Miner:                   queryResponse.PaymentAddress, // TODO: check
				MinerPeerID:             p.ID,
			}
		}
	}

	return out, nil
}

func (a *API) ClientImport(ctx context.Context, ref api.FileRef) (cid.Cid, error) {
	f, err := os.Open(ref.Path)
	if err != nil {
		return cid.Undef, err
	}

	if ref.IsCAR {
		result, err := car.LoadCar(a.Blockstore, f)
		if err != nil {
			return cid.Undef, err
		}

		if len(result.Roots) != 1 {
			return cid.Undef, xerrors.New("cannot import car with more than one root")
		}

		return result.Roots[0], nil
	}

	stat, err := f.Stat()
	if err != nil {
		return cid.Undef, err
	}

	file, err := files.NewReaderPathFile(ref.Path, f, stat)
	if err != nil {
		return cid.Undef, err
	}

	bufferedDS := ipld.NewBufferedDAG(ctx, a.LocalDAG)

	params := ihelper.DagBuilderParams{
		Maxlinks:   build.UnixfsLinksPerLevel,
		RawLeaves:  true,
		CidBuilder: nil,
		Dagserv:    bufferedDS,
		NoCopy:     true,
	}

	db, err := params.New(chunker.NewSizeSplitter(file, int64(build.UnixfsChunkSize)))
	if err != nil {
		return cid.Undef, err
	}
	nd, err := balanced.Layout(db)
	if err != nil {
		return cid.Undef, err
	}

	if err := bufferedDS.Commit(); err != nil {
		return cid.Undef, err
	}

	return nd.Cid(), nil
}

func (a *API) ClientImportLocal(ctx context.Context, f io.Reader) (cid.Cid, error) {
	file := files.NewReaderFile(f)

	bufferedDS := ipld.NewBufferedDAG(ctx, a.LocalDAG)

	params := ihelper.DagBuilderParams{
		Maxlinks:   build.UnixfsLinksPerLevel,
		RawLeaves:  true,
		CidBuilder: nil,
		Dagserv:    bufferedDS,
	}

	db, err := params.New(chunker.NewSizeSplitter(file, int64(build.UnixfsChunkSize)))
	if err != nil {
		return cid.Undef, err
	}
	nd, err := balanced.Layout(db)
	if err != nil {
		return cid.Undef, err
	}

	return nd.Cid(), bufferedDS.Commit()
}

func (a *API) ClientListImports(ctx context.Context) ([]api.Import, error) {
	if a.Filestore == nil {
		return nil, errors.New("listing imports is not supported with in-memory dag yet")
	}
	next, err := filestore.ListAll(a.Filestore, false)
	if err != nil {
		return nil, err
	}

	// TODO: make this less very bad by tracking root cids instead of using ListAll

	out := make([]api.Import, 0)
	for {
		r := next()
		if r == nil {
			return out, nil
		}
		if r.Offset != 0 {
			continue
		}
		out = append(out, api.Import{
			Status:   r.Status,
			Key:      r.Key,
			FilePath: r.FilePath,
			Size:     r.Size,
		})
	}
}

func (a *API) ClientRetrieve(ctx context.Context, order api.RetrievalOrder, ref api.FileRef) error {
	if order.MinerPeerID == "" {
		pid, err := a.StateMinerPeerID(ctx, order.Miner, types.EmptyTSK)
		if err != nil {
			return err
		}

		order.MinerPeerID = pid
	}

	if order.Size == 0 {
		return xerrors.Errorf("cannot make retrieval deal for zero bytes")
	}

	retrievalResult := make(chan error, 1)

	unsubscribe := a.Retrieval.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
		if state.PayloadCID.Equals(order.Root) {
			switch state.Status {
			case retrievalmarket.DealStatusFailed, retrievalmarket.DealStatusErrored:
				retrievalResult <- xerrors.Errorf("Retrieval Error: %s", state.Message)
			case retrievalmarket.DealStatusCompleted:
				retrievalResult <- nil
			}
		}
	})

	ppb := types.BigDiv(order.Total, types.NewInt(order.Size))

	a.Retrieval.Retrieve(
		ctx,
		order.Root,
		retrievalmarket.NewParamsV0(ppb, order.PaymentInterval, order.PaymentIntervalIncrease),
		order.Total,
		order.MinerPeerID,
		order.Client,
		order.Miner)
	select {
	case <-ctx.Done():
		return xerrors.New("Retrieval Timed Out")
	case err := <-retrievalResult:
		if err != nil {
			return xerrors.Errorf("RetrieveUnixfs: %w", err)
		}
	}

	unsubscribe()

	if ref.IsCAR {
		f, err := os.Open(ref.Path)
		if err != nil {
			return err
		}
		err = car.WriteCar(ctx, a.LocalDAG, []cid.Cid{order.Root}, f)
		if err != nil {
			return err
		}
		return f.Close()
	}

	nd, err := a.LocalDAG.Get(ctx, order.Root)
	if err != nil {
		return xerrors.Errorf("ClientRetrieve: %w", err)
	}
	file, err := unixfile.NewUnixfsFile(ctx, a.LocalDAG, nd)
	if err != nil {
		return xerrors.Errorf("ClientRetrieve: %w", err)
	}
	return files.WriteTo(file, ref.Path)
}

func (a *API) ClientQueryAsk(ctx context.Context, p peer.ID, miner address.Address) (*storagemarket.SignedStorageAsk, error) {
	info := utils.NewStorageProviderInfo(miner, address.Undef, 0, p)
	signedAsk, err := a.SMDealClient.GetAsk(ctx, info)
	if err != nil {
		return nil, err
	}
	return signedAsk, nil
}
