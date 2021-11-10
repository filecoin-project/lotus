package client

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	bstore "github.com/ipfs/go-ipfs-blockstore"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-car"
	carv2 "github.com/ipld/go-car/v2"
	carv2bs "github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	textselector "github.com/ipld/go-ipld-selector-text-lite"
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
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/stores"

	"github.com/filecoin-project/lotus/markets/retrievaladapter"
	"github.com/filecoin-project/lotus/markets/storageadapter"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/v3/actors/builtin/market"

	marketevents "github.com/filecoin-project/lotus/markets/loggers"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo/imports"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("client")

var DefaultHashFunction = uint64(mh.BLAKE2B_MIN + 31)

// 8 days ~=  SealDuration + PreCommit + MaxProveCommitDuration + 8 hour buffer
const dealStartBufferHours uint64 = 8 * 24
const DefaultDAGStoreDir = "dagstore"

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

	// accessors for imports and retrievals.
	Imports                   dtypes.ClientImportMgr
	StorageBlockstoreAccessor storagemarket.BlockstoreAccessor
	RtvlBlockstoreAccessor    rm.BlockstoreAccessor

	DataTransfer dtypes.ClientDataTransfer
	Host         host.Host

	Repo repo.LockedRepo
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

// importManager converts the injected type to the required type.
func (a *API) importManager() *imports.Manager {
	return a.Imports
}

func (a *API) ClientStartDeal(ctx context.Context, params *api.StartDealParams) (*cid.Cid, error) {
	return a.dealStarter(ctx, params, false)
}

func (a *API) ClientStatelessDeal(ctx context.Context, params *api.StartDealParams) (*cid.Cid, error) {
	return a.dealStarter(ctx, params, true)
}

func (a *API) dealStarter(ctx context.Context, params *api.StartDealParams, isStateless bool) (*cid.Cid, error) {
	if isStateless {
		if params.Data.TransferType != storagemarket.TTManual {
			return nil, xerrors.Errorf("invalid transfer type %s for stateless storage deal", params.Data.TransferType)
		}
		if !params.EpochPrice.IsZero() {
			return nil, xerrors.New("stateless storage deals can only be initiated with storage price of 0")
		}
	} else if params.Data.TransferType == storagemarket.TTGraphsync {
		bs, onDone, err := a.dealBlockstore(params.Data.Root)
		if err != nil {
			return nil, xerrors.Errorf("failed to find blockstore for root CID: %w", err)
		}
		if has, err := bs.Has(params.Data.Root); err != nil {
			return nil, xerrors.Errorf("failed to query blockstore for root CID: %w", err)
		} else if !has {
			return nil, xerrors.Errorf("failed to find root CID in blockstore: %w", err)
		}
		onDone()
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
			Addr:          params.Wallet,
			Info:          &providerInfo,
			Data:          params.Data,
			StartEpoch:    dealStart,
			EndEpoch:      calcDealExpiration(params.MinBlocksDuration, md, dealStart),
			Price:         params.EpochPrice,
			Collateral:    params.ProviderCollateral,
			Rt:            st,
			FastRetrieval: params.FastRetrieval,
			VerifiedDeal:  params.VerifiedDeal,
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

func (a *API) ClientHasLocal(_ context.Context, root cid.Cid) (bool, error) {
	_, onDone, err := a.dealBlockstore(root)
	if err != nil {
		return false, err
	}
	onDone()
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

		// do not rely on local data with respect to peer id
		// fetch an up-to-date miner peer id from chain
		mi, err := a.StateMinerInfo(ctx, p.Address, types.EmptyTSK)
		if err != nil {
			return nil, err
		}
		pp := rm.RetrievalPeer{
			Address: p.Address,
			ID:      *mi.PeerId,
		}

		out = append(out, a.makeRetrievalQuery(ctx, pp, root, piece, rm.QueryParams{}))
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

func (a *API) ClientImport(ctx context.Context, ref api.FileRef) (res *api.ImportRes, err error) {
	var (
		imgr    = a.importManager()
		id      imports.ID
		root    cid.Cid
		carPath string
	)

	id, err = imgr.CreateImport()
	if err != nil {
		return nil, xerrors.Errorf("failed to create import: %w", err)
	}

	if ref.IsCAR {
		// user gave us a CAR file, use it as-is
		// validate that it's either a carv1 or carv2, and has one root.
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

		carPath = ref.Path
		root = hd.Roots[0]
	} else {
		carPath, err = imgr.AllocateCAR(id)
		if err != nil {
			return nil, xerrors.Errorf("failed to create car path for import: %w", err)
		}

		// remove the import if something went wrong.
		defer func() {
			if err != nil {
				_ = os.Remove(carPath)
				_ = imgr.Remove(id)
			}
		}()

		// perform the unixfs chunking.
		root, err = a.createUnixFSFilestore(ctx, ref.Path, carPath)
		if err != nil {
			return nil, xerrors.Errorf("failed to import file using unixfs: %w", err)
		}
	}

	if err = imgr.AddLabel(id, imports.LSource, "import"); err != nil {
		return nil, err
	}
	if err = imgr.AddLabel(id, imports.LFileName, ref.Path); err != nil {
		return nil, err
	}
	if err = imgr.AddLabel(id, imports.LCARPath, carPath); err != nil {
		return nil, err
	}
	if err = imgr.AddLabel(id, imports.LRootCid, root.String()); err != nil {
		return nil, err
	}
	return &api.ImportRes{
		Root:     root,
		ImportID: id,
	}, nil
}

func (a *API) ClientRemoveImport(ctx context.Context, id imports.ID) error {
	info, err := a.importManager().Info(id)
	if err != nil {
		return xerrors.Errorf("failed to get import metadata: %w", err)
	}

	owner := info.Labels[imports.LCAROwner]
	path := info.Labels[imports.LCARPath]

	// CARv2 file was not provided by the user, delete it.
	if path != "" && owner == imports.CAROwnerImportMgr {
		_ = os.Remove(path)
	}

	return a.importManager().Remove(id)
}

// ClientImportLocal imports a standard file into this node as a UnixFS payload,
// storing it in a CARv2 file. Note that this method is NOT integrated with the
// IPFS blockstore. That is, if client-side IPFS integration is enabled, this
// method won't import the file into that
func (a *API) ClientImportLocal(ctx context.Context, r io.Reader) (cid.Cid, error) {
	file := files.NewReaderFile(r)

	// write payload to temp file
	id, err := a.importManager().CreateImport()
	if err != nil {
		return cid.Undef, err
	}
	if err := a.importManager().AddLabel(id, imports.LSource, "import-local"); err != nil {
		return cid.Undef, err
	}

	path, err := a.importManager().AllocateCAR(id)
	if err != nil {
		return cid.Undef, err
	}

	// writing a carv2 requires knowing the root ahead of time, which makes
	// streaming cases impossible.
	// https://github.com/ipld/go-car/issues/196
	// we work around this limitation by informing a placeholder root CID of the
	// same length as our unixfs chunking strategy will generate.
	// once the DAG is formed and the root is calculated, we overwrite the
	// inner carv1 header with the final root.

	b, err := unixFSCidBuilder()
	if err != nil {
		return cid.Undef, err
	}

	// placeholder payload needs to be larger than inline CID threshold; 256
	// bytes is a safe value.
	placeholderRoot, err := b.Sum(make([]byte, 256))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to calculate placeholder root: %w", err)
	}

	bs, err := carv2bs.OpenReadWrite(path, []cid.Cid{placeholderRoot}, carv2bs.UseWholeCIDs(true))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create carv2 read/write blockstore: %w", err)
	}

	root, err := buildUnixFS(ctx, file, bs, false)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to build unixfs dag: %w", err)
	}

	err = bs.Finalize()
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to finalize carv2 read/write blockstore: %w", err)
	}

	// record the root in the import manager.
	if err := a.importManager().AddLabel(id, imports.LRootCid, root.String()); err != nil {
		return cid.Undef, xerrors.Errorf("failed to record root CID in import manager: %w", err)
	}

	// now go ahead and overwrite the root in the carv1 header.
	reader, err := carv2.OpenReader(path)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create car reader: %w", err)
	}

	// save the header offset.
	headerOff := reader.Header.DataOffset

	// read the old header.
	dr := reader.DataReader()
	header, err := readHeader(dr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to read car reader: %w", err)
	}
	_ = reader.Close() // close the CAR reader.

	// write the old header into a buffer.
	var oldBuf bytes.Buffer
	if err = writeHeader(header, &oldBuf); err != nil {
		return cid.Undef, xerrors.Errorf("failed to write header into buffer: %w", err)
	}

	// replace the root.
	header.Roots = []cid.Cid{root}

	// write the new header into a buffer.
	var newBuf bytes.Buffer
	err = writeHeader(header, &newBuf)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to write header into buffer: %w", err)
	}

	// verify the length matches.
	if newBuf.Len() != oldBuf.Len() {
		return cid.Undef, xerrors.Errorf("failed to replace carv1 header; length mismatch (old: %d, new: %d)", oldBuf.Len(), newBuf.Len())
	}

	// open the file again, seek to the header position, and write.
	f, err := os.OpenFile(path, os.O_WRONLY, 0755)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to open car: %w", err)
	}
	defer f.Close() //nolint:errcheck

	n, err := f.WriteAt(newBuf.Bytes(), int64(headerOff))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to write new header to car (bytes written: %d): %w", n, err)
	}
	return root, nil
}

func (a *API) ClientListImports(_ context.Context) ([]api.Import, error) {
	ids, err := a.importManager().List()
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch imports: %w", err)
	}

	out := make([]api.Import, len(ids))
	for i, id := range ids {
		info, err := a.importManager().Info(id)
		if err != nil {
			out[i] = api.Import{
				Key: id,
				Err: xerrors.Errorf("getting info: %w", err).Error(),
			}
			continue
		}

		ai := api.Import{
			Key:      id,
			Source:   info.Labels[imports.LSource],
			FilePath: info.Labels[imports.LFileName],
			CARPath:  info.Labels[imports.LCARPath],
		}

		if info.Labels[imports.LRootCid] != "" {
			c, err := cid.Parse(info.Labels[imports.LRootCid])
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

func (a *API) ClientCancelRetrievalDeal(ctx context.Context, dealID rm.DealID) error {
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

func consumeAllEvents(ctx context.Context, dealID rm.DealID, subscribeEvents chan retrievalSubscribeEvent, events chan marketevents.RetrievalEvent) error {
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

	sel := selectorparse.CommonSelector_ExploreAllRecursively
	if order.DatamodelPathSelector != nil {

		ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)

		selspec, err := textselector.SelectorSpecFromPath(

			*order.DatamodelPathSelector,

			// URGH - this is a direct copy from https://github.com/filecoin-project/go-fil-markets/blob/v1.12.0/shared/selectors.go#L10-L16
			// Unable to use it because we need the SelectorSpec, and markets exposes just a reified node
			ssb.ExploreRecursive(
				selector.RecursionLimitNone(),
				ssb.ExploreAll(ssb.ExploreRecursiveEdge()),
			),
		)
		if err != nil {
			finish(xerrors.Errorf("failed to parse text-selector '%s': %w", *order.DatamodelPathSelector, err))
			return
		}

		sel = selspec.Node()
		log.Infof("partial retrieval of datamodel-path-selector %s/*", *order.DatamodelPathSelector)
	}

	// summary:
	// 1. if we're retrieving from an import, FromLocalCAR will be set.
	//    Skip the retrieval itself, and use the provided car as a blockstore further down
	//    to extract a CAR or UnixFS export from.
	// 2. if we're using an IPFS blockstore for retrieval, retrieve into it,
	//    then use the virtual blockstore to extract a CAR or UnixFS export from it.
	// 3. if we have to retrieve, perform a CARv2 retrieval, then either
	//    extract the CARv1 (with ExtractV1File) or use it as a blockstore further down.

	// this indicates we're proxying to IPFS.
	proxyBss, retrieveIntoIPFS := a.RtvlBlockstoreAccessor.(*retrievaladapter.ProxyBlockstoreAccessor)

	carBss, retrieveIntoCAR := a.RtvlBlockstoreAccessor.(*retrievaladapter.CARBlockstoreAccessor)

	carPath := order.FromLocalCAR

	// we actually need to retrieve from the network
	if carPath == "" {

		if !retrieveIntoIPFS && !retrieveIntoCAR {
			// we don't recognize the blockstore accessor.
			finish(xerrors.Errorf("unsupported retrieval blockstore accessor"))
			return
		}

		if order.MinerPeer == nil || order.MinerPeer.ID == "" {
			mi, err := a.StateMinerInfo(ctx, order.Miner, types.EmptyTSK)
			if err != nil {
				finish(err)
				return
			}

			order.MinerPeer = &rm.RetrievalPeer{
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

		params, err := rm.NewParamsV1(ppb, order.PaymentInterval, order.PaymentIntervalIncrease, sel, order.Piece, order.UnsealPrice)
		if err != nil {
			finish(xerrors.Errorf("Error in retrieval params: %s", err))
			return
		}

		// Subscribe to events before retrieving to avoid losing events.
		subscribeEvents := make(chan retrievalSubscribeEvent, 1)
		subscribeCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		unsubscribe := a.Retrieval.SubscribeToEvents(func(event rm.ClientEvent, state rm.ClientDealState) {
			// We'll check the deal IDs inside consumeAllEvents.
			if state.PayloadCID.Equals(order.Root) {
				select {
				case <-subscribeCtx.Done():
				case subscribeEvents <- retrievalSubscribeEvent{event, state}:
				}
			}
		})

		id := a.Retrieval.NextID()
		id, err = a.Retrieval.Retrieve(
			ctx,
			id,
			order.Root,
			params,
			order.Total,
			*order.MinerPeer,
			order.Client,
			order.Miner,
		)

		if err != nil {
			unsubscribe()
			finish(xerrors.Errorf("Retrieve failed: %w", err))
			return
		}

		err = consumeAllEvents(ctx, id, subscribeEvents, events)

		unsubscribe()
		if err != nil {
			finish(xerrors.Errorf("Retrieve: %w", err))
			return
		}

		if retrieveIntoCAR {
			carPath = carBss.PathFor(id)
		}
	}

	if ref == nil {
		// If ref is nil, it only fetches the data into the configured blockstore
		// (if fetching from network).
		finish(nil)
		return
	}

	// determine where did the retrieval go
	var retrievalBs bstore.Blockstore
	if retrieveIntoIPFS {
		retrievalBs = proxyBss.Blockstore
	} else {
		cbs, err := stores.ReadOnlyFilestore(carPath)
		if err != nil {
			finish(err)
			return
		}
		defer cbs.Close() //nolint:errcheck
		retrievalBs = cbs
	}

	// Are we outputting a CAR?
	if ref.IsCAR {

		// not IPFS and we do full selection - just extract the CARv1 from the CARv2 we stored the retrieval in
		if !retrieveIntoIPFS && order.DatamodelPathSelector == nil {
			finish(carv2.ExtractV1File(carPath, ref.Path))
			return
		}

		// generating a CARv1 from the configured blockstore
		f, err := os.OpenFile(ref.Path, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			finish(err)
			return
		}

		err = car.NewSelectiveCar(
			ctx,
			retrievalBs,
			[]car.Dag{{
				Root:     order.Root,
				Selector: sel,
			}},
			car.MaxTraversalLinks(config.MaxTraversalLinks),
		).Write(f)
		if err != nil {
			finish(err)
			return
		}

		finish(f.Close())
		return
	}

	// we are extracting a UnixFS file.
	ds := merkledag.NewDAGService(blockservice.New(retrievalBs, offline.Exchange(retrievalBs)))
	root := order.Root

	// if we used a selector - need to find the sub-root the user actually wanted to retrieve
	if order.DatamodelPathSelector != nil {

		var subRootFound bool

		// no err check - we just compiled this before starting, but now we do not wrap a `*`
		selspec, _ := textselector.SelectorSpecFromPath(*order.DatamodelPathSelector, nil) //nolint:errcheck
		if err := utils.TraverseDag(
			ctx,
			ds,
			root,
			selspec.Node(),
			func(p traversal.Progress, n ipld.Node, r traversal.VisitReason) error {
				if r == traversal.VisitReason_SelectionMatch {

					if p.LastBlock.Path.String() != p.Path.String() {
						return xerrors.Errorf("unsupported selection path '%s' does not correspond to a block boundary (a.k.a. CID link)", p.Path.String())
					}

					cidLnk, castOK := p.LastBlock.Link.(cidlink.Link)
					if !castOK {
						return xerrors.Errorf("cidlink cast unexpectedly failed on '%s'", p.LastBlock.Link.String())
					}

					root = cidLnk.Cid
					subRootFound = true
				}
				return nil
			},
		); err != nil {
			finish(xerrors.Errorf("error while locating partial retrieval sub-root: %w", err))
			return
		}

		if !subRootFound {
			finish(xerrors.Errorf("path selection '%s' does not match a node within %s", *order.DatamodelPathSelector, root))
			return
		}
	}

	nd, err := ds.Get(ctx, root)
	if err != nil {
		finish(xerrors.Errorf("ClientRetrieve: %w", err))
		return
	}
	file, err := unixfile.NewUnixfsFile(ctx, ds, nd)
	if err != nil {
		finish(xerrors.Errorf("ClientRetrieve: %w", err))
		return
	}

	finish(files.WriteTo(file, ref.Path))
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
	bs, onDone, err := a.dealBlockstore(root)
	if err != nil {
		return api.DataSize{}, err
	}
	defer onDone()

	dag := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	var w lenWriter
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
	bs, onDone, err := a.dealBlockstore(root)
	if err != nil {
		return api.DataCIDSize{}, err
	}
	defer onDone()

	dag := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
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
	// create a temporary import to represent this job and obtain a staging CAR.
	id, err := a.importManager().CreateImport()
	if err != nil {
		return xerrors.Errorf("failed to create temporary import: %w", err)
	}
	defer a.importManager().Remove(id) //nolint:errcheck

	tmp, err := a.importManager().AllocateCAR(id)
	if err != nil {
		return xerrors.Errorf("failed to allocate temporary CAR: %w", err)
	}
	defer os.Remove(tmp) //nolint:errcheck

	// generate and import the UnixFS DAG into a filestore (positional reference) CAR.
	root, err := a.createUnixFSFilestore(ctx, ref.Path, tmp)
	if err != nil {
		return xerrors.Errorf("failed to import file using unixfs: %w", err)
	}

	// open the positional reference CAR as a filestore.
	fs, err := stores.ReadOnlyFilestore(tmp)
	if err != nil {
		return xerrors.Errorf("failed to open filestore from carv2 in path %s: %w", tmp, err)
	}
	defer fs.Close() //nolint:errcheck

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}

	// build a dense deterministic CAR (dense = containing filled leaves)
	if err := car.NewSelectiveCar(
		ctx,
		fs,
		[]car.Dag{{
			Root:     root,
			Selector: selectorparse.CommonSelector_ExploreAllRecursively,
		}},
		car.MaxTraversalLinks(config.MaxTraversalLinks),
	).Write(
		f,
	); err != nil {
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

// dealBlockstore picks the source blockstore for a storage deal; either the
// IPFS blockstore, or an import CARv2 file. It also returns a function that
// must be called when done.
func (a *API) dealBlockstore(root cid.Cid) (bstore.Blockstore, func(), error) {
	switch acc := a.StorageBlockstoreAccessor.(type) {
	case *storageadapter.ImportsBlockstoreAccessor:
		bs, err := acc.Get(root)
		if err != nil {
			return nil, nil, xerrors.Errorf("no import found for root %s: %w", root, err)
		}

		doneFn := func() {
			_ = acc.Done(root) //nolint:errcheck
		}
		return bs, doneFn, nil

	case *storageadapter.ProxyBlockstoreAccessor:
		return acc.Blockstore, func() {}, nil

	default:
		return nil, nil, xerrors.Errorf("unsupported blockstore accessor type: %T", acc)
	}
}
