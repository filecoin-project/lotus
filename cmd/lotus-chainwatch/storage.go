package main

import (
	"database/sql"

	"github.com/ipfs/go-cid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
)

type storage struct {
	db *sql.DB
}

func openStorage() (*storage, error) {
	db, err := sql.Open("sqlite3", "./chainwatch.db")
	if err != nil {
		return nil, err
	}

	st := &storage{db: db}

	return st, st.setup()
}

func (st *storage) setup() error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
create table actors
  (
	id text not null,
	code text not null,
	head text not null,
	nonce int not null,
	balance text,
	constraint actors_pk
		unique (id, code, head, nonce, balance)
  );

create table id_address_map
(
	id text not null
		constraint id_address_map_actors_id_fk
			references actors (id),
	address text not null,
	constraint id_address_map_pk
		primary key (id, address)
);


create table messages
(
	cid text not null
		constraint messages_pk
			primary key,
	"from" text not null
		constraint messages_id_address_map_from_fk
			references id_address_map (address),
	"to" text not null
		constraint messages_id_address_map_to_fk
			references id_address_map (address),
	nonce int not null,
	value text not null,
	gasprice int not null,
	gaslimit int not null,
	method int,
	params blob
);

create unique index messages_cid_uindex
	on messages (cid);

create table blocks
(
	cid text not null
		constraint blocks_pk
			primary key,
	parentWeight numeric not null,
	height int not null,
	miner text not null
		constraint blocks_id_address_map_miner_fk
			references id_address_map (address),
	timestamp int not null
);

create unique index blocks_cid_uindex
	on blocks (cid);
	
create table block_messages
(
	block text not null
		constraint block_messages_blk_fk
			references blocks (cid),
	message text not null
		constraint block_messages_msg_fk
			references messages,
	constraint block_messages_pk
		unique (block, message)
);
`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (st *storage) hasBlock(bh *types.BlockHeader) bool {
	var exitsts bool
	err := st.db.QueryRow(`select exists (select 1 FROM blocks where cid=?)`, bh.Cid().String()).Scan(&exitsts)
	if err != nil {
		log.Error(err)
		return false
	}
	return exitsts
}

func (st *storage) storeActors(actors map[address.Address]map[types.Actor]struct{}) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into actors (id, code, head, nonce, balance) values (?, ?, ?, ?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for addr, acts := range actors {
		for act, _ := range acts {
			if _, err := stmt.Exec(addr.String(), act.Code.String(), act.Head.String(), act.Nonce, act.Balance.String()); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (st *storage) storeHeaders(bhs map[cid.Cid]*types.BlockHeader) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into blocks (cid, parentWeight, height, miner, "timestamp") values (?, ?, ?, ?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, bh := range bhs {
		if _, err := stmt.Exec(bh.Cid().String(), bh.ParentWeight.String(), bh.Height, bh.Miner.String(), bh.Timestamp); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (st *storage) storeMessages(msgs map[cid.Cid]*types.Message) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into messages (cid, "from", "to", nonce, "value", gasprice, gaslimit, method, params) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for c, m := range msgs {
		if _, err := stmt.Exec(
			c.String(),
			m.From.String(),
			m.To.String(),
			m.Nonce,
			m.Value.String(),
			m.GasPrice.String(),
			m.GasLimit.String(),
			m.Method,
			m.Params,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (st *storage) storeAddressMap(addrs map[address.Address]address.Address) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into id_address_map (id, address) VALUES (?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for a, i := range addrs {
		if i == address.Undef {
			continue
		}
		if _, err := stmt.Exec(
			i.String(),
			a.String(),
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (st *storage) storeMsgInclusions(incls map[cid.Cid][]cid.Cid) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into block_messages (block, message) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

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

	return tx.Commit()
}

func (st *storage) close() error {
	return st.db.Close()
}
