package main

import (
	"fmt"
	"hash/crc32"
	"strconv"

	"github.com/ipfs/go-cid"
	"gopkg.in/urfave/cli.v2"
)

var dotCmd = &cli.Command{
	Name:  "dot",
	Usage: "generate dot graphs",
	Action: func(cctx *cli.Context) error {
		st, err := openStorage()
		if err != nil {
			return err
		}

		minH, err := strconv.ParseInt(cctx.Args().Get(0), 10, 32)
		tosee, err := strconv.ParseInt(cctx.Args().Get(1), 10, 32)
		maxH := minH + tosee

		res, err := st.db.Query("select block, parent, b.miner, b.height from block_parents inner join blocks b on block_parents.block = b.cid where b.height > ? and b.height < ?", minH, maxH)
		if err != nil {
			return err
		}

		fmt.Println("digraph D {")

		for res.Next() {
			var block, parent, miner string
			var height uint64
			if err := res.Scan(&block, &parent, &miner, &height); err != nil {
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
				hasstr = " UNSYNCED"
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
