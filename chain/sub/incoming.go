package sub

import (
	"context"
	"time"

	logging "github.com/ipfs/go-log/v2"
	connmgr "github.com/libp2p/go-libp2p-core/connmgr"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
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

		blk, err := types.DecodeBlockMsg(msg.GetData())
		if err != nil {
			log.Error("got invalid block over pubsub: ", err)
			continue
		}

		if len(blk.BlsMessages)+len(blk.SecpkMessages) > build.BlockMessageLimit {
			log.Warnf("received block with too many messages over pubsub")
			continue
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

func HandleIncomingMessages(ctx context.Context, mpool *messagepool.MessagePool, msub *pubsub.Subscription) {
	for {
		msg, err := msub.Next(ctx)
		if err != nil {
			log.Warn("error from message subscription: ", err)
			if ctx.Err() != nil {
				log.Warn("quitting HandleIncomingMessages loop")
				return
			}
			continue
		}

		m, err := types.DecodeSignedMessage(msg.GetData())
		if err != nil {
			log.Errorf("got incorrectly formatted Message: %s", err)
			continue
		}

		if err := mpool.Add(m); err != nil {
			log.Warnf("failed to add message from network to message pool (From: %s, To: %s, Nonce: %d, Value: %s): %s", m.Message.From, m.Message.To, m.Message.Nonce, types.FIL(m.Message.Value), err)
			continue
		}
	}
}
