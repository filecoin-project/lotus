package full

import (
	"context"
	"errors"
	"os"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	chunker "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/deals"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/filecoin-project/go-lotus/retrieval"
	"github.com/filecoin-project/go-lotus/retrieval/discovery"
)

type ClientAPI struct {
	fx.In

	ChainAPI
	StateAPI
	WalletAPI
	PaychAPI

	DealClient   *deals.Client
	RetDiscovery discovery.PeerResolver
	Retrieval    *retrieval.Client
	Chain        *store.ChainStore

	LocalDAG   dtypes.ClientDAG
	Blockstore dtypes.ClientBlockstore
	Filestore  dtypes.ClientFilestore `optional:"true"`
}

func (a *ClientAPI) ClientStartDeal(ctx context.Context, data cid.Cid, miner address.Address, price types.BigInt, blocksDuration uint64) (*cid.Cid, error) {
	// TODO: make this a param
	self, err := a.WalletDefaultAddress(ctx)
	if err != nil {
		return nil, err
	}

	// get miner peerID
	msg := &types.Message{
		To:     miner,
		From:   miner,
		Method: actors.MAMethods.GetPeerID,
	}

	r, err := a.StateCall(ctx, msg, nil)
	if err != nil {
		return nil, err
	}
	pid, err := peer.IDFromBytes(r.Return)
	if err != nil {
		return nil, err
	}

	vd, err := a.DealClient.VerifyParams(ctx, data)
	if err != nil {
		return nil, err
	}

	voucherData, err := cbor.DumpObject(vd)
	if err != nil {
		return nil, err
	}

	// setup payments
	total := types.BigMul(price, types.NewInt(blocksDuration))

	// TODO: at least ping the miner before creating paych / locking the money
	extra := &types.ModVerifyParams{
		Actor:  miner,
		Method: actors.MAMethods.PaymentVerifyInclusion,
		Data:   voucherData,
	}

	head := a.Chain.GetHeaviestTipSet()
	payment, err := a.PaychNewPayment(ctx, self, miner, total, extra, head.Height()+blocksDuration, head.Height()+blocksDuration)
	if err != nil {
		return nil, err
	}

	proposal := deals.ClientDealProposal{
		Data:       data,
		TotalPrice: total,
		Duration:   blocksDuration,
		Payment: actors.PaymentInfo{
			PayChActor: payment.Channel,
			Payer:      self,
			Vouchers:   []*types.SignedVoucher{payment.Voucher},
		},
		MinerAddress:  miner,
		ClientAddress: self,
		MinerID:       pid,
	}

	c, err := a.DealClient.Start(ctx, proposal, vd)
	// TODO: send updated voucher with PaymentVerifySector for cheaper validation (validate the sector the miner sent us first!)
	return &c, err
}

func (a *ClientAPI) ClientHasLocal(ctx context.Context, root cid.Cid) (bool, error) {
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

func (a *ClientAPI) ClientFindData(ctx context.Context, root cid.Cid) ([]api.QueryOffer, error) {
	peers, err := a.RetDiscovery.GetPeers(root)
	if err != nil {
		return nil, err
	}

	out := make([]api.QueryOffer, len(peers))
	for k, p := range peers {
		out[k] = a.Retrieval.Query(ctx, p, root)
	}

	return out, nil
}

func (a *ClientAPI) ClientImport(ctx context.Context, path string) (cid.Cid, error) {
	f, err := os.Open(path)
	if err != nil {
		return cid.Undef, err
	}
	stat, err := f.Stat()
	if err != nil {
		return cid.Undef, err
	}

	file, err := files.NewReaderPathFile(path, f, stat)
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

	return nd.Cid(), bufferedDS.Commit()
}

func (a *ClientAPI) ClientListImports(ctx context.Context) ([]api.Import, error) {
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

func (a *ClientAPI) ClientRetrieve(ctx context.Context, order api.RetrievalOrder, path string) error {
	outFile, err := os.OpenFile(path, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return err
	}

	err = a.Retrieval.RetrieveUnixfs(ctx, order.Root, order.Size, order.MinerPeerID, order.Miner, outFile)
	if err != nil {
		_ = outFile.Close()
		return err
	}

	return outFile.Close()
}
