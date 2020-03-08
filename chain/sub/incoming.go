package sub

import (
	"context"
	"fmt"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	connmgr "github.com/libp2p/go-libp2p-core/connmgr"
	peer "github.com/libp2p/go-libp2p-peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"
)

var log = logging.Logger("sub")

func HandleIncomingBlocks(ctx context.Context, bsub *pubsub.Subscription, s *chain.Syncer, cmgr connmgr.ConnManager) {
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
		}

		go func() {
			log.Infof("New block over pubsub: %s", blk.Cid())

			start := time.Now()
			log.Debug("about to fetch messages for block from pubsub")
			bmsgs, err := s.Bsync.FetchMessagesByCids(context.TODO(), blk.BlsMessages)
			if err != nil {
				log.Errorf("failed to fetch all bls messages for block received over pubusb: %s", err)
				return
			}

			smsgs, err := s.Bsync.FetchSignedMessagesByCids(context.TODO(), blk.SecpkMessages)
			if err != nil {
				log.Errorf("failed to fetch all secpk messages for block received over pubusb: %s", err)
				return
			}

			took := time.Since(start)
			log.Infow("new block over pubsub", "cid", blk.Header.Cid(), "source", msg.GetFrom(), "msgfetch", took)
			if delay := time.Now().Unix() - int64(blk.Header.Timestamp); delay > 5 {
				log.Warnf("Received block with large delay %d from miner %s", delay, blk.Header.Miner)
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

type BlockValidator struct {
	peers *lru.TwoQueueCache

	killThresh int

	recvBlocks *blockReceiptCache

	blacklist func(peer.ID)
}

func NewBlockValidator(blacklist func(peer.ID)) *BlockValidator {
	p, _ := lru.New2Q(4096)
	return &BlockValidator{
		peers:      p,
		killThresh: 5,
		blacklist:  blacklist,
		recvBlocks: newBlockReceiptCache(),
	}
}

func (bv *BlockValidator) flagPeer(p peer.ID) {
	v, ok := bv.peers.Get(p)
	if !ok {
		bv.peers.Add(p, int(1))
		return
	}

	val := v.(int)

	if val >= bv.killThresh {
		bv.blacklist(p)
	}

	bv.peers.Add(p, v.(int)+1)
}

func (bv *BlockValidator) Validate(ctx context.Context, pid peer.ID, msg *pubsub.Message) bool {
	stats.Record(ctx, metrics.BlockReceived.M(1))
	ctx, _ = tag.New(
		ctx,
		tag.Insert(metrics.PeerID, pid.String()),
		tag.Insert(metrics.ReceivedFrom, msg.ReceivedFrom.String()),
	)
	blk, err := types.DecodeBlockMsg(msg.GetData())
	if err != nil {
		log.Error("got invalid block over pubsub: ", err)
		ctx, _ = tag.New(ctx, tag.Insert(metrics.FailureType, "invalid"))
		stats.Record(ctx, metrics.BlockValidationFailure.M(1))
		bv.flagPeer(pid)
		return false
	}

	if len(blk.BlsMessages)+len(blk.SecpkMessages) > build.BlockMessageLimit {
		log.Warnf("received block with too many messages over pubsub")
		ctx, _ = tag.New(ctx, tag.Insert(metrics.FailureType, "too_many_messages"))
		stats.Record(ctx, metrics.BlockValidationFailure.M(1))
		bv.flagPeer(pid)
		return false
	}

	if bv.recvBlocks.add(blk.Header.Cid()) > 0 {
		// TODO: once these changes propagate to the network, we can consider
		// dropping peers who send us the same block multiple times
		return false
	}

	msg.ValidatorData = blk
	stats.Record(ctx, metrics.BlockValidationSuccess.M(1))
	return true
}

type blockReceiptCache struct {
	blocks *lru.TwoQueueCache
}

func newBlockReceiptCache() *blockReceiptCache {
	c, _ := lru.New2Q(8192)

	return &blockReceiptCache{
		blocks: c,
	}
}

func (brc *blockReceiptCache) add(bcid cid.Cid) int {
	val, ok := brc.blocks.Get(bcid)
	if !ok {
		brc.blocks.Add(bcid, int(1))
		return 0
	}

	brc.blocks.Add(bcid, val.(int)+1)
	return val.(int)
}

type MessageValidator struct {
	mpool *messagepool.MessagePool
}

func NewMessageValidator(mp *messagepool.MessagePool) *MessageValidator {
	return &MessageValidator{mp}
}

func (mv *MessageValidator) Validate(ctx context.Context, pid peer.ID, msg *pubsub.Message) bool {
	stats.Record(ctx, metrics.MessageReceived.M(1))
	ctx, _ = tag.New(ctx, tag.Insert(metrics.PeerID, pid.String()))
	m, err := types.DecodeSignedMessage(msg.Message.GetData())
	if err != nil {
		log.Warnf("failed to decode incoming message: %s", err)
		ctx, _ = tag.New(ctx, tag.Insert(metrics.FailureType, "decode"))
		stats.Record(ctx, metrics.MessageValidationFailure.M(1))
		return false
	}

	if err := mv.mpool.Add(m); err != nil {
		log.Warnf("failed to add message from network to message pool (From: %s, To: %s, Nonce: %d, Value: %s): %s", m.Message.From, m.Message.To, m.Message.Nonce, types.FIL(m.Message.Value), err)
		ctx, _ = tag.New(
			ctx,
			tag.Insert(metrics.MessageFrom, m.Message.From.String()),
			tag.Insert(metrics.MessageTo, m.Message.To.String()),
			tag.Insert(metrics.MessageNonce, fmt.Sprint(m.Message.Nonce)),
			tag.Insert(metrics.FailureType, "add"),
		)
		stats.Record(ctx, metrics.MessageValidationFailure.M(1))
		return false
	}
	stats.Record(ctx, metrics.MessageValidationSuccess.M(1))
	return true
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
