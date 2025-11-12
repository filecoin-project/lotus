package messagepool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	stdbig "math/big"
	"sort"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/raulk/clock"
	"go.opencensus.io/stats"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	lps "github.com/filecoin-project/pubsub"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var log = logging.Logger("messagepool")

var futureDebug = false

var rbfNumBig = types.NewInt(uint64(ReplaceByFeePercentageMinimum))
var rbfDenomBig = types.NewInt(100)

var RepublishInterval = time.Duration(10*buildconstants.BlockDelaySecs+buildconstants.PropagationDelaySecs) * time.Second

var minimumBaseFee = types.NewInt(uint64(buildconstants.MinimumBaseFee))
var baseFeeLowerBoundFactor = types.NewInt(10)
var baseFeeLowerBoundFactorConservative = types.NewInt(100)

var MaxActorPendingMessages = 1000
var MaxUntrustedActorPendingMessages = 10

var MaxNonceGap = uint64(4)

const MaxMessageSize = 64 << 10 // 64KiB

// NOTE: When adding a new error type, please make sure to add the new error type in
// func (mv *MessageValidator) Validate(ctx context.Context, pid peer.ID, msg *pubsub.Message)
// in /chain/sub/incoming.go
var (
	ErrMessageTooBig = errors.New("message too big")

	ErrMessageValueTooHigh = errors.New("cannot send more filecoin than will ever exist")

	ErrNonceTooLow = errors.New("message nonce too low")

	ErrGasFeeCapTooLow = errors.New("gas fee cap too low")

	ErrNotEnoughFunds = errors.New("not enough funds to execute transaction")

	ErrInvalidToAddr = errors.New("message had invalid to address")

	ErrSoftValidationFailure  = errors.New("validation failure")
	ErrRBFTooLowPremium       = errors.New("replace by fee has too low GasPremium")
	ErrTooManyPendingMessages = errors.New("too many pending messages for actor")
	ErrNonceGap               = errors.New("unfulfilled nonce gap")
	ErrExistingNonce          = errors.New("message with nonce already exists")
)

const (
	localMsgsDs = "/mpool/local"

	localUpdates = "update"
)

// Journal event types.
const (
	evtTypeMpoolAdd = iota
	evtTypeMpoolRemove
	evtTypeMpoolRepub
)

// MessagePoolEvt is the journal entry for message pool events.
type MessagePoolEvt struct {
	Action   string
	Messages []MessagePoolEvtMessage
	Error    error `json:",omitempty"`
}

type MessagePoolEvtMessage struct {
	types.Message

	CID cid.Cid
}

func init() {
	// if the republish interval is too short compared to the pubsub timecache, adjust it
	minInterval := pubsub.TimeCacheDuration + time.Duration(buildconstants.PropagationDelaySecs)*time.Second
	if RepublishInterval < minInterval {
		RepublishInterval = minInterval
	}
}

type MessagePool struct {
	lk sync.RWMutex

	ds dtypes.MetadataDS

	addSema chan struct{}

	closer chan struct{}

	repubTk      *clock.Ticker
	repubTrigger chan struct{}

	republished map[cid.Cid]struct{}

	// do NOT access this map directly, use isLocal, setLocal, and forEachLocal respectively
	localAddrs map[address.Address]struct{}

	// do NOT access this map directly, use getPendingMset, setPendingMset, deletePendingMset, forEachPending, and clearPending respectively
	pending map[address.Address]*msgSet

	keyCache *lru.Cache[address.Address, address.Address]

	curTsLk sync.RWMutex // DO NOT LOCK INSIDE lk
	curTs   *types.TipSet

	cfgLk sync.RWMutex
	cfg   *types.MpoolConfig

	api Provider

	minGasPrice types.BigInt

	getNtwkVersion func(abi.ChainEpoch) (network.Version, error)

	currentSize int

	// pruneTrigger is a channel used to trigger a mempool pruning
	pruneTrigger chan struct{}

	// pruneCooldown is a channel used to allow a cooldown time between prunes
	pruneCooldown chan struct{}

	blsSigCache *lru.TwoQueueCache[cid.Cid, crypto.Signature]

	changes *lps.PubSub

	localMsgs datastore.Datastore

	netName dtypes.NetworkName

	sigValCache *lru.TwoQueueCache[string, struct{}]

	stateNonceCache *lru.Cache[stateNonceCacheKey, uint64]

	evtTypes [3]journal.EventType
	journal  journal.Journal
}

type stateNonceCacheKey struct {
	tsk  types.TipSetKey
	addr address.Address
}

type msgSet struct {
	msgs          map[uint64]*types.SignedMessage
	nextNonce     uint64
	requiredFunds *stdbig.Int
}

func newMsgSet(nonce uint64) *msgSet {
	return &msgSet{
		msgs:          make(map[uint64]*types.SignedMessage),
		nextNonce:     nonce,
		requiredFunds: stdbig.NewInt(0),
	}
}

func ComputeMinRBF(curPrem abi.TokenAmount) abi.TokenAmount {
	minPrice := types.BigDiv(types.BigMul(curPrem, rbfNumBig), rbfDenomBig)
	return types.BigAdd(minPrice, types.NewInt(1))
}

func ComputeRBF(curPrem abi.TokenAmount, replaceByFeeRatio types.Percent) abi.TokenAmount {
	rbfNumBig := types.NewInt(uint64(replaceByFeeRatio))
	minPrice := types.BigDiv(types.BigMul(curPrem, rbfNumBig), rbfDenomBig)
	return types.BigAdd(minPrice, types.NewInt(1))
}

func CapGasFee(mff dtypes.DefaultMaxFeeFunc, msg *types.Message, sendSpec *api.MessageSendSpec) {
	var maxFee abi.TokenAmount
	var maximizeFeeCap bool
	if sendSpec != nil {
		maxFee = sendSpec.MaxFee
		maximizeFeeCap = sendSpec.MaximizeFeeCap
	}
	if maxFee.Int == nil || maxFee.Equals(big.Zero()) {
		mf, err := mff()
		if err != nil {
			log.Errorf("failed to get default max gas fee: %+v", err)
			mf = big.Zero()
		}
		maxFee = mf
	}

	gaslimit := types.NewInt(uint64(msg.GasLimit))
	totalFee := types.BigMul(msg.GasFeeCap, gaslimit)
	if maximizeFeeCap || totalFee.GreaterThan(maxFee) {
		msg.GasFeeCap = big.Div(maxFee, gaslimit)
	}

	msg.GasPremium = big.Min(msg.GasFeeCap, msg.GasPremium) // cap premium at FeeCap
}

func (ms *msgSet) add(m *types.SignedMessage, mp *MessagePool, strict, untrusted bool) (bool, error) {
	nextNonce := ms.nextNonce
	nonceGap := false

	maxNonceGap := MaxNonceGap
	maxActorPendingMessages := MaxActorPendingMessages
	if untrusted {
		maxNonceGap = 0
		maxActorPendingMessages = MaxUntrustedActorPendingMessages
	}

	switch {
	case m.Message.Nonce == nextNonce:
		nextNonce++
		// advance if we are filling a gap
		for _, fillGap := ms.msgs[nextNonce]; fillGap; _, fillGap = ms.msgs[nextNonce] {
			nextNonce++
		}

	case strict && m.Message.Nonce > nextNonce+maxNonceGap:
		return false, xerrors.Errorf("message nonce has too big a gap from expected nonce (Nonce: %d, nextNonce: %d): %w", m.Message.Nonce, nextNonce, ErrNonceGap)

	case m.Message.Nonce > nextNonce:
		nonceGap = true
	}

	exms, has := ms.msgs[m.Message.Nonce]
	if has {
		// refuse RBF if we have a gap
		if strict && nonceGap {
			return false, xerrors.Errorf("rejecting replace by fee because of nonce gap (Nonce: %d, nextNonce: %d): %w", m.Message.Nonce, nextNonce, ErrNonceGap)
		}

		if m.Cid() != exms.Cid() {
			// check if RBF passes
			minPrice := ComputeMinRBF(exms.Message.GasPremium)
			if types.BigCmp(m.Message.GasPremium, minPrice) >= 0 {
				log.Debugw("add with RBF", "oldpremium", exms.Message.GasPremium,
					"newpremium", m.Message.GasPremium, "addr", m.Message.From, "nonce", m.Message.Nonce)
			} else {
				log.Debugf("add with duplicate nonce. message from %s with nonce %d already in mpool,"+
					" increase GasPremium to %s from %s to trigger replace by fee: %s",
					m.Message.From, m.Message.Nonce, minPrice, m.Message.GasPremium,
					ErrRBFTooLowPremium)
				return false, xerrors.Errorf("message from %s with nonce %d already in mpool,"+
					" increase GasPremium to %s from %s to trigger replace by fee: %w",
					m.Message.From, m.Message.Nonce, minPrice, m.Message.GasPremium,
					ErrRBFTooLowPremium)
			}
		} else {
			return false, xerrors.Errorf("message from %s with nonce %d already in mpool: %w",
				m.Message.From, m.Message.Nonce, ErrExistingNonce)
		}

		ms.requiredFunds.Sub(ms.requiredFunds, exms.Message.RequiredFunds().Int)
		// ms.requiredFunds.Sub(ms.requiredFunds, exms.Message.Value.Int)
	}

	if !has && len(ms.msgs) >= maxActorPendingMessages {
		log.Errorf("too many pending messages from actor %s", m.Message.From)
		return false, ErrTooManyPendingMessages
	}

	if strict && nonceGap {
		log.Debugf("adding nonce-gapped message from %s (nonce: %d, nextNonce: %d)",
			m.Message.From, m.Message.Nonce, nextNonce)
	}

	ms.nextNonce = nextNonce
	ms.msgs[m.Message.Nonce] = m
	ms.requiredFunds.Add(ms.requiredFunds, m.Message.RequiredFunds().Int)
	// ms.requiredFunds.Add(ms.requiredFunds, m.Message.Value.Int)

	return !has, nil
}

func (ms *msgSet) rm(nonce uint64, applied bool) {
	m, has := ms.msgs[nonce]
	if !has {
		if applied && nonce >= ms.nextNonce {
			// we removed a message we did not know about because it was applied
			// we need to adjust the nonce and check if we filled a gap
			ms.nextNonce = nonce + 1
			for _, fillGap := ms.msgs[ms.nextNonce]; fillGap; _, fillGap = ms.msgs[ms.nextNonce] {
				ms.nextNonce++
			}
		}
		return
	}

	ms.requiredFunds.Sub(ms.requiredFunds, m.Message.RequiredFunds().Int)
	// ms.requiredFunds.Sub(ms.requiredFunds, m.Message.Value.Int)
	delete(ms.msgs, nonce)

	// adjust next nonce
	if applied {
		// we removed a (known) message because it was applied in a tipset
		// we can't possibly have filled a gap in this case
		if nonce >= ms.nextNonce {
			ms.nextNonce = nonce + 1
		}
		return
	}

	// we removed a message because it was pruned
	// we have to adjust the nonce if it creates a gap or rewinds state
	if nonce < ms.nextNonce {
		ms.nextNonce = nonce
	}
}

func (ms *msgSet) getRequiredFunds(nonce uint64) types.BigInt {
	requiredFunds := new(stdbig.Int).Set(ms.requiredFunds)

	m, has := ms.msgs[nonce]
	if has {
		requiredFunds.Sub(requiredFunds, m.Message.RequiredFunds().Int)
		// requiredFunds.Sub(requiredFunds, m.Message.Value.Int)
	}

	return types.BigInt{Int: requiredFunds}
}

func (ms *msgSet) toSlice() []*types.SignedMessage {
	set := make([]*types.SignedMessage, 0, len(ms.msgs))

	for _, m := range ms.msgs {
		set = append(set, m)
	}

	sort.Slice(set, func(i, j int) bool {
		return set[i].Message.Nonce < set[j].Message.Nonce
	})

	return set
}

func New(ctx context.Context, api Provider, ds dtypes.MetadataDS, us stmgr.UpgradeSchedule, netName dtypes.NetworkName, j journal.Journal) (*MessagePool, error) {
	cache, _ := lru.New2Q[cid.Cid, crypto.Signature](buildconstants.BlsSignatureCacheSize)
	verifcache, _ := lru.New2Q[string, struct{}](buildconstants.VerifSigCacheSize)
	stateNonceCache, _ := lru.New[stateNonceCacheKey, uint64](32768) // 32k * ~200 bytes = 6MB
	keycache, _ := lru.New[address.Address, address.Address](1_000_000)

	cfg, err := loadConfig(ctx, ds)
	if err != nil {
		return nil, xerrors.Errorf("error loading mpool config: %w", err)
	}

	if j == nil {
		j = journal.NilJournal()
	}

	mp := &MessagePool{
		ds:              ds,
		addSema:         make(chan struct{}, 1),
		closer:          make(chan struct{}),
		repubTk:         build.Clock.Ticker(RepublishInterval),
		repubTrigger:    make(chan struct{}, 1),
		localAddrs:      make(map[address.Address]struct{}),
		pending:         make(map[address.Address]*msgSet),
		keyCache:        keycache,
		minGasPrice:     types.NewInt(0),
		getNtwkVersion:  us.GetNtwkVersion,
		pruneTrigger:    make(chan struct{}, 1),
		pruneCooldown:   make(chan struct{}, 1),
		blsSigCache:     cache,
		sigValCache:     verifcache,
		stateNonceCache: stateNonceCache,
		changes:         lps.New(50),
		localMsgs:       namespace.Wrap(ds, datastore.NewKey(localMsgsDs)),
		api:             api,
		netName:         netName,
		cfg:             cfg,
		evtTypes: [...]journal.EventType{
			evtTypeMpoolAdd:    j.RegisterEventType("mpool", "add"),
			evtTypeMpoolRemove: j.RegisterEventType("mpool", "remove"),
			evtTypeMpoolRepub:  j.RegisterEventType("mpool", "repub"),
		},
		journal: j,
	}

	// enable initial prunes
	mp.pruneCooldown <- struct{}{}

	ctx, cancel := context.WithCancel(context.TODO())

	// load the current tipset and subscribe to head changes _before_ loading local messages
	mp.curTs = api.SubscribeHeadChanges(func(rev, app []*types.TipSet) error {
		err := mp.HeadChange(ctx, rev, app)
		if err != nil {
			log.Errorf("mpool head notif handler error: %+v", err)
		}
		return err
	})

	mp.curTsLk.Lock()
	mp.lk.Lock()

	go func() {
		defer cancel()
		err := mp.loadLocal(ctx)

		mp.lk.Unlock()
		mp.curTsLk.Unlock()

		if err != nil {
			log.Errorf("loading local messages: %+v", err)
		}

		log.Info("mpool ready")

		mp.runLoop(ctx)
	}()

	return mp, nil
}

func (mp *MessagePool) ForEachPendingMessage(f func(cid.Cid) error) error {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	for _, mset := range mp.pending {
		for _, m := range mset.msgs {
			err := f(m.Cid())
			if err != nil {
				return err
			}

			err = f(m.Message.Cid())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (mp *MessagePool) resolveToKey(ctx context.Context, addr address.Address) (address.Address, error) {
	//if addr is not an ID addr, then it is already resolved to a key
	if addr.Protocol() != address.ID {
		return addr, nil
	}
	return mp.resolveToKeyFromID(ctx, addr)
}

func (mp *MessagePool) resolveToKeyFromID(ctx context.Context, addr address.Address) (address.Address, error) {

	// check the cache
	a, ok := mp.keyCache.Get(addr)
	if ok {
		return a, nil
	}

	// resolve the address
	ka, err := mp.api.StateDeterministicAddressAtFinality(ctx, addr, mp.curTs)
	if err != nil {
		return address.Undef, err
	}

	// place both entries in the cache (may both be key addresses, which is fine)
	mp.keyCache.Add(addr, ka)
	return ka, nil
}

func (mp *MessagePool) getPendingMset(ctx context.Context, addr address.Address) (*msgSet, bool, error) {
	ra, err := mp.resolveToKey(ctx, addr)
	if err != nil {
		return nil, false, err
	}

	ms, f := mp.pending[ra]

	return ms, f, nil
}

func (mp *MessagePool) setPendingMset(ctx context.Context, addr address.Address, ms *msgSet) error {
	ra, err := mp.resolveToKey(ctx, addr)
	if err != nil {
		return err
	}

	mp.pending[ra] = ms

	return nil
}

// This method isn't strictly necessary, since it doesn't resolve any addresses, but it's safer to have
func (mp *MessagePool) forEachPending(f func(address.Address, *msgSet)) {
	for la, ms := range mp.pending {
		f(la, ms)
	}
}

func (mp *MessagePool) deletePendingMset(ctx context.Context, addr address.Address) error {
	ra, err := mp.resolveToKey(ctx, addr)
	if err != nil {
		return err
	}

	delete(mp.pending, ra)

	return nil
}

// This method isn't strictly necessary, since it doesn't resolve any addresses, but it's safer to have
func (mp *MessagePool) clearPending() {
	mp.pending = make(map[address.Address]*msgSet)
}

func (mp *MessagePool) isLocal(ctx context.Context, addr address.Address) (bool, error) {
	ra, err := mp.resolveToKey(ctx, addr)
	if err != nil {
		return false, err
	}

	_, f := mp.localAddrs[ra]

	return f, nil
}

func (mp *MessagePool) setLocal(ctx context.Context, addr address.Address) error {
	ra, err := mp.resolveToKey(ctx, addr)
	if err != nil {
		return err
	}

	mp.localAddrs[ra] = struct{}{}

	return nil
}

// This method isn't strictly necessary, since it doesn't resolve any addresses, but it's safer to have
func (mp *MessagePool) forEachLocal(ctx context.Context, f func(context.Context, address.Address)) {
	for la := range mp.localAddrs {
		f(ctx, la)
	}
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

func (mp *MessagePool) runLoop(ctx context.Context) {
	for {
		select {
		case <-mp.repubTk.C:
			if err := mp.republishPendingMessages(ctx); err != nil {
				log.Errorf("error while republishing messages: %s", err)
			}
		case <-mp.repubTrigger:
			if err := mp.republishPendingMessages(ctx); err != nil {
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

func (mp *MessagePool) addLocal(ctx context.Context, m *types.SignedMessage) error {
	if err := mp.setLocal(ctx, m.Message.From); err != nil {
		return err
	}

	msgb, err := m.Serialize()
	if err != nil {
		return xerrors.Errorf("error serializing message: %w", err)
	}

	if err := mp.localMsgs.Put(ctx, datastore.NewKey(string(m.Cid().Bytes())), msgb); err != nil {
		return xerrors.Errorf("persisting local message: %w", err)
	}

	return nil
}

// verifyMsgBeforeAdd verifies that the message meets the minimum criteria for block inclusion
// and whether the message has enough funds to be included in the next 20 blocks.
// If the message is not valid for block inclusion, it returns an error.
// For local messages, if the message can be included in the next 20 blocks, it returns true to
// signal that it should be immediately published. If the message cannot be included in the next 20
// blocks, it returns false so that the message doesn't immediately get published (and ignored by our
// peers); instead it will be published through the republish loop, once the base fee has fallen
// sufficiently.
// For non local messages, if the message cannot be included in the next 20 blocks it returns
// a (soft) validation error.
func (mp *MessagePool) verifyMsgBeforeAdd(ctx context.Context, m *types.SignedMessage, curTs *types.TipSet, local bool) (bool, error) {
	epoch := curTs.Height() + 1
	minGas := vm.PricelistByEpoch(epoch).OnChainMessage(m.ChainLength())

	if err := m.VMMessage().ValidForBlockInclusion(minGas.Total(), mp.api.StateNetworkVersion(ctx, epoch)); err != nil {
		return false, xerrors.Errorf("message will not be included in a block: %w", err)
	}

	// this checks if the GasFeeCap is sufficiently high for inclusion in the next 20 blocks
	// if the GasFeeCap is too low, we soft reject the message (Ignore in pubsub) and rely
	// on republish to push it through later, if the baseFee has fallen.
	// this is a defensive check that stops minimum baseFee spam attacks from overloading validation
	// queues.
	// Note that for local messages, we always add them so that they can be accepted and republished
	// automatically.
	publish := local

	var baseFee big.Int
	if len(curTs.Blocks()) > 0 {
		baseFee = curTs.Blocks()[0].ParentBaseFee
	} else {
		var err error
		baseFee, err = mp.api.ChainComputeBaseFee(context.TODO(), curTs)
		if err != nil {
			return false, xerrors.Errorf("computing basefee: %w", err)
		}
	}

	baseFeeLowerBound := getBaseFeeLowerBound(baseFee, baseFeeLowerBoundFactorConservative)
	if m.Message.GasFeeCap.LessThan(baseFeeLowerBound) {
		if local {
			log.Warnf("local message will not be immediately published because GasFeeCap doesn't meet the lower bound for inclusion in the next 20 blocks (GasFeeCap: %s, baseFeeLowerBound: %s)",
				m.Message.GasFeeCap, baseFeeLowerBound)
			publish = false
		} else {
			return false, xerrors.Errorf("GasFeeCap doesn't meet base fee lower bound for inclusion in the next 20 blocks (GasFeeCap: %s, baseFeeLowerBound: %s): %w",
				m.Message.GasFeeCap, baseFeeLowerBound, ErrSoftValidationFailure)
		}
	}

	return publish, nil
}

// Push checks the signed message for any violations, adds the message to the message pool and
// publishes the message if the publish flag is set
func (mp *MessagePool) Push(ctx context.Context, m *types.SignedMessage, publish bool) (cid.Cid, error) {
	done := metrics.Timer(ctx, metrics.MpoolPushDuration)
	defer done()

	err := mp.checkMessage(ctx, m)
	if err != nil {
		return cid.Undef, err
	}

	// serialize push access to reduce lock contention
	mp.addSema <- struct{}{}
	defer func() {
		<-mp.addSema
	}()

	mp.curTsLk.Lock()
	ok, err := mp.addTs(ctx, m, mp.curTs, true, false)
	if err != nil {
		mp.curTsLk.Unlock()
		return cid.Undef, err
	}
	mp.curTsLk.Unlock()

	if ok && publish {
		msgb, err := m.Serialize()
		if err != nil {
			return cid.Undef, xerrors.Errorf("error serializing message: %w", err)
		}

		err = mp.api.PubSubPublish(build.MessagesTopic(mp.netName), msgb)
		if err != nil {
			return cid.Undef, xerrors.Errorf("error publishing message: %w", err)
		}
	}

	return m.Cid(), nil
}

func (mp *MessagePool) checkMessage(ctx context.Context, m *types.SignedMessage) error {
	// big messages are bad, anti DOS
	if m.Size() > MaxMessageSize {
		return xerrors.Errorf("mpool message too large (%dB): %w", m.Size(), ErrMessageTooBig)
	}

	// Perform syntactic validation, minGas=0 as we check the actual mingas before we add it
	if err := m.Message.ValidForBlockInclusion(0, mp.api.StateNetworkVersion(ctx, mp.curTs.Height())); err != nil {
		return xerrors.Errorf("message not valid for block inclusion: %w", err)
	}

	if m.Message.To == address.Undef {
		return ErrInvalidToAddr
	}

	if !m.Message.Value.LessThan(types.TotalFilecoinInt) {
		return ErrMessageValueTooHigh
	}

	if m.Message.GasFeeCap.LessThan(minimumBaseFee) {
		return ErrGasFeeCapTooLow
	}

	if err := mp.VerifyMsgSig(m); err != nil {
		return xerrors.Errorf("signature verification failed: %s", err)
	}

	return nil
}

func (mp *MessagePool) Add(ctx context.Context, m *types.SignedMessage) error {
	done := metrics.Timer(ctx, metrics.MpoolAddDuration)
	defer done()

	err := mp.checkMessage(ctx, m)
	if err != nil {
		return err
	}

	// serialize push access to reduce lock contention
	mp.addSema <- struct{}{}
	defer func() {
		<-mp.addSema
	}()

	mp.curTsLk.RLock()
	tmpCurTs := mp.curTs
	mp.curTsLk.RUnlock()

	//ensures computations are cached without holding lock
	_, _ = mp.api.GetActorAfter(m.Message.From, tmpCurTs)
	_, _ = mp.getStateNonce(ctx, m.Message.From, tmpCurTs)

	mp.curTsLk.Lock()
	if tmpCurTs != mp.curTs {
		//curTs has been updated so we want to cache the new one:
		tmpCurTs = mp.curTs
		//we want to release the lock, cache the computations then grab it again
		mp.curTsLk.Unlock()
		_, _ = mp.api.GetActorAfter(m.Message.From, tmpCurTs)
		_, _ = mp.getStateNonce(ctx, m.Message.From, tmpCurTs)
		mp.curTsLk.Lock()
		//now that we have the lock, we continue, we could do this as a loop forever, but that's bad to loop forever, and this was added as an optimization and it seems once is enough because the computation < block time
	} // else with the lock enabled, mp.curTs is the same Ts as we just had, so we know that our computations are cached

	defer mp.curTsLk.Unlock()

	_, err = mp.addTs(ctx, m, mp.curTs, false, false)
	return err
}

func sigCacheKey(m *types.SignedMessage) (string, error) {
	switch m.Signature.Type {
	case crypto.SigTypeBLS:
		if len(m.Signature.Data) != ffi.SignatureBytes {
			return "", fmt.Errorf("bls signature incorrectly sized")
		}
		hashCache := blake2b.Sum256(append(m.Cid().Bytes(), m.Signature.Data...))
		return string(hashCache[:]), nil
	case crypto.SigTypeSecp256k1, crypto.SigTypeDelegated:
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

	if err := consensus.AuthenticateMessage(m, m.Message.From); err != nil {
		return xerrors.Errorf("failed to validate signature: %w", err)
	}

	mp.sigValCache.Add(sck, struct{}{})

	return nil
}

func (mp *MessagePool) checkBalance(ctx context.Context, m *types.SignedMessage, curTs *types.TipSet) error {
	balance, err := mp.getStateBalance(ctx, m.Message.From, curTs)
	if err != nil {
		return xerrors.Errorf("failed to check sender balance: %s: %w", err, ErrSoftValidationFailure)
	}

	requiredFunds := m.Message.RequiredFunds()
	if balance.LessThan(requiredFunds) {
		return xerrors.Errorf("not enough funds (required: %s, balance: %s): %w", types.FIL(requiredFunds), types.FIL(balance), ErrNotEnoughFunds)
	}

	// add Value for soft failure check
	// requiredFunds = types.BigAdd(requiredFunds, m.Message.Value)

	mset, ok, err := mp.getPendingMset(ctx, m.Message.From)
	if err != nil {
		log.Debugf("mpoolcheckbalance failed to get pending mset: %s", err)
		return err
	}

	if ok {
		requiredFunds = types.BigAdd(requiredFunds, mset.getRequiredFunds(m.Message.Nonce))
	}

	if balance.LessThan(requiredFunds) {
		// Note: we fail here for ErrSoftValidationFailure to signal a soft failure because we might
		// be out of sync.
		return xerrors.Errorf("not enough funds including pending messages (required: %s, balance: %s): %w", types.FIL(requiredFunds), types.FIL(balance), ErrSoftValidationFailure)
	}

	return nil
}

func (mp *MessagePool) addTs(ctx context.Context, m *types.SignedMessage, curTs *types.TipSet, local, untrusted bool) (bool, error) {
	done := metrics.Timer(ctx, metrics.MpoolAddTsDuration)
	defer done()

	snonce, err := mp.getStateNonce(ctx, m.Message.From, curTs)
	if err != nil {
		return false, xerrors.Errorf("failed to look up actor state nonce: %s: %w", err, ErrSoftValidationFailure)
	}

	if snonce > m.Message.Nonce {
		return false, xerrors.Errorf("minimum expected nonce is %d, got %d: %w", snonce, m.Message.Nonce, ErrNonceTooLow)
	}

	senderAct, err := mp.api.GetActorAfter(m.Message.From, curTs)
	if err != nil {
		return false, xerrors.Errorf("failed to get sender actor: %w", err)
	}

	// This message can only be included in the _next_ epoch and beyond, hence the +1.
	epoch := curTs.Height() + 1
	nv := mp.api.StateNetworkVersion(ctx, epoch)

	// TODO: I'm not thrilled about depending on filcns here, but I prefer this to duplicating logic

	if m.Signature.Type == crypto.SigTypeDelegated && !consensus.IsValidEthTxForSending(nv, m) {
		return false, xerrors.Errorf("network version should be at least NV23 for sending legacy ETH transactions; but current network version is %d", nv)
	}

	if !consensus.IsValidForSending(nv, senderAct) {
		return false, xerrors.Errorf("sender actor %s is not a valid top-level sender", m.Message.From)
	}

	mp.lk.Lock()
	defer mp.lk.Unlock()

	publish, err := mp.verifyMsgBeforeAdd(ctx, m, curTs, local)
	if err != nil {
		return false, xerrors.Errorf("verify msg failed: %w", err)
	}

	if err := mp.checkBalance(ctx, m, curTs); err != nil {
		return false, xerrors.Errorf("failed to check balance: %w", err)
	}

	err = mp.addLocked(ctx, m, !local, untrusted)
	if err != nil {
		return false, xerrors.Errorf("failed to add locked: %w", err)
	}

	if local {
		err = mp.addLocal(ctx, m)
		if err != nil {
			return false, xerrors.Errorf("error persisting local message: %w", err)
		}
	}

	return publish, nil
}

func (mp *MessagePool) addLoaded(ctx context.Context, m *types.SignedMessage) error {
	err := mp.checkMessage(ctx, m)
	if err != nil {
		return err
	}

	curTs := mp.curTs

	if curTs == nil {
		return xerrors.Errorf("current tipset not loaded")
	}

	snonce, err := mp.getStateNonce(ctx, m.Message.From, curTs)
	if err != nil {
		return xerrors.Errorf("failed to look up actor state nonce: %s: %w", err, ErrSoftValidationFailure)
	}

	if snonce > m.Message.Nonce {
		return xerrors.Errorf("minimum expected nonce is %d: %w", snonce, ErrNonceTooLow)
	}

	_, err = mp.verifyMsgBeforeAdd(ctx, m, curTs, true)
	if err != nil {
		return err
	}

	if err := mp.checkBalance(ctx, m, curTs); err != nil {
		return err
	}

	return mp.addLocked(ctx, m, false, false)
}

func (mp *MessagePool) addSkipChecks(ctx context.Context, m *types.SignedMessage) error {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	return mp.addLocked(ctx, m, false, false)
}

func (mp *MessagePool) addLocked(ctx context.Context, m *types.SignedMessage, strict, untrusted bool) error {
	log.Debugf("mpooladd: %s %d", m.Message.From, m.Message.Nonce)
	if m.Signature.Type == crypto.SigTypeBLS {
		mp.blsSigCache.Add(m.Cid(), m.Signature)
	}

	if _, err := mp.api.PutMessage(ctx, m); err != nil {
		return xerrors.Errorf("mpooladd cs.PutMessage failed: %s", err)
	}

	if _, err := mp.api.PutMessage(ctx, &m.Message); err != nil {
		return xerrors.Errorf("mpooladd cs.PutMessage failed: %s", err)
	}

	// Note: If performance becomes an issue, making this getOrCreatePendingMset will save some work
	mset, ok, err := mp.getPendingMset(ctx, m.Message.From)
	if err != nil {
		log.Debug(err)
		return err
	}

	if !ok {
		nonce, err := mp.getStateNonce(ctx, m.Message.From, mp.curTs)
		if err != nil {
			return xerrors.Errorf("failed to get initial actor nonce: %w", err)
		}

		mset = newMsgSet(nonce)
		if err = mp.setPendingMset(ctx, m.Message.From, mset); err != nil {
			return xerrors.Errorf("failed to set pending mset: %w", err)
		}
	}

	incr, err := mset.add(m, mp, strict, untrusted)
	if err != nil {
		log.Debug(err)
		return err
	}

	if incr {
		mp.currentSize++
		if mp.currentSize > mp.getConfig().SizeLimitHigh {
			// send signal to prune messages if it hasn't already been sent
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

	mp.journal.RecordEvent(mp.evtTypes[evtTypeMpoolAdd], func() interface{} {
		return MessagePoolEvt{
			Action:   "add",
			Messages: []MessagePoolEvtMessage{{Message: m.Message, CID: m.Cid()}},
		}
	})

	// Record the current size of the Mpool
	stats.Record(ctx, metrics.MpoolMessageCount.M(int64(mp.currentSize)))

	return nil
}

func (mp *MessagePool) GetNonce(ctx context.Context, addr address.Address, _ types.TipSetKey) (uint64, error) {
	mp.curTsLk.RLock()
	defer mp.curTsLk.RUnlock()

	mp.lk.RLock()
	defer mp.lk.RUnlock()

	return mp.getNonceLocked(ctx, addr, mp.curTs)
}

// GetActor should not be used. It is only here to satisfy interface mess caused by lite node handling
func (mp *MessagePool) GetActor(_ context.Context, addr address.Address, _ types.TipSetKey) (*types.Actor, error) {
	mp.curTsLk.RLock()
	defer mp.curTsLk.RUnlock()
	return mp.api.GetActorAfter(addr, mp.curTs)
}

func (mp *MessagePool) getNonceLocked(ctx context.Context, addr address.Address, curTs *types.TipSet) (uint64, error) {
	stateNonce, err := mp.getStateNonce(ctx, addr, curTs) // sanity check
	if err != nil {
		return 0, err
	}

	mset, ok, err := mp.getPendingMset(ctx, addr)
	if err != nil {
		log.Debugf("mpoolgetnonce failed to get mset: %s", err)
		return 0, err
	}

	if ok {
		if stateNonce > mset.nextNonce {
			log.Errorf("state nonce was larger than mset.nextNonce (%d > %d)", stateNonce, mset.nextNonce)

			return stateNonce, nil
		}

		return mset.nextNonce, nil
	}

	return stateNonce, nil
}

func (mp *MessagePool) getStateNonce(ctx context.Context, addr address.Address, ts *types.TipSet) (uint64, error) {
	done := metrics.Timer(ctx, metrics.MpoolGetNonceDuration)
	defer done()

	nk := stateNonceCacheKey{
		tsk:  ts.Key(),
		addr: addr,
	}

	n, ok := mp.stateNonceCache.Get(nk)
	if ok {
		return n, nil
	}

	// get the nonce from the actor before ts
	actor, err := mp.api.GetActorBefore(addr, ts)
	if err != nil {
		return 0, err
	}
	nextNonce := actor.Nonce

	raddr, err := mp.resolveToKey(ctx, addr)
	if err != nil {
		return 0, err
	}

	// loop over all messages sent by 'addr' and find the highest nonce
	messages, err := mp.api.MessagesForTipset(ctx, ts)
	if err != nil {
		return 0, err
	}
	for _, message := range messages {
		msg := message.VMMessage()

		maddr, err := mp.resolveToKey(ctx, msg.From)
		if err != nil {
			log.Warnf("failed to resolve message from address: %s", err)
			continue
		}

		if maddr == raddr {
			if n := msg.Nonce + 1; n > nextNonce {
				nextNonce = n
			}
		}
	}

	mp.stateNonceCache.Add(nk, nextNonce)

	return nextNonce, nil
}

func (mp *MessagePool) getStateBalance(ctx context.Context, addr address.Address, ts *types.TipSet) (types.BigInt, error) {
	done := metrics.Timer(ctx, metrics.MpoolGetBalanceDuration)
	defer done()

	act, err := mp.api.GetActorAfter(addr, ts)
	if err != nil {
		return types.EmptyInt, err
	}

	return act.Balance, nil
}

// PushUntrusted is provided for the gateway to push messages.
// differences from Push:
//   - strict checks are enabled
//   - extra strict add checks are used when adding the messages to the msgSet
//     that means: no nonce gaps, at most 10 pending messages for the actor
func (mp *MessagePool) PushUntrusted(ctx context.Context, m *types.SignedMessage) (cid.Cid, error) {
	err := mp.checkMessage(ctx, m)
	if err != nil {
		return cid.Undef, err
	}

	// serialize push access to reduce lock contention
	mp.addSema <- struct{}{}
	defer func() {
		<-mp.addSema
	}()

	mp.curTsLk.Lock()
	publish, err := mp.addTs(ctx, m, mp.curTs, true, true)
	if err != nil {
		mp.curTsLk.Unlock()
		return cid.Undef, err
	}
	mp.curTsLk.Unlock()

	if publish {
		msgb, err := m.Serialize()
		if err != nil {
			return cid.Undef, xerrors.Errorf("error serializing message: %w", err)
		}

		err = mp.api.PubSubPublish(build.MessagesTopic(mp.netName), msgb)
		if err != nil {
			return cid.Undef, xerrors.Errorf("error publishing message: %w", err)
		}
	}

	return m.Cid(), nil
}

func (mp *MessagePool) Remove(ctx context.Context, from address.Address, nonce uint64, applied bool) {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	mp.remove(ctx, from, nonce, applied)
}

func (mp *MessagePool) remove(ctx context.Context, from address.Address, nonce uint64, applied bool) {
	mset, ok, err := mp.getPendingMset(ctx, from)
	if err != nil {
		log.Debugf("mpoolremove failed to get mset: %s", err)
		return
	}

	if !ok {
		return
	}

	if m, ok := mset.msgs[nonce]; ok {
		mp.changes.Pub(api.MpoolUpdate{
			Type:    api.MpoolRemove,
			Message: m,
		}, localUpdates)

		mp.journal.RecordEvent(mp.evtTypes[evtTypeMpoolRemove], func() interface{} {
			return MessagePoolEvt{
				Action:   "remove",
				Messages: []MessagePoolEvtMessage{{Message: m.Message, CID: m.Cid()}}}
		})

		mp.currentSize--
	}

	// NB: This deletes any message with the given nonce. This makes sense
	// as two messages with the same sender cannot have the same nonce
	mset.rm(nonce, applied)

	if len(mset.msgs) == 0 {
		if err = mp.deletePendingMset(ctx, from); err != nil {
			log.Debugf("mpoolremove failed to delete mset: %s", err)
			return
		}
	}

	// Record the current size of the Mpool
	stats.Record(ctx, metrics.MpoolMessageCount.M(int64(mp.currentSize)))
}

func (mp *MessagePool) Pending(ctx context.Context) ([]*types.SignedMessage, *types.TipSet) {
	mp.curTsLk.RLock()
	defer mp.curTsLk.RUnlock()

	mp.lk.RLock()
	defer mp.lk.RUnlock()

	return mp.allPending(ctx)
}

func (mp *MessagePool) allPending(ctx context.Context) ([]*types.SignedMessage, *types.TipSet) {
	out := make([]*types.SignedMessage, 0)

	mp.forEachPending(func(a address.Address, mset *msgSet) {
		out = append(out, mset.toSlice()...)
	})

	return out, mp.curTs
}

func (mp *MessagePool) PendingFor(ctx context.Context, a address.Address) ([]*types.SignedMessage, *types.TipSet) {
	mp.curTsLk.RLock()
	defer mp.curTsLk.RUnlock()

	mp.lk.RLock()
	defer mp.lk.RUnlock()
	return mp.pendingFor(ctx, a), mp.curTs
}

func (mp *MessagePool) pendingFor(ctx context.Context, a address.Address) []*types.SignedMessage {
	mset, ok, err := mp.getPendingMset(ctx, a)
	if err != nil {
		log.Debugf("mpoolpendingfor failed to get mset: %s", err)
		return nil
	}

	if mset == nil || !ok || len(mset.msgs) == 0 {
		return nil
	}

	return mset.toSlice()
}

func (mp *MessagePool) HeadChange(ctx context.Context, revert []*types.TipSet, apply []*types.TipSet) error {
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
			mp.Remove(ctx, from, nonce, true)
			return
		}

		if _, ok := s[nonce]; ok {
			delete(s, nonce)
			return
		}

		mp.Remove(ctx, from, nonce, true)
	}

	maybeRepub := func(cid cid.Cid) {
		if !repubTrigger {
			mp.lk.RLock()
			_, republished := mp.republished[cid]
			mp.lk.RUnlock()
			if republished {
				repubTrigger = true
			}
		}
	}

	var merr error

	for _, ts := range revert {
		pts, err := mp.api.LoadTipSet(ctx, ts.Parents())
		if err != nil {
			log.Errorf("error loading reverted tipset parent: %s", err)
			merr = multierror.Append(merr, err)
			continue
		}

		mp.curTs = pts

		msgs, err := mp.MessagesForBlocks(ctx, ts.Blocks())
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
			bmsgs, smsgs, err := mp.api.MessagesForBlock(ctx, b)
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
			if err := mp.addSkipChecks(ctx, msg); err != nil {
				log.Errorf("Failed to read message from reorg to mpool: %s", err)
			}
		}
	}

	if len(revert) > 0 && futureDebug {
		mp.lk.RLock()
		msgs, ts := mp.allPending(ctx)
		mp.lk.RUnlock()

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

func (mp *MessagePool) runHeadChange(ctx context.Context, from *types.TipSet, to *types.TipSet, rmsgs map[address.Address]map[uint64]*types.SignedMessage) error {
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

	revert, apply, err := store.ReorgOps(ctx, mp.api.LoadTipSet, from, to)
	if err != nil {
		return xerrors.Errorf("failed to compute reorg ops for mpool pending messages: %w", err)
	}

	var merr error

	for _, ts := range revert {
		msgs, err := mp.MessagesForBlocks(ctx, ts.Blocks())
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
			bmsgs, smsgs, err := mp.api.MessagesForBlock(ctx, b)
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

func (mp *MessagePool) MessagesForBlocks(ctx context.Context, blks []*types.BlockHeader) ([]*types.SignedMessage, error) {
	out := make([]*types.SignedMessage, 0)

	for _, b := range blks {
		bmsgs, smsgs, err := mp.api.MessagesForBlock(ctx, b)
		if err != nil {
			return nil, xerrors.Errorf("failed to get messages for apply block %s(height %d) (msgroot = %s): %w", b.Cid(), b.Height, b.Messages, err)
		}
		out = append(out, smsgs...)

		for _, msg := range bmsgs {
			smsg := mp.RecoverSig(msg)
			if smsg != nil {
				out = append(out, smsg)
			} else {
				log.Debugf("could not recover signature for bls message %s", msg.Cid())
			}
		}
	}

	return out, nil
}

func (mp *MessagePool) RecoverSig(msg *types.Message) *types.SignedMessage {
	sig, ok := mp.blsSigCache.Get(msg.Cid())
	if !ok {
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
		defer mp.changes.Unsub(sub)
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

func (mp *MessagePool) loadLocal(ctx context.Context) error {
	res, err := mp.localMsgs.Query(ctx, query.Query{})
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

		if err := mp.addLoaded(ctx, &sm); err != nil {
			if errors.Is(err, ErrNonceTooLow) {
				continue // todo: drop the message from local cache (if above certain confidence threshold)
			}

			log.Errorf("adding local message: %+v", err)
		}

		if err = mp.setLocal(ctx, sm.Message.From); err != nil {
			log.Debugf("mpoolloadLocal errored: %s", err)
			return err
		}
	}

	return nil
}

func (mp *MessagePool) Clear(ctx context.Context, local bool) {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	// remove everything if local is true, including removing local messages from
	// the datastore
	if local {
		mp.forEachLocal(ctx, func(ctx context.Context, la address.Address) {
			mset, ok, err := mp.getPendingMset(ctx, la)
			if err != nil {
				log.Warnf("errored while getting pending mset: %w", err)
				return
			}

			if ok {
				for _, m := range mset.msgs {
					err := mp.localMsgs.Delete(ctx, datastore.NewKey(string(m.Cid().Bytes())))
					if err != nil {
						log.Warnf("error deleting local message: %s", err)
					}
				}
			}
		})

		mp.clearPending()
		mp.republished = nil

		return
	}

	mp.forEachPending(func(a address.Address, ms *msgSet) {
		isLocal, err := mp.isLocal(ctx, a)
		if err != nil {
			log.Warnf("errored while determining isLocal: %w", err)
			return
		}

		if isLocal {
			return
		}

		if err = mp.deletePendingMset(ctx, a); err != nil {
			log.Warnf("errored while deleting mset: %w", err)
			return
		}
	})
}

func getBaseFeeLowerBound(baseFee, factor types.BigInt) types.BigInt {
	baseFeeLowerBound := types.BigDiv(baseFee, factor)
	if baseFeeLowerBound.LessThan(minimumBaseFee) {
		baseFeeLowerBound = minimumBaseFee
	}

	return baseFeeLowerBound
}

type MpoolNonceAPI interface {
	GetNonce(context.Context, address.Address, types.TipSetKey) (uint64, error)
	GetActor(context.Context, address.Address, types.TipSetKey) (*types.Actor, error)
}
