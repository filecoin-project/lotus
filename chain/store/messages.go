package store

import (
	"context"
	"errors"
	"unsafe"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amtv2 "github.com/filecoin-project/go-amt-ipld/v2"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
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
	cids, err := readBlockAMT[cbg.CborCid](ctx, cs.ActorStore(ctx), root, uint64(buildconstants.BlockMessageLimit), nil)
	if err != nil {
		return nil, xerrors.Errorf("reading message amt: %w", err)
	}
	out := make([]cid.Cid, len(cids))
	for i, c := range cids {
		out[i] = cid.Cid(c)
	}
	return out, nil
}

// readBlockAMT reads the entries of a block message or receipt AMT. These keep
// the go-amt-ipld/v2 encoding ([height, count, node], bitwidth 3) and are
// dense, so entries are read by index from 0 to Count-1. A non-zero maxCount
// rejects larger arrays before any entries are read; a non-nil each is called
// on every entry before it is kept.
func readBlockAMT[T any, P interface {
	*T
	cbg.CBORUnmarshaler
}](ctx context.Context, cst cbor.IpldStore, root cid.Cid, maxCount uint64, each func(P) error) ([]T, error) {
	arr, err := amtv2.LoadAMT(ctx, cst, root)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
	}
	if maxCount > 0 && arr.Count > maxCount {
		return nil, xerrors.Errorf("amt has %d entries, limit is %d", arr.Count, maxCount)
	}

	entries := []T{}
	for i := uint64(0); i < arr.Count; i++ {
		var v T
		if err := arr.Get(ctx, i, P(&v)); err != nil {
			var nf *amtv2.ErrNotFound
			if errors.As(err, &nf) {
				return nil, xerrors.Errorf("amt entry %d of %d not found", i, arr.Count)
			}
			return nil, xerrors.Errorf("amt entry %d: %w", i, err)
		}
		if each != nil {
			if err := each(&v); err != nil {
				return nil, err
			}
		}
		entries = append(entries, v)
	}
	return entries, nil
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

	useIds := false
	selectMsg := func(m *types.Message) (bool, error) {
		var sender address.Address
		if ts.Height() >= buildconstants.UpgradeHyperdriveHeight {
			if useIds {
				sender, err = st.LookupIDAddress(m.From)
				if err != nil {
					return false, xerrors.Errorf("failed to resolve sender: %w", err)
				}
			} else {
				if m.From.Protocol() != address.ID {
					// we haven't been told to use IDs, just use the robust addr
					sender = m.From
				} else {
					// uh-oh, we actually have an ID-sender!
					useIds = true
					for robust, nonce := range applied {
						resolved, err := st.LookupIDAddress(robust)
						if err != nil {
							return false, xerrors.Errorf("failed to resolve sender: %w", err)
						}
						applied[resolved] = nonce
					}

					sender, err = st.LookupIDAddress(m.From)
					if err != nil {
						return false, xerrors.Errorf("failed to resolve sender: %w", err)
					}
				}
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
		out = append(out, bm.BlsMessages...)
		out = append(out, bm.SecpkMessages...)
	}

	return out, nil
}

type mmCids struct {
	bls   []cid.Cid
	secpk []cid.Cid
}

func (cs *ChainStore) ReadMsgMetaCids(ctx context.Context, mmc cid.Cid) ([]cid.Cid, []cid.Cid, error) {
	if mmcids, ok := cs.mmCache.Get(mmc); ok {
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

	if n := len(blscids) + len(secpkcids); n > buildconstants.BlockMessageLimit {
		return nil, nil, xerrors.Errorf("block has %d messages, limit is %d", n, buildconstants.BlockMessageLimit)
	}

	cs.mmCache.Add(mmc, mmCids{
		bls:   blscids,
		secpk: secpkcids,
	})

	return blscids, secpkcids, nil
}

// ReadReceipts loads the receipts under a receipts AMT root. Receipt AMTs are
// dense, so entries are read by index from 0 to Count-1. A non-zero maxBytes
// bounds the decoded receipts' estimated size: struct size plus return data.
func (cs *ChainStore) ReadReceipts(ctx context.Context, root cid.Cid, maxBytes uint64) ([]types.MessageReceipt, error) {
	var size uint64
	return readBlockAMT(ctx, cs.ActorStore(ctx), root, 0, func(r *types.MessageReceipt) error {
		size += uint64(unsafe.Sizeof(*r)) + uint64(len(r.Return))
		if maxBytes > 0 && size > maxBytes {
			return xerrors.Errorf("receipts exceed %d bytes", maxBytes)
		}
		return nil
	})
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

func (cs *ChainStore) SecpkMessagesForBlock(ctx context.Context, b *types.BlockHeader) ([]*types.SignedMessage, error) {
	_, secpkcids, err := cs.ReadMsgMetaCids(ctx, b.Messages)
	if err != nil {
		return nil, err
	}

	secpkmsgs, err := cs.LoadSignedMessagesFromCids(ctx, secpkcids)
	if err != nil {
		return nil, xerrors.Errorf("loading secpk messages for block: %w", err)
	}

	return secpkmsgs, nil
}

func (cs *ChainStore) GetParentReceipt(ctx context.Context, b *types.BlockHeader, i int) (*types.MessageReceipt, error) {
	a, err := amtv2.LoadAMT(ctx, cs.ActorStore(ctx), b.ParentMessageReceipts)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
	}

	var r types.MessageReceipt
	if err := a.Get(ctx, uint64(i), &r); err != nil {
		var nf *amtv2.ErrNotFound
		if errors.As(err, &nf) {
			return nil, xerrors.Errorf("failed to find receipt %d", i)
		}
		return nil, err
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
