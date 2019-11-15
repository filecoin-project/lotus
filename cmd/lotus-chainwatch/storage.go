package main

import (
	"database/sql"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	_ "github.com/mattn/go-sqlite3"
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
create table messages
(
	cid text not null
		constraint messages_pk
			primary key,
	"from" text not null,
	"to" text not null,
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
	timestamp text not null
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

func (st *storage) storeHeaders(bhs []*types.BlockHeader) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into blocks (cid, parentWeight, height, "timestamp") values (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, bh := range bhs {
		if _, err := stmt.Exec(bh.Cid().String(), bh.ParentWeight.String(), bh.Height, bh.Timestamp); err != nil {
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

	stmt, err := tx.Prepare(`insert into messages (cid, "from", "to", nonce, "value", gasprice, gaslimit, method, params) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
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
