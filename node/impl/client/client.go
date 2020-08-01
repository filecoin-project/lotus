package client

import (
	"context"
	"fmt"
	"io"
	"os"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil"
	chunker "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-car"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	mh "github.com/multiformats/go-multihash"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-padreader"
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
	"github.com/filecoin-project/lotus/node/repo/importmgr"
)

var DefaultHashFunction = uint64(mh.BLAKE2B_MIN + 31)

const dealStartBufferHours uint64 = 24

type API struct {
	fx.In

	full.ChainAPI
	full.StateAPI
	full.WalletAPI
	paych.PaychAPI

	SMDealClient storagemarket.StorageClient
	RetDiscovery rm.PeerResolver
	Retrieval    rm.RetrievalClient
	Chain        *store.ChainStore

	Imports dtypes.ClientImportMgr

	CombinedBstore    dtypes.ClientBlockstore // TODO: try to remove
	RetrievalStoreMgr dtypes.ClientRetrievalStoreManager
}

func calcDealExpiration(minDuration uint64, md *miner.DeadlineInfo, startEpoch abi.ChainEpoch) abi.ChainEpoch {
	// Make sure we give some time for the miner to seal
	minExp := startEpoch + abi.ChainEpoch(minDuration)

	// Align on miners ProvingPeriodBoundary
	return minExp + miner.WPoStProvingPeriod - (minExp % miner.WPoStProvingPeriod) + (md.PeriodStart % miner.WPoStProvingPeriod) - 1
}

func (a *API) imgr() *importmgr.Mgr {
	return a.Imports
}

func (a *API) ClientStartDeal(ctx context.Context, params *api.StartDealParams) (*cid.Cid, error) {
	var storeID *multistore.StoreID
	if params.Data.TransferType == storagemarket.TTGraphsync {
		importIDs := a.imgr().List()
		for _, importID := range importIDs {
			info, err := a.imgr().Info(importID)
			if err != nil {
				continue
			}
			if info.Labels[importmgr.LRootCid] == "" {
				continue
			}
			c, err := cid.Parse(info.Labels[importmgr.LRootCid])
			if err != nil {
				continue
			}
			if c.Equals(params.Data.Root) {
				storeID = &importID
				break
			}
		}
	}
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

	multiaddrs := make([]multiaddr.Multiaddr, 0, len(mi.Multiaddrs))
	for _, a := range mi.Multiaddrs {
		maddr, err := multiaddr.NewMultiaddrBytes(a)
		if err != nil {
			return nil, err
		}
		multiaddrs = append(multiaddrs, maddr)
	}

	md, err := a.StateMinerProvingDeadline(ctx, params.Miner, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed getting miner's deadline info: %w", err)
	}

	rt, err := ffiwrapper.SealProofTypeFromSectorSize(mi.SectorSize)
	if err != nil {
		return nil, xerrors.Errorf("bad sector size: %w", err)
	}

	if uint64(params.Data.PieceSize.Padded()) > uint64(mi.SectorSize) {
		return nil, xerrors.New("data doesn't fit in a sector")
	}

	providerInfo := utils.NewStorageProviderInfo(params.Miner, mi.Worker, mi.SectorSize, mi.PeerId, multiaddrs)

	dealStart := params.DealStartEpoch
	if dealStart <= 0 { // unset, or explicitly 'epoch undefined'
		ts, err := a.ChainHead(ctx)
		if err != nil {
			return nil, xerrors.Errorf("failed getting chain height: %w", err)
		}

		blocksPerHour := 60 * 60 / build.BlockDelaySecs
		dealStart = ts.Height() + abi.ChainEpoch(dealStartBufferHours*blocksPerHour)
	}

	result, err := a.SMDealClient.ProposeStorageDeal(ctx, storagemarket.ProposeStorageDealParams{
		Addr:          params.Wallet,
		Info:          &providerInfo,
		Data:          params.Data,
		StartEpoch:    dealStart,
		EndEpoch:      calcDealExpiration(params.MinBlocksDuration, md, dealStart),
		Price:         params.EpochPrice,
		Collateral:    big.Zero(),
		Rt:            rt,
		FastRetrieval: params.FastRetrieval,
		VerifiedDeal:  params.VerifiedDeal,
		StoreID:       storeID,
	})

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
			DataRef:     v.DataRef,
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

	offExch := merkledag.NewDAGService(blockservice.New(a.Imports.Blockstore, offline.Exchange(a.Imports.Blockstore)))
	_, err := offExch.Get(ctx, root)
	if err == ipld.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *API) ClientFindData(ctx context.Context, root cid.Cid, piece *cid.Cid) ([]api.QueryOffer, error) {
	peers, err := a.RetDiscovery.GetPeers(root)
	if err != nil {
		return nil, err
	}

	out := make([]api.QueryOffer, 0, len(peers))
	for _, p := range peers {
		if piece != nil && !piece.Equals(*p.PieceCID) {
			continue
		}
		out = append(out, a.makeRetrievalQuery(ctx, p, root, piece, rm.QueryParams{}))
	}

	return out, nil
}

func (a *API) ClientMinerQueryOffer(ctx context.Context, miner address.Address, root cid.Cid, piece *cid.Cid) (api.QueryOffer, error) {
	mi, err := a.StateMinerInfo(ctx, miner, types.EmptyTSK)
	if err != nil {
		return api.QueryOffer{}, err
	}
	rp := rm.RetrievalPeer{
		Address: miner,
		ID:      mi.PeerId,
	}
	return a.makeRetrievalQuery(ctx, rp, root, piece, rm.QueryParams{}), nil
}

func (a *API) makeRetrievalQuery(ctx context.Context, rp rm.RetrievalPeer, payload cid.Cid, piece *cid.Cid, qp rm.QueryParams) api.QueryOffer {
	queryResponse, err := a.Retrieval.Query(ctx, rp, payload, qp)
	if err != nil {
		return api.QueryOffer{Err: err.Error(), Miner: rp.Address, MinerPeerID: rp.ID}
	}
	var errStr string
	switch queryResponse.Status {
	case rm.QueryResponseAvailable:
		errStr = ""
	case rm.QueryResponseUnavailable:
		errStr = fmt.Sprintf("retrieval query offer was unavailable: %s", queryResponse.Message)
	case rm.QueryResponseError:
		errStr = fmt.Sprintf("retrieval query offer errored: %s", queryResponse.Message)
	}

	return api.QueryOffer{
		Root:                    payload,
		Piece:                   piece,
		Size:                    queryResponse.Size,
		MinPrice:                queryResponse.PieceRetrievalPrice(),
		UnsealPrice:             queryResponse.UnsealPrice,
		PaymentInterval:         queryResponse.MaxPaymentInterval,
		PaymentIntervalIncrease: queryResponse.MaxPaymentIntervalIncrease,
		Miner:                   queryResponse.PaymentAddress, // TODO: check
		MinerPeerID:             rp.ID,
		Err:                     errStr,
	}
}

func (a *API) ClientImport(ctx context.Context, ref api.FileRef) (*api.ImportRes, error) {
	id, st, err := a.imgr().NewStore()
	if err != nil {
		return nil, err
	}
	if err := a.imgr().AddLabel(id, importmgr.LSource, "import"); err != nil {
		return nil, err
	}

	if err := a.imgr().AddLabel(id, importmgr.LFileName, ref.Path); err != nil {
		return nil, err
	}

	nd, err := a.clientImport(ctx, ref, st)
	if err != nil {
		return nil, err
	}

	if err := a.imgr().AddLabel(id, importmgr.LRootCid, nd.String()); err != nil {
		return nil, err
	}

	return &api.ImportRes{
		Root:     nd,
		ImportID: id,
	}, nil
}

func (a *API) ClientRemoveImport(ctx context.Context, importID multistore.StoreID) error {
	return a.imgr().Remove(importID)
}

func (a *API) ClientImportLocal(ctx context.Context, f io.Reader) (cid.Cid, error) {
	file := files.NewReaderFile(f)

	id, st, err := a.imgr().NewStore()
	if err != nil {
		return cid.Undef, err
	}
	if err := a.imgr().AddLabel(id, "source", "import-local"); err != nil {
		return cid.Cid{}, err
	}

	bufferedDS := ipld.NewBufferedDAG(ctx, st.DAG)

	prefix, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return cid.Undef, err
	}
	prefix.MhType = DefaultHashFunction

	params := ihelper.DagBuilderParams{
		Maxlinks:  build.UnixfsLinksPerLevel,
		RawLeaves: true,
		CidBuilder: cidutil.InlineBuilder{
			Builder: prefix,
			Limit:   126,
		},
		Dagserv: bufferedDS,
	}

	db, err := params.New(chunker.NewSizeSplitter(file, int64(build.UnixfsChunkSize)))
	if err != nil {
		return cid.Undef, err
	}
	nd, err := balanced.Layout(db)
	if err != nil {
		return cid.Undef, err
	}
	if err := a.imgr().AddLabel(id, "root", nd.Cid().String()); err != nil {
		return cid.Cid{}, err
	}

	return nd.Cid(), bufferedDS.Commit()
}

func (a *API) ClientListImports(ctx context.Context) ([]api.Import, error) {
	importIDs := a.imgr().List()

	out := make([]api.Import, len(importIDs))
	for i, id := range importIDs {
		info, err := a.imgr().Info(id)
		if err != nil {
			out[i] = api.Import{
				Key: id,
				Err: xerrors.Errorf("getting info: %w", err).Error(),
			}
			continue
		}

		ai := api.Import{
			Key:      id,
			Source:   info.Labels[importmgr.LSource],
			FilePath: info.Labels[importmgr.LFileName],
		}

		if info.Labels[importmgr.LRootCid] != "" {
			c, err := cid.Parse(info.Labels[importmgr.LRootCid])
			if err != nil {
				ai.Err = err.Error()
			} else {
				ai.Root = &c
			}
		}

		out[i] = ai
	}

	return out, nil
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

	/*id, st, err := a.imgr().NewStore()
	if err != nil {
		return err
	}
	if err := a.imgr().AddLabel(id, "source", "retrieval"); err != nil {
		return err
	}*/

	retrievalResult := make(chan error, 1)

	unsubscribe := a.Retrieval.SubscribeToEvents(func(event rm.ClientEvent, state rm.ClientDealState) {
		if state.PayloadCID.Equals(order.Root) {
			switch state.Status {
			case rm.DealStatusCompleted:
				retrievalResult <- nil
			case rm.DealStatusRejected:
				retrievalResult <- xerrors.Errorf("Retrieval Proposal Rejected: %s", state.Message)
			case
				rm.DealStatusDealNotFound,
				rm.DealStatusErrored:
				retrievalResult <- xerrors.Errorf("Retrieval Error: %s", state.Message)
			}
		}
	})

	ppb := types.BigDiv(order.Total, types.NewInt(order.Size))

	params, err := rm.NewParamsV1(ppb, order.PaymentInterval, order.PaymentIntervalIncrease, shared.AllSelector(), order.Piece, order.UnsealPrice)
	if err != nil {
		return xerrors.Errorf("Error in retrieval params: %s", err)
	}

	store, err := a.RetrievalStoreMgr.NewStore()
	if err != nil {
		return xerrors.Errorf("Error setting up new store: %w", err)
	}

	defer func() {
		_ = a.RetrievalStoreMgr.ReleaseStore(store)
	}()

	_, err = a.Retrieval.Retrieve(
		ctx,
		order.Root,
		params,
		order.Total,
		order.MinerPeerID,
		order.Client,
		order.Miner,
		store.StoreID())

	if err != nil {
		return xerrors.Errorf("Retrieve failed: %w", err)
	}
	select {
	case <-ctx.Done():
		return xerrors.New("Retrieval Timed Out")
	case err := <-retrievalResult:
		if err != nil {
			return xerrors.Errorf("Retrieve: %w", err)
		}
	}

	unsubscribe()

	// If ref is nil, it only fetches the data into the configured blockstore.
	if ref == nil {
		return nil
	}

	rdag := store.DAGService()

	if ref.IsCAR {
		f, err := os.OpenFile(ref.Path, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		err = car.WriteCar(ctx, rdag, []cid.Cid{order.Root}, f)
		if err != nil {
			return err
		}
		return f.Close()
	}

	nd, err := rdag.Get(ctx, order.Root)
	if err != nil {
		return xerrors.Errorf("ClientRetrieve: %w", err)
	}
	file, err := unixfile.NewUnixfsFile(ctx, rdag, nd)
	if err != nil {
		return xerrors.Errorf("ClientRetrieve: %w", err)
	}
	return files.WriteTo(file, ref.Path)
}

func (a *API) ClientQueryAsk(ctx context.Context, p peer.ID, miner address.Address) (*storagemarket.SignedStorageAsk, error) {
	info := utils.NewStorageProviderInfo(miner, address.Undef, 0, p, nil)
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
	defer rdr.Close()

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

type lenWriter int64

func (w *lenWriter) Write(p []byte) (n int, err error) {
	*w += lenWriter(len(p))
	return len(p), nil
}

func (a *API) ClientDealSize(ctx context.Context, root cid.Cid) (api.DataSize, error) {
	dag := merkledag.NewDAGService(blockservice.New(a.CombinedBstore, offline.Exchange(a.CombinedBstore)))

	w := lenWriter(0)

	err := car.WriteCar(ctx, dag, []cid.Cid{root}, &w)
	if err != nil {
		return api.DataSize{}, err
	}

	up := padreader.PaddedSize(uint64(w))

	return api.DataSize{
		PayloadSize: int64(w),
		PieceSize:   up.Padded(),
	}, nil
}

func (a *API) ClientGenCar(ctx context.Context, ref api.FileRef, outputPath string) error {
	id, st, err := a.imgr().NewStore()
	if err != nil {
		return err
	}
	if err := a.imgr().AddLabel(id, "source", "gen-car"); err != nil {
		return err
	}

	bufferedDS := ipld.NewBufferedDAG(ctx, st.DAG)
	c, err := a.clientImport(ctx, ref, st)

	if err != nil {
		return err
	}

	// TODO: does that defer mean to remove the whole blockstore?
	defer bufferedDS.Remove(ctx, c) //nolint:errcheck
	ssb := builder.NewSelectorSpecBuilder(basicnode.Style.Any)

	// entire DAG selector
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}

	sc := car.NewSelectiveCar(ctx, st.Bstore, []car.Dag{{Root: c, Selector: allSelector}})
	if err = sc.Write(f); err != nil {
		return err
	}

	return f.Close()
}

func (a *API) clientImport(ctx context.Context, ref api.FileRef, store *multistore.Store) (cid.Cid, error) {
	f, err := os.Open(ref.Path)
	if err != nil {
		return cid.Undef, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return cid.Undef, err
	}

	file, err := files.NewReaderPathFile(ref.Path, f, stat)
	if err != nil {
		return cid.Undef, err
	}

	if ref.IsCAR {
		var st car.Store
		if store.Fstore == nil {
			st = store.Bstore
		} else {
			st = store.Fstore
		}
		result, err := car.LoadCar(st, file)
		if err != nil {
			return cid.Undef, err
		}

		if len(result.Roots) != 1 {
			return cid.Undef, xerrors.New("cannot import car with more than one root")
		}

		return result.Roots[0], nil
	}

	bufDs := ipld.NewBufferedDAG(ctx, store.DAG)

	prefix, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return cid.Undef, err
	}
	prefix.MhType = DefaultHashFunction

	params := ihelper.DagBuilderParams{
		Maxlinks:  build.UnixfsLinksPerLevel,
		RawLeaves: true,
		CidBuilder: cidutil.InlineBuilder{
			Builder: prefix,
			Limit:   126,
		},
		Dagserv: bufDs,
		NoCopy:  true,
	}

	db, err := params.New(chunker.NewSizeSplitter(file, int64(build.UnixfsChunkSize)))
	if err != nil {
		return cid.Undef, err
	}
	nd, err := balanced.Layout(db)
	if err != nil {
		return cid.Undef, err
	}

	if err := bufDs.Commit(); err != nil {
		return cid.Undef, err
	}

	return nd.Cid(), nil
}
