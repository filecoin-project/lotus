package client

import (
	"context"
	"errors"

	"github.com/filecoin-project/go-fil-markets/pieceio"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"

	"io"
	"os"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-blockservice"
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
	"github.com/ipld/go-car"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"

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

func calcDealExpiration(minDuration uint64, md *miner.DeadlineInfo, startEpoch abi.ChainEpoch) abi.ChainEpoch {
	// Make sure we give some time for the miner to seal
	minExp := startEpoch + abi.ChainEpoch(minDuration)

	// Align on miners ProvingPeriodBoundary
	return minExp + miner.WPoStProvingPeriod - (minExp % miner.WPoStProvingPeriod) + (md.PeriodStart % miner.WPoStProvingPeriod) - 1
}

func (a *API) ClientStartDeal(ctx context.Context, params *api.StartDealParams) (*cid.Cid, error) {
	exist, err := a.WalletHas(ctx, params.Wallet)
	if err != nil {
		return nil, xerrors.Errorf("failed getting addr from wallet: %w", params.Wallet)
	}
	if !exist {
		return nil, xerrors.Errorf("provided address doesn't exist in wallet")
	}

	mi, err := a.StateMinerInfo(ctx, params.Miner, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed getting peer ID: %w", err)
	}

	md, err := a.StateMinerProvingDeadline(ctx, params.Miner, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed getting peer ID: %w", err)
	}

	rt, err := ffiwrapper.SealProofTypeFromSectorSize(mi.SectorSize)
	if err != nil {
		return nil, xerrors.Errorf("bad sector size: %w", err)
	}

	if uint64(params.Data.PieceSize.Padded()) > uint64(mi.SectorSize) {
		return nil, xerrors.New("data doesn't fit in a sector")
	}

	providerInfo := utils.NewStorageProviderInfo(params.Miner, mi.Worker, mi.SectorSize, mi.PeerId)

	dealStart := params.DealStartEpoch
	if dealStart <= 0 { // unset, or explicitly 'epoch undefined'
		ts, err := a.ChainHead(ctx)
		if err != nil {
			return nil, xerrors.Errorf("failed getting chain height: %w", err)
		}

		dealStart = ts.Height() + dealStartBuffer
	}

	result, err := a.SMDealClient.ProposeStorageDeal(
		ctx,
		params.Wallet,
		&providerInfo,
		params.Data,
		dealStart,
		calcDealExpiration(params.MinBlocksDuration, md, dealStart),
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
	deals, err := a.SMDealClient.ListLocalDeals(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]api.DealInfo, len(deals))
	for k, v := range deals {
		out[k] = api.DealInfo{
			ProposalCid: v.ProposalCid,
			State:       v.State,
			Message:     v.Message,
			Provider:    v.Proposal.Provider,

			PieceCID: v.Proposal.PieceCID,
			Size:     uint64(v.Proposal.PieceSize.Unpadded()),

			PricePerEpoch: v.Proposal.StoragePricePerEpoch,
			Duration:      uint64(v.Proposal.Duration()),
			DealID:        v.DealID,
		}
	}

	return out, nil
}

func (a *API) ClientGetDealInfo(ctx context.Context, d cid.Cid) (*api.DealInfo, error) {
	v, err := a.SMDealClient.GetLocalDeal(ctx, d)
	if err != nil {
		return nil, err
	}

	return &api.DealInfo{
		ProposalCid:   v.ProposalCid,
		State:         v.State,
		Message:       v.Message,
		Provider:      v.Proposal.Provider,
		PieceCID:      v.Proposal.PieceCID,
		Size:          uint64(v.Proposal.PieceSize.Unpadded()),
		PricePerEpoch: v.Proposal.StoragePricePerEpoch,
		Duration:      uint64(v.Proposal.Duration()),
		DealID:        v.DealID,
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

	bufferedDS := ipld.NewBufferedDAG(ctx, a.LocalDAG)
	nd, err := a.clientImport(ref, bufferedDS)

	if err != nil {
		return cid.Undef, err
	}

	return nd, nil
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
	lowest := make([]uint64, 0)
	for {
		r := next()
		if r == nil {
			return out, nil
		}
		matched := false
		for i := range out {
			if out[i].FilePath == r.FilePath {
				matched = true
				if lowest[i] > r.Offset {
					lowest[i] = r.Offset
					out[i] = api.Import{
						Status:   r.Status,
						Key:      r.Key,
						FilePath: r.FilePath,
						Size:     r.Size,
					}
				}
				break
			}
		}
		if !matched {
			out = append(out, api.Import{
				Status:   r.Status,
				Key:      r.Key,
				FilePath: r.FilePath,
				Size:     r.Size,
			})
			lowest = append(lowest, r.Offset)
		}
	}
}

func (a *API) ClientRetrieve(ctx context.Context, order api.RetrievalOrder, ref *api.FileRef) error {
	if order.MinerPeerID == "" {
		mi, err := a.StateMinerInfo(ctx, order.Miner, types.EmptyTSK)
		if err != nil {
			return err
		}

		order.MinerPeerID = mi.PeerId
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

	_, err := a.Retrieval.Retrieve(
		ctx,
		order.Root,
		retrievalmarket.NewParamsV0(ppb, order.PaymentInterval, order.PaymentIntervalIncrease),
		order.Total,
		order.MinerPeerID,
		order.Client,
		order.Miner)
	if err != nil {
		return xerrors.Errorf("Retrieve failed: %w", err)
	}
	select {
	case <-ctx.Done():
		return xerrors.New("Retrieval Timed Out")
	case err := <-retrievalResult:
		if err != nil {
			return xerrors.Errorf("RetrieveUnixfs: %w", err)
		}
	}

	unsubscribe()

	// If ref is nil, it only fetches the data into the configured blockstore.
	if ref == nil {
		return nil
	}

	if ref.IsCAR {
		f, err := os.OpenFile(ref.Path, os.O_CREATE|os.O_WRONLY, 0644)
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

func (a *API) ClientCalcCommP(ctx context.Context, inpath string, miner address.Address) (*api.CommPRet, error) {
	mi, err := a.StateMinerInfo(ctx, miner, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed checking miners sector size: %w", err)
	}

	rt, err := ffiwrapper.SealProofTypeFromSectorSize(mi.SectorSize)
	if err != nil {
		return nil, xerrors.Errorf("bad sector size: %w", err)
	}

	rdr, err := os.Open(inpath)
	if err != nil {
		return nil, err
	}

	stat, err := rdr.Stat()
	if err != nil {
		return nil, err
	}

	commP, pieceSize, err := pieceio.GeneratePieceCommitment(rt, rdr, uint64(stat.Size()))

	if err != nil {
		return nil, xerrors.Errorf("computing commP failed: %w", err)
	}

	return &api.CommPRet{
		Root: commP,
		Size: pieceSize,
	}, nil
}

func (a *API) ClientGenCar(ctx context.Context, ref api.FileRef, outputPath string) error {

	bufferedDS := ipld.NewBufferedDAG(ctx, a.LocalDAG)
	c, err := a.clientImport(ref, bufferedDS)

	if err != nil {
		return err
	}

	defer bufferedDS.Remove(ctx, c)
	ssb := builder.NewSelectorSpecBuilder(basicnode.Style.Any)

	// entire DAG selector
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	f, err := os.Create(outputPath)
	defer f.Close()
	if err != nil {
		return err
	}

	sc := car.NewSelectiveCar(ctx, a.Blockstore, []car.Dag{{Root: c, Selector: allSelector}})
	if err = sc.Write(f); err != nil {
		return err
	}

	return nil
}

func (a *API) clientImport(ref api.FileRef, bufferedDS *ipld.BufferedDAG) (cid.Cid, error) {
	f, err := os.Open(ref.Path)
	if err != nil {
		return cid.Undef, err
	}

	stat, err := f.Stat()
	if err != nil {
		return cid.Undef, err
	}

	file, err := files.NewReaderPathFile(ref.Path, f, stat)
	if err != nil {
		return cid.Undef, err
	}

	if ref.IsCAR {
		var store car.Store
		if a.Filestore == nil {
			store = a.Blockstore
		} else {
			store = (*filestore.Filestore)(a.Filestore)
		}
		result, err := car.LoadCar(store, file)
		if err != nil {
			return cid.Undef, err
		}

		if len(result.Roots) != 1 {
			return cid.Undef, xerrors.New("cannot import car with more than one root")
		}

		return result.Roots[0], nil
	}

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
