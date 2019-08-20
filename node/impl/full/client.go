package full

import (
	"context"
	"errors"
	"os"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/deals"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	chunker "github.com/ipfs/go-ipfs-chunker"
	files "github.com/ipfs/go-ipfs-files"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/fx"
)

type ClientAPI struct {
	fx.In

	ChainAPI
	WalletAPI
	PaychAPI

	DealClient *deals.Client

	LocalDAG  dtypes.ClientDAG
	Filestore dtypes.ClientFilestore `optional:"true"`
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

	r, err := a.ChainCall(ctx, msg, nil)
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
	paych, paychMsg, err := a.paychCreate(ctx, self, miner, total)
	if err != nil {
		return nil, err
	}

	voucher := types.SignedVoucher{
		// TimeLock:       0, // TODO: do we want to use this somehow?
		Extra: &types.ModVerifyParams{
			Actor:  miner,
			Method: actors.MAMethods.PaymentVerifyInclusion,
			Data:   voucherData,
		},
		Lane:           0,
		Amount:         total,
		MinCloseHeight: blocksDuration, // TODO: some way to start this after initial piece inclusion by actor? (also, at least add current height)
	}

	sv, err := a.paychVoucherCreate(ctx, paych, voucher)
	if err != nil {
		return nil, err
	}

	proposal := deals.ClientDealProposal{
		Data:       data,
		TotalPrice: total,
		Duration:   blocksDuration,
		Payment: actors.PaymentInfo{
			PayChActor:     paych,
			Payer:          self,
			ChannelMessage: paychMsg,
			Vouchers:       []types.SignedVoucher{*sv},
		},
		MinerAddress:  miner,
		ClientAddress: self,
		MinerID:       pid,
	}

	c, err := a.DealClient.Start(ctx, proposal, vd)
	// TODO: send updated voucher with PaymentVerifySector for cheaper validation (validate the sector the miner sent us first!)
	return &c, err
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
		Maxlinks:   ihelper.DefaultLinksPerBlock,
		RawLeaves:  true,
		CidBuilder: nil,
		Dagserv:    bufferedDS,
		NoCopy:     true,
	}

	db, err := params.New(chunker.DefaultSplitter(file))
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
