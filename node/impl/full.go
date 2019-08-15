package impl

import (
	"context"
	"fmt"
	"strconv"

	"github.com/filecoin-project/go-lotus/lib/bufbstore"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/deals"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"github.com/filecoin-project/go-lotus/chain/wallet"
	"github.com/filecoin-project/go-lotus/miner"
	"github.com/filecoin-project/go-lotus/node/client"
	"github.com/filecoin-project/go-lotus/paych"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"golang.org/x/xerrors"
)

var log = logging.Logger("node")

type FullNodeAPI struct {
	client.LocalStorage

	CommonAPI

	DealClient *deals.Client
	Chain      *store.ChainStore
	PubSub     *pubsub.PubSub
	Mpool      *chain.MessagePool
	Wallet     *wallet.Wallet
	PaychMgr   *paych.Manager
}

func (a *FullNodeAPI) ClientStartDeal(ctx context.Context, data cid.Cid, miner address.Address, price types.BigInt, blocksDuration uint64) (*cid.Cid, error) {
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
			Method: actors.MAMethods.PaymentVerify,
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
	return &c, err
}

func (a *FullNodeAPI) ChainNotify(ctx context.Context) (<-chan *store.HeadChange, error) {
	return a.Chain.SubHeadChanges(ctx), nil
}

func (a *FullNodeAPI) ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error {
	if err := a.Chain.AddBlock(blk.Header); err != nil {
		return err
	}

	b, err := blk.Serialize()
	if err != nil {
		return err
	}

	// TODO: anything else to do here?
	return a.PubSub.Publish("/fil/blocks", b)
}

func (a *FullNodeAPI) ChainHead(context.Context) (*types.TipSet, error) {
	return a.Chain.GetHeaviestTipSet(), nil
}

func (a *FullNodeAPI) ChainGetRandomness(ctx context.Context, pts *types.TipSet) ([]byte, error) {
	// TODO: this needs to look back in the chain for the right random beacon value
	return []byte("foo bar random"), nil
}

func (a *FullNodeAPI) ChainWaitMsg(ctx context.Context, msg cid.Cid) (*api.MsgWait, error) {
	blkcid, recpt, err := a.Chain.WaitForMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &api.MsgWait{
		InBlock: blkcid,
		Receipt: *recpt,
	}, nil
}

func (a *FullNodeAPI) ChainGetBlock(ctx context.Context, msg cid.Cid) (*types.BlockHeader, error) {
	return a.Chain.GetBlock(msg)
}

func (a *FullNodeAPI) ChainGetBlockMessages(ctx context.Context, msg cid.Cid) (*api.BlockMessages, error) {
	b, err := a.Chain.GetBlock(msg)
	if err != nil {
		return nil, err
	}

	bmsgs, smsgs, err := a.Chain.MessagesForBlock(b)
	if err != nil {
		return nil, err
	}

	return &api.BlockMessages{
		BlsMessages:   bmsgs,
		SecpkMessages: smsgs,
	}, nil
}

func (a *FullNodeAPI) ChainGetBlockReceipts(ctx context.Context, bcid cid.Cid) ([]*types.MessageReceipt, error) {
	b, err := a.Chain.GetBlock(bcid)
	if err != nil {
		return nil, err
	}

	// TODO: need to get the number of messages better than this
	bm, sm, err := a.Chain.MessagesForBlock(b)
	if err != nil {
		return nil, err
	}

	var out []*types.MessageReceipt
	for i := 0; i < len(bm)+len(sm); i++ {
		r, err := a.Chain.GetReceipt(b, i)
		if err != nil {
			return nil, err
		}

		out = append(out, r)
	}

	return out, nil
}

func (a *FullNodeAPI) ChainCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	return vm.Call(ctx, a.Chain, msg, ts)
}

func (a *FullNodeAPI) stateForTs(ts *types.TipSet) (*state.StateTree, error) {
	if ts == nil {
		ts = a.Chain.GetHeaviestTipSet()
	}

	st, err := a.Chain.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	buf := bufbstore.NewBufferedBstore(a.Chain.Blockstore())
	cst := hamt.CSTFromBstore(buf)
	return state.LoadStateTree(cst, st)
}

func (a *FullNodeAPI) ChainGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error) {
	state, err := a.stateForTs(ts)
	if err != nil {
		return nil, err
	}

	return state.GetActor(actor)
}

func (a *FullNodeAPI) ChainReadState(ctx context.Context, act *types.Actor, ts *types.TipSet) (*api.ActorState, error) {
	state, err := a.stateForTs(ts)
	if err != nil {
		return nil, err
	}

	var oif interface{}
	if err := state.Store.Get(context.TODO(), act.Head, &oif); err != nil {
		return nil, err
	}

	return &api.ActorState{
		Balance: act.Balance,
		State:   oif,
	}, nil
}

func (a *FullNodeAPI) MpoolPending(ctx context.Context, ts *types.TipSet) ([]*types.SignedMessage, error) {
	// TODO: need to make sure we don't return messages that were already included in the referenced chain
	// also need to accept ts == nil just fine, assume nil == chain.Head()
	return a.Mpool.Pending(), nil
}

func (a *FullNodeAPI) MpoolPush(ctx context.Context, smsg *types.SignedMessage) error {
	msgb, err := smsg.Serialize()
	if err != nil {
		return err
	}
	if err := a.Mpool.Add(smsg); err != nil {
		return err
	}

	return a.PubSub.Publish("/fil/messages", msgb)
}

func (a *FullNodeAPI) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	return a.Mpool.GetNonce(addr)
}

func (a *FullNodeAPI) MinerStart(ctx context.Context, addr address.Address) error {
	// hrm...
	m := miner.NewMiner(a, addr)

	go m.Mine(context.TODO())

	return nil
}

func (a *FullNodeAPI) MinerCreateBlock(ctx context.Context, addr address.Address, parents *types.TipSet, tickets []types.Ticket, proof types.ElectionProof, msgs []*types.SignedMessage) (*chain.BlockMsg, error) {
	fblk, err := gen.MinerCreateBlock(ctx, a.Chain, addr, parents, tickets, proof, msgs)
	if err != nil {
		return nil, err
	}

	var out chain.BlockMsg
	out.Header = fblk.Header
	for _, msg := range fblk.BlsMessages {
		out.BlsMessages = append(out.BlsMessages, msg.Cid())
	}
	for _, msg := range fblk.SecpkMessages {
		out.SecpkMessages = append(out.SecpkMessages, msg.Cid())
	}

	return &out, nil
}

func (a *FullNodeAPI) WalletNew(ctx context.Context, typ string) (address.Address, error) {
	return a.Wallet.GenerateKey(typ)
}

func (a *FullNodeAPI) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	return a.Wallet.HasKey(addr)
}

func (a *FullNodeAPI) WalletList(ctx context.Context) ([]address.Address, error) {
	return a.Wallet.ListAddrs()
}

func (a *FullNodeAPI) WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error) {
	return a.Chain.GetBalance(addr)
}

func (a *FullNodeAPI) WalletSign(ctx context.Context, k address.Address, msg []byte) (*types.Signature, error) {
	return a.Wallet.Sign(k, msg)
}

func (a *FullNodeAPI) WalletSignMessage(ctx context.Context, k address.Address, msg *types.Message) (*types.SignedMessage, error) {
	msgbytes, err := msg.Serialize()
	if err != nil {
		return nil, err
	}

	sig, err := a.WalletSign(ctx, k, msgbytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign message: %w", err)
	}

	return &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}, nil
}

func (a *FullNodeAPI) WalletDefaultAddress(ctx context.Context) (address.Address, error) {
	addrs, err := a.Wallet.ListAddrs()
	if err != nil {
		return address.Undef, err
	}
	if len(addrs) == 0 {
		return address.Undef, xerrors.New("no addresses in wallet")
	}

	// TODO: store a default address in the config or 'wallet' portion of the repo
	return addrs[0], nil
}

func (a *FullNodeAPI) StateMinerSectors(ctx context.Context, addr address.Address) ([]*api.SectorInfo, error) {
	ts := a.Chain.GetHeaviestTipSet()

	stc, err := a.Chain.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(a.Chain.Blockstore())

	st, err := state.LoadStateTree(cst, stc)
	if err != nil {
		return nil, err
	}

	act, err := st.GetActor(addr)
	if err != nil {
		return nil, err
	}

	var minerState actors.StorageMinerActorState
	if err := cst.Get(ctx, act.Head, &minerState); err != nil {
		return nil, err
	}

	nd, err := hamt.LoadNode(ctx, cst, minerState.Sectors)
	if err != nil {
		return nil, err
	}

	log.Info("miner sector count: ", minerState.SectorSetSize)

	var sinfos []*api.SectorInfo
	// Note to self: the hamt isnt a great data structure to use here... need to implement the sector set
	err = nd.ForEach(ctx, func(k string, val interface{}) error {
		sid, err := strconv.ParseUint(k, 10, 64)
		if err != nil {
			return err
		}

		bval, ok := val.([]byte)
		if !ok {
			return fmt.Errorf("expected to get bytes in sector set hamt")
		}

		var comms [][]byte
		if err := cbor.DecodeInto(bval, &comms); err != nil {
			return err
		}

		sinfos = append(sinfos, &api.SectorInfo{
			SectorID: sid,
			CommR:    comms[0],
			CommD:    comms[1],
		})
		return nil
	})
	return sinfos, nil
}

func (a *FullNodeAPI) StateMinerProvingSet(ctx context.Context, addr address.Address) ([]*api.SectorInfo, error) {
	ts := a.Chain.GetHeaviestTipSet()

	stc, err := a.Chain.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(a.Chain.Blockstore())

	st, err := state.LoadStateTree(cst, stc)
	if err != nil {
		return nil, err
	}

	act, err := st.GetActor(addr)
	if err != nil {
		return nil, err
	}

	var minerState actors.StorageMinerActorState
	if err := cst.Get(ctx, act.Head, &minerState); err != nil {
		return nil, err
	}

	nd, err := hamt.LoadNode(ctx, cst, minerState.ProvingSet)
	if err != nil {
		return nil, err
	}

	var sinfos []*api.SectorInfo
	// Note to self: the hamt isnt a great data structure to use here... need to implement the sector set
	err = nd.ForEach(ctx, func(k string, val interface{}) error {
		sid, err := strconv.ParseUint(k, 10, 64)
		if err != nil {
			return err
		}

		bval, ok := val.([]byte)
		if !ok {
			return fmt.Errorf("expected to get bytes in sector set hamt")
		}

		var comms [][]byte
		if err := cbor.DecodeInto(bval, &comms); err != nil {
			return err
		}

		sinfos = append(sinfos, &api.SectorInfo{
			SectorID: sid,
			CommR:    comms[0],
			CommD:    comms[1],
		})
		return nil
	})
	return sinfos, nil
}

func (a *FullNodeAPI) PaychCreate(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, error) {
	act, _, err := a.paychCreate(ctx, from, to, amt)
	return act, err
}

func (a *FullNodeAPI) paychCreate(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, cid.Cid, error) {
	params, aerr := actors.SerializeParams(&actors.PCAConstructorParams{To: to})
	if aerr != nil {
		return address.Undef, cid.Undef, aerr
	}

	nonce, err := a.MpoolGetNonce(ctx, from)
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	enc, err := actors.SerializeParams(&actors.ExecParams{
		Params: params,
		Code:   actors.PaymentChannelActorCodeCid,
	})

	msg := &types.Message{
		To:       actors.InitActorAddress,
		From:     from,
		Value:    amt,
		Nonce:    nonce,
		Method:   actors.IAMethods.Exec,
		Params:   enc,
		GasLimit: types.NewInt(1000),
		GasPrice: types.NewInt(0),
	}

	ser, err := msg.Serialize()
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	sig, err := a.WalletSign(ctx, from, ser)
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	smsg := &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}

	if err := a.MpoolPush(ctx, smsg); err != nil {
		return address.Undef, cid.Undef, err
	}

	mwait, err := a.ChainWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	if mwait.Receipt.ExitCode != 0 {
		return address.Undef, cid.Undef, fmt.Errorf("payment channel creation failed (exit code %d)", mwait.Receipt.ExitCode)
	}

	paychaddr, err := address.NewFromBytes(mwait.Receipt.Return)
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	if err := a.PaychMgr.TrackOutboundChannel(ctx, paychaddr); err != nil {
		return address.Undef, cid.Undef, err
	}

	return paychaddr, msg.Cid(), nil
}

func (a *FullNodeAPI) PaychList(ctx context.Context) ([]address.Address, error) {
	return a.PaychMgr.ListChannels()
}

func (a *FullNodeAPI) PaychStatus(ctx context.Context, pch address.Address) (*api.PaychStatus, error) {
	panic("nyi")
}

func (a *FullNodeAPI) PaychClose(ctx context.Context, addr address.Address) (cid.Cid, error) {
	ci, err := a.PaychMgr.GetChannelInfo(addr)
	if err != nil {
		return cid.Undef, err
	}

	nonce, err := a.MpoolGetNonce(ctx, ci.ControlAddr)
	if err != nil {
		return cid.Undef, err
	}

	msg := &types.Message{
		To:     addr,
		From:   ci.ControlAddr,
		Value:  types.NewInt(0),
		Method: actors.PCAMethods.Close,
		Nonce:  nonce,

		GasLimit: types.NewInt(500),
		GasPrice: types.NewInt(0),
	}

	smsg, err := a.WalletSignMessage(ctx, ci.ControlAddr, msg)
	if err != nil {
		return cid.Undef, err
	}

	if err := a.MpoolPush(ctx, smsg); err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}

func (a *FullNodeAPI) PaychVoucherCheckValid(ctx context.Context, ch address.Address, sv *types.SignedVoucher) error {
	return a.PaychMgr.CheckVoucherValid(ctx, ch, sv)
}

func (a *FullNodeAPI) PaychVoucherCheckSpendable(ctx context.Context, ch address.Address, sv *types.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	return a.PaychMgr.CheckVoucherSpendable(ctx, ch, sv, secret, proof)
}

func (a *FullNodeAPI) PaychVoucherAdd(ctx context.Context, ch address.Address, sv *types.SignedVoucher) error {
	if err := a.PaychVoucherCheckValid(ctx, ch, sv); err != nil {
		return err
	}

	return a.PaychMgr.AddVoucher(ctx, ch, sv)
}

// PaychVoucherCreate creates a new signed voucher on the given payment channel
// with the given lane and amount.  The value passed in is exactly the value
// that will be used to create the voucher, so if previous vouchers exist, the
// actual additional value of this voucher will only be the difference between
// the two.
func (a *FullNodeAPI) PaychVoucherCreate(ctx context.Context, pch address.Address, amt types.BigInt, lane uint64) (*types.SignedVoucher, error) {
	return a.paychVoucherCreate(ctx, pch, types.SignedVoucher{Amount: amt, Lane: lane})
}

func (a *FullNodeAPI) paychVoucherCreate(ctx context.Context, pch address.Address, voucher types.SignedVoucher) (*types.SignedVoucher, error) {
	ci, err := a.PaychMgr.GetChannelInfo(pch)
	if err != nil {
		return nil, err
	}

	nonce, err := a.PaychMgr.NextNonceForLane(ctx, pch, voucher.Lane)
	if err != nil {
		return nil, err
	}

	sv := &voucher
	sv.Nonce = nonce

	vb, err := sv.SigningBytes()
	if err != nil {
		return nil, err
	}

	sig, err := a.WalletSign(ctx, ci.ControlAddr, vb)
	if err != nil {
		return nil, err
	}

	sv.Signature = sig

	if err := a.PaychMgr.AddVoucher(ctx, pch, sv); err != nil {
		return nil, xerrors.Errorf("failed to persist voucher: %w", err)
	}

	return sv, nil
}

func (a *FullNodeAPI) PaychVoucherList(ctx context.Context, pch address.Address) ([]*types.SignedVoucher, error) {
	return a.PaychMgr.ListVouchers(ctx, pch)
}

func (a *FullNodeAPI) PaychVoucherSubmit(ctx context.Context, ch address.Address, sv *types.SignedVoucher) (cid.Cid, error) {
	ci, err := a.PaychMgr.GetChannelInfo(ch)
	if err != nil {
		return cid.Undef, err
	}

	nonce, err := a.MpoolGetNonce(ctx, ci.ControlAddr)
	if err != nil {
		return cid.Undef, err
	}

	if sv.Extra != nil || len(sv.SecretPreimage) > 0 {
		return cid.Undef, fmt.Errorf("cant handle more advanced payment channel stuff yet")
	}

	enc, err := actors.SerializeParams(&actors.PCAUpdateChannelStateParams{
		Sv: *sv,
	})
	if err != nil {
		return cid.Undef, err
	}

	msg := &types.Message{
		From:     ci.ControlAddr,
		To:       ch,
		Value:    types.NewInt(0),
		Nonce:    nonce,
		Method:   actors.PCAMethods.UpdateChannelState,
		Params:   enc,
		GasLimit: types.NewInt(100000),
		GasPrice: types.NewInt(0),
	}

	smsg, err := a.WalletSignMessage(ctx, ci.ControlAddr, msg)
	if err != nil {
		return cid.Undef, err
	}

	if err := a.MpoolPush(ctx, smsg); err != nil {
		return cid.Undef, err
	}

	// TODO: should we wait for it...?
	return smsg.Cid(), nil
}

var _ api.FullNode = &FullNodeAPI{}
