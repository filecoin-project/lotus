package sub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/xerrors"

	address "github.com/filecoin-project/go-address"
	lru "github.com/hashicorp/golang-lru"
	blocks "github.com/ipfs/go-block-format"
	bserv "github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	connmgr "github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/lib/bufbstore"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/metrics"
)

var log = logging.Logger("sub")

var ErrSoftFailure = errors.New("soft validation failure")
var ErrInsufficientPower = errors.New("incoming block's miner does not have minimum power")

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
				log.Errorf("failed to fetch all bls messages for block received over pubusb: %s; source: %s", err, src)
				return
			}

			smsgs, err := FetchSignedMessagesByCids(ctx, ses, blk.SecpkMessages)
			if err != nil {
				log.Errorf("failed to fetch all secpk messages for block received over pubusb: %s; source: %s", err, src)
				return
			}

			took := build.Clock.Since(start)
			log.Infow("new block over pubsub", "cid", blk.Header.Cid(), "source", msg.GetFrom(), "msgfetch", took)
			if delay := build.Clock.Now().Unix() - int64(blk.Header.Timestamp); delay > 5 {
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
//  by the fetch message functions above.
// We check that each block is received only once and we do not received
//  blocks we did not request.
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
		cidIndex[c] = i
	}
	if len(cids) != len(cidIndex) {
		return fmt.Errorf("duplicate CIDs in fetchCids input")
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
			err = fmt.Errorf("failed to fetch %d messages for unknown reasons", len(cidIndex))
		}
		return err
	}

	return nil
}

type BlockValidator struct {
	self peer.ID

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

func NewBlockValidator(self peer.ID, chain *store.ChainStore, stmgr *stmgr.StateManager, blacklist func(peer.ID)) *BlockValidator {
	p, _ := lru.New2Q(4096)
	return &BlockValidator{
		self:       self,
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
	if pid == bv.self {
		return bv.validateLocalBlock(ctx, msg)
	}

	// track validation time
	begin := build.Clock.Now()
	defer func() {
		log.Debugf("block validation time: %s", build.Clock.Since(begin))
	}()

	stats.Record(ctx, metrics.BlockReceived.M(1))

	recordFailureFlagPeer := func(what string) {
		recordFailure(ctx, metrics.BlockValidationFailure, what)
		bv.flagPeer(pid)
	}

	blk, what, err := bv.decodeAndCheckBlock(msg)
	if err != nil {
		log.Error("got invalid block over pubsub: ", err)
		recordFailureFlagPeer(what)
		return pubsub.ValidationReject
	}

	// validate the block meta: the Message CID in the header must match the included messages
	err = bv.validateMsgMeta(ctx, blk)
	if err != nil {
		log.Warnf("error validating message metadata: %s", err)
		recordFailureFlagPeer("invalid_block_meta")
		return pubsub.ValidationReject
	}

	// we want to ensure that it is a block from a known miner; we reject blocks from unknown miners
	// to prevent spam attacks.
	// the logic works as follows: we lookup the miner in the chain for its key.
	// if we can find it then it's a known miner and we can validate the signature.
	// if we can't find it, we check whether we are (near) synced in the chain.
	// if we are not synced we cannot validate the block and we must ignore it.
	// if we are synced and the miner is unknown, then the block is rejcected.
	key, err := bv.checkPowerAndGetWorkerKey(ctx, blk.Header)
	if err != nil {
		if err != ErrSoftFailure && bv.isChainNearSynced() {
			log.Warnf("received block from unknown miner or miner that doesn't meet min power over pubsub; rejecting message")
			recordFailureFlagPeer("unknown_miner")
			return pubsub.ValidationReject
		}

		log.Warnf("cannot validate block message; unknown miner or miner that doesn't meet min power in unsynced chain")
		return pubsub.ValidationIgnore
	}

	err = sigs.CheckBlockSignature(ctx, blk.Header, key)
	if err != nil {
		log.Errorf("block signature verification failed: %s", err)
		recordFailureFlagPeer("signature_verification_failed")
		return pubsub.ValidationReject
	}

	if blk.Header.ElectionProof.WinCount < 1 {
		log.Errorf("block is not claiming to be winning")
		recordFailureFlagPeer("not_winning")
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

func (bv *BlockValidator) validateLocalBlock(ctx context.Context, msg *pubsub.Message) pubsub.ValidationResult {
	stats.Record(ctx, metrics.BlockPublished.M(1))

	blk, what, err := bv.decodeAndCheckBlock(msg)
	if err != nil {
		log.Errorf("got invalid local block: %s", err)
		ctx, _ = tag.New(ctx, tag.Insert(metrics.FailureType, what))
		stats.Record(ctx, metrics.BlockValidationFailure.M(1))
		return pubsub.ValidationIgnore
	}

	if count := bv.recvBlocks.add(blk.Header.Cid()); count > 0 {
		log.Warnf("local block has been seen %d times; ignoring", count)
		return pubsub.ValidationIgnore
	}

	msg.ValidatorData = blk
	stats.Record(ctx, metrics.BlockValidationSuccess.M(1))
	return pubsub.ValidationAccept
}

func (bv *BlockValidator) decodeAndCheckBlock(msg *pubsub.Message) (*types.BlockMsg, string, error) {
	blk, err := types.DecodeBlockMsg(msg.GetData())
	if err != nil {
		return nil, "invalid", xerrors.Errorf("error decoding block: %w", err)
	}

	if count := len(blk.BlsMessages) + len(blk.SecpkMessages); count > build.BlockMessageLimit {
		return nil, "too_many_messages", fmt.Errorf("block contains too many messages (%d)", count)
	}

	// make sure we have a signature
	if blk.Header.BlockSig == nil {
		return nil, "missing_signature", fmt.Errorf("block without a signature")
	}

	return blk, "", nil
}

func (bv *BlockValidator) isChainNearSynced() bool {
	ts := bv.chain.GetHeaviestTipSet()
	timestamp := ts.MinTimestamp()
	timestampTime := time.Unix(int64(timestamp), 0)
	return build.Clock.Since(timestampTime) < 6*time.Hour
}

func (bv *BlockValidator) validateMsgMeta(ctx context.Context, msg *types.BlockMsg) error {
	// TODO there has to be a simpler way to do this without the blockstore dance
	// block headers use adt0
	store := adt0.WrapStore(ctx, cbor.NewCborStore(blockstore.NewTemporary()))
	bmArr := adt0.MakeEmptyArray(store)
	smArr := adt0.MakeEmptyArray(store)

	for i, m := range msg.BlsMessages {
		c := cbg.CborCid(m)
		if err := bmArr.Set(uint64(i), &c); err != nil {
			return err
		}
	}

	for i, m := range msg.SecpkMessages {
		c := cbg.CborCid(m)
		if err := smArr.Set(uint64(i), &c); err != nil {
			return err
		}
	}

	bmroot, err := bmArr.Root()
	if err != nil {
		return err
	}

	smroot, err := smArr.Root()
	if err != nil {
		return err
	}

	mrcid, err := store.Put(store.Context(), &types.MsgMeta{
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

func (bv *BlockValidator) checkPowerAndGetWorkerKey(ctx context.Context, bh *types.BlockHeader) (address.Address, error) {
	addr := bh.Miner

	bv.mx.Lock()
	key, ok := bv.keycache[addr.String()]
	bv.mx.Unlock()
	if !ok {
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

		mst, err := miner.Load(bv.chain.Store(ctx), act)
		if err != nil {
			return address.Undef, err
		}

		info, err := mst.Info()
		if err != nil {
			return address.Undef, err
		}

		worker := info.Worker
		key, err = bv.stmgr.ResolveToKeyAddress(ctx, worker, ts)
		if err != nil {
			return address.Undef, err
		}

		bv.mx.Lock()
		bv.keycache[addr.String()] = key
		bv.mx.Unlock()
	}

	// we check that the miner met the minimum power at the lookback tipset

	baseTs := bv.chain.GetHeaviestTipSet()
	lbts, err := stmgr.GetLookbackTipSetForRound(ctx, bv.stmgr, baseTs, bh.Height)
	if err != nil {
		log.Warnf("failed to load lookback tipset for incoming block: %s", err)
		return address.Undef, ErrSoftFailure
	}

	eligible, err := stmgr.MinerEligibleToMine(ctx, bv.stmgr, bh.Miner, baseTs, lbts)
	if err != nil {
		log.Warnf("failed to determine if incoming block's miner has minimum power: %s", err)
		return address.Undef, ErrSoftFailure
	}

	if !eligible {
		log.Warnf("incoming block's miner is ineligible")
		return address.Undef, ErrInsufficientPower
	}

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
		case xerrors.Is(err, messagepool.ErrNonceTooLow):
			return pubsub.ValidationIgnore
		default:
			return pubsub.ValidationReject
		}
	}
	stats.Record(ctx, metrics.MessageValidationSuccess.M(1))
	return pubsub.ValidationAccept
}

func (mv *MessageValidator) validateLocalMessage(ctx context.Context, msg *pubsub.Message) pubsub.ValidationResult {
	ctx, _ = tag.New(
		ctx,
		tag.Upsert(metrics.Local, "true"),
	)
	// do some lightweight validation
	stats.Record(ctx, metrics.MessagePublished.M(1))

	m, err := types.DecodeSignedMessage(msg.Message.GetData())
	if err != nil {
		log.Warnf("failed to decode local message: %s", err)
		recordFailure(ctx, metrics.MessageValidationFailure, "decode")
		return pubsub.ValidationIgnore
	}

	if m.Size() > 32*1024 {
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
