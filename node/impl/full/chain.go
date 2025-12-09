package full

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/boxo/blockservice"
	offline "github.com/ipfs/boxo/exchange/offline"
	"github.com/ipfs/boxo/ipld/merkledag"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	mh "github.com/multiformats/go-multihash"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/oldpath"
	"github.com/filecoin-project/lotus/lib/oldpath/oldresolver"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("fullnode")

type ChainModuleAPI interface {
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainHead(context.Context) (*types.TipSet, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
	ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetFinalizedTipSet(ctx context.Context) (*types.TipSet, error)
	ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetAfterHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*api.HeadChange, error)
}

var _ ChainModuleAPI = *new(api.FullNode)

// ChainModule provides a default implementation of ChainModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type ChainModule struct {
	fx.In

	Chain   *store.ChainStore
	F3      lf3.F3Backend         `optional:"true"`
	F3Certs F3CertificateProvider `optional:"true"`

	// ExposedBlockstore is the global monolith blockstore that is safe to
	// expose externally. In the future, this will be segregated into two
	// blockstores.
	ExposedBlockstore dtypes.ExposedBlockstore
}

var _ ChainModuleAPI = (*ChainModule)(nil)

type ChainAPI struct {
	fx.In

	WalletAPI
	ChainModuleAPI

	Chain  *store.ChainStore
	TsExec stmgr.Executor

	// ExposedBlockstore is the global monolith blockstore that is safe to
	// expose externally. In the future, this will be segregated into two
	// blockstores.
	ExposedBlockstore dtypes.ExposedBlockstore

	// BaseBlockstore is the underlying blockstore
	BaseBlockstore dtypes.BaseBlockstore

	Repo repo.LockedRepo
}

func (m *ChainModule) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	return m.Chain.SubHeadChanges(ctx), nil
}

func (m *ChainModule) ChainHead(context.Context) (*types.TipSet, error) {
	return m.Chain.GetHeaviestTipSet(), nil
}

func (a *ChainAPI) ChainGetBlock(ctx context.Context, msg cid.Cid) (*types.BlockHeader, error) {
	return a.Chain.GetBlock(ctx, msg)
}

func (m *ChainModule) ChainGetTipSet(ctx context.Context, key types.TipSetKey) (*types.TipSet, error) {
	return m.Chain.LoadTipSet(ctx, key)
}

func (m *ChainModule) ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*api.HeadChange, error) {
	return m.Chain.GetPath(ctx, from, to)
}

func (m *ChainModule) ChainGetBlockMessages(ctx context.Context, msg cid.Cid) (*api.BlockMessages, error) {
	b, err := m.Chain.GetBlock(ctx, msg)
	if err != nil {
		return nil, err
	}

	bmsgs, smsgs, err := m.Chain.MessagesForBlock(ctx, b)
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
	b, err := a.Chain.GetBlock(ctx, bcid)
	if err != nil {
		return nil, err
	}

	// genesis block has no parent messages...
	if b.Height == 0 {
		return nil, nil
	}

	// TODO: need to get the number of messages better than this
	pts, err := a.Chain.LoadTipSet(ctx, types.NewTipSetKey(b.Parents...))
	if err != nil {
		return nil, err
	}

	cm, err := a.Chain.MessagesForTipset(ctx, pts)
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
	b, err := a.Chain.GetBlock(ctx, bcid)
	if err != nil {
		return nil, err
	}

	if b.Height == 0 {
		return nil, nil
	}

	receipts, err := a.Chain.ReadReceipts(ctx, b.ParentMessageReceipts)
	if err != nil {
		return nil, err
	}

	out := make([]*types.MessageReceipt, len(receipts))
	for i := range receipts {
		out[i] = &receipts[i]
	}

	return out, nil
}

func (a *ChainAPI) ChainGetMessagesInTipset(ctx context.Context, tsk types.TipSetKey) ([]api.Message, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, err
	}

	// genesis block has no parent messages...
	if ts.Height() == 0 {
		return nil, nil
	}

	cm, err := a.Chain.MessagesForTipset(ctx, ts)
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

func (m *ChainModule) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return m.Chain.GetTipsetByHeight(ctx, h, ts, true)
}

func (m *ChainModule) ChainGetTipSetAfterHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return m.Chain.GetTipsetByHeight(ctx, h, ts, false)
}

func (m *ChainModule) ChainGetFinalizedTipSet(ctx context.Context) (*types.TipSet, error) {
	head := m.Chain.GetHeaviestTipSet()
	ecFinalityHeight := head.Height() - policy.ChainFinality

	// Try F3-based finality first.
	if ts := m.tryF3Finality(ctx, ecFinalityHeight); ts != nil {
		return ts, nil
	}

	// Fallback: EC finalized tipset (rewind for null rounds)
	return m.Chain.GetTipsetByHeight(ctx, ecFinalityHeight, head, true)
}

// latestF3Cert attempts to retrieve the most recent finality certificate.
// Precedence: local F3 (m.F3) first; if no certificate is available there,
// fall back to a possible external / remote provider (m.F3Certs).
func (m *ChainModule) latestF3Cert(ctx context.Context) *certs.FinalityCertificate {
	if m.F3 != nil {
		if c, err := m.F3.GetLatestCert(ctx); err != nil {
			log.Warnf("failed to get latest certificate from F3: %s", err)
		} else if c != nil {
			return c
		}
	}
	if m.F3Certs != nil {
		if c, err := m.F3Certs.F3GetLatestCertificate(ctx); err != nil {
			log.Warnf("failed to get latest certificate from F3CertificateProvider: %s", err)
		} else if c != nil {
			return c
		}
	}
	return nil
}

// tryF3Finality validates and converts a usable F3 finality certificate into a
// finalized tipset. Returns nil if no suitable certificate is available, errors
// encountered here are non-fatal and logged.
func (m *ChainModule) tryF3Finality(ctx context.Context, ecFinalityHeight abi.ChainEpoch) *types.TipSet {
	cert := m.latestF3Cert(ctx)
	if cert == nil {
		return nil
	}
	if cert.ECChain == nil || len(cert.ECChain.TipSets) == 0 {
		log.Warn("F3 finalized certificate has empty EC chain")
		return nil
	}
	finalizedTipSet := cert.ECChain.TipSets[len(cert.ECChain.TipSets)-1]
	if finalizedTipSet == nil {
		log.Warn("F3 finalized tipset in certificate is nil")
		return nil
	}
	if finalizedTipSet.Epoch < int64(ecFinalityHeight) { // ensure it's not behind EC finality
		log.Warnf("F3 finalized tipset epoch %d is earlier than EC finality height %d, ignoring F3", finalizedTipSet.Epoch, ecFinalityHeight)
		return nil
	}
	tsk, err := types.TipSetKeyFromBytes(finalizedTipSet.Key)
	if err != nil {
		log.Errorf("failed to decode F3 finalized tipset key: %s", err)
		return nil
	}
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		log.Errorf("failed to get F3 finalized tipset at epoch %d: %s", finalizedTipSet.Epoch, err)
		return nil
	}
	return ts
}

func (m *ChainModule) ChainReadObj(ctx context.Context, obj cid.Cid) ([]byte, error) {
	blk, err := m.ExposedBlockstore.Get(ctx, obj)
	if err != nil {
		return nil, xerrors.Errorf("blockstore get: %w", err)
	}

	return blk.RawData(), nil
}

func (a *ChainAPI) ChainPutObj(ctx context.Context, obj blocks.Block) error {
	return a.ExposedBlockstore.Put(ctx, obj)
}

func (a *ChainAPI) ChainDeleteObj(ctx context.Context, obj cid.Cid) error {
	return a.ExposedBlockstore.DeleteBlock(ctx, obj)
}

func (m *ChainModule) ChainHasObj(ctx context.Context, obj cid.Cid) (bool, error) {
	return m.ExposedBlockstore.Has(ctx, obj)
}

func (a *ChainAPI) ChainStatObj(ctx context.Context, obj cid.Cid, base cid.Cid) (api.ObjStat, error) {
	bs := a.ExposedBlockstore
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
	newHeadTs, err := a.Chain.GetTipSetFromKey(ctx, tsk)
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

	return a.Chain.SetHead(ctx, newHeadTs)
}

func (a *ChainAPI) ChainGetGenesis(ctx context.Context) (*types.TipSet, error) {
	genb, err := a.Chain.GetGenesis(ctx)
	if err != nil {
		return nil, err
	}

	return types.NewTipSet([]*types.BlockHeader{genb})
}

func (a *ChainAPI) ChainTipSetWeight(ctx context.Context, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
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
func resolveOnce(bs blockstore.Blockstore, tse stmgr.Executor) func(ctx context.Context, ds ipld.NodeGetter, nd ipld.Node, names []string) (*ipld.Link, []string, error) {
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

			if err := bs.Put(ctx, n); err != nil {
				return nil, nil, xerrors.Errorf("put hamt val: %w", err)
			}

			if len(names) == 1 {
				return &ipld.Link{
					Name: names[0][3:],
					Cid:  n.Cid(),
				}, nil, nil
			}

			return resolveOnce(bs, tse)(ctx, ds, n, names[1:])
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

			if err := bs.Put(ctx, n); err != nil {
				return nil, nil, xerrors.Errorf("put amt val: %w", err)
			}

			if len(names) == 1 {
				return &ipld.Link{
					Name: names[0][3:],
					Size: 0,
					Cid:  n.Cid(),
				}, nil, nil
			}

			return resolveOnce(bs, tse)(ctx, ds, n, names[1:])
		}

		if names[0] == "@state" {
			var act types.Actor
			if err := act.UnmarshalCBOR(bytes.NewReader(nd.RawData())); err != nil {
				return nil, nil, xerrors.Errorf("unmarshalling actor struct for @state: %w", err)
			}

			head, err := ds.Get(ctx, act.Head)
			if err != nil {
				return nil, nil, xerrors.Errorf("getting actor head for @state: %w", err)
			}

			m, err := vm.DumpActorState(tse.NewActorRegistry(), &act, head.RawData())
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

			if err := bs.Put(ctx, n); err != nil {
				return nil, nil, xerrors.Errorf("put amt val: %w", err)
			}

			if len(names) == 1 {
				return &ipld.Link{
					Name: "state",
					Size: 0,
					Cid:  n.Cid(),
				}, nil, nil
			}

			return resolveOnce(bs, tse)(ctx, ds, n, names[1:])
		}

		return nd.ResolveLink(names)
	}
}

func (a *ChainAPI) ChainGetNode(ctx context.Context, p string) (*api.IpldObject, error) {
	ip, err := oldpath.ParsePath(p)
	if err != nil {
		return nil, xerrors.Errorf("parsing path: %w", err)
	}

	bs := a.ExposedBlockstore
	bsvc := blockservice.New(bs, offline.Exchange(bs))
	dag := merkledag.NewDAGService(bsvc)

	r := &oldresolver.Resolver{
		DAG:         dag,
		ResolveOnce: resolveOnce(bs, a.TsExec),
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

func (m *ChainModule) ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error) {
	cm, err := m.Chain.GetCMessage(ctx, mc)
	if err != nil {
		return nil, err
	}

	return cm.VMMessage(), nil
}

func (a ChainAPI) ChainExportRangeInternal(ctx context.Context, head, tail types.TipSetKey, cfg api.ChainExportConfig) error {
	headTs, err := a.Chain.GetTipSetFromKey(ctx, head)
	if err != nil {
		return xerrors.Errorf("loading tipset %s: %w", head, err)
	}
	tailTs, err := a.Chain.GetTipSetFromKey(ctx, tail)
	if err != nil {
		return xerrors.Errorf("loading tipset %s: %w", tail, err)
	}
	if headTs.Height() < tailTs.Height() {
		return xerrors.Errorf("Height of head-tipset (%d) must be greater or equal to the height of the tail-tipset (%d)", headTs.Height(), tailTs.Height())
	}

	fileName := filepath.Join(a.Repo.Path(), fmt.Sprintf("snapshot_%d_%d_%d.car", tailTs.Height(), headTs.Height(), time.Now().Unix()))
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}

	log.Infow("Exporting chain range", "path", fileName)
	// buffer writes to the chain export file.
	bw := bufio.NewWriterSize(f, cfg.WriteBufferSize)

	defer func() {
		if err := bw.Flush(); err != nil {
			log.Errorw("failed to flush buffer", "error", err)
		}
		if err := f.Close(); err != nil {
			log.Errorw("failed to close file", "error", err)
		}
	}()

	if err := a.Chain.ExportRange(ctx,
		bw,
		headTs, tailTs,
		cfg.IncludeMessages, cfg.IncludeReceipts, cfg.IncludeStateRoots,
		cfg.NumWorkers,
	); err != nil {
		return fmt.Errorf("exporting chain range: %w", err)
	}

	return nil
}

func (a *ChainAPI) ChainExport(ctx context.Context, nroots abi.ChainEpoch, skipoldmsgs bool, tsk types.TipSetKey) (<-chan []byte, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	r, w := io.Pipe()
	out := make(chan []byte)
	go func() {
		bw := bufio.NewWriterSize(w, 1<<20)

		err = a.Chain.ExportV1(ctx, ts, nroots, skipoldmsgs, bw)
		_ = bw.Flush()            // it is a write to a pipe
		_ = w.CloseWithError(err) // it is a pipe
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

func (a *ChainAPI) ChainCheckBlockstore(ctx context.Context) error {
	checker, ok := a.BaseBlockstore.(interface{ Check() error })
	if !ok {
		return xerrors.Errorf("underlying blockstore does not support health checks")
	}

	return checker.Check()
}

func (a *ChainAPI) ChainBlockstoreInfo(ctx context.Context) (map[string]interface{}, error) {
	info, ok := a.BaseBlockstore.(interface{ Info() map[string]interface{} })
	if !ok {
		return nil, xerrors.Errorf("underlying blockstore does not provide info")
	}

	return info.Info(), nil
}

// ChainGetEvents returns the events under an event AMT root CID.
//
// TODO (raulk) make copies of this logic elsewhere use this (e.g. itests, CLI, events filter).
func (a *ChainAPI) ChainGetEvents(ctx context.Context, root cid.Cid) ([]types.Event, error) {
	store := cbor.NewCborStore(a.ExposedBlockstore)
	evtArr, err := amt4.LoadAMT(ctx, store, root, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
	if err != nil {
		return nil, xerrors.Errorf("load events amt: %w", err)
	}

	ret := make([]types.Event, 0, evtArr.Len())
	var evt types.Event
	err = evtArr.ForEach(ctx, func(u uint64, deferred *cbg.Deferred) error {
		if u > math.MaxInt {
			return xerrors.Errorf("too many events")
		}
		if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
			return err
		}

		ret = append(ret, evt)
		return nil
	})

	return ret, err
}

func (a *ChainAPI) ChainPrune(ctx context.Context, opts api.PruneOpts) error {
	pruner, ok := a.BaseBlockstore.(interface {
		PruneChain(opts api.PruneOpts) error
	})
	if !ok {
		return xerrors.Errorf("base blockstore does not support pruning (%T)", a.BaseBlockstore)
	}

	return pruner.PruneChain(opts)
}

func (a *ChainAPI) ChainHotGC(ctx context.Context, opts api.HotGCOpts) error {
	pruner, ok := a.BaseBlockstore.(interface {
		GCHotStore(api.HotGCOpts) error
	})
	if !ok {
		return xerrors.Errorf("base blockstore does not support hot GC (%T)", a.BaseBlockstore)
	}

	return pruner.GCHotStore(opts)
}
