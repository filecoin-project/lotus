package client

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/go-state-types/big"
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
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	mh "github.com/multiformats/go-multihash"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/discovery"
	"github.com/filecoin-project/go-fil-markets/pieceio"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	marketevents "github.com/filecoin-project/lotus/markets/loggers"

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

const dealStartBufferHours uint64 = 49

type API struct {
	fx.In

	full.ChainAPI
	full.StateAPI
	full.WalletAPI
	paych.PaychAPI

	SMDealClient storagemarket.StorageClient
	RetDiscovery discovery.PeerResolver
	Retrieval    rm.RetrievalClient
	Chain        *store.ChainStore

	Imports dtypes.ClientImportMgr

	CombinedBstore    dtypes.ClientBlockstore // TODO: try to remove
	RetrievalStoreMgr dtypes.ClientRetrievalStoreManager
	DataTransfer      dtypes.ClientDataTransfer
	Host              host.Host
}

func calcDealExpiration(minDuration uint64, md *dline.Info, startEpoch abi.ChainEpoch) abi.ChainEpoch {
	// Make sure we give some time for the miner to seal
	minExp := startEpoch + abi.ChainEpoch(minDuration)

	// Align on miners ProvingPeriodBoundary
	return minExp + md.WPoStProvingPeriod - (minExp % md.WPoStProvingPeriod) + (md.PeriodStart % md.WPoStProvingPeriod) - 1
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
				storeID = &importID //nolint
				break
			}
		}
	}

	walletKey, err := a.StateAPI.StateManager.ResolveToKeyAddress(ctx, params.Wallet, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed resolving params.Wallet addr: %w", params.Wallet)
	}

	exist, err := a.WalletHas(ctx, walletKey)
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
		return nil, xerrors.Errorf("failed getting miner's deadline info: %w", err)
	}

	rt, err := ffiwrapper.SealProofTypeFromSectorSize(mi.SectorSize)
	if err != nil {
		return nil, xerrors.Errorf("bad sector size: %w", err)
	}

	if uint64(params.Data.PieceSize.Padded()) > uint64(mi.SectorSize) {
		return nil, xerrors.New("data doesn't fit in a sector")
	}

	providerInfo := utils.NewStorageProviderInfo(params.Miner, mi.Worker, mi.SectorSize, *mi.PeerId, mi.Multiaddrs)

	dealStart := params.DealStartEpoch
	if dealStart <= 0 { // unset, or explicitly 'epoch undefined'
		ts, err := a.ChainHead(ctx)
		if err != nil {
			return nil, xerrors.Errorf("failed getting chain height: %w", err)
		}

		blocksPerHour := 60 * 60 / build.BlockDelaySecs
		dealStart = ts.Height() + abi.ChainEpoch(dealStartBufferHours*blocksPerHour) // TODO: Get this from storage ask
	}

	result, err := a.SMDealClient.ProposeStorageDeal(ctx, storagemarket.ProposeStorageDealParams{
		Addr:          params.Wallet,
		Info:          &providerInfo,
		Data:          params.Data,
		StartEpoch:    dealStart,
		EndEpoch:      calcDealExpiration(params.MinBlocksDuration, md, dealStart),
		Price:         params.EpochPrice,
		Collateral:    params.ProviderCollateral,
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
			CreationTime:  v.CreationTime.Time(),
			Verified:      v.Proposal.VerifiedDeal,
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
		CreationTime:  v.CreationTime.Time(),
		Verified:      v.Proposal.VerifiedDeal,
	}, nil
}

func (a *API) ClientGetDealUpdates(ctx context.Context) (<-chan api.DealInfo, error) {
	updates := make(chan api.DealInfo)

	unsub := a.SMDealClient.SubscribeToEvents(func(_ storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		updates <- newDealInfo(deal)
	})

	go func() {
		defer unsub()
		<-ctx.Done()
	}()

	return updates, nil
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
		ID:      *mi.PeerId,
	}
	return a.makeRetrievalQuery(ctx, rp, root, piece, rm.QueryParams{}), nil
}

func (a *API) makeRetrievalQuery(ctx context.Context, rp rm.RetrievalPeer, payload cid.Cid, piece *cid.Cid, qp rm.QueryParams) api.QueryOffer {
	queryResponse, err := a.Retrieval.Query(ctx, rp, payload, qp)
	if err != nil {
		return api.QueryOffer{Err: err.Error(), Miner: rp.Address, MinerPeer: rp}
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
		MinerPeer:               rp,
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
	events := make(chan marketevents.RetrievalEvent)
	go a.clientRetrieve(ctx, order, ref, events)

	for {
		select {
		case evt, ok := <-events:
			if !ok { // done successfully
				return nil
			}

			if evt.Err != "" {
				return xerrors.Errorf("retrieval failed: %s", evt.Err)
			}
		case <-ctx.Done():
			return xerrors.Errorf("retrieval timed out")
		}
	}
}

func (a *API) ClientRetrieveWithEvents(ctx context.Context, order api.RetrievalOrder, ref *api.FileRef) (<-chan marketevents.RetrievalEvent, error) {
	events := make(chan marketevents.RetrievalEvent)
	go a.clientRetrieve(ctx, order, ref, events)
	return events, nil
}

type retrievalSubscribeEvent struct {
	event rm.ClientEvent
	state rm.ClientDealState
}

func readSubscribeEvents(ctx context.Context, dealID retrievalmarket.DealID, subscribeEvents chan retrievalSubscribeEvent, events chan marketevents.RetrievalEvent) error {
	for {
		var subscribeEvent retrievalSubscribeEvent
		select {
		case <-ctx.Done():
			return xerrors.New("Retrieval Timed Out")
		case subscribeEvent = <-subscribeEvents:
			if subscribeEvent.state.ID != dealID {
				// we can't check the deal ID ahead of time because:
				// 1. We need to subscribe before retrieving.
				// 2. We won't know the deal ID until after retrieving.
				continue
			}
		}

		select {
		case <-ctx.Done():
			return xerrors.New("Retrieval Timed Out")
		case events <- marketevents.RetrievalEvent{
			Event:         subscribeEvent.event,
			Status:        subscribeEvent.state.Status,
			BytesReceived: subscribeEvent.state.TotalReceived,
			FundsSpent:    subscribeEvent.state.FundsSpent,
		}:
		}

		state := subscribeEvent.state
		switch state.Status {
		case rm.DealStatusCompleted:
			return nil
		case rm.DealStatusRejected:
			return xerrors.Errorf("Retrieval Proposal Rejected: %s", state.Message)
		case
			rm.DealStatusDealNotFound,
			rm.DealStatusErrored:
			return xerrors.Errorf("Retrieval Error: %s", state.Message)
		}
	}
}

func (a *API) clientRetrieve(ctx context.Context, order api.RetrievalOrder, ref *api.FileRef, events chan marketevents.RetrievalEvent) {
	defer close(events)

	finish := func(e error) {
		if e != nil {
			events <- marketevents.RetrievalEvent{Err: e.Error(), FundsSpent: big.Zero()}
		}
	}

	if order.MinerPeer.ID == "" {
		mi, err := a.StateMinerInfo(ctx, order.Miner, types.EmptyTSK)
		if err != nil {
			finish(err)
			return
		}

		order.MinerPeer = retrievalmarket.RetrievalPeer{
			ID:      *mi.PeerId,
			Address: order.Miner,
		}
	}

	if order.Size == 0 {
		finish(xerrors.Errorf("cannot make retrieval deal for zero bytes"))
		return
	}

	/*id, st, err := a.imgr().NewStore()
	if err != nil {
		return err
	}
	if err := a.imgr().AddLabel(id, "source", "retrieval"); err != nil {
		return err
	}*/

	ppb := types.BigDiv(order.Total, types.NewInt(order.Size))

	params, err := rm.NewParamsV1(ppb, order.PaymentInterval, order.PaymentIntervalIncrease, shared.AllSelector(), order.Piece, order.UnsealPrice)
	if err != nil {
		finish(xerrors.Errorf("Error in retrieval params: %s", err))
		return
	}

	store, err := a.RetrievalStoreMgr.NewStore()
	if err != nil {
		finish(xerrors.Errorf("Error setting up new store: %w", err))
		return
	}

	defer func() {
		_ = a.RetrievalStoreMgr.ReleaseStore(store)
	}()

	// Subscribe to events before retrieving to avoid losing events.
	subscribeEvents := make(chan retrievalSubscribeEvent, 1)
	subscribeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	unsubscribe := a.Retrieval.SubscribeToEvents(func(event rm.ClientEvent, state rm.ClientDealState) {
		// We'll check the deal IDs inside readSubscribeEvents.
		if state.PayloadCID.Equals(order.Root) {
			select {
			case <-subscribeCtx.Done():
			case subscribeEvents <- retrievalSubscribeEvent{event, state}:
			}
		}
	})

	dealID, err := a.Retrieval.Retrieve(
		ctx,
		order.Root,
		params,
		order.Total,
		order.MinerPeer,
		order.Client,
		order.Miner,
		store.StoreID())

	if err != nil {
		unsubscribe()
		finish(xerrors.Errorf("Retrieve failed: %w", err))
		return
	}

	err = readSubscribeEvents(ctx, dealID, subscribeEvents, events)

	unsubscribe()
	if err != nil {
		finish(xerrors.Errorf("Retrieve: %w", err))
		return
	}

	// If ref is nil, it only fetches the data into the configured blockstore.
	if ref == nil {
		finish(nil)
		return
	}

	rdag := store.DAGService()

	if ref.IsCAR {
		f, err := os.OpenFile(ref.Path, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			finish(err)
			return
		}
		err = car.WriteCar(ctx, rdag, []cid.Cid{order.Root}, f)
		if err != nil {
			finish(err)
			return
		}
		finish(f.Close())
		return
	}

	nd, err := rdag.Get(ctx, order.Root)
	if err != nil {
		finish(xerrors.Errorf("ClientRetrieve: %w", err))
		return
	}
	file, err := unixfile.NewUnixfsFile(ctx, rdag, nd)
	if err != nil {
		finish(xerrors.Errorf("ClientRetrieve: %w", err))
		return
	}
	finish(files.WriteTo(file, ref.Path))
	return
}

func (a *API) ClientQueryAsk(ctx context.Context, p peer.ID, miner address.Address) (*storagemarket.StorageAsk, error) {
	mi, err := a.StateMinerInfo(ctx, miner, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed getting miner info: %w", err)
	}

	info := utils.NewStorageProviderInfo(miner, mi.Worker, mi.SectorSize, p, mi.Multiaddrs)
	ask, err := a.SMDealClient.GetAsk(ctx, info)
	if err != nil {
		return nil, err
	}
	return ask, nil
}

func (a *API) ClientCalcCommP(ctx context.Context, inpath string) (*api.CommPRet, error) {

	// Hard-code the sector size to 32GiB, because:
	// - pieceio.GeneratePieceCommitment requires a RegisteredSealProof
	// - commP itself is sector-size independent, with rather low probability of that changing
	//   ( note how the final rust call is identical for every RegSP type )
	//   https://github.com/filecoin-project/rust-filecoin-proofs-api/blob/v5.0.0/src/seal.rs#L1040-L1050
	//
	// IF/WHEN this changes in the future we will have to be able to calculate
	// "old style" commP, and thus will need to introduce a version switch or similar
	arbitrarySectorSize := abi.SectorSize(32 << 30)

	rt, err := ffiwrapper.SealProofTypeFromSectorSize(arbitrarySectorSize)
	if err != nil {
		return nil, xerrors.Errorf("bad sector size: %w", err)
	}

	rdr, err := os.Open(inpath)
	if err != nil {
		return nil, err
	}
	defer rdr.Close() //nolint:errcheck

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
	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)

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
	defer f.Close() //nolint:errcheck

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

func (a *API) ClientListDataTransfers(ctx context.Context) ([]api.DataTransferChannel, error) {
	inProgressChannels, err := a.DataTransfer.InProgressChannels(ctx)
	if err != nil {
		return nil, err
	}

	apiChannels := make([]api.DataTransferChannel, 0, len(inProgressChannels))
	for _, channelState := range inProgressChannels {
		apiChannels = append(apiChannels, api.NewDataTransferChannel(a.Host.ID(), channelState))
	}

	return apiChannels, nil
}

func (a *API) ClientDataTransferUpdates(ctx context.Context) (<-chan api.DataTransferChannel, error) {
	channels := make(chan api.DataTransferChannel)

	unsub := a.DataTransfer.SubscribeToEvents(func(evt datatransfer.Event, channelState datatransfer.ChannelState) {
		channel := api.NewDataTransferChannel(a.Host.ID(), channelState)
		select {
		case <-ctx.Done():
		case channels <- channel:
		}
	})

	go func() {
		defer unsub()
		<-ctx.Done()
	}()

	return channels, nil
}

func (a *API) ClientRestartDataTransfer(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error {
	selfPeer := a.Host.ID()
	if isInitiator {
		return a.DataTransfer.RestartDataTransferChannel(ctx, datatransfer.ChannelID{Initiator: selfPeer, Responder: otherPeer, ID: transferID})
	}
	return a.DataTransfer.RestartDataTransferChannel(ctx, datatransfer.ChannelID{Initiator: otherPeer, Responder: selfPeer, ID: transferID})
}

func newDealInfo(v storagemarket.ClientDeal) api.DealInfo {
	return api.DealInfo{
		ProposalCid:   v.ProposalCid,
		DataRef:       v.DataRef,
		State:         v.State,
		Message:       v.Message,
		Provider:      v.Proposal.Provider,
		PieceCID:      v.Proposal.PieceCID,
		Size:          uint64(v.Proposal.PieceSize.Unpadded()),
		PricePerEpoch: v.Proposal.StoragePricePerEpoch,
		Duration:      uint64(v.Proposal.Duration()),
		DealID:        v.DealID,
		CreationTime:  v.CreationTime.Time(),
		Verified:      v.Proposal.VerifiedDeal,
	}
}

func (a *API) ClientRetrieveTryRestartInsufficientFunds(ctx context.Context, paymentChannel address.Address) error {
	return a.Retrieval.TryRestartInsufficientFunds(paymentChannel)
}
