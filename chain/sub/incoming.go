package sub

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/xerrors"

	address "github.com/filecoin-project/go-address"
	amt "github.com/filecoin-project/go-amt-ipld/v2"
	miner "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	connmgr "github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/bufbstore"
	"github.com/filecoin-project/lotus/lib/sigs"
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
			return
		}

		//nolint:golint
		src := peer.ID(msg.GetFrom())

		go func() {
			log.Infof("New block over pubsub: %s", blk.Cid())

			start := time.Now()
			log.Debug("about to fetch messages for block from pubsub")
			bmsgs, err := s.Bsync.FetchMessagesByCids(context.TODO(), blk.BlsMessages)
			if err != nil {
				log.Errorf("failed to fetch all bls messages for block received over pubusb: %s; source: %s", err, src)
				return
			}

			smsgs, err := s.Bsync.FetchSignedMessagesByCids(context.TODO(), blk.SecpkMessages)
			if err != nil {
				log.Errorf("failed to fetch all secpk messages for block received over pubusb: %s; source: %s", err, src)
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

	// necessary for block validation
	chain *store.ChainStore
	stmgr *stmgr.StateManager

	mx       sync.Mutex
	keycache map[string]address.Address
}

func NewBlockValidator(chain *store.ChainStore, stmgr *stmgr.StateManager, blacklist func(peer.ID)) *BlockValidator {
	p, _ := lru.New2Q(4096)
	return &BlockValidator{
		peers:      p,
		killThresh: 10,
		blacklist:  blacklist,
		recvBlocks: newBlockReceiptCache(),
		chain:      chain,
		stmgr:      stmgr,
		keycache:   make(map[string]address.Address),
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
		log.Warnf("blacklisting peer %s", p)
		bv.blacklist(p)
		return
	}

	bv.peers.Add(p, v.(int)+1)
}

func (bv *BlockValidator) Validate(ctx context.Context, pid peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	// track validation time
	begin := time.Now()
	defer func() {
		end := time.Now()
		log.Infof("block validation time: %s", end.Sub(begin))
	}()

	stats.Record(ctx, metrics.BlockReceived.M(1))

	recordFailure := func(what string) {
		ctx, _ = tag.New(ctx, tag.Insert(metrics.FailureType, what))
		stats.Record(ctx, metrics.BlockValidationFailure.M(1))
		bv.flagPeer(pid)
	}

	// make sure the block can be decoded
	blk, err := types.DecodeBlockMsg(msg.GetData())
	if err != nil {
		log.Error("got invalid block over pubsub: ", err)
		recordFailure("invalid")
		return pubsub.ValidationReject
	}

	// check the message limit constraints
	if len(blk.BlsMessages)+len(blk.SecpkMessages) > build.BlockMessageLimit {
		log.Warnf("received block with too many messages over pubsub")
		recordFailure("too_many_messages")
		return pubsub.ValidationReject
	}

	// make sure we have a signature
	if blk.Header.BlockSig == nil {
		log.Warnf("received block without a signature over pubsub")
		recordFailure("missing_signature")
		return pubsub.ValidationReject
	}

	// validate the block meta: the Message CID in the header must match the included messages
	err = bv.validateMsgMeta(ctx, blk)
	if err != nil {
		log.Warnf("error validating message metadata: %s", err)
		recordFailure("invalid_block_meta")
		return pubsub.ValidationReject
	}

	// we want to ensure that it is a block from a known miner; we reject blocks from unknown miners
	// to prevent spam attacks.
	// the logic works as follows: we lookup the miner in the chain for its key.
	// if we can find it then it's a known miner and we can validate the signature.
	// if we can't find it, we check whether we are (near) synced in the chain.
	// if we are not synced we cannot validate the block and we must ignore it.
	// if we are synced and the miner is unknown, then the block is rejcected.
	key, err := bv.getMinerWorkerKey(ctx, blk)
	if err != nil {
		if bv.isChainNearSynced() {
			log.Warnf("received block message from unknown miner over pubsub; rejecting message")
			recordFailure("unknown_miner")
			return pubsub.ValidationReject
		} else {
			log.Warnf("cannot validate block message; unknown miner in unsynced chain")
			return pubsub.ValidationIgnore
		}
	}

	err = sigs.CheckBlockSignature(blk.Header, ctx, key)
	if err != nil {
		log.Errorf("block signature verification failed: %s", err)
		recordFailure("signature_verification_failed")
		return pubsub.ValidationReject
	}

	// it's a good block! make sure we've only seen it once
	if bv.recvBlocks.add(blk.Header.Cid()) > 0 {
		// TODO: once these changes propagate to the network, we can consider
		// dropping peers who send us the same block multiple times
		return pubsub.ValidationIgnore
	}

	// all good, accept the block
	msg.ValidatorData = blk
	stats.Record(ctx, metrics.BlockValidationSuccess.M(1))
	return pubsub.ValidationAccept
}

func (bv *BlockValidator) isChainNearSynced() bool {
	ts := bv.chain.GetHeaviestTipSet()
	timestamp := ts.MinTimestamp()
	now := time.Now().UnixNano()
	cutoff := uint64(now) - uint64(6*time.Hour)
	return timestamp > cutoff
}

func (bv *BlockValidator) validateMsgMeta(ctx context.Context, msg *types.BlockMsg) error {
	var bcids, scids []cbg.CBORMarshaler
	for _, m := range msg.BlsMessages {
		c := cbg.CborCid(m)
		bcids = append(bcids, &c)
	}

	for _, m := range msg.SecpkMessages {
		c := cbg.CborCid(m)
		scids = append(scids, &c)
	}

	// TODO there has to be a simpler way to do this without the blockstore dance
	bs := cbor.NewCborStore(bstore.NewBlockstore(dstore.NewMapDatastore()))

	bmroot, err := amt.FromArray(ctx, bs, bcids)
	if err != nil {
		return err
	}

	smroot, err := amt.FromArray(ctx, bs, scids)
	if err != nil {
		return err
	}

	mrcid, err := bs.Put(ctx, &types.MsgMeta{
		BlsMessages:   bmroot,
		SecpkMessages: smroot,
	})

	if err != nil {
		return err
	}

	if msg.Header.Messages != mrcid {
		return fmt.Errorf("messages didn't match root cid in header")
	}

	return nil
}

func (bv *BlockValidator) getMinerWorkerKey(ctx context.Context, msg *types.BlockMsg) (address.Address, error) {
	addr := msg.Header.Miner

	bv.mx.Lock()
	key, ok := bv.keycache[addr.String()]
	bv.mx.Unlock()
	if ok {
		return key, nil
	}

	// TODO I have a feeling all this can be simplified by cleverer DI to use the API
	ts := bv.chain.GetHeaviestTipSet()
	st, _, err := bv.stmgr.TipSetState(ctx, ts)
	if err != nil {
		return address.Undef, err
	}
	buf := bufbstore.NewBufferedBstore(bv.chain.Blockstore())
	cst := cbor.NewCborStore(buf)
	state, err := state.LoadStateTree(cst, st)
	if err != nil {
		return address.Undef, err
	}
	act, err := state.GetActor(addr)
	if err != nil {
		return address.Undef, err
	}

	blk, err := bv.chain.Blockstore().Get(act.Head)
	if err != nil {
		return address.Undef, err
	}
	aso := blk.RawData()

	var mst miner.State
	err = mst.UnmarshalCBOR(bytes.NewReader(aso))
	if err != nil {
		return address.Undef, err
	}

	worker := mst.Info.Worker
	key, err = bv.stmgr.ResolveToKeyAddress(ctx, worker, ts)
	if err != nil {
		return address.Undef, err
	}

	bv.mx.Lock()
	bv.keycache[addr.String()] = key
	bv.mx.Unlock()

	return key, nil
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

func (mv *MessageValidator) Validate(ctx context.Context, pid peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	stats.Record(ctx, metrics.MessageReceived.M(1))
	m, err := types.DecodeSignedMessage(msg.Message.GetData())
	if err != nil {
		log.Warnf("failed to decode incoming message: %s", err)
		ctx, _ = tag.New(ctx, tag.Insert(metrics.FailureType, "decode"))
		stats.Record(ctx, metrics.MessageValidationFailure.M(1))
		return pubsub.ValidationReject
	}

	if err := mv.mpool.Add(m); err != nil {
		log.Debugf("failed to add message from network to message pool (From: %s, To: %s, Nonce: %d, Value: %s): %s", m.Message.From, m.Message.To, m.Message.Nonce, types.FIL(m.Message.Value), err)
		ctx, _ = tag.New(
			ctx,
			tag.Insert(metrics.FailureType, "add"),
		)
		stats.Record(ctx, metrics.MessageValidationFailure.M(1))
		switch {
		case xerrors.Is(err, messagepool.ErrBroadcastAnyway):
			return pubsub.ValidationIgnore
		default:
			return pubsub.ValidationReject
		}
	}
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
