package store

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	block "github.com/ipfs/go-block-format"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"

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

func PutMessage(bs bstore.Blockstore, m storable) (cid.Cid, error) {
	b, err := m.ToStorageBlock()
	if err != nil {
		return cid.Undef, err
	}

	if err := bs.Put(b); err != nil {
		return cid.Undef, err
	}

	return b.Cid(), nil
}

func (cs *ChainStore) PutMessage(m storable) (cid.Cid, error) {
	return PutMessage(cs.chainBlockstore, m)
}

func (cs *ChainStore) GetCMessage(c cid.Cid) (types.ChainMsg, error) {
	m, err := cs.GetMessage(c)
	if err == nil {
		return m, nil
	}
	if err != bstore.ErrNotFound {
		log.Warnf("GetCMessage: unexpected error getting unsigned message: %s", err)
	}

	return cs.GetSignedMessage(c)
}

func (cs *ChainStore) GetMessage(c cid.Cid) (*types.Message, error) {
	var msg *types.Message
	err := cs.chainLocalBlockstore.View(c, func(b []byte) (err error) {
		msg, err = types.DecodeMessage(b)
		return err
	})
	return msg, err
}

func (cs *ChainStore) GetSignedMessage(c cid.Cid) (*types.SignedMessage, error) {
	var msg *types.SignedMessage
	err := cs.chainLocalBlockstore.View(c, func(b []byte) (err error) {
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

func (cs *ChainStore) BlockMsgsForTipset(ts *types.TipSet) ([]BlockMessages, error) {
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

		bms, sms, err := cs.MessagesForBlock(b)
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

func (cs *ChainStore) MessagesForTipset(ts *types.TipSet) ([]types.ChainMsg, error) {
	bmsgs, err := cs.BlockMsgsForTipset(ts)
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

func (cs *ChainStore) ReadMsgMetaCids(mmc cid.Cid) ([]cid.Cid, []cid.Cid, error) {
	o, ok := cs.mmCache.Get(mmc)
	if ok {
		mmcids := o.(*mmCids)
		return mmcids.bls, mmcids.secpk, nil
	}

	cst := cbor.NewCborStore(cs.chainLocalBlockstore)
	var msgmeta types.MsgMeta
	if err := cst.Get(context.TODO(), mmc, &msgmeta); err != nil {
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

func (cs *ChainStore) MessagesForBlock(b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	blscids, secpkcids, err := cs.ReadMsgMetaCids(b.Messages)
	if err != nil {
		return nil, nil, err
	}

	blsmsgs, err := cs.LoadMessagesFromCids(blscids)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading bls messages for block: %w", err)
	}

	secpkmsgs, err := cs.LoadSignedMessagesFromCids(secpkcids)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading secpk messages for block: %w", err)
	}

	return blsmsgs, secpkmsgs, nil
}

func (cs *ChainStore) GetParentReceipt(b *types.BlockHeader, i int) (*types.MessageReceipt, error) {
	ctx := context.TODO()
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

func (cs *ChainStore) LoadMessagesFromCids(cids []cid.Cid) ([]*types.Message, error) {
	msgs := make([]*types.Message, 0, len(cids))
	for i, c := range cids {
		m, err := cs.GetMessage(c)
		if err != nil {
			return nil, xerrors.Errorf("failed to get message: (%s):%d: %w", c, i, err)
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (cs *ChainStore) LoadSignedMessagesFromCids(cids []cid.Cid) ([]*types.SignedMessage, error) {
	msgs := make([]*types.SignedMessage, 0, len(cids))
	for i, c := range cids {
		m, err := cs.GetSignedMessage(c)
		if err != nil {
			return nil, xerrors.Errorf("failed to get message: (%s):%d: %w", c, i, err)
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}
