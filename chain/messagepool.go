package chain

import (
	"encoding/base64"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/stmgr"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/pkg/errors"
)

type MessagePool struct {
	lk sync.Mutex

	pending map[address.Address]*msgSet

	sm *stmgr.StateManager

	ps *pubsub.PubSub
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

	log.Infof("mpooladd: %s", base64.StdEncoding.EncodeToString(data))

	if err := m.Signature.Verify(m.Message.From, data); err != nil {
		return err
	}

	if _, err := mp.sm.ChainStore().PutMessage(m); err != nil {
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
		return mset.startNonce + uint64(len(mset.msgs)), nil
	}

	act, err := mp.sm.GetActor(addr, nil)
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
		delete(mp.pending, from)
	}
}

func (mp *MessagePool) Pending() []*types.SignedMessage {
	mp.lk.Lock()
	defer mp.lk.Unlock()
	out := make([]*types.SignedMessage, 0)
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
