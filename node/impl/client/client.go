package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/filecoin-project/go-fil-markets/filestorecaradapter"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-car"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multibase"
	mh "github.com/multiformats/go-multihash"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-commp-utils/ffiwrapper"
	"github.com/filecoin-project/go-commp-utils/writer"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/discovery"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/v3/actors/builtin/market"

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

// 8 days ~=  SealDuration + PreCommit + MaxProveCommitDuration + 8 hour buffer
const dealStartBufferHours uint64 = 8 * 24

type API struct {
	fx.In

	full.ChainAPI
	full.WalletAPI
	paych.PaychAPI
	full.StateAPI

	SMDealClient storagemarket.StorageClient
	RetDiscovery discovery.PeerResolver
	Retrieval    rm.RetrievalClient
	Chain        *store.ChainStore

	Imports dtypes.ClientImportMgr

	DataTransfer dtypes.ClientDataTransfer
	Host         host.Host

	RetrievalStoreMgr dtypes.ClientRetrievalStoreManager
}

func calcDealExpiration(minDuration uint64, md *dline.Info, startEpoch abi.ChainEpoch) abi.ChainEpoch {
	// Make sure we give some time for the miner to seal
	minExp := startEpoch + abi.ChainEpoch(minDuration)

	// Align on miners ProvingPeriodBoundary
	exp := minExp + md.WPoStProvingPeriod - (minExp % md.WPoStProvingPeriod) + (md.PeriodStart % md.WPoStProvingPeriod) - 1
	// Should only be possible for miners created around genesis
	for exp < minExp {
		exp += md.WPoStProvingPeriod
	}

	return exp
}

func (a *API) imgr() *importmgr.Mgr {
	return a.Imports
}

func (a *API) ClientStartDeal(ctx context.Context, params *api.StartDealParams) (*cid.Cid, error) {
	return a.dealStarter(ctx, params, false)
}

func (a *API) ClientStatelessDeal(ctx context.Context, params *api.StartDealParams) (*cid.Cid, error) {
	return a.dealStarter(ctx, params, true)
}

func (a *API) dealStarter(ctx context.Context, params *api.StartDealParams, isStateless bool) (*cid.Cid, error) {
	var CARV2FilePath string
	if isStateless {
		if params.Data.TransferType != storagemarket.TTManual {
			return nil, xerrors.Errorf("invalid transfer type %s for stateless storage deal", params.Data.TransferType)
		}
		if !params.EpochPrice.IsZero() {
			return nil, xerrors.New("stateless storage deals can only be initiated with storage price of 0")
		}
	} else if params.Data.TransferType == storagemarket.TTGraphsync {
		fc, err := a.imgr().FilestoreCARV2FilePathFor(params.Data.Root)
		if err != nil {
			return nil, xerrors.Errorf("failed to find CARv2 file path: %w", err)
		}
		if fc == "" {
			return nil, xerrors.New("no CARv2 file path for deal")
		}
		CARV2FilePath = fc
	}

	walletKey, err := a.StateAccountKey(ctx, params.Wallet, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed resolving params.Wallet addr (%s): %w", params.Wallet, err)
	}

	exist, err := a.WalletHas(ctx, walletKey)
	if err != nil {
		return nil, xerrors.Errorf("failed getting addr from wallet (%s): %w", params.Wallet, err)
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

	if uint64(params.Data.PieceSize.Padded()) > uint64(mi.SectorSize) {
		return nil, xerrors.New("data doesn't fit in a sector")
	}

	dealStart := params.DealStartEpoch
	if dealStart <= 0 { // unset, or explicitly 'epoch undefined'
		ts, err := a.ChainHead(ctx)
		if err != nil {
			return nil, xerrors.Errorf("failed getting chain height: %w", err)
		}

		blocksPerHour := 60 * 60 / build.BlockDelaySecs
		dealStart = ts.Height() + abi.ChainEpoch(dealStartBufferHours*blocksPerHour) // TODO: Get this from storage ask
	}

	networkVersion, err := a.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("failed to get network version: %w", err)
	}

	st, err := miner.PreferredSealProofTypeFromWindowPoStType(networkVersion, mi.WindowPoStProofType)
	if err != nil {
		return nil, xerrors.Errorf("failed to get seal proof type: %w", err)
	}

	// regular flow
	if !isStateless {
		providerInfo := utils.NewStorageProviderInfo(params.Miner, mi.Worker, mi.SectorSize, *mi.PeerId, mi.Multiaddrs)

		result, err := a.SMDealClient.ProposeStorageDeal(ctx, storagemarket.ProposeStorageDealParams{
			Addr:                   params.Wallet,
			Info:                   &providerInfo,
			Data:                   params.Data,
			StartEpoch:             dealStart,
			EndEpoch:               calcDealExpiration(params.MinBlocksDuration, md, dealStart),
			Price:                  params.EpochPrice,
			Collateral:             params.ProviderCollateral,
			Rt:                     st,
			FastRetrieval:          params.FastRetrieval,
			VerifiedDeal:           params.VerifiedDeal,
			FilestoreCARv2FilePath: CARV2FilePath,
		})

		if err != nil {
			return nil, xerrors.Errorf("failed to start deal: %w", err)
		}

		return &result.ProposalCid, nil
	}

	//
	// stateless flow from here to the end
	//

	dealProposal := &market.DealProposal{
		PieceCID:             *params.Data.PieceCid,
		PieceSize:            params.Data.PieceSize.Padded(),
		Client:               walletKey,
		Provider:             params.Miner,
		Label:                params.Data.Root.Encode(multibase.MustNewEncoder('u')),
		StartEpoch:           dealStart,
		EndEpoch:             calcDealExpiration(params.MinBlocksDuration, md, dealStart),
		StoragePricePerEpoch: big.Zero(),
		ProviderCollateral:   params.ProviderCollateral,
		ClientCollateral:     big.Zero(),
		VerifiedDeal:         params.VerifiedDeal,
	}

	if dealProposal.ProviderCollateral.IsZero() {
		networkCollateral, err := a.StateDealProviderCollateralBounds(ctx, params.Data.PieceSize.Padded(), params.VerifiedDeal, types.EmptyTSK)
		if err != nil {
			return nil, xerrors.Errorf("failed to determine minimum provider collateral: %w", err)
		}
		dealProposal.ProviderCollateral = networkCollateral.Min
	}

	dealProposalSerialized, err := cborutil.Dump(dealProposal)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize deal proposal: %w", err)
	}

	dealProposalSig, err := a.WalletSign(ctx, walletKey, dealProposalSerialized)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign proposal : %w", err)
	}

	dealProposalSigned := &market.ClientDealProposal{
		Proposal:        *dealProposal,
		ClientSignature: *dealProposalSig,
	}
	dStream, err := network.NewFromLibp2pHost(a.Host,
		// params duplicated from .../node/modules/client.go
		// https://github.com/filecoin-project/lotus/pull/5961#discussion_r629768011
		network.RetryParameters(time.Second, 5*time.Minute, 15, 5),
	).NewDealStream(ctx, *mi.PeerId)
	if err != nil {
		return nil, xerrors.Errorf("opening dealstream to %s/%s failed: %w", params.Miner, *mi.PeerId, err)
	}

	if err = dStream.WriteDealProposal(network.Proposal{
		FastRetrieval: true,
		DealProposal:  dealProposalSigned,
		Piece: &storagemarket.DataRef{
			TransferType: storagemarket.TTManual,
			Root:         params.Data.Root,
			PieceCid:     params.Data.PieceCid,
			PieceSize:    params.Data.PieceSize,
		},
	}); err != nil {
		return nil, xerrors.Errorf("sending deal proposal failed: %w", err)
	}

	resp, _, err := dStream.ReadDealResponse()
	if err != nil {
		return nil, xerrors.Errorf("reading proposal response failed: %w", err)
	}

	dealProposalIpld, err := cborutil.AsIpld(dealProposalSigned)
	if err != nil {
		return nil, xerrors.Errorf("serializing proposal node failed: %w", err)
	}

	if !dealProposalIpld.Cid().Equals(resp.Response.Proposal) {
		return nil, xerrors.Errorf("provider returned proposal cid %s but we expected %s", resp.Response.Proposal, dealProposalIpld.Cid())
	}

	if resp.Response.State != storagemarket.StorageDealWaitingForData {
		return nil, xerrors.Errorf("provider returned unexpected state %d for proposal %s, with message: %s", resp.Response.State, resp.Response.Proposal, resp.Response.Message)
	}

	return &resp.Response.Proposal, nil
}

func (a *API) ClientListDeals(ctx context.Context) ([]api.DealInfo, error) {
	deals, err := a.SMDealClient.ListLocalDeals(ctx)
	if err != nil {
		return nil, err
	}

	// Get a map of transfer ID => DataTransfer
	dataTransfersByID, err := a.transfersByID(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]api.DealInfo, len(deals))
	for k, v := range deals {
		// Find the data transfer associated with this deal
		var transferCh *api.DataTransferChannel
		if v.TransferChannelID != nil {
			if ch, ok := dataTransfersByID[*v.TransferChannelID]; ok {
				transferCh = &ch
			}
		}

		out[k] = a.newDealInfoWithTransfer(transferCh, v)
	}

	return out, nil
}

func (a *API) transfersByID(ctx context.Context) (map[datatransfer.ChannelID]api.DataTransferChannel, error) {
	inProgressChannels, err := a.DataTransfer.InProgressChannels(ctx)
	if err != nil {
		return nil, err
	}

	dataTransfersByID := make(map[datatransfer.ChannelID]api.DataTransferChannel, len(inProgressChannels))
	for id, channelState := range inProgressChannels {
		ch := api.NewDataTransferChannel(a.Host.ID(), channelState)
		dataTransfersByID[id] = ch
	}
	return dataTransfersByID, nil
}

func (a *API) ClientGetDealInfo(ctx context.Context, d cid.Cid) (*api.DealInfo, error) {
	v, err := a.SMDealClient.GetLocalDeal(ctx, d)
	if err != nil {
		return nil, err
	}

	di := a.newDealInfo(ctx, v)
	return &di, nil
}

func (a *API) ClientGetDealUpdates(ctx context.Context) (<-chan api.DealInfo, error) {
	updates := make(chan api.DealInfo)

	unsub := a.SMDealClient.SubscribeToEvents(func(_ storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		updates <- a.newDealInfo(ctx, deal)
	})

	go func() {
		defer unsub()
		<-ctx.Done()
	}()

	return updates, nil
}

func (a *API) newDealInfo(ctx context.Context, v storagemarket.ClientDeal) api.DealInfo {
	// Find the data transfer associated with this deal
	var transferCh *api.DataTransferChannel
	if v.TransferChannelID != nil {
		state, err := a.DataTransfer.ChannelState(ctx, *v.TransferChannelID)

		// Note: If there was an error just ignore it, as the data transfer may
		// be not found if it's no longer active
		if err == nil {
			ch := api.NewDataTransferChannel(a.Host.ID(), state)
			ch.Stages = state.Stages()
			transferCh = &ch
		}
	}

	di := a.newDealInfoWithTransfer(transferCh, v)
	di.DealStages = v.DealStages
	return di
}

func (a *API) newDealInfoWithTransfer(transferCh *api.DataTransferChannel, v storagemarket.ClientDeal) api.DealInfo {
	return api.DealInfo{
		ProposalCid:       v.ProposalCid,
		DataRef:           v.DataRef,
		State:             v.State,
		Message:           v.Message,
		Provider:          v.Proposal.Provider,
		PieceCID:          v.Proposal.PieceCID,
		Size:              uint64(v.Proposal.PieceSize.Unpadded()),
		PricePerEpoch:     v.Proposal.StoragePricePerEpoch,
		Duration:          uint64(v.Proposal.Duration()),
		DealID:            v.DealID,
		CreationTime:      v.CreationTime.Time(),
		Verified:          v.Proposal.VerifiedDeal,
		TransferChannelID: v.TransferChannelID,
		DataTransfer:      transferCh,
	}
}

func (a *API) ClientHasLocal(ctx context.Context, root cid.Cid) (bool, error) {
	// TODO: check if we have the ENTIRE dag
	fc, err := a.imgr().FilestoreCARV2FilePathFor(root)
	if err != nil {
		return false, err
	}
	if len(fc) == 0 {
		return false, nil
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

func (a *API) ClientImport(ctx context.Context, ref api.FileRef) (res *api.ImportRes, finalErr error) {
	id, err := a.imgr().NewStore()
	if err != nil {
		return nil, err
	}

	var root cid.Cid
	var carFile string
	if ref.IsCAR {
		// if user has given us a CAR file -> just ensure it's either a v1 or a v2, has one root and save it as it is as markets can do deal making for both.
		f, err := os.Open(ref.Path)
		if err != nil {
			return nil, xerrors.Errorf("failed to open CAR file: %w", err)
		}
		defer f.Close() //nolint:errcheck
		hd, _, err := car.ReadHeader(bufio.NewReader(f))
		if err != nil {
			return nil, xerrors.Errorf("failed to read CAR header: %w", err)
		}
		if len(hd.Roots) != 1 {
			return nil, xerrors.New("car file can have one and only one header")
		}
		if hd.Version != 1 && hd.Version != 2 {
			return nil, xerrors.Errorf("car version must be 1 or 2, is %d", hd.Version)
		}

		carFile = ref.Path
		root = hd.Roots[0]
	} else {
		carFile, err = a.imgr().NewTempFile(id)
		if err != nil {
			return nil, xerrors.Errorf("failed to create temp CARv2 file: %w", err)
		}
		// make sure to remove the CARv2 file if anything goes wrong from here on.
		defer func() {
			if finalErr != nil {
				_ = os.Remove(carFile)
			}
		}()

		root, err = a.importNormalFileToFilestoreCARv2(ctx, id, ref.Path, carFile)
		if err != nil {
			return nil, xerrors.Errorf("failed to import normal file to CARv2: %w", err)
		}
	}

	if err := a.imgr().AddLabel(id, importmgr.LSource, "import"); err != nil {
		return nil, err
	}
	if err := a.imgr().AddLabel(id, importmgr.LFileName, ref.Path); err != nil {
		return nil, err
	}
	if err := a.imgr().AddLabel(id, importmgr.LFileStoreCARv2FilePath, carFile); err != nil {
		return nil, err
	}
	if err := a.imgr().AddLabel(id, importmgr.LRootCid, root.String()); err != nil {
		return nil, err
	}

	return &api.ImportRes{
		Root:     root,
		ImportID: id,
	}, nil
}

func (a *API) ClientRemoveImport(ctx context.Context, importID importmgr.ImportID) error {
	info, err := a.imgr().Info(importID)
	if err != nil {
		return xerrors.Errorf("failed to fetch import info: %w", err)
	}

	// remove the CARv2 file if we've created one.
	if path := info.Labels[importmgr.LFileStoreCARv2FilePath]; path != "" {
		_ = os.Remove(path)
	}

	return a.imgr().Remove(importID)
}

func (a *API) ClientImportLocal(ctx context.Context, r io.Reader) (cid.Cid, error) {
	// write payload to temp file
	tmpPath, err := a.imgr().NewTempFile(importmgr.ImportID(rand.Uint64()))
	if err != nil {
		return cid.Undef, err
	}
	defer os.Remove(tmpPath) //nolint:errcheck
	tmpF, err := os.Open(tmpPath)
	if err != nil {
		return cid.Undef, err
	}
	defer tmpF.Close() //nolint:errcheck
	if _, err := io.Copy(tmpF, r); err != nil {
		return cid.Undef, err
	}
	if err := tmpF.Close(); err != nil {
		return cid.Undef, err
	}

	res, err := a.ClientImport(ctx, api.FileRef{
		Path:  tmpPath,
		IsCAR: false,
	})
	if err != nil {
		return cid.Undef, err
	}
	return res.Root, nil
}

func (a *API) ClientListImports(ctx context.Context) ([]api.Import, error) {
	importIDs, err := a.imgr().List()
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch imports: %w", err)
	}

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
			Key:           id,
			Source:        info.Labels[importmgr.LSource],
			FilePath:      info.Labels[importmgr.LFileName],
			CARv2FilePath: info.Labels[importmgr.LFileStoreCARv2FilePath],
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

func (a *API) ClientCancelRetrievalDeal(ctx context.Context, dealID retrievalmarket.DealID) error {
	cerr := make(chan error)
	go func() {
		err := a.Retrieval.CancelDeal(dealID)

		select {
		case cerr <- err:
		case <-ctx.Done():
		}
	}()

	select {
	case err := <-cerr:
		if err != nil {
			return xerrors.Errorf("failed to cancel retrieval deal: %w", err)
		}

		return nil
	case <-ctx.Done():
		return xerrors.Errorf("context timeout while canceling retrieval deal: %w", ctx.Err())
	}
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
		case rm.DealStatusCancelled:
			return xerrors.Errorf("Retrieval was cancelled externally: %s", state.Message)
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

	var carV2FilePath string
	if order.LocalCARV2FilePath == "" {
		if order.MinerPeer == nil || order.MinerPeer.ID == "" {
			mi, err := a.StateMinerInfo(ctx, order.Miner, types.EmptyTSK)
			if err != nil {
				finish(err)
				return
			}

			order.MinerPeer = &retrievalmarket.RetrievalPeer{
				ID:      *mi.PeerId,
				Address: order.Miner,
			}
		}

		if order.Total.Int == nil {
			finish(xerrors.Errorf("cannot make retrieval deal for null total"))
			return
		}

		if order.Size == 0 {
			finish(xerrors.Errorf("cannot make retrieval deal for zero bytes"))
			return
		}

		ppb := types.BigDiv(order.Total, types.NewInt(order.Size))

		params, err := rm.NewParamsV1(ppb, order.PaymentInterval, order.PaymentIntervalIncrease, shared.AllSelector(), order.Piece, order.UnsealPrice)
		if err != nil {
			finish(xerrors.Errorf("Error in retrieval params: %s", err))
			return
		}

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

		resp, err := a.Retrieval.Retrieve(
			ctx,
			order.Root,
			params,
			order.Total,
			*order.MinerPeer,
			order.Client,
			order.Miner)

		if err != nil {
			unsubscribe()
			finish(xerrors.Errorf("Retrieve failed: %w", err))
			return
		}

		err = readSubscribeEvents(ctx, resp.DealID, subscribeEvents, events)

		unsubscribe()
		if err != nil {
			finish(xerrors.Errorf("Retrieve: %w", err))
			return
		}

		carV2FilePath = resp.CarFilePath
		// remove the temp CARv2 file when retrieval is complete
		defer os.Remove(carV2FilePath) //nolint:errcheck
	} else {
		carV2FilePath = order.LocalCARV2FilePath
	}

	// TODO We only support this currently for the IPFS Retrieval use case
	// where users want to write out filecoin retrievals directly to IPFS.
	// If users haven' configured the Ipfs retrieval flag, the blockstore we get here will be a "no-op" blockstore.
	// write out the CARv2 file to the retrieval block-store (which is really an IPFS node behind the scenes).
	rs, err := a.RetrievalStoreMgr.NewStore()
	defer a.RetrievalStoreMgr.ReleaseStore(rs) //nolint:errcheck
	if err != nil {
		finish(xerrors.Errorf("Error setting up new store: %w", err))
		return
	}
	if rs.IsIPFSRetrieval() {
		// write out the CARv1 blocks of the CARv2 file to the IPFS blockstore.
		carv2Reader, err := carv2.OpenReader(carV2FilePath)
		if err != nil {
			finish(err)
			return
		}
		defer carv2Reader.Close() //nolint:errcheck

		if _, err := car.LoadCar(rs.Blockstore(), carv2Reader.DataReader()); err != nil {
			finish(err)
			return
		}
	}

	// If ref is nil, it only fetches the data into the configured blockstore.
	if ref == nil {
		finish(nil)
		return
	}

	if ref.IsCAR {
		// user wants a CAR file, transform the CARv2 to a CARv1 and write it out.
		f, err := os.OpenFile(ref.Path, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			finish(err)
			return
		}

		carv2Reader, err := carv2.OpenReader(carV2FilePath)
		if err != nil {
			finish(err)
			return
		}
		defer carv2Reader.Close() //nolint:errcheck
		if _, err := io.Copy(f, carv2Reader.DataReader()); err != nil {
			finish(err)
			return
		}

		finish(f.Close())
		return
	}

	readOnly, err := blockstore.OpenReadOnly(carV2FilePath, carv2.ZeroLengthSectionAsEOF(true), blockstore.UseWholeCIDs(true))
	if err != nil {
		finish(err)
		return
	}
	defer readOnly.Close() //nolint:errcheck
	bsvc := blockservice.New(readOnly, offline.Exchange(readOnly))
	dag := merkledag.NewDAGService(bsvc)

	nd, err := dag.Get(ctx, order.Root)
	if err != nil {
		finish(xerrors.Errorf("ClientRetrieve: %w", err))
		return
	}
	file, err := unixfile.NewUnixfsFile(ctx, dag, nd)
	if err != nil {
		finish(xerrors.Errorf("ClientRetrieve: %w", err))
		return
	}
	finish(files.WriteTo(file, ref.Path))

	return
}

func (a *API) ClientListRetrievals(ctx context.Context) ([]api.RetrievalInfo, error) {
	deals, err := a.Retrieval.ListDeals()
	if err != nil {
		return nil, err
	}
	dataTransfersByID, err := a.transfersByID(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.RetrievalInfo, 0, len(deals))
	for _, v := range deals {
		// Find the data transfer associated with this deal
		var transferCh *api.DataTransferChannel
		if v.ChannelID != nil {
			if ch, ok := dataTransfersByID[*v.ChannelID]; ok {
				transferCh = &ch
			}
		}
		out = append(out, a.newRetrievalInfoWithTransfer(transferCh, v))
	}
	sort.Slice(out, func(a, b int) bool {
		return out[a].ID < out[b].ID
	})
	return out, nil
}

func (a *API) ClientGetRetrievalUpdates(ctx context.Context) (<-chan api.RetrievalInfo, error) {
	updates := make(chan api.RetrievalInfo)

	unsub := a.Retrieval.SubscribeToEvents(func(_ rm.ClientEvent, deal rm.ClientDealState) {
		updates <- a.newRetrievalInfo(ctx, deal)
	})

	go func() {
		defer unsub()
		<-ctx.Done()
	}()

	return updates, nil
}

func (a *API) newRetrievalInfoWithTransfer(ch *api.DataTransferChannel, deal rm.ClientDealState) api.RetrievalInfo {
	return api.RetrievalInfo{
		PayloadCID:        deal.PayloadCID,
		ID:                deal.ID,
		PieceCID:          deal.PieceCID,
		PricePerByte:      deal.PricePerByte,
		UnsealPrice:       deal.UnsealPrice,
		Status:            deal.Status,
		Message:           deal.Message,
		Provider:          deal.Sender,
		BytesReceived:     deal.TotalReceived,
		BytesPaidFor:      deal.BytesPaidFor,
		TotalPaid:         deal.FundsSpent,
		TransferChannelID: deal.ChannelID,
		DataTransfer:      ch,
	}
}

func (a *API) newRetrievalInfo(ctx context.Context, v rm.ClientDealState) api.RetrievalInfo {
	// Find the data transfer associated with this deal
	var transferCh *api.DataTransferChannel
	if v.ChannelID != nil {
		state, err := a.DataTransfer.ChannelState(ctx, *v.ChannelID)

		// Note: If there was an error just ignore it, as the data transfer may
		// be not found if it's no longer active
		if err == nil {
			ch := api.NewDataTransferChannel(a.Host.ID(), state)
			ch.Stages = state.Stages()
			transferCh = &ch
		}
	}

	return a.newRetrievalInfoWithTransfer(transferCh, v)
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

	// Hard-code the sector type to 32GiBV1_1, because:
	// - ffiwrapper.GeneratePieceCIDFromFile requires a RegisteredSealProof
	// - commP itself is sector-size independent, with rather low probability of that changing
	//   ( note how the final rust call is identical for every RegSP type )
	//   https://github.com/filecoin-project/rust-filecoin-proofs-api/blob/v5.0.0/src/seal.rs#L1040-L1050
	//
	// IF/WHEN this changes in the future we will have to be able to calculate
	// "old style" commP, and thus will need to introduce a version switch or similar
	arbitraryProofType := abi.RegisteredSealProof_StackedDrg32GiBV1_1

	rdr, err := os.Open(inpath)
	if err != nil {
		return nil, err
	}
	defer rdr.Close() //nolint:errcheck

	stat, err := rdr.Stat()
	if err != nil {
		return nil, err
	}

	// check that the data is a car file; if it's not, retrieval won't work
	_, _, err = car.ReadHeader(bufio.NewReader(rdr))
	if err != nil {
		return nil, xerrors.Errorf("not a car file: %w", err)
	}

	if _, err := rdr.Seek(0, io.SeekStart); err != nil {
		return nil, xerrors.Errorf("seek to start: %w", err)
	}

	pieceReader, pieceSize := padreader.New(rdr, uint64(stat.Size()))
	commP, err := ffiwrapper.GeneratePieceCIDFromFile(arbitraryProofType, pieceReader, pieceSize)

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
	fc, err := a.imgr().FilestoreCARV2FilePathFor(root)
	if err != nil {
		return api.DataSize{}, xerrors.Errorf("failed to find CARv2 file for root: %w", err)
	}
	if len(fc) == 0 {
		return api.DataSize{}, xerrors.New("no CARv2 file for root")
	}

	rdOnly, err := filestorecaradapter.NewReadOnlyFileStore(fc)
	if err != nil {
		return api.DataSize{}, xerrors.Errorf("failed to open read only filestore: %w", err)
	}
	defer rdOnly.Close() //nolint:errcheck

	dag := merkledag.NewDAGService(blockservice.New(rdOnly, offline.Exchange(rdOnly)))
	w := lenWriter(0)

	err = car.WriteCar(ctx, dag, []cid.Cid{root}, &w)
	if err != nil {
		return api.DataSize{}, err
	}

	up := padreader.PaddedSize(uint64(w))

	return api.DataSize{
		PayloadSize: int64(w),
		PieceSize:   up.Padded(),
	}, nil
}

func (a *API) ClientDealPieceCID(ctx context.Context, root cid.Cid) (api.DataCIDSize, error) {
	fc, err := a.imgr().FilestoreCARV2FilePathFor(root)
	if err != nil {
		return api.DataCIDSize{}, xerrors.Errorf("failed to find CARv2 file for root: %w", err)
	}
	if len(fc) == 0 {
		return api.DataCIDSize{}, xerrors.New("no CARv2 file for root")
	}

	rdOnly, err := filestorecaradapter.NewReadOnlyFileStore(fc)
	if err != nil {
		return api.DataCIDSize{}, xerrors.Errorf("failed to open read only blockstore: %w", err)
	}
	defer rdOnly.Close() //nolint:errcheck

	dag := merkledag.NewDAGService(blockservice.New(rdOnly, offline.Exchange(rdOnly)))
	w := &writer.Writer{}
	bw := bufio.NewWriterSize(w, int(writer.CommPBuf))

	err = car.WriteCar(ctx, dag, []cid.Cid{root}, w)
	if err != nil {
		return api.DataCIDSize{}, err
	}

	if err := bw.Flush(); err != nil {
		return api.DataCIDSize{}, err
	}

	dataCIDSize, err := w.Sum()
	return api.DataCIDSize(dataCIDSize), err
}

func (a *API) ClientGenCar(ctx context.Context, ref api.FileRef, outputPath string) error {
	id := importmgr.ImportID(rand.Uint64())
	tmpCARv2File, err := a.imgr().NewTempFile(id)
	if err != nil {
		return xerrors.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpCARv2File) //nolint:errcheck

	root, err := a.importNormalFileToFilestoreCARv2(ctx, id, ref.Path, tmpCARv2File)
	if err != nil {
		return xerrors.Errorf("failed to import normal file to CARv2")
	}

	// generate a deterministic CARv1 payload from the UnixFS DAG by doing an IPLD
	// traversal over the Unixfs DAG in the CARv2 file using the "all selector" i.e the entire DAG selector.
	fs, err := filestorecaradapter.NewReadOnlyFileStore(tmpCARv2File)
	if err != nil {
		return xerrors.Errorf("failed to open read only CARv2 blockstore: %w", err)
	}
	defer fs.Close() //nolint:errcheck

	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()
	sc := car.NewSelectiveCar(ctx, fs, []car.Dag{{Root: root, Selector: allSelector}})
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	if err = sc.Write(f); err != nil {
		return xerrors.Errorf("failed to write CAR to output file: %w", err)
	}

	return f.Close()
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

func (a *API) ClientCancelDataTransfer(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error {
	selfPeer := a.Host.ID()
	if isInitiator {
		return a.DataTransfer.CloseDataTransferChannel(ctx, datatransfer.ChannelID{Initiator: selfPeer, Responder: otherPeer, ID: transferID})
	}
	return a.DataTransfer.CloseDataTransferChannel(ctx, datatransfer.ChannelID{Initiator: otherPeer, Responder: selfPeer, ID: transferID})
}

func (a *API) ClientRetrieveTryRestartInsufficientFunds(ctx context.Context, paymentChannel address.Address) error {
	return a.Retrieval.TryRestartInsufficientFunds(paymentChannel)
}

func (a *API) ClientGetDealStatus(ctx context.Context, statusCode uint64) (string, error) {
	ststr, ok := storagemarket.DealStates[statusCode]
	if !ok {
		return "", fmt.Errorf("no such deal state %d", statusCode)
	}

	return ststr, nil
}
