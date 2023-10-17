package sub

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	bserv "github.com/ipfs/boxo/blockservice"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipni/go-libipni/announce/message"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/sub/ratelimit"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node/impl/client"
	"github.com/filecoin-project/lotus/node/impl/full"
)

var log = logging.Logger("sub")

var msgCidPrefix = cid.Prefix{
	Version:  1,
	Codec:    cid.DagCBOR,
	MhType:   client.DefaultHashFunction,
	MhLength: 32,
}

func HandleIncomingBlocks(ctx context.Context, bsub *pubsub.Subscription, s *chain.Syncer, bs bserv.BlockService, cmgr connmgr.ConnManager) {
	// Timeout after (block time + propagation delay). This is useless at
	// this point.
	timeout := time.Duration(build.BlockDelaySecs+build.PropagationDelaySecs) * time.Second

	for {
		msg, err := bsub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Warn("quitting HandleIncomingBlocks loop")
				return
			}
			log.Error("error from block subscription: ", err)
			continue
		}

		blk, ok := msg.ValidatorData.(*types.BlockMsg)
		if !ok {
			log.Warnf("pubsub block validator passed on wrong type: %#v", msg.ValidatorData)
			return
		}

		src := msg.GetFrom()

		go func() {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// NOTE: we could also share a single session between
			// all requests but that may have other consequences.
			ses := bserv.NewSession(ctx, bs)

			start := build.Clock.Now()
			log.Debug("about to fetch messages for block from pubsub")
			bmsgs, err := FetchMessagesByCids(ctx, ses, blk.BlsMessages)
			if err != nil {
				log.Errorf("failed to fetch all bls messages for block received over pubsub: %s; source: %s", err, src)
				return
			}

			smsgs, err := FetchSignedMessagesByCids(ctx, ses, blk.SecpkMessages)
			if err != nil {
				log.Errorf("failed to fetch all secpk messages for block received over pubsub: %s; source: %s", err, src)
				return
			}

			took := build.Clock.Since(start)
			log.Debugw("new block over pubsub", "cid", blk.Header.Cid(), "source", msg.GetFrom(), "msgfetch", took)
			if took > 3*time.Second {
				log.Warnw("Slow msg fetch", "cid", blk.Header.Cid(), "source", msg.GetFrom(), "msgfetch", took)
			}
			if delay := build.Clock.Now().Unix() - int64(blk.Header.Timestamp); delay > 5 {
				_ = stats.RecordWithTags(ctx,
					[]tag.Mutator{tag.Insert(metrics.MinerID, blk.Header.Miner.String())},
					metrics.BlockDelay.M(delay),
				)
				log.Warnw("received block with large delay from miner", "block", blk.Cid(), "delay", delay, "miner", blk.Header.Miner)
			}

			if s.InformNewBlock(msg.ReceivedFrom, &types.FullBlock{
				Header:        blk.Header,
				BlsMessages:   bmsgs,
				SecpkMessages: smsgs,
			}) {
				cmgr.TagPeer(msg.ReceivedFrom, "blkprop", 5)
			}
		}()
	}
}

func FetchMessagesByCids(
	ctx context.Context,
	bserv bserv.BlockGetter,
	cids []cid.Cid,
) ([]*types.Message, error) {
	out := make([]*types.Message, len(cids))

	err := fetchCids(ctx, bserv, cids, func(i int, b blocks.Block) error {
		msg, err := types.DecodeMessage(b.RawData())
		if err != nil {
			return err
		}

		out[i] = msg
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FIXME: Duplicate of above.
func FetchSignedMessagesByCids(
	ctx context.Context,
	bserv bserv.BlockGetter,
	cids []cid.Cid,
) ([]*types.SignedMessage, error) {
	out := make([]*types.SignedMessage, len(cids))

	err := fetchCids(ctx, bserv, cids, func(i int, b blocks.Block) error {
		smsg, err := types.DecodeSignedMessage(b.RawData())
		if err != nil {
			return err
		}

		out[i] = smsg
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Fetch `cids` from the block service, apply `cb` on each of them. Used
//
//	by the fetch message functions above.
//
// We check that each block is received only once and we do not received
//
//	blocks we did not request.
func fetchCids(
	ctx context.Context,
	bserv bserv.BlockGetter,
	cids []cid.Cid,
	cb func(int, blocks.Block) error,
) error {

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cidIndex := make(map[cid.Cid]int)
	for i, c := range cids {
		if c.Prefix() != msgCidPrefix {
			return xerrors.Errorf("invalid msg CID: %s", c)
		}
		cidIndex[c] = i
	}
	if len(cids) != len(cidIndex) {
		return xerrors.Errorf("duplicate CIDs in fetchCids input")
	}

	for block := range bserv.GetBlocks(ctx, cids) {
		ix, ok := cidIndex[block.Cid()]
		if !ok {
			// Ignore duplicate/unexpected blocks. This shouldn't
			// happen, but we can be safe.
			log.Errorw("received duplicate/unexpected block when syncing", "cid", block.Cid())
			continue
		}

		// Record that we've received the block.
		delete(cidIndex, block.Cid())

		if err := cb(ix, block); err != nil {
			return err
		}
	}

	if len(cidIndex) > 0 {
		err := ctx.Err()
		if err == nil {
			err = xerrors.Errorf("failed to fetch %d messages for unknown reasons", len(cidIndex))
		}
		return err
	}

	return nil
}

type BlockValidator struct {
	self peer.ID

	peers *lru.TwoQueueCache[peer.ID, int]

	killThresh int

	recvBlocks *blockReceiptCache

	blacklist func(peer.ID)

	// necessary for block validation
	chain     *store.ChainStore
	consensus consensus.Consensus
}

func NewBlockValidator(self peer.ID, chain *store.ChainStore, cns consensus.Consensus, blacklist func(peer.ID)) *BlockValidator {
	p, _ := lru.New2Q[peer.ID, int](4096)
	return &BlockValidator{
		self:       self,
		peers:      p,
		killThresh: 10,
		blacklist:  blacklist,
		recvBlocks: newBlockReceiptCache(),
		chain:      chain,
		consensus:  cns,
	}
}

func (bv *BlockValidator) flagPeer(p peer.ID) {
	val, ok := bv.peers.Get(p)
	if !ok {
		bv.peers.Add(p, 1)
		return
	}

	if val >= bv.killThresh {
		log.Warnf("blacklisting peer %s", p)
		bv.blacklist(p)
		return
	}

	bv.peers.Add(p, val+1)
}

func (bv *BlockValidator) Validate(ctx context.Context, pid peer.ID, msg *pubsub.Message) (res pubsub.ValidationResult) {
	defer func() {
		if rerr := recover(); rerr != nil {
			err := xerrors.Errorf("validate block: %s", rerr)
			recordFailure(ctx, metrics.BlockValidationFailure, err.Error())
			bv.flagPeer(pid)
			res = pubsub.ValidationReject
			return
		}
	}()

	var what string
	res, what = consensus.ValidateBlockPubsub(ctx, bv.consensus, pid == bv.self, msg)
	if res == pubsub.ValidationAccept {
		// it's a good block! make sure we've only seen it once
		if count := bv.recvBlocks.add(msg.ValidatorData.(*types.BlockMsg).Cid()); count > 0 {
			if pid == bv.self {
				log.Warnf("local block has been seen %d times; ignoring", count)
			}

			// TODO: once these changes propagate to the network, we can consider
			// dropping peers who send us the same block multiple times
			return pubsub.ValidationIgnore
		}
	} else {
		recordFailure(ctx, metrics.BlockValidationFailure, what)
	}

	return res
}

type blockReceiptCache struct {
	blocks *lru.TwoQueueCache[cid.Cid, int]
}

func newBlockReceiptCache() *blockReceiptCache {
	c, _ := lru.New2Q[cid.Cid, int](8192)

	return &blockReceiptCache{
		blocks: c,
	}
}

func (brc *blockReceiptCache) add(bcid cid.Cid) int {
	val, ok := brc.blocks.Get(bcid)
	if !ok {
		brc.blocks.Add(bcid, 1)
		return 0
	}

	brc.blocks.Add(bcid, val+1)
	return val
}

type MessageValidator struct {
	self  peer.ID
	mpool *messagepool.MessagePool
}

func NewMessageValidator(self peer.ID, mp *messagepool.MessagePool) *MessageValidator {
	return &MessageValidator{self: self, mpool: mp}
}

func (mv *MessageValidator) Validate(ctx context.Context, pid peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	if pid == mv.self {
		return mv.validateLocalMessage(ctx, msg)
	}

	start := time.Now()
	defer func() {
		ms := time.Now().Sub(start).Microseconds()
		stats.Record(ctx, metrics.MessageValidationDuration.M(float64(ms)/1000))
	}()

	stats.Record(ctx, metrics.MessageReceived.M(1))
	m, err := types.DecodeSignedMessage(msg.Message.GetData())
	if err != nil {
		log.Warnf("failed to decode incoming message: %s", err)
		ctx, _ = tag.New(ctx, tag.Insert(metrics.FailureType, "decode"))
		stats.Record(ctx, metrics.MessageValidationFailure.M(1))
		return pubsub.ValidationReject
	}

	if err := mv.mpool.Add(ctx, m); err != nil {
		log.Debugf("failed to add message from network to message pool (From: %s, To: %s, Nonce: %d, Value: %s): %s", m.Message.From, m.Message.To, m.Message.Nonce, types.FIL(m.Message.Value), err)
		ctx, _ = tag.New(
			ctx,
			tag.Upsert(metrics.Local, "false"),
		)
		recordFailure(ctx, metrics.MessageValidationFailure, "add")
		switch {

		case xerrors.Is(err, messagepool.ErrSoftValidationFailure):
			fallthrough
		case xerrors.Is(err, messagepool.ErrRBFTooLowPremium):
			fallthrough
		case xerrors.Is(err, messagepool.ErrTooManyPendingMessages):
			fallthrough
		case xerrors.Is(err, messagepool.ErrNonceGap):
			fallthrough
		case xerrors.Is(err, messagepool.ErrGasFeeCapTooLow):
			fallthrough
		case xerrors.Is(err, messagepool.ErrNonceTooLow):
			fallthrough
		case xerrors.Is(err, messagepool.ErrNotEnoughFunds):
			fallthrough
		case xerrors.Is(err, messagepool.ErrExistingNonce):
			return pubsub.ValidationIgnore

		case xerrors.Is(err, messagepool.ErrMessageTooBig):
			fallthrough
		case xerrors.Is(err, messagepool.ErrMessageValueTooHigh):
			fallthrough
		case xerrors.Is(err, messagepool.ErrInvalidToAddr):
			fallthrough
		default:
			return pubsub.ValidationReject
		}
	}

	ctx, _ = tag.New(
		ctx,
		tag.Upsert(metrics.MsgValid, "true"),
	)

	stats.Record(ctx, metrics.MessageValidationSuccess.M(1))
	return pubsub.ValidationAccept
}

func (mv *MessageValidator) validateLocalMessage(ctx context.Context, msg *pubsub.Message) pubsub.ValidationResult {
	ctx, _ = tag.New(
		ctx,
		tag.Upsert(metrics.Local, "true"),
	)

	start := time.Now()
	defer func() {
		ms := time.Now().Sub(start).Microseconds()
		stats.Record(ctx, metrics.MessageValidationDuration.M(float64(ms)/1000))
	}()

	// do some lightweight validation
	stats.Record(ctx, metrics.MessagePublished.M(1))

	m, err := types.DecodeSignedMessage(msg.Message.GetData())
	if err != nil {
		log.Warnf("failed to decode local message: %s", err)
		recordFailure(ctx, metrics.MessageValidationFailure, "decode")
		return pubsub.ValidationIgnore
	}

	if m.Size() > messagepool.MaxMessageSize {
		log.Warnf("local message is too large! (%dB)", m.Size())
		recordFailure(ctx, metrics.MessageValidationFailure, "oversize")
		return pubsub.ValidationIgnore
	}

	if m.Message.To == address.Undef {
		log.Warn("local message has invalid destination address")
		recordFailure(ctx, metrics.MessageValidationFailure, "undef-addr")
		return pubsub.ValidationIgnore
	}

	if !m.Message.Value.LessThan(types.TotalFilecoinInt) {
		log.Warnf("local messages has too high value: %s", m.Message.Value)
		recordFailure(ctx, metrics.MessageValidationFailure, "value-too-high")
		return pubsub.ValidationIgnore
	}

	if err := mv.mpool.VerifyMsgSig(m); err != nil {
		log.Warnf("signature verification failed for local message: %s", err)
		recordFailure(ctx, metrics.MessageValidationFailure, "verify-sig")
		return pubsub.ValidationIgnore
	}

	ctx, _ = tag.New(
		ctx,
		tag.Upsert(metrics.MsgValid, "true"),
	)

	stats.Record(ctx, metrics.MessageValidationSuccess.M(1))
	return pubsub.ValidationAccept
}

func HandleIncomingMessages(ctx context.Context, mpool *messagepool.MessagePool, msub *pubsub.Subscription) {
	for {
		_, err := msub.Next(ctx)
		if err != nil {
			log.Warn("error from message subscription: ", err)
			if ctx.Err() != nil {
				log.Warn("quitting HandleIncomingMessages loop")
				return
			}
			continue
		}

		// Do nothing... everything happens in validate
	}
}

func recordFailure(ctx context.Context, metric *stats.Int64Measure, failureType string) {
	ctx, _ = tag.New(
		ctx,
		tag.Upsert(metrics.FailureType, failureType),
	)
	stats.Record(ctx, metric.M(1))
}

type peerMsgInfo struct {
	peerID    peer.ID
	lastCid   cid.Cid
	lastSeqno uint64
	rateLimit *ratelimit.Window
	mutex     sync.Mutex
}

type IndexerMessageValidator struct {
	self peer.ID

	peerCache *lru.TwoQueueCache[address.Address, *peerMsgInfo]
	chainApi  full.ChainModuleAPI
	stateApi  full.StateModuleAPI
}

func NewIndexerMessageValidator(self peer.ID, chainApi full.ChainModuleAPI, stateApi full.StateModuleAPI) *IndexerMessageValidator {
	peerCache, _ := lru.New2Q[address.Address, *peerMsgInfo](8192)

	return &IndexerMessageValidator{
		self:      self,
		peerCache: peerCache,
		chainApi:  chainApi,
		stateApi:  stateApi,
	}
}

func (v *IndexerMessageValidator) Validate(ctx context.Context, pid peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	// This chain-node should not be publishing its own messages.  These are
	// relayed from market-nodes.  If a node appears to be local, reject it.
	if pid == v.self {
		log.Debug("ignoring indexer message from self")
		stats.Record(ctx, metrics.IndexerMessageValidationFailure.M(1))
		return pubsub.ValidationIgnore
	}
	originPeer := msg.GetFrom()
	if originPeer == v.self {
		log.Debug("ignoring indexer message originating from self")
		stats.Record(ctx, metrics.IndexerMessageValidationFailure.M(1))
		return pubsub.ValidationIgnore
	}

	idxrMsg := message.Message{}
	err := idxrMsg.UnmarshalCBOR(bytes.NewBuffer(msg.Data))
	if err != nil {
		log.Errorw("Could not decode indexer pubsub message", "err", err)
		return pubsub.ValidationReject
	}
	if len(idxrMsg.ExtraData) == 0 {
		log.Debugw("ignoring messsage missing miner id", "peer", originPeer)
		return pubsub.ValidationIgnore
	}

	// Get miner info from lotus
	minerAddr, err := address.NewFromBytes(idxrMsg.ExtraData)
	if err != nil {
		log.Warnw("cannot parse extra data as miner address", "err", err, "extraData", idxrMsg.ExtraData)
		return pubsub.ValidationReject
	}

	msgCid := idxrMsg.Cid

	msgInfo, cached := v.peerCache.Get(minerAddr)
	if !cached {
		msgInfo = &peerMsgInfo{}
	}

	// Lock this peer's message info.
	msgInfo.mutex.Lock()
	defer msgInfo.mutex.Unlock()

	var seqno uint64
	if cached {
		// Reject replayed messages.
		seqno = binary.BigEndian.Uint64(msg.Message.GetSeqno())
		if seqno <= msgInfo.lastSeqno {
			log.Debugf("ignoring replayed indexer message")
			return pubsub.ValidationIgnore
		}
	}

	if !cached || originPeer != msgInfo.peerID {
		// Check that the miner ID maps to the peer that sent the message.
		err = v.authenticateMessage(ctx, minerAddr, originPeer)
		if err != nil {
			log.Warnw("cannot authenticate messsage", "err", err, "peer", originPeer, "minerID", minerAddr)
			stats.Record(ctx, metrics.IndexerMessageValidationFailure.M(1))
			return pubsub.ValidationReject
		}
		msgInfo.peerID = originPeer
		if !cached {
			// Add msgInfo to cache only after being authenticated.  If two
			// messages from the same peer are handled concurrently, there is a
			// small chance that one msgInfo could replace the other here when
			// the info is first cached.  This is OK, so no need to prevent it.
			v.peerCache.Add(minerAddr, msgInfo)
		}
	}

	// Update message info cache with the latest message's sequence number.
	msgInfo.lastSeqno = seqno

	// See if message needs to be ignored due to rate limiting.
	if v.rateLimitPeer(msgInfo, msgCid) {
		return pubsub.ValidationIgnore
	}

	stats.Record(ctx, metrics.IndexerMessageValidationSuccess.M(1))
	return pubsub.ValidationAccept
}

func (v *IndexerMessageValidator) rateLimitPeer(msgInfo *peerMsgInfo, msgCid cid.Cid) bool {
	const (
		msgLimit        = 5
		msgTimeLimit    = 10 * time.Second
		repeatTimeLimit = 2 * time.Hour
	)

	timeWindow := msgInfo.rateLimit

	// Check overall message rate.
	if timeWindow == nil {
		timeWindow = ratelimit.NewWindow(msgLimit, msgTimeLimit)
		msgInfo.rateLimit = timeWindow
	} else if msgInfo.lastCid == msgCid {
		// Check if this is a repeat of the previous message data.
		if time.Since(timeWindow.Newest()) < repeatTimeLimit {
			log.Warnw("ignoring repeated indexer message", "sender", msgInfo.peerID)
			return true
		}
	}

	err := timeWindow.Add()
	if err != nil {
		log.Warnw("ignoring indexer message", "sender", msgInfo.peerID, "err", err)
		return true
	}

	msgInfo.lastCid = msgCid

	return false
}

func (v *IndexerMessageValidator) authenticateMessage(ctx context.Context, minerAddress address.Address, peerID peer.ID) error {
	ts, err := v.chainApi.ChainHead(ctx)
	if err != nil {
		return err
	}

	minerInfo, err := v.stateApi.StateMinerInfo(ctx, minerAddress, ts.Key())
	if err != nil {
		return err
	}

	if minerInfo.PeerId == nil {
		return xerrors.New("no peer id for miner")
	}
	if *minerInfo.PeerId != peerID {
		return xerrors.New("miner id does not map to peer that sent message")
	}

	return nil
}
