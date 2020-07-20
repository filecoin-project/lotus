package processor

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/parmap"
)

func (p *Processor) setupMessages() error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
create table if not exists messages
(
	cid text not null
		constraint messages_pk
			primary key,
	"from" text not null,
	"to" text not null,
	nonce bigint not null,
	value text not null,
	gasprice bigint not null,
	gaslimit bigint not null,
	method bigint,
	params bytea
);

create unique index if not exists messages_cid_uindex
	on messages (cid);

create index if not exists messages_from_index
	on messages ("from");

create index if not exists messages_to_index
	on messages ("to");

create table if not exists block_messages
(
	block text not null
	    constraint blocks_block_cids_cid_fk
			references block_cids (cid),
	message text not null,
	constraint block_messages_pk
		primary key (block, message)
);

create table if not exists mpool_messages
(
	msg text not null
		constraint mpool_messages_pk
			primary key
		constraint mpool_messages_messages_cid_fk
			references messages,
	add_ts int not null
);

create unique index if not exists mpool_messages_msg_uindex
	on mpool_messages (msg);

create table if not exists receipts
(
	msg text not null,
	state text not null,
	idx int not null,
	exit int not null,
	gas_used int not null,
	return bytea,
	constraint receipts_pk
		primary key (msg, state)
);

create index if not exists receipts_msg_state_index
	on receipts (msg, state);
`); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *Processor) HandleMessageChanges(ctx context.Context, blocks map[cid.Cid]*types.BlockHeader) error {
	if err := p.persistMessagesAndReceipts(ctx, blocks); err != nil {
		return err
	}
	return nil
}

func (p *Processor) persistMessagesAndReceipts(ctx context.Context, blocks map[cid.Cid]*types.BlockHeader) error {
	messages, inclusions := p.fetchMessages(ctx, blocks)
	receipts := p.fetchParentReceipts(ctx, blocks)

	grp, _ := errgroup.WithContext(ctx)

	grp.Go(func() error {
		return p.storeMessages(messages)
	})

	grp.Go(func() error {
		return p.storeMsgInclusions(inclusions)
	})

	grp.Go(func() error {
		return p.storeReceipts(receipts)
	})

	return grp.Wait()
}

func (p *Processor) storeReceipts(recs map[mrec]*types.MessageReceipt) error {
	start := time.Now()
	defer func() {
		log.Debugw("Persisted Receipts", "duration", time.Since(start).String())
	}()
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
create temp table recs (like receipts excluding constraints) on commit drop;
`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy recs (msg, state, idx, exit, gas_used, return) from stdin `)
	if err != nil {
		return err
	}

	for c, m := range recs {
		if _, err := stmt.Exec(
			c.msg.String(),
			c.state.String(),
			c.idx,
			m.ExitCode,
			m.GasUsed,
			m.Return,
		); err != nil {
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into receipts select * from recs on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (p *Processor) storeMsgInclusions(incls map[cid.Cid][]cid.Cid) error {
	start := time.Now()
	defer func() {
		log.Debugw("Persisted Message Inclusions", "duration", time.Since(start).String())
	}()
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
create temp table mi (like block_messages excluding constraints) on commit drop;
`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy mi (block, message) from STDIN `)
	if err != nil {
		return err
	}

	for b, msgs := range incls {
		for _, msg := range msgs {
			if _, err := stmt.Exec(
				b.String(),
				msg.String(),
			); err != nil {
				return err
			}
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into block_messages select * from mi on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (p *Processor) storeMessages(msgs map[cid.Cid]*types.Message) error {
	start := time.Now()
	defer func() {
		log.Debugw("Persisted Messages", "duration", time.Since(start).String())
	}()
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
create temp table msgs (like messages excluding constraints) on commit drop;
`); err != nil {
		return xerrors.Errorf("prep temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy msgs (cid, "from", "to", nonce, "value", gasprice, gaslimit, method, params) from stdin `)
	if err != nil {
		return err
	}

	for c, m := range msgs {
		if _, err := stmt.Exec(
			c.String(),
			m.From.String(),
			m.To.String(),
			m.Nonce,
			m.Value.String(),
			m.GasPrice.String(),
			m.GasLimit,
			m.Method,
			m.Params,
		); err != nil {
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec(`insert into messages select * from msgs on conflict do nothing `); err != nil {
		return xerrors.Errorf("actor put: %w", err)
	}

	return tx.Commit()
}

func (p *Processor) fetchMessages(ctx context.Context, blocks map[cid.Cid]*types.BlockHeader) (map[cid.Cid]*types.Message, map[cid.Cid][]cid.Cid) {
	var lk sync.Mutex
	messages := map[cid.Cid]*types.Message{}
	inclusions := map[cid.Cid][]cid.Cid{} // block -> msgs

	parmap.Par(50, parmap.MapArr(blocks), func(header *types.BlockHeader) {
		msgs, err := p.node.ChainGetBlockMessages(ctx, header.Cid())
		if err != nil {
			panic(err)
		}

		vmm := make([]*types.Message, 0, len(msgs.Cids))
		for _, m := range msgs.BlsMessages {
			vmm = append(vmm, m)
		}

		for _, m := range msgs.SecpkMessages {
			vmm = append(vmm, &m.Message)
		}

		lk.Lock()
		for _, message := range vmm {
			messages[message.Cid()] = message
			inclusions[header.Cid()] = append(inclusions[header.Cid()], message.Cid())
		}
		lk.Unlock()
	})

	return messages, inclusions
}

type mrec struct {
	msg   cid.Cid
	state cid.Cid
	idx   int
}

func (p *Processor) fetchParentReceipts(ctx context.Context, toSync map[cid.Cid]*types.BlockHeader) map[mrec]*types.MessageReceipt {
	var lk sync.Mutex
	out := map[mrec]*types.MessageReceipt{}

	parmap.Par(50, parmap.MapArr(toSync), func(header *types.BlockHeader) {
		recs, err := p.node.ChainGetParentReceipts(ctx, header.Cid())
		if err != nil {
			panic(err)
		}
		msgs, err := p.node.ChainGetParentMessages(ctx, header.Cid())
		if err != nil {
			panic(err)
		}

		lk.Lock()
		for i, r := range recs {
			out[mrec{
				msg:   msgs[i].Cid,
				state: header.ParentStateRoot,
				idx:   i,
			}] = r
		}
		lk.Unlock()
	})

	return out
}
