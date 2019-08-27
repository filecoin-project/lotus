package full

import (
	"context"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"github.com/filecoin-project/go-lotus/lib/bufbstore"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"
)

type ChainAPI struct {
	fx.In

	WalletAPI

	Chain  *store.ChainStore
	PubSub *pubsub.PubSub
}

func (a *ChainAPI) ChainNotify(ctx context.Context) (<-chan *store.HeadChange, error) {
	return a.Chain.SubHeadChanges(ctx), nil
}

func (a *ChainAPI) ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error {
	if err := a.Chain.AddBlock(blk.Header); err != nil {
		return xerrors.Errorf("AddBlock failed: %w", err)
	}

	b, err := blk.Serialize()
	if err != nil {
		return xerrors.Errorf("serializing block for pubsub publishing failed: %w", err)
	}

	// TODO: anything else to do here?
	return a.PubSub.Publish("/fil/blocks", b)
}

func (a *ChainAPI) ChainHead(context.Context) (*types.TipSet, error) {
	return a.Chain.GetHeaviestTipSet(), nil
}

func (a *ChainAPI) ChainGetRandomness(ctx context.Context, pts *types.TipSet) ([]byte, error) {
	// TODO: this needs to look back in the chain for the right random beacon value
	return []byte("foo bar random"), nil
}

func (a *ChainAPI) ChainWaitMsg(ctx context.Context, msg cid.Cid) (*api.MsgWait, error) {
	blkcid, recpt, err := a.Chain.WaitForMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &api.MsgWait{
		InBlock: blkcid,
		Receipt: *recpt,
	}, nil
}

func (a *ChainAPI) ChainGetBlock(ctx context.Context, msg cid.Cid) (*types.BlockHeader, error) {
	return a.Chain.GetBlock(msg)
}

func (a *ChainAPI) ChainGetBlockMessages(ctx context.Context, msg cid.Cid) (*api.BlockMessages, error) {
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

func (a *ChainAPI) ChainGetBlockReceipts(ctx context.Context, bcid cid.Cid) ([]*types.MessageReceipt, error) {
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

func (a *ChainAPI) ChainCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	return vm.Call(ctx, a.Chain, msg, ts)
}

func (a *ChainAPI) stateForTs(ts *types.TipSet) (*state.StateTree, error) {
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

func (a *ChainAPI) ChainGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error) {
	state, err := a.stateForTs(ts)
	if err != nil {
		return nil, err
	}

	return state.GetActor(actor)
}

func (a *ChainAPI) ChainReadState(ctx context.Context, act *types.Actor, ts *types.TipSet) (*api.ActorState, error) {
	state, err := a.stateForTs(ts)
	if err != nil {
		return nil, err
	}

	blk, err := state.Store.Blocks.GetBlock(ctx, act.Head)
	if err != nil {
		return nil, err
	}

	oif, err := vm.DumpActorState(act.Code, blk.RawData())
	if err != nil {
		return nil, err
	}

	return &api.ActorState{
		Balance: act.Balance,
		State:   oif,
	}, nil
}

// This is on ChainAPI because miner.Miner requires this, and MinerAPI requires miner.Miner
func (a *ChainAPI) MinerCreateBlock(ctx context.Context, addr address.Address, parents *types.TipSet, tickets []*types.Ticket, proof types.ElectionProof, msgs []*types.SignedMessage, ts uint64) (*chain.BlockMsg, error) {
	fblk, err := gen.MinerCreateBlock(ctx, a.Chain, a.Wallet, addr, parents, tickets, proof, msgs, ts)
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
