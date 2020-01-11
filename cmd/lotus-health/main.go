package main

import (
	"context"
	"os"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"gopkg.in/urfave/cli.v2"
)

type CidWindow [][]cid.Cid

var log = logging.Logger("lotus-health")

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting health agent")

	local := []*cli.Command{
		watchHeadCmd,
	}

	app := &cli.App{
		Name:     "lotus-health",
		Usage:    "Tools for monitoring lotus daemon health",
		Version:  build.UserVersion,
		Commands: local,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Warn(err)
		return
	}
}

var watchHeadCmd = &cli.Command{
	Name: "watch-head",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "threshold",
			Value: 3,
			Usage: "number of times head remains unchanged before failing health check",
		},
		&cli.IntFlag{
			Name:  "interval",
			Value: build.BlockDelay,
			Usage: "interval in seconds between chain head checks",
		},
		&cli.StringFlag{
			Name:  "systemd-unit",
			Value: "lotus-daemon.service",
			Usage: "systemd unit name to restart on health check failure",
		},
	},
	Action: func(c *cli.Context) error {
		threshold := c.Int("threshold")
		interval := time.Duration(c.Int("interval")) * time.Second
		name := c.String("systemd-unit")

		var headCheckWindow CidWindow

		api, closer, err := lcli.GetFullNodeAPI(c)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(c)

		if err := WaitForSyncComplete(ctx, api); err != nil {
			log.Fatal(err)
		}

		ch := make(chan CidWindow, 1)
		aCh := make(chan interface{}, 1)

		go func() {
			for {
				headCheckWindow, err = updateWindow(ctx, api, headCheckWindow, threshold, ch)
				if err != nil {
					log.Fatal(err)
				}
				time.Sleep(interval)
			}
		}()

		go func() {
			result, err := alertHandler(name, aCh)
			if err != nil {
				log.Fatal(err)
			}
			if result != "done" {
				log.Fatal("systemd unit failed to restart:", result)
			}
			log.Info("restarting health agent")
			// Exit health agent and let supervisor restart health agent
			// Restarting lotus systemd unit kills api connection
			os.Exit(130)
		}()

		for {
			ok := checkWindow(ch, int(interval))
			if !ok {
				log.Warn("chain head has not updated. Restarting systemd service")
				aCh <- nil
				break
			}
			log.Info("chain head is healthy")
		}
		return nil
	},
}

/*
 * reads channel of slices of Cids
 * compares slices of Cids when len is greater or equal to `t` - threshold
 * if all slices are equal, head has not updated and returns false
 */
func checkWindow(ch chan CidWindow, t int) bool {
	select {
	case window := <-ch:
		var dup int
		windowLen := len(window)
		if windowLen >= t {
		cidWindow:
			for i, cids := range window {
				next := windowLen - 1 - i
				// if array length is different, head is changing
				if next >= 1 && len(window[next]) != len(window[next-1]) {
					break cidWindow
				}
				// if cids are different, head is changing
				for j := range cids {
					if next >= 1 && window[next][j] != window[next-1][j] {
						break cidWindow
					}
				}
				if i < (t - 1) {
					dup++
				}
			}

			if dup == (t - 1) {
				return false
			}
		}
		return true
	}
}

/*
 * reads channel of slices of slices of Cids
 * compares Cids when len of window is greater or equal to `t` - threshold
 * if all slices are the equal, head has not updated and returns false
 */
func updateWindow(ctx context.Context, a api.FullNode, w CidWindow, t int, ch chan CidWindow) (CidWindow, error) {
	head, err := a.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	window := appendCIDsToWindow(w, head.Cids(), t)
	ch <- window
	return window, nil
}

/*
 * appends slice of Cids to window slice
 * keeps a fixed window slice size, dropping older slices
 * returns new window
 */
func appendCIDsToWindow(w CidWindow, c []cid.Cid, t int) CidWindow {
	offset := len(w) - t + 1
	if offset >= 0 {
		return append(w[offset:], c)
	}
	return append(w, c)
}

/*
 * wait for node to sync
 */
func WaitForSyncComplete(ctx context.Context, napi api.FullNode) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
			head, err := napi.ChainHead(ctx)
			if err != nil {
				return err
			}

			if time.Now().Unix()-int64(head.MinTimestamp()) < build.BlockDelay {
				return nil
			}
		}
	}
}
