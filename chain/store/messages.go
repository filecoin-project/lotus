package store

import (
	"context"
	"fmt"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

type storable interface {
	ToStorageBlock() (block.Block, error)
}

func PutMessage(ctx context.Context, bs bstore.Blockstore, m storable) (cid.Cid, error) {
	b, err := m.ToStorageBlock()
	if err != nil {
		return cid.Undef, err
	}

	if err := bs.Put(ctx, b); err != nil {
		return cid.Undef, err
	}

	return b.Cid(), nil
}

func (cs *ChainStore) PutMessage(ctx context.Context, m storable) (cid.Cid, error) {
	return PutMessage(ctx, cs.chainBlockstore, m)
}

func (cs *ChainStore) GetCMessage(ctx context.Context, c cid.Cid) (types.ChainMsg, error) {
	m, err := cs.GetMessage(ctx, c)
	if err == nil {
		return m, nil
	}
	if !ipld.IsNotFound(err) {
		log.Warnf("GetCMessage: unexpected error getting unsigned message: %s", err)
	}

	return cs.GetSignedMessage(ctx, c)
}

func (cs *ChainStore) GetMessage(ctx context.Context, c cid.Cid) (*types.Message, error) {
	var msg *types.Message
	err := cs.chainLocalBlockstore.View(ctx, c, func(b []byte) (err error) {
		msg, err = types.DecodeMessage(b)
		return err
	})
	return msg, err
}

func (cs *ChainStore) GetSignedMessage(ctx context.Context, c cid.Cid) (*types.SignedMessage, error) {
	var msg *types.SignedMessage
	err := cs.chainLocalBlockstore.View(ctx, c, func(b []byte) (err error) {
		msg, err = types.DecodeSignedMessage(b)
		return err
	})
	return msg, err
}

func (cs *ChainStore) readAMTCids(root cid.Cid) ([]cid.Cid, error) {
	ctx := context.TODO()
	// block headers use adt0, for now.
	a, err := blockadt.AsArray(cs.ActorStore(ctx), root)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
	}

	var (
		cids    []cid.Cid
		cborCid cbg.CborCid
	)
	if err := a.ForEach(&cborCid, func(i int64) error {
		c := cid.Cid(cborCid)
		cids = append(cids, c)
		return nil
	}); err != nil {
		return nil, xerrors.Errorf("failed to traverse amt: %w", err)
	}

	if uint64(len(cids)) != a.Length() {
		return nil, xerrors.Errorf("found %d cids, expected %d", len(cids), a.Length())
	}

	return cids, nil
}

type BlockMessages struct {
	Miner         address.Address
	BlsMessages   []types.ChainMsg
	SecpkMessages []types.ChainMsg
}

func (cs *ChainStore) BlockMsgsForTipset(ctx context.Context, ts *types.TipSet) ([]BlockMessages, error) {
	// returned BlockMessages match block order in tipset

	applied := make(map[address.Address]uint64)

	cst := cbor.NewCborStore(cs.stateBlockstore)
	st, err := state.LoadStateTree(cst, ts.Blocks()[0].ParentStateRoot)
	if err != nil {
		return nil, xerrors.Errorf("failed to load state tree at tipset %s: %w", ts, err)
	}

	selectMsg := func(m *types.Message) (bool, error) {
		var sender address.Address
		if ts.Height() >= build.UpgradeHyperdriveHeight {
			sender, err = st.LookupID(m.From)
			if err != nil {
				return false, err
			}
		} else {
			sender = m.From
		}

		// The first match for a sender is guaranteed to have correct nonce -- the block isn't valid otherwise
		if _, ok := applied[sender]; !ok {
			applied[sender] = m.Nonce
		}

		if applied[sender] != m.Nonce {
			return false, nil
		}

		applied[sender]++

		return true, nil
	}

	var out []BlockMessages
	for _, b := range ts.Blocks() {

		bms, sms, err := cs.MessagesForBlock(ctx, b)
		if err != nil {
			return nil, xerrors.Errorf("failed to get messages for block: %w", err)
		}

		bm := BlockMessages{
			Miner:         b.Miner,
			BlsMessages:   make([]types.ChainMsg, 0, len(bms)),
			SecpkMessages: make([]types.ChainMsg, 0, len(sms)),
		}

		for _, bmsg := range bms {
			b, err := selectMsg(bmsg.VMMessage())
			if err != nil {
				return nil, xerrors.Errorf("failed to decide whether to select message for block: %w", err)
			}

			if b {
				bm.BlsMessages = append(bm.BlsMessages, bmsg)
			}
		}

		for _, smsg := range sms {
			b, err := selectMsg(smsg.VMMessage())
			if err != nil {
				return nil, xerrors.Errorf("failed to decide whether to select message for block: %w", err)
			}

			if b {
				bm.SecpkMessages = append(bm.SecpkMessages, smsg)
			}
		}

		out = append(out, bm)
	}

	return out, nil
}

func (cs *ChainStore) MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error) {
	bmsgs, err := cs.BlockMsgsForTipset(ctx, ts)
	if err != nil {
		return nil, err
	}

	var out []types.ChainMsg
	for _, bm := range bmsgs {
		for _, blsm := range bm.BlsMessages {
			out = append(out, blsm)
		}

		for _, secm := range bm.SecpkMessages {
			out = append(out, secm)
		}
	}

	return out, nil
}

type mmCids struct {
	bls   []cid.Cid
	secpk []cid.Cid
}

func (cs *ChainStore) ReadMsgMetaCids(ctx context.Context, mmc cid.Cid) ([]cid.Cid, []cid.Cid, error) {
	o, ok := cs.mmCache.Get(mmc)
	if ok {
		mmcids := o.(*mmCids)
		return mmcids.bls, mmcids.secpk, nil
	}

	cst := cbor.NewCborStore(cs.chainLocalBlockstore)
	var msgmeta types.MsgMeta
	if err := cst.Get(ctx, mmc, &msgmeta); err != nil {
		return nil, nil, xerrors.Errorf("failed to load msgmeta (%s): %w", mmc, err)
	}

	blscids, err := cs.readAMTCids(msgmeta.BlsMessages)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading bls message cids for block: %w", err)
	}

	secpkcids, err := cs.readAMTCids(msgmeta.SecpkMessages)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading secpk message cids for block: %w", err)
	}

	cs.mmCache.Add(mmc, &mmCids{
		bls:   blscids,
		secpk: secpkcids,
	})

	return blscids, secpkcids, nil
}

func (cs *ChainStore) MessagesForBlock(ctx context.Context, b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	blscids, secpkcids, err := cs.ReadMsgMetaCids(ctx, b.Messages)
	if err != nil {
		return nil, nil, err
	}

	blsmsgs, err := cs.LoadMessagesFromCids(ctx, blscids)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading bls messages for block: %w", err)
	}

	secpkmsgs, err := cs.LoadSignedMessagesFromCids(ctx, secpkcids)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading secpk messages for block: %w", err)
	}

	return blsmsgs, secpkmsgs, nil
}

func (cs *ChainStore) GetParentReceipt(ctx context.Context, b *types.BlockHeader, i int) (*types.MessageReceipt, error) {
	// block headers use adt0, for now.
	a, err := blockadt.AsArray(cs.ActorStore(ctx), b.ParentMessageReceipts)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
	}

	var r types.MessageReceipt
	if found, err := a.Get(uint64(i), &r); err != nil {
		return nil, err
	} else if !found {
		return nil, xerrors.Errorf("failed to find receipt %d", i)
	}

	return &r, nil
}

func (cs *ChainStore) LoadMessagesFromCids(ctx context.Context, cids []cid.Cid) ([]*types.Message, error) {
	msgs := make([]*types.Message, 0, len(cids))
	for i, c := range cids {
		m, err := cs.GetMessage(ctx, c)
		if err != nil {
			return nil, xerrors.Errorf("failed to get message: (%s):%d: %w", c, i, err)
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (cs *ChainStore) LoadSignedMessagesFromCids(ctx context.Context, cids []cid.Cid) ([]*types.SignedMessage, error) {
	msgs := make([]*types.SignedMessage, 0, len(cids))
	for i, c := range cids {
		m, err := cs.GetSignedMessage(ctx, c)
		if err != nil {
			return nil, xerrors.Errorf("failed to get message: (%s):%d: %w", c, i, err)
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

// TipSetBlockMessagesReceipts returns the blocks and messages in `pts` and their corresponding receipts from `ts` matching block order in tipset (`pts`).
func (cs *ChainStore) TipSetBlockMessagesReceipts(ctx context.Context, ts, pts *types.TipSet) ([]*BlockMessageReceipts, error) {
	// sanity check args
	if ts.Key().IsEmpty() {
		return nil, fmt.Errorf("tipset cannot be empty")
	}
	if pts.Key().IsEmpty() {
		return nil, fmt.Errorf("parent tipset cannot be empty")
	}
	if !types.CidArrsEqual(ts.Parents().Cids(), pts.Cids()) {
		return nil, fmt.Errorf("mismatching tipset (%s) and parent tipset (%s)", ts.Key().String(), pts.Key().String())
	}

	// returned BlockMessages match block order in tipset
	blkMsgs, err := cs.BlockMsgsForTipset(ctx, pts)
	if err != nil {
		return nil, err
	}
	if len(blkMsgs) != len(pts.Blocks()) {
		// logic error somewhere
		return nil, fmt.Errorf("mismatching number of blocks returned from block messages, got %d wanted %d", len(blkMsgs), len(pts.Blocks()))
	}

	// retrieve receipts using a block from the child (ts) tipset
	// TODO this operation can fail when the node is imported from a snapshot that does not contain receipts (most don't)
	// the solution is to compute the tipset state which will create the receipts we load here
	rs, err := blockadt.AsArray(cs.ActorStore(ctx), ts.ParentMessageReceipts())
	if err != nil {
		return nil, fmt.Errorf("loading message receipts %w", err)
	}
	// so we only load the receipt array once
	getReceipt := func(idx int) (*types.MessageReceipt, error) {
		var r types.MessageReceipt
		if found, err := rs.Get(uint64(idx), &r); err != nil {
			return nil, err
		} else if !found {
			return nil, fmt.Errorf("failed to find receipt %d", idx)
		}
		return &r, nil
	}

	out := make([]*BlockMessageReceipts, len(pts.Blocks()))
	executionIndex := 0
	// walk each block in tipset, `pts.Blocks()` has same ordering as `blkMsgs`.
	for blkIdx := range pts.Blocks() {
		// bls and secp messages for block
		msgs := blkMsgs[blkIdx]
		// index of messages in `out.Messages`
		msgIdx := 0
		// index or receipts in `out.Receipts`
		receiptIdx := 0
		out[blkIdx] = &BlockMessageReceipts{
			// block containing messages
			Block: pts.Blocks()[blkIdx],
			// total messages returned equal to sum of bls and secp messages
			Messages: make([]types.ChainMsg, len(msgs.BlsMessages)+len(msgs.SecpkMessages)),
			// total receipts returned equal to sum of bls and secp messages
			Receipts: make([]*types.MessageReceipt, len(msgs.BlsMessages)+len(msgs.SecpkMessages)),
			// index of message indicating execution order.
			MessageExecutionIndex: make(map[types.ChainMsg]int),
		}
		// walk bls messages and extract their receipts
		for blsIdx := range msgs.BlsMessages {
			// location in receipt array corresponds to message execution order across all blocks
			receipt, err := getReceipt(executionIndex)
			if err != nil {
				return nil, err
			}
			out[blkIdx].Messages[msgIdx] = msgs.BlsMessages[blsIdx]
			out[blkIdx].Receipts[receiptIdx] = receipt
			out[blkIdx].MessageExecutionIndex[msgs.BlsMessages[blsIdx]] = executionIndex
			msgIdx++
			receiptIdx++
			executionIndex++
		}
		// walk secp messages and extract their receipts
		for secpIdx := range msgs.SecpkMessages {
			// location in receipt array corresponds to message execution order across all blocks
			receipt, err := getReceipt(executionIndex)
			if err != nil {
				return nil, err
			}
			out[blkIdx].Messages[msgIdx] = msgs.SecpkMessages[secpIdx]
			out[blkIdx].Receipts[receiptIdx] = receipt
			out[blkIdx].MessageExecutionIndex[msgs.SecpkMessages[secpIdx]] = executionIndex
			msgIdx++
			receiptIdx++
			executionIndex++
		}
	}
	return out, nil
}

// BlockMessageReceipts contains a block its messages and their corresponding receipts.
// The Receipts are one-to-one with Messages index.
type BlockMessageReceipts struct {
	Block *types.BlockHeader
	// Messages contained in Block.
	Messages []types.ChainMsg
	// Receipts contained in Block.
	Receipts []*types.MessageReceipt
	// MessageExectionIndex contains a mapping of Messages to their execution order in the tipset they were included.
	MessageExecutionIndex map[types.ChainMsg]int
}

type MessageReceiptIterator struct {
	// index in msgs and receipts.
	idx      int
	msgs     []types.ChainMsg
	receipts []*types.MessageReceipt
	// maps msgs to their execution order in tipset application.
	exeIdx map[types.ChainMsg]int
}

// Iterator returns a MessageReceiptIterator to conveniently iterate messages, their execution index, and their respective receipts.
func (bmr *BlockMessageReceipts) Iterator() (*MessageReceiptIterator, error) {
	if len(bmr.Messages) != len(bmr.Receipts) {
		return nil, fmt.Errorf("invalid construction, expected equal number receipts (%d) and messages (%d)", len(bmr.Receipts), len(bmr.Messages))
	}
	return &MessageReceiptIterator{
		idx:      0,
		msgs:     bmr.Messages,
		receipts: bmr.Receipts,
		exeIdx:   bmr.MessageExecutionIndex,
	}, nil
}

// HasNext returns `true` while there are messages/receipts to iterate.
func (mri *MessageReceiptIterator) HasNext() bool {
	if mri.idx < len(mri.msgs) {
		return true
	}
	return false
}

// Next returns the next message, its execution index, and receipt in the MessageReceiptIterator.
func (mri *MessageReceiptIterator) Next() (types.ChainMsg, int, *types.MessageReceipt) {
	if mri.HasNext() {
		msg := mri.msgs[mri.idx]
		exeIdx := mri.exeIdx[msg]
		rec := mri.receipts[mri.idx]
		mri.idx++
		return msg, exeIdx, rec
	}
	return nil, -1, nil
}

// Reset resets the MessageReceiptIterator to the first message/receipt.
func (mri *MessageReceiptIterator) Reset() {
	mri.idx = 0
}
