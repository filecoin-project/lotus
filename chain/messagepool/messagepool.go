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

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/hashicorp/go-multierror"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	lps "github.com/whyrusleeping/pubsub"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/node/modules/dtypes"

	"github.com/raulk/clock"
)

var log = logging.Logger("messagepool")

var futureDebug = false

var rbfNumBig = types.NewInt(uint64((ReplaceByFeeRatioDefault - 1) * RbfDenom))
var rbfDenomBig = types.NewInt(RbfDenom)

const RbfDenom = 256

var RepublishInterval = pubsub.TimeCacheDuration + time.Duration(5*build.BlockDelaySecs+build.PropagationDelaySecs)*time.Second

var (
	ErrMessageTooBig = errors.New("message too big")

	ErrMessageValueTooHigh = errors.New("cannot send more filecoin than will ever exist")

	ErrNonceTooLow = errors.New("message nonce too low")

	ErrNotEnoughFunds = errors.New("not enough funds to execute transaction")

	ErrInvalidToAddr = errors.New("message had invalid to address")

	ErrBroadcastAnyway  = errors.New("broadcasting message despite validation fail")
	ErrRBFTooLowPremium = errors.New("replace by fee has too low GasPremium")

	ErrTryAgain = errors.New("state inconsistency while pushing message; please try again")
)

const (
	localMsgsDs = "/mpool/local"

	localUpdates = "update"
)

type MessagePool struct {
	lk sync.Mutex

	ds dtypes.MetadataDS

	addSema chan struct{}

	closer chan struct{}

	repubTk      *clock.Ticker
	repubTrigger chan struct{}

	republished map[cid.Cid]struct{}

	localAddrs map[address.Address]struct{}

	pending map[address.Address]*msgSet

	curTsLk sync.Mutex // DO NOT LOCK INSIDE lk
	curTs   *types.TipSet

	cfgLk sync.Mutex
	cfg   *types.MpoolConfig

	api Provider

	minGasPrice types.BigInt

	currentSize int

	// pruneTrigger is a channel used to trigger a mempool pruning
	pruneTrigger chan struct{}

	// pruneCooldown is a channel used to allow a cooldown time between prunes
	pruneCooldown chan struct{}

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

func (ms *msgSet) add(m *types.SignedMessage, mp *MessagePool) (bool, error) {
	if len(ms.msgs) == 0 || m.Message.Nonce >= ms.nextNonce {
		ms.nextNonce = m.Message.Nonce + 1
	}
	exms, has := ms.msgs[m.Message.Nonce]
	if has {
		if m.Cid() != exms.Cid() {
			// check if RBF passes
			minPrice := exms.Message.GasPremium
			minPrice = types.BigAdd(minPrice, types.BigDiv(types.BigMul(minPrice, rbfNumBig), rbfDenomBig))
			minPrice = types.BigAdd(minPrice, types.NewInt(1))
			if types.BigCmp(m.Message.GasPremium, minPrice) >= 0 {
				log.Infow("add with RBF", "oldpremium", exms.Message.GasPremium,
					"newpremium", m.Message.GasPremium, "addr", m.Message.From, "nonce", m.Message.Nonce)
			} else {
				log.Info("add with duplicate nonce")
				return false, xerrors.Errorf("message from %s with nonce %d already in mpool,"+
					" increase GasPremium to %s from %s to trigger replace by fee: %w",
					m.Message.From, m.Message.Nonce, minPrice, m.Message.GasPremium,
					ErrRBFTooLowPremium)
			}
		}
	}
	ms.msgs[m.Message.Nonce] = m

	return !has, nil
}

func New(api Provider, ds dtypes.MetadataDS, netName dtypes.NetworkName) (*MessagePool, error) {
	cache, _ := lru.New2Q(build.BlsSignatureCacheSize)
	verifcache, _ := lru.New2Q(build.VerifSigCacheSize)

	cfg, err := loadConfig(ds)
	if err != nil {
		if err != nil {
			return nil, xerrors.Errorf("error loading mpool config: %w", err)
		}
	}

	mp := &MessagePool{
		ds:            ds,
		addSema:       make(chan struct{}, 1),
		closer:        make(chan struct{}),
		repubTk:       build.Clock.Ticker(RepublishInterval),
		repubTrigger:  make(chan struct{}, 1),
		localAddrs:    make(map[address.Address]struct{}),
		pending:       make(map[address.Address]*msgSet),
		minGasPrice:   types.NewInt(0),
		pruneTrigger:  make(chan struct{}, 1),
		pruneCooldown: make(chan struct{}, 1),
		blsSigCache:   cache,
		sigValCache:   verifcache,
		changes:       lps.New(50),
		localMsgs:     namespace.Wrap(ds, datastore.NewKey(localMsgsDs)),
		api:           api,
		netName:       netName,
		cfg:           cfg,
	}

	// enable initial prunes
	mp.pruneCooldown <- struct{}{}

	// load the current tipset and subscribe to head changes _before_ loading local messages
	mp.curTs = api.SubscribeHeadChanges(func(rev, app []*types.TipSet) error {
		err := mp.HeadChange(rev, app)
		if err != nil {
			log.Errorf("mpool head notif handler error: %+v", err)
		}
		return err
	})

	if err := mp.loadLocal(); err != nil {
		log.Errorf("loading local messages: %+v", err)
	}

	go mp.runLoop()

	return mp, nil
}

func (mp *MessagePool) Close() error {
	close(mp.closer)
	return nil
}

func (mp *MessagePool) Prune() {
	// this magic incantation of triggering prune thrice is here to make the Prune method
	// synchronous:
	// so, its a single slot buffered channel. The first send fills the channel,
	// the second send goes through when the pruning starts,
	// and the third send goes through (and noops) after the pruning finishes
	// and goes through the loop again
	mp.pruneTrigger <- struct{}{}
	mp.pruneTrigger <- struct{}{}
	mp.pruneTrigger <- struct{}{}
}

func (mp *MessagePool) runLoop() {
	for {
		select {
		case <-mp.repubTk.C:
			if err := mp.republishPendingMessages(); err != nil {
				log.Errorf("error while republishing messages: %s", err)
			}
		case <-mp.repubTrigger:
			if err := mp.republishPendingMessages(); err != nil {
				log.Errorf("error while republishing messages: %s", err)
			}
		case <-mp.pruneTrigger:
			if err := mp.pruneExcessMessages(); err != nil {
				log.Errorf("failed to prune excess messages from mempool: %s", err)
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

func (mp *MessagePool) verifyMsgBeforePush(m *types.SignedMessage, epoch abi.ChainEpoch) error {
	minGas := vm.PricelistByEpoch(epoch).OnChainMessage(m.ChainLength())

	if err := m.VMMessage().ValidForBlockInclusion(minGas.Total()); err != nil {
		return xerrors.Errorf("message will not be included in a block: %w", err)
	}
	return nil
}

func (mp *MessagePool) Push(m *types.SignedMessage) (cid.Cid, error) {
	err := mp.checkMessage(m)
	if err != nil {
		return cid.Undef, err
	}

	// serialize push access to reduce lock contention
	mp.addSema <- struct{}{}
	defer func() {
		<-mp.addSema
	}()

	mp.curTsLk.Lock()
	curTs := mp.curTs
	epoch := curTs.Height()
	mp.curTsLk.Unlock()
	if err := mp.verifyMsgBeforePush(m, epoch); err != nil {
		return cid.Undef, err
	}

	msgb, err := m.Serialize()
	if err != nil {
		return cid.Undef, err
	}

	mp.curTsLk.Lock()
	if mp.curTs != curTs {
		mp.curTsLk.Unlock()
		return cid.Undef, ErrTryAgain
	}

	if err := mp.addTs(m, mp.curTs); err != nil {
		mp.curTsLk.Unlock()
		return cid.Undef, err
	}
	mp.curTsLk.Unlock()

	mp.lk.Lock()
	if err := mp.addLocal(m, msgb); err != nil {
		mp.lk.Unlock()
		return cid.Undef, err
	}
	mp.lk.Unlock()

	return m.Cid(), mp.api.PubSubPublish(build.MessagesTopic(mp.netName), msgb)
}

func (mp *MessagePool) checkMessage(m *types.SignedMessage) error {
	// big messages are bad, anti DOS
	if m.Size() > 32*1024 {
		return xerrors.Errorf("mpool message too large (%dB): %w", m.Size(), ErrMessageTooBig)
	}

	// Perform syntaxtic validation, minGas=0 as we check if correctly in select messages
	if err := m.Message.ValidForBlockInclusion(0); err != nil {
		return xerrors.Errorf("message not valid for block inclusion: %w", err)
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

	return nil
}

func (mp *MessagePool) Add(m *types.SignedMessage) error {
	err := mp.checkMessage(m)
	if err != nil {
		return err
	}

	// serialize push access to reduce lock contention
	mp.addSema <- struct{}{}
	defer func() {
		<-mp.addSema
	}()

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

	incr, err := mset.add(m, mp)
	if err != nil {
		log.Info(err)
		return err
	}

	if incr {
		mp.currentSize++
		if mp.currentSize > mp.cfg.SizeLimitHigh {
			// send signal to prune messages if it hasnt already been sent
			select {
			case mp.pruneTrigger <- struct{}{}:
			default:
			}
		}
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
	act, err := mp.api.GetActorAfter(addr, curTs)
	if err != nil {
		return 0, err
	}

	return act.Nonce, nil
}

func (mp *MessagePool) getStateBalance(addr address.Address, ts *types.TipSet) (types.BigInt, error) {
	act, err := mp.api.GetActorAfter(addr, ts)
	if err != nil {
		return types.EmptyInt, err
	}

	return act.Balance, nil
}

func (mp *MessagePool) PushWithNonce(ctx context.Context, addr address.Address, cb func(address.Address, uint64) (*types.SignedMessage, error)) (*types.SignedMessage, error) {
	// serialize push access to reduce lock contention
	mp.addSema <- struct{}{}
	defer func() {
		<-mp.addSema
	}()

	mp.curTsLk.Lock()
	mp.lk.Lock()

	curTs := mp.curTs

	fromKey := addr
	if fromKey.Protocol() == address.ID {
		var err error
		fromKey, err = mp.api.StateAccountKey(ctx, fromKey, mp.curTs)
		if err != nil {
			mp.lk.Unlock()
			mp.curTsLk.Unlock()
			return nil, xerrors.Errorf("resolving sender key: %w", err)
		}
	}

	nonce, err := mp.getNonceLocked(fromKey, mp.curTs)
	if err != nil {
		mp.lk.Unlock()
		mp.curTsLk.Unlock()
		return nil, xerrors.Errorf("get nonce locked failed: %w", err)
	}

	// release the locks for signing
	mp.lk.Unlock()
	mp.curTsLk.Unlock()

	msg, err := cb(fromKey, nonce)
	if err != nil {
		return nil, err
	}

	// reacquire the locks and check state for consistency
	mp.curTsLk.Lock()
	defer mp.curTsLk.Unlock()

	if mp.curTs != curTs {
		return nil, ErrTryAgain
	}

	mp.lk.Lock()
	defer mp.lk.Unlock()

	nonce2, err := mp.getNonceLocked(fromKey, mp.curTs)
	if err != nil {
		return nil, xerrors.Errorf("get nonce locked failed: %w", err)
	}

	if nonce2 != nonce {
		return nil, ErrTryAgain
	}

	if err := mp.verifyMsgBeforePush(msg, mp.curTs.Height()); err != nil {
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

	mp.remove(from, nonce)
}

func (mp *MessagePool) remove(from address.Address, nonce uint64) {
	mset, ok := mp.pending[from]
	if !ok {
		return
	}

	if m, ok := mset.msgs[nonce]; ok {
		mp.changes.Pub(api.MpoolUpdate{
			Type:    api.MpoolRemove,
			Message: m,
		}, localUpdates)

		mp.currentSize--
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

	return mp.allPending()
}

func (mp *MessagePool) allPending() ([]*types.SignedMessage, *types.TipSet) {
	out := make([]*types.SignedMessage, 0)
	for a := range mp.pending {
		out = append(out, mp.pendingFor(a)...)
	}

	return out, mp.curTs
}

func (mp *MessagePool) PendingFor(a address.Address) ([]*types.SignedMessage, *types.TipSet) {
	mp.curTsLk.Lock()
	defer mp.curTsLk.Unlock()

	mp.lk.Lock()
	defer mp.lk.Unlock()
	return mp.pendingFor(a), mp.curTs
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

	repubTrigger := false
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

	maybeRepub := func(cid cid.Cid) {
		if !repubTrigger {
			mp.lk.Lock()
			_, republished := mp.republished[cid]
			mp.lk.Unlock()
			if republished {
				repubTrigger = true
			}
		}
	}

	var merr error

	for _, ts := range revert {
		pts, err := mp.api.LoadTipSet(ts.Parents())
		if err != nil {
			log.Errorf("error loading reverted tipset parent: %s", err)
			merr = multierror.Append(merr, err)
			continue
		}

		mp.curTs = pts

		msgs, err := mp.MessagesForBlocks(ts.Blocks())
		if err != nil {
			log.Errorf("error retrieving messages for reverted block: %s", err)
			merr = multierror.Append(merr, err)
			continue
		}

		for _, msg := range msgs {
			add(msg)
		}
	}

	for _, ts := range apply {
		mp.curTs = ts

		for _, b := range ts.Blocks() {
			bmsgs, smsgs, err := mp.api.MessagesForBlock(b)
			if err != nil {
				xerr := xerrors.Errorf("failed to get messages for apply block %s(height %d) (msgroot = %s): %w", b.Cid(), b.Height, b.Messages, err)
				log.Errorf("error retrieving messages for block: %s", xerr)
				merr = multierror.Append(merr, xerr)
				continue
			}

			for _, msg := range smsgs {
				rm(msg.Message.From, msg.Message.Nonce)
				maybeRepub(msg.Cid())
			}

			for _, msg := range bmsgs {
				rm(msg.From, msg.Nonce)
				maybeRepub(msg.Cid())
			}
		}
	}

	if repubTrigger {
		select {
		case mp.repubTrigger <- struct{}{}:
		default:
		}
	}

	for _, s := range rmsgs {
		for _, msg := range s {
			if err := mp.addSkipChecks(msg); err != nil {
				log.Errorf("Failed to readd message from reorg to mpool: %s", err)
			}
		}
	}

	if len(revert) > 0 && futureDebug {
		mp.lk.Lock()
		msgs, ts := mp.allPending()
		mp.lk.Unlock()

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
			// TODO that might not be correct with GatActorAfter but it is only debug code
			act, err := mp.api.GetActorAfter(a, ts)
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

	return merr
}

func (mp *MessagePool) runHeadChange(from *types.TipSet, to *types.TipSet, rmsgs map[address.Address]map[uint64]*types.SignedMessage) error {
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
			return
		}

		if _, ok := s[nonce]; ok {
			delete(s, nonce)
			return
		}

	}

	revert, apply, err := store.ReorgOps(mp.api.LoadTipSet, from, to)
	if err != nil {
		return xerrors.Errorf("failed to compute reorg ops for mpool pending messages: %w", err)
	}

	var merr error

	for _, ts := range revert {
		msgs, err := mp.MessagesForBlocks(ts.Blocks())
		if err != nil {
			log.Errorf("error retrieving messages for reverted block: %s", err)
			merr = multierror.Append(merr, err)
			continue
		}

		for _, msg := range msgs {
			add(msg)
		}
	}

	for _, ts := range apply {
		for _, b := range ts.Blocks() {
			bmsgs, smsgs, err := mp.api.MessagesForBlock(b)
			if err != nil {
				xerr := xerrors.Errorf("failed to get messages for apply block %s(height %d) (msgroot = %s): %w", b.Cid(), b.Height, b.Messages, err)
				log.Errorf("error retrieving messages for block: %s", xerr)
				merr = multierror.Append(merr, xerr)
				continue
			}

			for _, msg := range smsgs {
				rm(msg.Message.From, msg.Message.Nonce)
			}

			for _, msg := range bmsgs {
				rm(msg.From, msg.Nonce)
			}
		}
	}

	return merr
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
		defer close(out)

		for {
			select {
			case u := <-sub:
				select {
				case out <- u.(api.MpoolUpdate):
				case <-ctx.Done():
					return
				case <-mp.closer:
					return
				}
			case <-ctx.Done():
				return
			case <-mp.closer:
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

		mp.localAddrs[sm.Message.From] = struct{}{}
	}

	return nil
}

func (mp *MessagePool) Clear(local bool) {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	// remove everything if local is true, including removing local messages from
	// the datastore
	if local {
		for a := range mp.localAddrs {
			mset, ok := mp.pending[a]
			if !ok {
				continue
			}

			for _, m := range mset.msgs {
				err := mp.localMsgs.Delete(datastore.NewKey(string(m.Cid().Bytes())))
				if err != nil {
					log.Warnf("error deleting local message: %s", err)
				}
			}
		}

		mp.pending = make(map[address.Address]*msgSet)
		mp.republished = nil

		return
	}

	// remove everything except the local messages
	for a := range mp.pending {
		_, isLocal := mp.localAddrs[a]
		if isLocal {
			continue
		}
		delete(mp.pending, a)
	}
}
