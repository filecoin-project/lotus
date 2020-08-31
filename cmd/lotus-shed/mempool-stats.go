package main

import (
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"github.com/urfave/cli/v2"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

type msgInfo struct {
	msg  *types.SignedMessage
	seen time.Time
}

var mpoolStatsCmd = &cli.Command{
	Name: "mpool-stats",
	Action: func(cctx *cli.Context) error {
		logging.SetLogLevel("rpc", "ERROR")

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		updates, err := api.MpoolSub(ctx)
		if err != nil {
			return err
		}

		tracker := make(map[cid.Cid]*msgInfo)
		tick := time.Tick(time.Second)
		for {
			select {
			case u := <-updates:
				switch u.Type {
				case lapi.MpoolAdd:
					tracker[u.Message.Cid()] = &msgInfo{
						msg:  u.Message,
						seen: time.Now(),
					}
				case lapi.MpoolRemove:
					mi, ok := tracker[u.Message.Cid()]
					if !ok {
						continue
					}
					fmt.Printf("%s was in the mempool for %s (feecap=%s, prem=%s)\n", u.Message.Cid(), time.Since(mi.seen), u.Message.Message.GasFeeCap, u.Message.Message.GasPremium)
					delete(tracker, u.Message.Cid())
				default:
					return fmt.Errorf("unrecognized mpool update state: %d", u.Type)
				}
			case <-tick:
				if len(tracker) == 0 {
					continue
				}
				var avg time.Duration
				var oldest time.Duration
				for _, v := range tracker {
					age := time.Since(v.seen)
					if age > oldest {
						oldest = age
					}

					avg += age
				}
				fmt.Printf("%d messages in mempool for average of %s, max=%s\n", len(tracker), avg/time.Duration(len(tracker)), oldest)
			}
		}
		return nil
	},
}
