package lpmessage

import (
	"context"
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

/*
create table message_waits (

	signed_message_cid text primary key references message_sends (signed_cid),
	waiter_machine_id int references harmony_machines (id) on delete set null,

	executed_tsk_cid text,
	executed_tsk_epoch bigint,
	executed_msg_cid text,
	executed_msg_data jsonb,

	executed_rcpt_exitcode bigint,
	executed_rcpt_return bytea,
	executed_rcpt_gas_used bigint

)

create table message_sends
(

	from_key     text   not null,
	to_addr      text   not null,
	send_reason  text   not null,
	send_task_id bigint not null,

	unsigned_data bytea not null,
	unsigned_cid  text  not null,

	nonce        bigint,
	signed_data  bytea,
	signed_json  jsonb,
	signed_cid   text,

	send_time    timestamp default null,
	send_success boolean   default null,
	send_error   text,

	constraint message_sends_pk
	    primary key (send_task_id, from_key)

);
*/
type MessageWaiterApi interface {
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
}

type MessageWaiter struct {
	db  *harmonydb.DB
	ht  *harmonytask.TaskEngine
	api MessageWaiterApi

	stopping, stopped chan struct{}

	updateCh chan struct{}
	bestTs   atomic.Pointer[types.TipSetKey]
}

func NewMessageWaiter(db *harmonydb.DB, ht *harmonytask.TaskEngine, pcs *chainsched.ProviderChainSched) (*MessageWaiter, error) {
	mw := &MessageWaiter{
		db:       db,
		ht:       ht,
		api:      api,
		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
		updateCh: make(chan struct{}),
	}
	mw.run()
	if err := pcs.AddHandler(mw.processHeadChange); err != nil {
		return nil, err
	}
	return mw, nil
}

func (mw *MessageWaiter) run() {
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

func (mw *MessageWaiter) update() {
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
		n, err := mw.db.Exec(ctx, `UPDATE message_waits SET owner_machine_id = $1 WHERE owner_machine_id IS NULL AND executed_tsk_cid IS NULL`, machineID)
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
	err = mw.db.Select(ctx, &msgs, `SELECT signed_message_cid, from_key, nonce FROM message_wait
                          JOIN message_sends ON signed_message_cid = signed_cid
                          WHERE owner_machine_id = $1 LIMIT 10000`, machineID)
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
		look, err := mw.api.StateSearchMsg(ctx, lbtsk, cid.MustParse(msg.Cid), api.LookbackNoLimit, false)
		if err != nil {
			log.Errorf("failed to search for message: %+v", err)
			return
		}
	}
}

func (mw *MessageWaiter) Stop(ctx context.Context) error {
	close(mw.stopping)
	select {
	case <-mw.stopped:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (mw *MessageWaiter) processHeadChange(ctx context.Context, revert *types.TipSet, apply *types.TipSet) error {
	best := apply.Key()
	mw.bestTs.Store(&best)
	select {
	case mw.updateCh <- struct{}{}:
	default:
	}
	return nil
}
