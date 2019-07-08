package sub

import (
	"context"
	"fmt"

	logging "github.com/ipfs/go-log"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/filecoin-project/go-lotus/chain"
)

var log = logging.Logger("sub")

func HandleIncomingBlocks(ctx context.Context, bsub *pubsub.Subscription, s *chain.Syncer) {
	for {
		msg, err := bsub.Next(ctx)
		if err != nil {
			fmt.Println("error from block subscription: ", err)
			continue
		}

		blk, err := chain.DecodeBlockMsg(msg.GetData())
		if err != nil {
			log.Error("got invalid block over pubsub: ", err)
			continue
		}

		go func() {
			msgs, err := s.Bsync.FetchMessagesByCids(blk.Messages)
			if err != nil {
				log.Errorf("failed to fetch all messages for block received over pubusb: %s", err)
				return
			}
			fmt.Println("inform new block over pubsub")
			s.InformNewBlock(msg.GetFrom(), &chain.FullBlock{
				Header:   blk.Header,
				Messages: msgs,
			})
		}()
	}
}

func HandleIncomingMessages(ctx context.Context, mpool *chain.MessagePool, msub *pubsub.Subscription) {
	for {
		msg, err := msub.Next(ctx)
		if err != nil {
			fmt.Println("error from message subscription: ", err)
			continue
		}

		m, err := chain.DecodeSignedMessage(msg.GetData())
		if err != nil {
			log.Errorf("got incorrectly formatted Message: %s", err)
			continue
		}

		if err := mpool.Add(m); err != nil {
			log.Errorf("failed to add message from network to message pool: %s", err)
			continue
		}
	}
}
