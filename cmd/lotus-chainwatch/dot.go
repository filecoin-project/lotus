package main

import (
	"database/sql"
	"fmt"
	"hash/crc32"
	"strconv"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var dotCmd = &cli.Command{
	Name:      "dot",
	Usage:     "generate dot graphs",
	ArgsUsage: "<minHeight> <toseeHeight>",
	Action: func(cctx *cli.Context) error {
		ll := cctx.String("log-level")
		if err := logging.SetLogLevel("*", ll); err != nil {
			return err
		}

		db, err := sql.Open("postgres", cctx.String("db"))
		if err != nil {
			return err
		}
		defer func() {
			if err := db.Close(); err != nil {
				log.Errorw("Failed to close database", "error", err)
			}
		}()

		if err := db.Ping(); err != nil {
			return xerrors.Errorf("Database failed to respond to ping (is it online?): %w", err)
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

		res, err := db.Query(`select block, parent, b.miner, b.height, p.height from block_parents
    inner join blocks b on block_parents.block = b.cid
    inner join blocks p on block_parents.parent = p.cid
where b.height > $1 and b.height < $2`, minH, maxH)

		if err != nil {
			return err
		}

		fmt.Println("digraph D {")

		hl, err := syncedBlocks(db)
		if err != nil {
			log.Fatal(err)
		}

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

			_, has := hl[bc]

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

func syncedBlocks(db *sql.DB) (map[cid.Cid]struct{}, error) {
	// timestamp is used to return a configurable amount of rows based on when they were last added.
	rws, err := db.Query(`select cid FROM blocks_synced`)
	if err != nil {
		return nil, xerrors.Errorf("Failed to query blocks_synced: %w", err)
	}
	out := map[cid.Cid]struct{}{}

	for rws.Next() {
		var c string
		if err := rws.Scan(&c); err != nil {
			return nil, xerrors.Errorf("Failed to scan blocks_synced: %w", err)
		}

		ci, err := cid.Parse(c)
		if err != nil {
			return nil, xerrors.Errorf("Failed to parse blocks_synced: %w", err)
		}

		out[ci] = struct{}{}
	}
	return out, nil
}
