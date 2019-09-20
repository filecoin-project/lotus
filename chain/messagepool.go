package chain

import (
	"encoding/base64"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/stmgr"
	"github.com/filecoin-project/go-lotus/chain/types"
)

type MessagePool struct {
	lk sync.Mutex

	pending map[address.Address]*msgSet

	sm *stmgr.StateManager

	ps *pubsub.PubSub
}

type msgSet struct {
	msgs      map[uint64]*types.SignedMessage
	nextNonce uint64
}

func newMsgSet() *msgSet {
	return &msgSet{
		msgs: make(map[uint64]*types.SignedMessage),
	}
}

func (ms *msgSet) add(m *types.SignedMessage) error {
	if len(ms.msgs) == 0 || m.Message.Nonce >= ms.nextNonce {
		ms.nextNonce = m.Message.Nonce + 1
	}
	if _, has := ms.msgs[m.Message.Nonce]; has {
		if m.Cid() != ms.msgs[m.Message.Nonce].Cid() {
			log.Error("Add with duplicate nonce")
			return xerrors.Errorf("message to %s with nonce %d already in mpool")
		}
		log.Warn("Add called with the same message multiple times")
	}
	ms.msgs[m.Message.Nonce] = m

	return nil
}

func NewMessagePool(sm *stmgr.StateManager, ps *pubsub.PubSub) *MessagePool {
	mp := &MessagePool{
		pending: make(map[address.Address]*msgSet),
		sm:      sm,
		ps:      ps,
	}
	sm.ChainStore().SubscribeHeadChanges(mp.HeadChange)

	return mp
}

func (mp *MessagePool) Push(m *types.SignedMessage) error {
	msgb, err := m.Serialize()
	if err != nil {
		return err
	}

	if err := mp.Add(m); err != nil {
		return err
	}

	return mp.ps.Publish("/fil/messages", msgb)
}

func (mp *MessagePool) Add(m *types.SignedMessage) error {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	return mp.addLocked(m)
}

func (mp *MessagePool) addLocked(m *types.SignedMessage) error {
	data, err := m.Message.Serialize()
	if err != nil {
		return err
	}

	log.Infof("mpooladd: %d %s", m.Message.Nonce, base64.StdEncoding.EncodeToString(data))

	if err := m.Signature.Verify(m.Message.From, data); err != nil {
		log.Warnf("mpooladd signature verification failed: %s", err)
		return err
	}

	if _, err := mp.sm.ChainStore().PutMessage(m); err != nil {
		log.Warnf("mpooladd cs.PutMessage failed: %s", err)
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

	return mp.getNonceLocked(addr)
}

func (mp *MessagePool) getNonceLocked(addr address.Address) (uint64, error) {
	mset, ok := mp.pending[addr]
	if ok {
		return mset.nextNonce, nil
	}

	act, err := mp.sm.GetActor(addr)
	if err != nil {
		return 0, err
	}

	return act.Nonce, nil
}

func (mp *MessagePool) PushWithNonce(addr address.Address, cb func(uint64) (*types.SignedMessage, error)) (*types.SignedMessage, error) {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	nonce, err := mp.getNonceLocked(addr)
	if err != nil {
		return nil, err
	}

	msg, err := cb(nonce)
	if err != nil {
		return nil, err
	}

	msgb, err := msg.Serialize()
	if err != nil {
		return nil, err
	}

	if err := mp.addLocked(msg); err != nil {
		return nil, err
	}

	return msg, mp.ps.Publish("/fil/messages", msgb)
}

func (mp *MessagePool) Remove(from address.Address, nonce uint64) {
	mp.lk.Lock()
	defer mp.lk.Unlock()

	mset, ok := mp.pending[from]
	if !ok {
		return
	}

	// NB: This deletes any message with the given nonce. This makes sense
	// as two messages with the same sender cannot have the same nonce
	delete(mset.msgs, nonce)

	if len(mset.msgs) == 0 {
		// FIXME: This is racy
		//delete(mp.pending, from)
	} else {
		var max uint64
		for nonce := range mset.msgs {
			if max < nonce {
				max = nonce
			}
		}
		mset.nextNonce = max + 1
	}
}

func (mp *MessagePool) Pending() []*types.SignedMessage {
	mp.lk.Lock()
	defer mp.lk.Unlock()
	out := make([]*types.SignedMessage, 0)
	for _, mset := range mp.pending {
		if len(mset.msgs) == 0 {
			continue
		}

		set := make([]*types.SignedMessage, len(mset.msgs))
		var i uint64

		for i = mset.nextNonce - 1; mset.msgs[i] != nil; i-- {
			set[len(mset.msgs)-int(mset.nextNonce-i)] = mset.msgs[i]
		}

		out = append(out, set[len(mset.msgs)-int(mset.nextNonce-i-1):]...)
	}

	return out
}

func (mp *MessagePool) HeadChange(revert []*types.TipSet, apply []*types.TipSet) error {
	for _, ts := range revert {
		for _, b := range ts.Blocks() {
			bmsgs, smsgs, err := mp.sm.ChainStore().MessagesForBlock(b)
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
			bmsgs, smsgs, err := mp.sm.ChainStore().MessagesForBlock(b)
			if err != nil {
				return errors.Wrapf(err, "failed to get messages for apply block %s(height %d) (msgroot = %s)", b.Cid(), b.Height, b.Messages)
			}
			for _, msg := range smsgs {
				mp.Remove(msg.Message.From, msg.Message.Nonce)
			}

			for _, msg := range bmsgs {
				mp.Remove(msg.From, msg.Nonce)
			}
		}
	}

	return nil
}

func (mp *MessagePool) RecoverSig(msg *types.Message) *types.SignedMessage {
	// TODO: persist signatures for BLS messages for a little while in case of reorgs
	return nil
}
