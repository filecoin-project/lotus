package impl

import (
	"context"
	"fmt"
	"strconv"

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

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
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
}

func (a *FullNodeAPI) ClientStartDeal(ctx context.Context, data cid.Cid, miner address.Address, price types.BigInt, blocksDuration uint64) (*cid.Cid, error) {
	self, err := a.WalletDefaultAddress(ctx)
	if err != nil {
		return nil, err
	}

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

	total := types.BigMul(price, types.NewInt(blocksDuration))
	c, err := a.DealClient.Start(ctx, data, total, self, miner, pid, blocksDuration)
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
	if ts == nil {
		ts = a.Chain.GetHeaviestTipSet()
	}
	state, err := a.Chain.TipSetState(ts.Cids())
	if err != nil {
		return nil, err
	}

	vmi, err := vm.NewVM(state, ts.Height(), ts.Blocks()[0].Miner, a.Chain)
	if err != nil {
		return nil, xerrors.Errorf("failed to set up vm: %w", err)
	}

	if msg.GasLimit == types.EmptyInt {
		msg.GasLimit = types.NewInt(10000000000)
	}
	if msg.GasPrice == types.EmptyInt {
		msg.GasPrice = types.NewInt(0)
	}
	if msg.Value == types.EmptyInt {
		msg.Value = types.NewInt(0)
	}
	if msg.Params == nil {
		msg.Params, err = actors.SerializeParams(struct{}{})
		if err != nil {
			return nil, err
		}
	}

	// TODO: maybe just use the invoker directly?
	ret, err := vmi.ApplyMessage(ctx, msg)
	if ret.ActorErr != nil {
		log.Warnf("chain call failed: %s", ret.ActorErr)
	}
	return &ret.MessageReceipt, err
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

func (a *FullNodeAPI) WalletList(ctx context.Context) ([]address.Address, error) {
	return a.Wallet.ListAddrs()
}

func (a *FullNodeAPI) WalletBalance(ctx context.Context, addr address.Address) (types.BigInt, error) {
	return a.Chain.GetBalance(addr)
}

func (a *FullNodeAPI) WalletSign(ctx context.Context, k address.Address, msg []byte) (*types.Signature, error) {
	return a.Wallet.Sign(k, msg)
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

var _ api.FullNode = &FullNodeAPI{}
