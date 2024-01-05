package lpmessage

import (
	"context"
	"encoding/json"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/ipfs/go-cid"
	"sync/atomic"
)

const MinConfidence = 6

type MessageWaiterApi interface {
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
}

type MessageWatcher struct {
	db  *harmonydb.DB
	ht  *harmonytask.TaskEngine
	api MessageWaiterApi

	stopping, stopped chan struct{}

	updateCh chan struct{}
	bestTs   atomic.Pointer[types.TipSetKey]
}

func NewMessageWatcher(db *harmonydb.DB, ht *harmonytask.TaskEngine, pcs *chainsched.ProviderChainSched, api MessageWaiterApi) (*MessageWatcher, error) {
	mw := &MessageWatcher{
		db:       db,
		ht:       ht,
		api:      api,
		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
		updateCh: make(chan struct{}),
	}
	go mw.run()
	if err := pcs.AddHandler(mw.processHeadChange); err != nil {
		return nil, err
	}
	return mw, nil
}

func (mw *MessageWatcher) run() {
	defer close(mw.stopped)

	for {
		select {
		case <-mw.stopping:
			// todo cleanup assignments
			return
		case <-mw.updateCh:
			mw.update()
		}
	}
}

func (mw *MessageWatcher) update() {
	ctx := context.Background()

	tsk := *mw.bestTs.Load()

	ts, err := mw.api.ChainGetTipSet(ctx, tsk)
	if err != nil {
		log.Errorf("failed to get tipset: %+v", err)
		return
	}

	lbts, err := mw.api.ChainGetTipSetByHeight(ctx, ts.Height()-MinConfidence, tsk)
	if err != nil {
		log.Errorf("failed to get tipset: %+v", err)
		return
	}
	lbtsk := lbts.Key()

	machineID := mw.ht.ResourcesAvailable().MachineID

	// first if we see pending messages with null owner, assign them to ourselves
	{
		n, err := mw.db.Exec(ctx, `UPDATE message_waits SET waiter_machine_id = $1 WHERE waiter_machine_id IS NULL AND executed_tsk_cid IS NULL`, machineID)
		if err != nil {
			log.Errorf("failed to assign pending messages: %+v", err)
			return
		}
		if n > 0 {
			log.Debugw("assigned pending messages to ourselves", "assigned", n)
		}
	}

	// get messages assigned to us
	var msgs []struct {
		Cid   string `db:"signed_message_cid"`
		From  string `db:"from_key"`
		Nonce uint64 `db:"nonce"`

		FromAddr address.Address `db:"-"`
	}

	// really large limit in case of things getting stuck and backlogging severely
	err = mw.db.Select(ctx, &msgs, `SELECT signed_message_cid, from_key, nonce FROM message_waits
                          JOIN message_sends ON signed_message_cid = signed_cid
                          WHERE waiter_machine_id = $1 LIMIT 10000`, machineID)
	if err != nil {
		log.Errorf("failed to get assigned messages: %+v", err)
		return
	}

	// get address/nonce set to check
	toCheck := make(map[address.Address]uint64)

	for i := range msgs {
		msgs[i].FromAddr, err = address.NewFromString(msgs[i].From)
		if err != nil {
			log.Errorf("failed to parse from address: %+v", err)
			return
		}
		toCheck[msgs[i].FromAddr] = 0
	}

	// get the nonce for each address
	for addr := range toCheck {
		act, err := mw.api.StateGetActor(ctx, addr, lbtsk)
		if err != nil {
			log.Errorf("failed to get actor: %+v", err)
			return
		}

		toCheck[addr] = act.Nonce
	}

	// check if any of the messages we have assigned to us are now on chain, and have been for MinConfidence epochs
	for _, msg := range msgs {
		if msg.Nonce > toCheck[msg.FromAddr] {
			continue // definitely not on chain yet
		}

		look, err := mw.api.StateSearchMsg(ctx, lbtsk, cid.MustParse(msg.Cid), api.LookbackNoLimit, false)
		if err != nil {
			log.Errorf("failed to search for message: %+v", err)
			return
		}

		if look == nil {
			continue // not on chain yet (or not executed yet)
		}

		tskCid, err := look.TipSet.Cid()
		if err != nil {
			log.Errorf("failed to get tipset cid: %+v", err)
			return
		}

		emsg, err := mw.api.ChainGetMessage(ctx, look.Message)
		if err != nil {
			log.Errorf("failed to get message: %+v", err)
			return
		}

		execMsg, err := json.Marshal(emsg)
		if err != nil {
			log.Errorf("failed to marshal message: %+v", err)
			return
		}

		// record in db
		_, err = mw.db.Exec(ctx, `UPDATE message_waits SET
			waiter_machine_id = NULL,
			executed_tsk_cid = $1, executed_tsk_epoch = $2,
			executed_msg_cid = $3, executed_msg_data = $4,
			executed_rcpt_exitcode = $5, executed_rcpt_return = $6, executed_rcpt_gas_used = $7
                     WHERE signed_message_cid = $8`, tskCid, look.Height,
			look.Message, execMsg,
			look.Receipt.ExitCode, look.Receipt.Return, look.Receipt.GasUsed,
			msg.Cid)
		if err != nil {
			log.Errorf("failed to update message wait: %+v", err)
			return
		}
	}
}

func (mw *MessageWatcher) Stop(ctx context.Context) error {
	close(mw.stopping)
	select {
	case <-mw.stopped:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (mw *MessageWatcher) processHeadChange(ctx context.Context, revert *types.TipSet, apply *types.TipSet) error {
	best := apply.Key()
	mw.bestTs.Store(&best)
	select {
	case mw.updateCh <- struct{}{}:
	default:
	}
	return nil
}
