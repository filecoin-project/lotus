package main

import (
	"container/list"
	"context"
	"database/sql"
	"fmt"
	"hash/crc32"
	"strconv"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

var chainwatchCmd = &cli.Command{
	Name:  "chainwatch",
	Usage: "lotus chainwatch",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "db",
			EnvVars: []string{"CHAINWATCH_DB"},
			Value:   "./chainwatch.db",
		},
	},
	Subcommands: []*cli.Command{
		chainwatchRunCmd,
		chainwatchDotCmd,
	},
}

var chainwatchDotCmd = &cli.Command{
	Name:      "dot",
	Usage:     "generate dot graphs",
	ArgsUsage: "<minHeight> <toseeHeight>",
	Action: func(cctx *cli.Context) error {
		st, err := cwOpenStorage(cctx.String("db"))
		if err != nil {
			return err
		}

		minH, err := strconv.ParseInt(cctx.Args().Get(0), 10, 32)
		if err != nil {
			return err
		}
		tosee, err := strconv.ParseInt(cctx.Args().Get(1), 10, 32)
		if err != nil {
			return err
		}
		maxH := minH + tosee

		res, err := st.db.Query(`select block, parent, b.miner, b.height, p.height from block_parents
    inner join blocks b on block_parents.block = b.cid
    inner join blocks p on block_parents.parent = p.cid
where b.height > ? and b.height < ?`, minH, maxH)

		if err != nil {
			return err
		}

		fmt.Println("digraph D {")

		for res.Next() {
			var block, parent, miner string
			var height, ph uint64
			if err := res.Scan(&block, &parent, &miner, &height, &ph); err != nil {
				return err
			}

			bc, err := cid.Parse(block)
			if err != nil {
				return err
			}

			has := st.hasBlock(bc)

			col := crc32.Checksum([]byte(miner), crc32.MakeTable(crc32.Castagnoli))&0xc0c0c0c0 + 0x30303030

			hasstr := ""
			if !has {
				//col = 0xffffffff
				hasstr = " UNSYNCED"
			}

			nulls := height - ph - 1
			for i := uint64(0); i < nulls; i++ {
				name := block + "NP" + fmt.Sprint(i)

				fmt.Printf("%s [label = \"NULL:%d\", fillcolor = \"#ffddff\", style=filled, forcelabels=true]\n%s -> %s\n",
					name, height-nulls+i, name, parent)

				parent = name
			}

			fmt.Printf("%s [label = \"%s:%d%s\", fillcolor = \"#%06x\", style=filled, forcelabels=true]\n%s -> %s\n", block, miner, height, hasstr, col, block, parent)
		}
		if res.Err() != nil {
			return res.Err()
		}

		fmt.Println("}")

		return nil
	},
}

var chainwatchRunCmd = &cli.Command{
	Name:  "run",
	Usage: "Start lotus chainwatch",

	Action: func(cctx *cli.Context) error {
		api, closer, err := cliutil.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := cliutil.ReqContext(cctx)

		v, err := api.Version(ctx)
		if err != nil {
			return err
		}

		log.Infof("Remote version: %s", v.Version)

		st, err := cwOpenStorage(cctx.String("db")) // todo flag
		if err != nil {
			return err
		}
		defer st.close() // nolint:errcheck

		cwRunSyncer(ctx, api, st)
		go cwSubBlocks(ctx, api, st)

		<-ctx.Done()
		return nil
	},
}

func cwSubBlocks(ctx context.Context, api api.FullNode, st *cwStorage) {
	sub, err := api.SyncIncomingBlocks(ctx)
	if err != nil {
		log.Error(err)
		return
	}

	for bh := range sub {
		err := st.storeHeaders(map[cid.Cid]*types.BlockHeader{
			bh.Cid(): bh,
		}, false)
		if err != nil {
			log.Error(err)
		}
	}
}

func cwRunSyncer(ctx context.Context, api api.FullNode, st *cwStorage) {
	notifs, err := api.ChainNotify(ctx)
	if err != nil {
		panic(err)
	}
	go func() {
		for notif := range notifs {
			for _, change := range notif {
				switch change.Type {
				case store.HCCurrent:
					fallthrough
				case store.HCApply:
					syncHead(ctx, api, st, change.Val)
				case store.HCRevert:
					log.Warnf("revert todo")
				}
			}
		}
	}()
}

func syncHead(ctx context.Context, api api.FullNode, st *cwStorage, ts *types.TipSet) {
	log.Infof("Getting headers / actors")

	toSync := map[cid.Cid]*types.BlockHeader{}
	toVisit := list.New()

	for _, header := range ts.Blocks() {
		toVisit.PushBack(header)
	}

	for toVisit.Len() > 0 {
		bh := toVisit.Remove(toVisit.Back()).(*types.BlockHeader)

		if _, seen := toSync[bh.Cid()]; seen || st.hasBlock(bh.Cid()) {
			continue
		}

		toSync[bh.Cid()] = bh

		if len(toSync)%500 == 10 {
			log.Infof("todo: (%d) %s", len(toSync), bh.Cid())
		}

		if len(bh.Parents) == 0 {
			continue
		}

		if bh.Height <= 530000 {
			continue
		}

		pts, err := api.ChainGetTipSet(ctx, types.NewTipSetKey(bh.Parents...))
		if err != nil {
			log.Error(err)
			continue
		}

		for _, header := range pts.Blocks() {
			toVisit.PushBack(header)
		}
	}

	log.Infof("Syncing %d blocks", len(toSync))

	log.Infof("Persisting headers")
	if err := st.storeHeaders(toSync, true); err != nil {
		log.Error(err)
		return
	}

	log.Infof("Sync done")
}

type cwStorage struct {
	db *sql.DB

	headerLk sync.Mutex
}

func cwOpenStorage(dbSource string) (*cwStorage, error) {
	db, err := sql.Open("sqlite3", dbSource)
	if err != nil {
		return nil, err
	}

	st := &cwStorage{db: db}

	return st, st.setup()
}

func (st *cwStorage) setup() error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
create table if not exists blocks
(
	cid text not null
		constraint blocks_pk
			primary key,
	parentWeight numeric not null,
	parentStateRoot text not null,
	height int not null,
	miner text not null
		constraint blocks_id_address_map_miner_fk
			references id_address_map (address),
	timestamp int not null,
	vrfproof blob
);

create unique index if not exists block_cid_uindex
	on blocks (cid);

create table if not exists blocks_synced
(
	cid text not null
		constraint blocks_synced_pk
			primary key
		constraint blocks_synced_blocks_cid_fk
			references blocks,
	add_ts int not null
);

create unique index if not exists blocks_synced_cid_uindex
	on blocks_synced (cid);

create table if not exists block_parents
(
	block text not null
		constraint block_parents_blocks_cid_fk
			references blocks,
	parent text not null
		constraint block_parents_blocks_cid_fk_2
			references blocks
);

create unique index if not exists block_parents_block_parent_uindex
	on block_parents (block, parent);

create unique index if not exists blocks_cid_uindex
	on blocks (cid);
`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (st *cwStorage) hasBlock(bh cid.Cid) bool {
	var exitsts bool
	err := st.db.QueryRow(`select exists (select 1 FROM blocks_synced where cid=?)`, bh.String()).Scan(&exitsts)
	if err != nil {
		log.Error(err)
		return false
	}
	return exitsts
}

func (st *cwStorage) storeHeaders(bhs map[cid.Cid]*types.BlockHeader, sync bool) error {
	st.headerLk.Lock()
	defer st.headerLk.Unlock()

	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`insert into blocks (cid, parentWeight, parentStateRoot, height, miner, "timestamp", vrfproof) values (?, ?, ?, ?, ?, ?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt.Close() // nolint:errcheck
	for _, bh := range bhs {
		if _, err := stmt.Exec(bh.Cid().String(),
			bh.ParentWeight.String(),
			bh.ParentStateRoot.String(),
			bh.Height,
			bh.Miner.String(),
			bh.Timestamp,
			bh.Ticket.VRFProof,
		); err != nil {
			return err
		}
	}

	stmt2, err := tx.Prepare(`insert into block_parents (block, parent) values (?, ?) on conflict do nothing`)
	if err != nil {
		return err
	}
	defer stmt2.Close() // nolint:errcheck
	for _, bh := range bhs {
		for _, parent := range bh.Parents {
			if _, err := stmt2.Exec(bh.Cid().String(), parent.String()); err != nil {
				return err
			}
		}
	}

	if sync {
		stmt, err := tx.Prepare(`insert into blocks_synced (cid, add_ts) values (?, ?) on conflict do nothing`)
		if err != nil {
			return err
		}
		defer stmt.Close() // nolint:errcheck
		now := time.Now().Unix()

		for _, bh := range bhs {
			if _, err := stmt.Exec(bh.Cid().String(), now); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (st *cwStorage) close() error {
	return st.db.Close()
}
