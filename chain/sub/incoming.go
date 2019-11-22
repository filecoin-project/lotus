package sub

import (
	"context"

	logging "github.com/ipfs/go-log"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("sub")

func HandleIncomingBlocks(ctx context.Context, bsub *pubsub.Subscription, s *chain.Syncer) {
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

		go func() {
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

			log.Debugw("new block over pubsub", "cid", blk.Header.Cid(), "source", msg.GetFrom())
			s.InformNewBlock(msg.GetFrom(), &types.FullBlock{
				Header:        blk.Header,
				BlsMessages:   bmsgs,
				SecpkMessages: smsgs,
			})
		}()
	}
}

func HandleIncomingMessages(ctx context.Context, mpool *chain.MessagePool, msub *pubsub.Subscription) {
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
			log.Warnf("failed to add message from network to message pool (From: %s, To: %s, Nonce: %d, Value: %s): %+v", m.Message.From, m.Message.To, m.Message.Nonce, types.FIL(m.Message.Value), err)
			continue
		}
	}
}
