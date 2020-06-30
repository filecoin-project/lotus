package messagepool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/filecoin-project/specs-actors/actors/crypto"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	lps "github.com/whyrusleeping/pubsub"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var log = logging.Logger("messagepool")

const futureDebug = false

const ReplaceByFeeRatio = 1.25

var (
	rbfNum   = types.NewInt(uint64((ReplaceByFeeRatio - 1) * 256))
	rbfDenom = types.NewInt(256)
)

var (
	ErrMessageTooBig = errors.New("message too big")

	ErrMessageValueTooHigh = errors.New("cannot send more filecoin than will ever exist")

	ErrNonceTooLow = errors.New("message nonce too low")

	ErrNotEnoughFunds = errors.New("not enough funds to execute transaction")

	ErrInvalidToAddr = errors.New("message had invalid to address")

	ErrBroadcastAnyway = errors.New("broadcasting message despite validation fail")
)

const (
	localMsgsDs = "/mpool/local"

	localUpdates = "update"
)

type MessagePool struct {
	lk sync.Mutex

	closer  chan struct{}
	repubTk *time.Ticker

	localAddrs map[address.Address]struct{}

	pending map[address.Address]*msgSet

	curTsLk sync.Mutex // DO NOT LOCK INSIDE lk
	curTs   *types.TipSet

	api Provider

	minGasPrice types.BigInt

	maxTxPoolSize int

	blsSigCache *lru.TwoQueueCache

	changes *lps.PubSub

	localMsgs datastore.Datastore

	netName dtypes.NetworkName

	sigValCache *lru.TwoQueueCache
}

type msgSet struct {
	msgs      map[uint64]*types.SignedMessage
	nextNonce uint64
}

func newMsgSet() *msgSet {
	return &msgSet{
		msgs: make(map[uint64]*types.SignedMessage),
	}
}

func (ms *msgSet) add(m *types.SignedMessage) error {
	if len(ms.msgs) == 0 || m.Message.Nonce >= ms.nextNonce {
		ms.nextNonce = m.Message.Nonce + 1
	}
	exms, has := ms.msgs[m.Message.Nonce]
	if has {
		if m.Cid() != exms.Cid() {
			// check if RBF passes
			minPrice := exms.Message.GasPrice
			minPrice = types.BigAdd(minPrice, types.BigDiv(types.BigMul(minPrice, rbfNum), rbfDenom))
			minPrice = types.BigAdd(minPrice, types.NewInt(1))
			if types.BigCmp(m.Message.GasPrice, minPrice) > 0 {
				log.Infow("add with RBF", "oldprice", exms.Message.GasPrice,
					"newprice", m.Message.GasPrice, "addr", m.Message.From, "nonce", m.Message.Nonce)
			} else {
				log.Info("add with duplicate nonce")
				return xerrors.Errorf("message to %s with nonce %d already in mpool", m.Message.To, m.Message.Nonce)
			}
		}
	}
	ms.msgs[m.Message.Nonce] = m

	return nil
}

type Provider interface {
	SubscribeHeadChanges(func(rev, app []*types.TipSet) error) *types.TipSet
	PutMessage(m types.ChainMsg) (cid.Cid, error)
	PubSubPublish(string, []byte) error
	StateGetActor(address.Address, *types.TipSet) (*types.Actor, error)
	StateAccountKey(context.Context, address.Address, *types.TipSet) (address.Address, error)
	MessagesForBlock(*types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error)
	MessagesForTipset(*types.TipSet) ([]types.ChainMsg, error)
	LoadTipSet(tsk types.TipSetKey) (*types.TipSet, error)
}

type mpoolProvider struct {
	sm *stmgr.StateManager
	ps *pubsub.PubSub
}

func NewProvider(sm *stmgr.StateManager, ps *pubsub.PubSub) Provider {
	return &mpoolProvider{sm, ps}
}

func (mpp *mpoolProvider) SubscribeHeadChanges(cb func(rev, app []*types.TipSet) error) *types.TipSet {
	mpp.sm.ChainStore().SubscribeHeadChanges(cb)
	return mpp.sm.ChainStore().GetHeaviestTipSet()
}

func (mpp *mpoolProvider) PutMessage(m types.ChainMsg) (cid.Cid, error) {
	return mpp.sm.ChainStore().PutMessage(m)
}

func (mpp *mpoolProvider) PubSubPublish(k string, v []byte) error {
	return mpp.ps.Publish(k, v)
}

func (mpp *mpoolProvider) StateGetActor(addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	return mpp.sm.GetActor(addr, ts)
}

func (mpp *mpoolProvider) StateAccountKey(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	return mpp.sm.ResolveToKeyAddress(ctx, addr, ts)
}

func (mpp *mpoolProvider) MessagesForBlock(h *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	return mpp.sm.ChainStore().MessagesForBlock(h)
}

func (mpp *mpoolProvider) MessagesForTipset(ts *types.TipSet) ([]types.ChainMsg, error) {
	return mpp.sm.ChainStore().MessagesForTipset(ts)
}

func (mpp *mpoolProvider) LoadTipSet(tsk types.TipSetKey) (*types.TipSet, error) {
	return mpp.sm.ChainStore().LoadTipSet(tsk)
}

func New(api Provider, ds dtypes.MetadataDS, netName dtypes.NetworkName) (*MessagePool, error) {
	cache, _ := lru.New2Q(build.BlsSignatureCacheSize)
	verifcache, _ := lru.New2Q(build.VerifSigCacheSize)

	mp := &MessagePool{
		closer:        make(chan struct{}),
		repubTk:       time.NewTicker(time.Duration(build.BlockDelaySecs) * 10 * time.Second),
		localAddrs:    make(map[address.Address]struct{}),
		pending:       make(map[address.Address]*msgSet),
		minGasPrice:   types.NewInt(0),
		maxTxPoolSize: 5000,
		blsSigCache:   cache,
		sigValCache:   verifcache,
		changes:       lps.New(50),
		localMsgs:     namespace.Wrap(ds, datastore.NewKey(localMsgsDs)),
		api:           api,
		netName:       netName,
	}

	if err := mp.loadLocal(); err != nil {
		log.Errorf("loading local messages: %+v", err)
	}

	go mp.repubLocal()

	mp.curTs = api.SubscribeHeadChanges(func(rev, app []*types.TipSet) error {
		err := mp.HeadChange(rev, app)
		if err != nil {
			log.Errorf("mpool head notif handler error: %+v", err)
		}
		return err
	})

	return mp, nil
}

func (mp *MessagePool) Close() error {
	close(mp.closer)
	return nil
}

func (mp *MessagePool) repubLocal() {
	for {
		select {
		case <-mp.repubTk.C:
			mp.lk.Lock()

			msgsForAddr := make(map[address.Address][]*types.SignedMessage)
			for a := range mp.localAddrs {
				msgsForAddr[a] = mp.pendingFor(a)
			}

			mp.lk.Unlock()

			var errout error
			outputMsgs := []*types.SignedMessage{}

			for a, msgs := range msgsForAddr {
				a, err := mp.api.StateGetActor(a, nil)
				if err != nil {
					errout = multierr.Append(errout, xerrors.Errorf("could not get actor state: %w", err))
					continue
				}

				curNonce := a.Nonce
				for _, m := range msgs {
					if m.Message.Nonce < curNonce {
						continue
					}
					if m.Message.Nonce != curNonce {
						break
					}
					outputMsgs = append(outputMsgs, m)
					curNonce++
				}

			}

			if len(outputMsgs) != 0 {
				log.Infow("republishing local messages", "n", len(outputMsgs))
			}

			for _, msg := range outputMsgs {
				msgb, err := msg.Serialize()
				if err != nil {
					errout = multierr.Append(errout, xerrors.Errorf("could not serialize: %w", err))
					continue
				}

				err = mp.api.PubSubPublish(build.MessagesTopic(mp.netName), msgb)
				if err != nil {
					errout = multierr.Append(errout, xerrors.Errorf("could not publish: %w", err))
					continue
				}
			}

			if errout != nil {
				log.Errorf("errors while republishing: %+v", errout)
			}
		case <-mp.closer:
			mp.repubTk.Stop()
			return
		}
	}

}

func (mp *MessagePool) addLocal(m *types.SignedMessage, msgb []byte) error {
	mp.localAddrs[m.Message.From] = struct{}{}

	if err := mp.localMsgs.Put(datastore.NewKey(string(m.Cid().Bytes())), msgb); err != nil {
		return xerrors.Errorf("persisting local message: %w", err)
	}

	return nil
}

func (mp *MessagePool) Push(m *types.SignedMessage) (cid.Cid, error) {
	msgb, err := m.Serialize()
	if err != nil {
		return cid.Undef, err
	}

	if err := mp.Add(m); err != nil {
		return cid.Undef, err
	}

	mp.lk.Lock()
	if err := mp.addLocal(m, msgb); err != nil {
		mp.lk.Unlock()
		return cid.Undef, err
	}
	mp.lk.Unlock()

	return m.Cid(), mp.api.PubSubPublish(build.MessagesTopic(mp.netName), msgb)
}

func (mp *MessagePool) Add(m *types.SignedMessage) error {
	// big messages are bad, anti DOS
	if m.Size() > 32*1024 {
		return xerrors.Errorf("mpool message too large (%dB): %w", m.Size(), ErrMessageTooBig)
	}

	if m.Message.To == address.Undef {
		return ErrInvalidToAddr
	}

	if !m.Message.Value.LessThan(types.TotalFilecoinInt) {
		return ErrMessageValueTooHigh
	}

	if err := mp.VerifyMsgSig(m); err != nil {
		log.Warnf("mpooladd signature verification failed: %s", err)
		return err
	}

	mp.curTsLk.Lock()
	defer mp.curTsLk.Unlock()
	return mp.addTs(m, mp.curTs)
}

func sigCacheKey(m *types.SignedMessage) (string, error) {
	switch m.Signature.Type {
	case crypto.SigTypeBLS:
		if len(m.Signature.Data) < 90 {
			return "", fmt.Errorf("bls signature too short")
		}

		return string(m.Cid().Bytes()) + string(m.Signature.Data[64:]), nil
	case crypto.SigTypeSecp256k1:
		return string(m.Cid().Bytes()), nil
	default:
		return "", xerrors.Errorf("unrecognized signature type: %d", m.Signature.Type)
	}
}

func (mp *MessagePool) VerifyMsgSig(m *types.SignedMessage) error {
	sck, err := sigCacheKey(m)
	if err != nil {
		return err
	}

	_, ok := mp.sigValCache.Get(sck)
	if ok {
		// already validated, great
		return nil
	}

	if err := sigs.Verify(&m.Signature, m.Message.From, m.Message.Cid().Bytes()); err != nil {
		return err
	}

	mp.sigValCache.Add(sck, struct{}{})

	return nil
}

func (mp *MessagePool) addTs(m *types.SignedMessage, curTs *types.TipSet) error {
	snonce, err := mp.getStateNonce(m.Message.From, curTs)
	if err != nil {
		return xerrors.Errorf("failed to look up actor state nonce: %s: %w", err, ErrBroadcastAnyway)
	}

	if snonce > m.Message.Nonce {
		return xerrors.Errorf("minimum expected nonce is %d: %w", snonce, ErrNonceTooLow)
	}

	balance, err := mp.getStateBalance(m.Message.From, curTs)
	if err != nil {
		return xerrors.Errorf("failed to check sender balance: %s: %w", err, ErrBroadcastAnyway)
	}

	if balance.LessThan(m.Message.RequiredFunds()) {
		return xerrors.Errorf("not enough funds (required: %s, balance: %s): %w", types.FIL(m.Message.RequiredFunds()), types.FIL(balance), ErrNotEnoughFunds)
	}

	mp.lk.Lock()
	defer mp.lk.Unlock()

	return mp.addLocked(m)
}

func (mp *MessagePool) addSkipChecks(m *types.SignedMessage) error {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	return mp.addLocked(m)
}

func (mp *MessagePool) addLocked(m *types.SignedMessage) error {
	log.Debugf("mpooladd: %s %d", m.Message.From, m.Message.Nonce)
	if m.Signature.Type == crypto.SigTypeBLS {
		mp.blsSigCache.Add(m.Cid(), m.Signature)
	}

	if m.Message.GasLimit > build.BlockGasLimit {
		return xerrors.Errorf("given message has too high of a gas limit")
	}

	if _, err := mp.api.PutMessage(m); err != nil {
		log.Warnf("mpooladd cs.PutMessage failed: %s", err)
		return err
	}

	if _, err := mp.api.PutMessage(&m.Message); err != nil {
		log.Warnf("mpooladd cs.PutMessage failed: %s", err)
		return err
	}

	mset, ok := mp.pending[m.Message.From]
	if !ok {
		mset = newMsgSet()
		mp.pending[m.Message.From] = mset
	}

	if err := mset.add(m); err != nil {
		log.Info(err)
	}

	mp.changes.Pub(api.MpoolUpdate{
		Type:    api.MpoolAdd,
		Message: m,
	}, localUpdates)
	return nil
}

func (mp *MessagePool) GetNonce(addr address.Address) (uint64, error) {
	mp.curTsLk.Lock()
	defer mp.curTsLk.Unlock()

	mp.lk.Lock()
	defer mp.lk.Unlock()

	return mp.getNonceLocked(addr, mp.curTs)
}

func (mp *MessagePool) getNonceLocked(addr address.Address, curTs *types.TipSet) (uint64, error) {
	stateNonce, err := mp.getStateNonce(addr, curTs) // sanity check
	if err != nil {
		return 0, err
	}

	mset, ok := mp.pending[addr]
	if ok {
		if stateNonce > mset.nextNonce {
			log.Errorf("state nonce was larger than mset.nextNonce (%d > %d)", stateNonce, mset.nextNonce)

			return stateNonce, nil
		}

		return mset.nextNonce, nil
	}

	return stateNonce, nil
}

func (mp *MessagePool) getStateNonce(addr address.Address, curTs *types.TipSet) (uint64, error) {
	// TODO: this method probably should be cached

	act, err := mp.api.StateGetActor(addr, curTs)
	if err != nil {
		return 0, err
	}

	baseNonce := act.Nonce

	// TODO: the correct thing to do here is probably to set curTs to chain.head
	// but since we have an accurate view of the world until a head change occurs,
	// this should be fine
	if curTs == nil {
		return baseNonce, nil
	}

	msgs, err := mp.api.MessagesForTipset(curTs)
	if err != nil {
		return 0, xerrors.Errorf("failed to check messages for tipset: %w", err)
	}

	for _, m := range msgs {
		msg := m.VMMessage()
		if msg.From == addr {
			if msg.Nonce != baseNonce {
				return 0, xerrors.Errorf("tipset %s has bad nonce ordering (%d != %d)", curTs.Cids(), msg.Nonce, baseNonce)
			}
			baseNonce++
		}
	}

	return baseNonce, nil
}

func (mp *MessagePool) getStateBalance(addr address.Address, ts *types.TipSet) (types.BigInt, error) {
	act, err := mp.api.StateGetActor(addr, ts)
	if err != nil {
		return types.EmptyInt, err
	}

	return act.Balance, nil
}

func (mp *MessagePool) PushWithNonce(ctx context.Context, addr address.Address, cb func(address.Address, uint64) (*types.SignedMessage, error)) (*types.SignedMessage, error) {
	mp.curTsLk.Lock()
	defer mp.curTsLk.Unlock()

	mp.lk.Lock()
	defer mp.lk.Unlock()

	fromKey := addr
	if fromKey.Protocol() == address.ID {
		var err error
		fromKey, err = mp.api.StateAccountKey(ctx, fromKey, mp.curTs)
		if err != nil {
			return nil, xerrors.Errorf("resolving sender key: %w", err)
		}
	}

	nonce, err := mp.getNonceLocked(fromKey, mp.curTs)
	if err != nil {
		return nil, xerrors.Errorf("get nonce locked failed: %w", err)
	}

	msg, err := cb(fromKey, nonce)
	if err != nil {
		return nil, err
	}

	msgb, err := msg.Serialize()
	if err != nil {
		return nil, err
	}

	if err := mp.addLocked(msg); err != nil {
		return nil, xerrors.Errorf("add locked failed: %w", err)
	}
	if err := mp.addLocal(msg, msgb); err != nil {
		log.Errorf("addLocal failed: %+v", err)
	}

	return msg, mp.api.PubSubPublish(build.MessagesTopic(mp.netName), msgb)
}

func (mp *MessagePool) Remove(from address.Address, nonce uint64) {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	mset, ok := mp.pending[from]
	if !ok {
		return
	}

	if m, ok := mset.msgs[nonce]; ok {
		mp.changes.Pub(api.MpoolUpdate{
			Type:    api.MpoolRemove,
			Message: m,
		}, localUpdates)
	}

	// NB: This deletes any message with the given nonce. This makes sense
	// as two messages with the same sender cannot have the same nonce
	delete(mset.msgs, nonce)

	if len(mset.msgs) == 0 {
		delete(mp.pending, from)
	} else {
		var max uint64
		for nonce := range mset.msgs {
			if max < nonce {
				max = nonce
			}
		}
		if max < nonce {
			max = nonce // we could have not seen the removed message before
		}

		mset.nextNonce = max + 1
	}
}

func (mp *MessagePool) Pending() ([]*types.SignedMessage, *types.TipSet) {
	mp.curTsLk.Lock()
	defer mp.curTsLk.Unlock()

	mp.lk.Lock()
	defer mp.lk.Unlock()

	out := make([]*types.SignedMessage, 0)
	for a := range mp.pending {
		out = append(out, mp.pendingFor(a)...)
	}

	return out, mp.curTs
}

func (mp *MessagePool) pendingFor(a address.Address) []*types.SignedMessage {
	mset := mp.pending[a]
	if mset == nil || len(mset.msgs) == 0 {
		return nil
	}

	set := make([]*types.SignedMessage, 0, len(mset.msgs))

	for _, m := range mset.msgs {
		set = append(set, m)
	}

	sort.Slice(set, func(i, j int) bool {
		return set[i].Message.Nonce < set[j].Message.Nonce
	})

	return set
}

func (mp *MessagePool) HeadChange(revert []*types.TipSet, apply []*types.TipSet) error {
	mp.curTsLk.Lock()
	defer mp.curTsLk.Unlock()

	rmsgs := make(map[address.Address]map[uint64]*types.SignedMessage)
	add := func(m *types.SignedMessage) {
		s, ok := rmsgs[m.Message.From]
		if !ok {
			s = make(map[uint64]*types.SignedMessage)
			rmsgs[m.Message.From] = s
		}
		s[m.Message.Nonce] = m
	}
	rm := func(from address.Address, nonce uint64) {
		s, ok := rmsgs[from]
		if !ok {
			mp.Remove(from, nonce)
			return
		}

		if _, ok := s[nonce]; ok {
			delete(s, nonce)
			return
		}

		mp.Remove(from, nonce)
	}

	for _, ts := range revert {
		pts, err := mp.api.LoadTipSet(ts.Parents())
		if err != nil {
			return err
		}

		msgs, err := mp.MessagesForBlocks(ts.Blocks())
		if err != nil {
			return err
		}

		mp.curTs = pts

		for _, msg := range msgs {
			add(msg)
		}
	}

	for _, ts := range apply {
		for _, b := range ts.Blocks() {
			bmsgs, smsgs, err := mp.api.MessagesForBlock(b)
			if err != nil {
				return xerrors.Errorf("failed to get messages for apply block %s(height %d) (msgroot = %s): %w", b.Cid(), b.Height, b.Messages, err)
			}
			for _, msg := range smsgs {
				rm(msg.Message.From, msg.Message.Nonce)
			}

			for _, msg := range bmsgs {
				rm(msg.From, msg.Nonce)
			}
		}

		mp.curTs = ts
	}

	for _, s := range rmsgs {
		for _, msg := range s {
			if err := mp.addSkipChecks(msg); err != nil {
				log.Errorf("Failed to readd message from reorg to mpool: %s", err)
			}
		}
	}

	if len(revert) > 0 && futureDebug {
		msgs, ts := mp.Pending()

		buckets := map[address.Address]*statBucket{}

		for _, v := range msgs {
			bkt, ok := buckets[v.Message.From]
			if !ok {
				bkt = &statBucket{
					msgs: map[uint64]*types.SignedMessage{},
				}
				buckets[v.Message.From] = bkt
			}

			bkt.msgs[v.Message.Nonce] = v
		}

		for a, bkt := range buckets {
			act, err := mp.api.StateGetActor(a, ts)
			if err != nil {
				log.Debugf("%s, err: %s\n", a, err)
				continue
			}

			var cmsg *types.SignedMessage
			var ok bool

			cur := act.Nonce
			for {
				cmsg, ok = bkt.msgs[cur]
				if !ok {
					break
				}
				cur++
			}

			ff := uint64(math.MaxUint64)
			for k := range bkt.msgs {
				if k > cur && k < ff {
					ff = k
				}
			}

			if ff != math.MaxUint64 {
				m := bkt.msgs[ff]

				// cmsg can be nil if no messages from the current nonce are in the mpool
				ccid := "nil"
				if cmsg != nil {
					ccid = cmsg.Cid().String()
				}

				log.Debugw("Nonce gap",
					"actor", a,
					"future_cid", m.Cid(),
					"future_nonce", ff,
					"current_cid", ccid,
					"current_nonce", cur,
					"revert_tipset", revert[0].Key(),
					"new_head", ts.Key(),
				)
			}
		}
	}

	return nil
}

type statBucket struct {
	msgs map[uint64]*types.SignedMessage
}

func (mp *MessagePool) MessagesForBlocks(blks []*types.BlockHeader) ([]*types.SignedMessage, error) {
	out := make([]*types.SignedMessage, 0)

	for _, b := range blks {
		bmsgs, smsgs, err := mp.api.MessagesForBlock(b)
		if err != nil {
			return nil, xerrors.Errorf("failed to get messages for apply block %s(height %d) (msgroot = %s): %w", b.Cid(), b.Height, b.Messages, err)
		}
		out = append(out, smsgs...)

		for _, msg := range bmsgs {
			smsg := mp.RecoverSig(msg)
			if smsg != nil {
				out = append(out, smsg)
			} else {
				log.Warnf("could not recover signature for bls message %s", msg.Cid())
			}
		}
	}

	return out, nil
}

func (mp *MessagePool) RecoverSig(msg *types.Message) *types.SignedMessage {
	val, ok := mp.blsSigCache.Get(msg.Cid())
	if !ok {
		return nil
	}
	sig, ok := val.(crypto.Signature)
	if !ok {
		log.Errorf("value in signature cache was not a signature (got %T)", val)
		return nil
	}

	return &types.SignedMessage{
		Message:   *msg,
		Signature: sig,
	}
}

func (mp *MessagePool) Updates(ctx context.Context) (<-chan api.MpoolUpdate, error) {
	out := make(chan api.MpoolUpdate, 20)
	sub := mp.changes.Sub(localUpdates)

	go func() {
		defer mp.changes.Unsub(sub, localUpdates)

		for {
			select {
			case u := <-sub:
				select {
				case out <- u.(api.MpoolUpdate):
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (mp *MessagePool) loadLocal() error {
	res, err := mp.localMsgs.Query(query.Query{})
	if err != nil {
		return xerrors.Errorf("query local messages: %w", err)
	}

	for r := range res.Next() {
		if r.Error != nil {
			return xerrors.Errorf("r.Error: %w", r.Error)
		}

		var sm types.SignedMessage
		if err := sm.UnmarshalCBOR(bytes.NewReader(r.Value)); err != nil {
			return xerrors.Errorf("unmarshaling local message: %w", err)
		}

		if err := mp.Add(&sm); err != nil {
			if xerrors.Is(err, ErrNonceTooLow) {
				continue // todo: drop the message from local cache (if above certain confidence threshold)
			}

			log.Errorf("adding local message: %+v", err)
		}
	}

	return nil
}

const MinGasPrice = 0

func (mp *MessagePool) EstimateGasPrice(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error) {
	// TODO: something smarter obviously
	switch nblocksincl {
	case 0:
		return types.NewInt(MinGasPrice + 2), nil
	case 1:
		return types.NewInt(MinGasPrice + 1), nil
	default:
		return types.NewInt(MinGasPrice), nil
	}
}
