package full

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-path"
	"github.com/ipfs/go-path/resolver"
	mh "github.com/multiformats/go-multihash"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"
)

var log = logging.Logger("fullnode")

type ChainModuleAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
}

// ChainModule provides a default implementation of ChainModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type ChainModule struct {
	fx.In

	Chain *store.ChainStore
}

var _ ChainModuleAPI = (*ChainModule)(nil)

type ChainAPI struct {
	fx.In

	WalletAPI
	ChainModuleAPI

	Chain *store.ChainStore
}

func (a *ChainAPI) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	return a.Chain.SubHeadChanges(ctx), nil
}

func (m *ChainModule) ChainHead(context.Context) (*types.TipSet, error) {
	return m.Chain.GetHeaviestTipSet(), nil
}

func (a *ChainAPI) ChainGetRandomnessFromTickets(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	pts, err := a.Chain.LoadTipSet(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset key: %w", err)
	}

	return a.Chain.GetChainRandomness(ctx, pts.Cids(), personalization, randEpoch, entropy)
}

func (a *ChainAPI) ChainGetRandomnessFromBeacon(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	pts, err := a.Chain.LoadTipSet(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset key: %w", err)
	}

	return a.Chain.GetBeaconRandomness(ctx, pts.Cids(), personalization, randEpoch, entropy)
}

func (a *ChainAPI) ChainGetBlock(ctx context.Context, msg cid.Cid) (*types.BlockHeader, error) {
	return a.Chain.GetBlock(msg)
}

func (m *ChainModule) ChainGetTipSet(ctx context.Context, key types.TipSetKey) (*types.TipSet, error) {
	return m.Chain.LoadTipSet(key)
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

	cids := make([]cid.Cid, len(bmsgs)+len(smsgs))

	for i, m := range bmsgs {
		cids[i] = m.Cid()
	}

	for i, m := range smsgs {
		cids[i+len(bmsgs)] = m.Cid()
	}

	return &api.BlockMessages{
		BlsMessages:   bmsgs,
		SecpkMessages: smsgs,
		Cids:          cids,
	}, nil
}

func (a *ChainAPI) ChainGetPath(ctx context.Context, from types.TipSetKey, to types.TipSetKey) ([]*api.HeadChange, error) {
	return a.Chain.GetPath(ctx, from, to)
}

func (a *ChainAPI) ChainGetParentMessages(ctx context.Context, bcid cid.Cid) ([]api.Message, error) {
	b, err := a.Chain.GetBlock(bcid)
	if err != nil {
		return nil, err
	}

	// genesis block has no parent messages...
	if b.Height == 0 {
		return nil, nil
	}

	// TODO: need to get the number of messages better than this
	pts, err := a.Chain.LoadTipSet(types.NewTipSetKey(b.Parents...))
	if err != nil {
		return nil, err
	}

	cm, err := a.Chain.MessagesForTipset(pts)
	if err != nil {
		return nil, err
	}

	var out []api.Message
	for _, m := range cm {
		out = append(out, api.Message{
			Cid:     m.Cid(),
			Message: m.VMMessage(),
		})
	}

	return out, nil
}

func (a *ChainAPI) ChainGetParentReceipts(ctx context.Context, bcid cid.Cid) ([]*types.MessageReceipt, error) {
	b, err := a.Chain.GetBlock(bcid)
	if err != nil {
		return nil, err
	}

	if b.Height == 0 {
		return nil, nil
	}

	// TODO: need to get the number of messages better than this
	pts, err := a.Chain.LoadTipSet(types.NewTipSetKey(b.Parents...))
	if err != nil {
		return nil, err
	}

	cm, err := a.Chain.MessagesForTipset(pts)
	if err != nil {
		return nil, err
	}

	var out []*types.MessageReceipt
	for i := 0; i < len(cm); i++ {
		r, err := a.Chain.GetParentReceipt(b, i)
		if err != nil {
			return nil, err
		}

		out = append(out, r)
	}

	return out, nil
}

func (m *ChainModule) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	ts, err := m.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return m.Chain.GetTipsetByHeight(ctx, h, ts, true)
}

func (a *ChainAPI) ChainReadObj(ctx context.Context, obj cid.Cid) ([]byte, error) {
	blk, err := a.Chain.Blockstore().Get(obj)
	if err != nil {
		return nil, xerrors.Errorf("blockstore get: %w", err)
	}

	return blk.RawData(), nil
}

func (a *ChainAPI) ChainDeleteObj(ctx context.Context, obj cid.Cid) error {
	return a.Chain.Blockstore().DeleteBlock(obj)
}

func (a *ChainAPI) ChainHasObj(ctx context.Context, obj cid.Cid) (bool, error) {
	return a.Chain.Blockstore().Has(obj)
}

func (a *ChainAPI) ChainStatObj(ctx context.Context, obj cid.Cid, base cid.Cid) (api.ObjStat, error) {
	bs := a.Chain.Blockstore()
	bsvc := blockservice.New(bs, offline.Exchange(bs))

	dag := merkledag.NewDAGService(bsvc)

	seen := cid.NewSet()

	var statslk sync.Mutex
	var stats api.ObjStat
	var collect = true

	walker := func(ctx context.Context, c cid.Cid) ([]*ipld.Link, error) {
		if c.Prefix().Codec == cid.FilCommitmentSealed || c.Prefix().Codec == cid.FilCommitmentUnsealed {
			return []*ipld.Link{}, nil
		}

		nd, err := dag.Get(ctx, c)
		if err != nil {
			return nil, err
		}

		if collect {
			s := uint64(len(nd.RawData()))
			statslk.Lock()
			stats.Size = stats.Size + s
			stats.Links = stats.Links + 1
			statslk.Unlock()
		}

		return nd.Links(), nil
	}

	if base != cid.Undef {
		collect = false
		if err := merkledag.Walk(ctx, walker, base, seen.Visit, merkledag.Concurrent()); err != nil {
			return api.ObjStat{}, err
		}
		collect = true
	}

	if err := merkledag.Walk(ctx, walker, obj, seen.Visit, merkledag.Concurrent()); err != nil {
		return api.ObjStat{}, err
	}

	return stats, nil
}

func (a *ChainAPI) ChainSetHead(ctx context.Context, tsk types.TipSetKey) error {
	newHeadTs, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	currentTs, err := a.ChainHead(ctx)
	if err != nil {
		return xerrors.Errorf("getting head: %w", err)
	}

	for currentTs.Height() >= newHeadTs.Height() {
		for _, blk := range currentTs.Key().Cids() {
			err = a.Chain.UnmarkBlockAsValidated(ctx, blk)
			if err != nil {
				return xerrors.Errorf("unmarking block as validated %s: %w", blk, err)
			}
		}

		currentTs, err = a.ChainGetTipSet(ctx, currentTs.Parents())
		if err != nil {
			return xerrors.Errorf("loading tipset: %w", err)
		}
	}

	return a.Chain.SetHead(newHeadTs)
}

func (a *ChainAPI) ChainGetGenesis(ctx context.Context) (*types.TipSet, error) {
	genb, err := a.Chain.GetGenesis()
	if err != nil {
		return nil, err
	}

	return types.NewTipSet([]*types.BlockHeader{genb})
}

func (a *ChainAPI) ChainTipSetWeight(ctx context.Context, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return a.Chain.Weight(ctx, ts)
}

// This allows us to lookup string keys in the actor's adt.Map type.
type stringKey string

func (s stringKey) Key() string {
	return (string)(s)
}

// TODO: ActorUpgrade: this entire function is a problem (in theory) as we don't know the HAMT version.
// In practice, hamt v0 should work "just fine" for reading.
func resolveOnce(bs blockstore.Blockstore) func(ctx context.Context, ds ipld.NodeGetter, nd ipld.Node, names []string) (*ipld.Link, []string, error) {
	return func(ctx context.Context, ds ipld.NodeGetter, nd ipld.Node, names []string) (*ipld.Link, []string, error) {
		store := adt.WrapStore(ctx, cbor.NewCborStore(bs))

		if strings.HasPrefix(names[0], "@Ha:") {
			addr, err := address.NewFromString(names[0][4:])
			if err != nil {
				return nil, nil, xerrors.Errorf("parsing addr: %w", err)
			}

			names[0] = "@H:" + string(addr.Bytes())
		}

		if strings.HasPrefix(names[0], "@Hi:") {
			i, err := strconv.ParseInt(names[0][4:], 10, 64)
			if err != nil {
				return nil, nil, xerrors.Errorf("parsing int64: %w", err)
			}

			ik := abi.IntKey(i)

			names[0] = "@H:" + ik.Key()
		}

		if strings.HasPrefix(names[0], "@Hu:") {
			i, err := strconv.ParseUint(names[0][4:], 10, 64)
			if err != nil {
				return nil, nil, xerrors.Errorf("parsing uint64: %w", err)
			}

			ik := abi.UIntKey(i)

			names[0] = "@H:" + ik.Key()
		}

		if strings.HasPrefix(names[0], "@H:") {
			h, err := adt.AsMap(store, nd.Cid())
			if err != nil {
				return nil, nil, xerrors.Errorf("resolving hamt link: %w", err)
			}

			var deferred cbg.Deferred
			if found, err := h.Get(stringKey(names[0][3:]), &deferred); err != nil {
				return nil, nil, xerrors.Errorf("resolve hamt: %w", err)
			} else if !found {
				return nil, nil, xerrors.Errorf("resolve hamt: not found")
			}
			var m interface{}
			if err := cbor.DecodeInto(deferred.Raw, &m); err != nil {
				return nil, nil, xerrors.Errorf("failed to decode cbor object: %w", err)
			}
			if c, ok := m.(cid.Cid); ok {
				return &ipld.Link{
					Name: names[0][3:],
					Cid:  c,
				}, names[1:], nil
			}

			n, err := cbor.WrapObject(m, mh.SHA2_256, 32)
			if err != nil {
				return nil, nil, err
			}

			if err := bs.Put(n); err != nil {
				return nil, nil, xerrors.Errorf("put hamt val: %w", err)
			}

			if len(names) == 1 {
				return &ipld.Link{
					Name: names[0][3:],
					Cid:  n.Cid(),
				}, nil, nil
			}

			return resolveOnce(bs)(ctx, ds, n, names[1:])
		}

		if strings.HasPrefix(names[0], "@A:") {
			a, err := adt.AsArray(store, nd.Cid())
			if err != nil {
				return nil, nil, xerrors.Errorf("load amt: %w", err)
			}

			idx, err := strconv.ParseUint(names[0][3:], 10, 64)
			if err != nil {
				return nil, nil, xerrors.Errorf("parsing amt index: %w", err)
			}

			var deferred cbg.Deferred
			if found, err := a.Get(idx, &deferred); err != nil {
				return nil, nil, xerrors.Errorf("resolve amt: %w", err)
			} else if !found {
				return nil, nil, xerrors.Errorf("resolve amt: not found")
			}
			var m interface{}
			if err := cbor.DecodeInto(deferred.Raw, &m); err != nil {
				return nil, nil, xerrors.Errorf("failed to decode cbor object: %w", err)
			}

			if c, ok := m.(cid.Cid); ok {
				return &ipld.Link{
					Name: names[0][3:],
					Cid:  c,
				}, names[1:], nil
			}

			n, err := cbor.WrapObject(m, mh.SHA2_256, 32)
			if err != nil {
				return nil, nil, err
			}

			if err := bs.Put(n); err != nil {
				return nil, nil, xerrors.Errorf("put amt val: %w", err)
			}

			if len(names) == 1 {
				return &ipld.Link{
					Name: names[0][3:],
					Size: 0,
					Cid:  n.Cid(),
				}, nil, nil
			}

			return resolveOnce(bs)(ctx, ds, n, names[1:])
		}

		if names[0] == "@state" {
			var act types.Actor
			if err := act.UnmarshalCBOR(bytes.NewReader(nd.RawData())); err != nil {
				return nil, nil, xerrors.Errorf("unmarshaling actor struct for @state: %w", err)
			}

			head, err := ds.Get(ctx, act.Head)
			if err != nil {
				return nil, nil, xerrors.Errorf("getting actor head for @state: %w", err)
			}

			m, err := vm.DumpActorState(&act, head.RawData())
			if err != nil {
				return nil, nil, err
			}

			// a hack to workaround struct aliasing in refmt
			ms := map[string]interface{}{}
			{
				mstr, err := json.Marshal(m)
				if err != nil {
					return nil, nil, err
				}
				if err := json.Unmarshal(mstr, &ms); err != nil {
					return nil, nil, err
				}
			}

			n, err := cbor.WrapObject(ms, mh.SHA2_256, 32)
			if err != nil {
				return nil, nil, err
			}

			if err := bs.Put(n); err != nil {
				return nil, nil, xerrors.Errorf("put amt val: %w", err)
			}

			if len(names) == 1 {
				return &ipld.Link{
					Name: "state",
					Size: 0,
					Cid:  n.Cid(),
				}, nil, nil
			}

			return resolveOnce(bs)(ctx, ds, n, names[1:])
		}

		return nd.ResolveLink(names)
	}
}

func (a *ChainAPI) ChainGetNode(ctx context.Context, p string) (*api.IpldObject, error) {
	ip, err := path.ParsePath(p)
	if err != nil {
		return nil, xerrors.Errorf("parsing path: %w", err)
	}

	bs := a.Chain.Blockstore()
	bsvc := blockservice.New(bs, offline.Exchange(bs))

	dag := merkledag.NewDAGService(bsvc)

	r := &resolver.Resolver{
		DAG:         dag,
		ResolveOnce: resolveOnce(bs),
	}

	node, err := r.ResolvePath(ctx, ip)
	if err != nil {
		return nil, err
	}

	return &api.IpldObject{
		Cid: node.Cid(),
		Obj: node,
	}, nil
}

func (a *ChainAPI) ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error) {
	cm, err := a.Chain.GetCMessage(mc)
	if err != nil {
		return nil, err
	}

	return cm.VMMessage(), nil
}

func (a *ChainAPI) ChainExport(ctx context.Context, nroots abi.ChainEpoch, skipoldmsgs bool, tsk types.TipSetKey) (<-chan []byte, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	r, w := io.Pipe()
	out := make(chan []byte)
	go func() {
		bw := bufio.NewWriterSize(w, 1<<20)

		err := a.Chain.Export(ctx, ts, nroots, skipoldmsgs, bw)
		bw.Flush()            //nolint:errcheck // it is a write to a pipe
		w.CloseWithError(err) //nolint:errcheck // it is a pipe
	}()

	go func() {
		defer close(out)
		for {
			buf := make([]byte, 1<<20)
			n, err := r.Read(buf)
			if err != nil && err != io.EOF {
				log.Errorf("chain export pipe read failed: %s", err)
				return
			}
			if n > 0 {
				select {
				case out <- buf[:n]:
				case <-ctx.Done():
					log.Warnf("export writer failed: %s", ctx.Err())
					return
				}
			}
			if err == io.EOF {
				// send empty slice to indicate correct eof
				select {
				case out <- []byte{}:
				case <-ctx.Done():
					log.Warnf("export writer failed: %s", ctx.Err())
					return
				}

				return
			}
		}
	}()

	return out, nil
}
