package chain

import (
	"encoding/base64"
	"sync"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/pkg/errors"
)

type MessagePool struct {
	lk sync.Mutex

	pending map[address.Address]*msgSet

	cs *store.ChainStore
}

type msgSet struct {
	msgs       map[uint64]*types.SignedMessage
	startNonce uint64
}

func newMsgSet() *msgSet {
	return &msgSet{
		msgs: make(map[uint64]*types.SignedMessage),
	}
}

func (ms *msgSet) add(m *types.SignedMessage) {
	if len(ms.msgs) == 0 || m.Message.Nonce < ms.startNonce {
		ms.startNonce = m.Message.Nonce
	}
	ms.msgs[m.Message.Nonce] = m
}

func NewMessagePool(cs *store.ChainStore) *MessagePool {
	mp := &MessagePool{
		pending: make(map[address.Address]*msgSet),
		cs:      cs,
	}
	cs.SubscribeHeadChanges(mp.HeadChange)

	return mp
}

func (mp *MessagePool) Add(m *types.SignedMessage) error {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	data, err := m.Message.Serialize()
	if err != nil {
		return err
	}

	log.Info("mpooladd: %s", base64.StdEncoding.EncodeToString(data))

	if err := m.Signature.Verify(m.Message.From, data); err != nil {
		return err
	}

	if _, err := mp.cs.PutMessage(m); err != nil {
		return err
	}

	mset, ok := mp.pending[m.Message.From]
	if !ok {
		mset = newMsgSet()
		mp.pending[m.Message.From] = mset
	}

	mset.add(m)
	return nil
}

func (mp *MessagePool) GetNonce(addr address.Address) (uint64, error) {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	mset, ok := mp.pending[addr]
	if ok {
		return mset.startNonce + uint64(len(mset.msgs)), nil
	}

	act, err := mp.cs.GetActor(addr)
	if err != nil {
		return 0, err
	}

	return act.Nonce, nil
}

func (mp *MessagePool) Remove(m *types.SignedMessage) {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	mset, ok := mp.pending[m.Message.From]
	if !ok {
		return
	}

	// NB: This deletes any message with the given nonce. This makes sense
	// as two messages with the same sender cannot have the same nonce
	delete(mset.msgs, m.Message.Nonce)

	if len(mset.msgs) == 0 {
		delete(mp.pending, m.Message.From)
	}
}

func (mp *MessagePool) Pending() []*types.SignedMessage {
	mp.lk.Lock()
	defer mp.lk.Unlock()
	var out []*types.SignedMessage
	for _, mset := range mp.pending {
		for i := mset.startNonce; true; i++ {
			m, ok := mset.msgs[i]
			if !ok {
				break
			}
			out = append(out, m)
		}
	}

	return out
}

func (mp *MessagePool) HeadChange(revert []*types.TipSet, apply []*types.TipSet) error {
	for _, ts := range revert {
		for _, b := range ts.Blocks() {
			bmsgs, smsgs, err := mp.cs.MessagesForBlock(b)
			if err != nil {
				return errors.Wrapf(err, "failed to get messages for revert block %s(height %d)", b.Cid(), b.Height)
			}
			for _, msg := range smsgs {
				if err := mp.Add(msg); err != nil {
					return err
				}
			}

			for _, msg := range bmsgs {
				smsg := mp.RecoverSig(msg)
				if smsg != nil {
					if err := mp.Add(smsg); err != nil {
						return err
					}
				} else {
					log.Warnf("could not recover signature for bls message %s during a reorg revert", msg.Cid())
				}
			}
		}
	}

	for _, ts := range apply {
		for _, b := range ts.Blocks() {
			bmsgs, smsgs, err := mp.cs.MessagesForBlock(b)
			if err != nil {
				return errors.Wrapf(err, "failed to get messages for apply block %s(height %d) (msgroot = %s)", b.Cid(), b.Height, b.Messages)
			}
			for _, msg := range smsgs {
				mp.Remove(msg)
			}

			for _, msg := range bmsgs {
				smsg := mp.RecoverSig(msg)
				if smsg != nil {
					mp.Remove(smsg)
				} else {
					// TODO: this one is likely fine
					log.Warnf("could not recover signature for bls message %s during a reorg apply", msg.Cid())
				}
			}
		}
	}

	return nil
}

func (mp *MessagePool) RecoverSig(msg *types.Message) *types.SignedMessage {
	// TODO: persist signatures for BLS messages for a little while in case of reorgs
	return nil
}
