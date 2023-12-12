package main

import (
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/web"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"net"
)

var webCmd = &cli.Command{
	Name:        "web",
	Usage:       "Start lotus provider web interface",
	Description: "Start an instance of lotus provider web interface.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Usage: "Address to listen on",
			Value: "127.0.0.1:4701",
		},
	},
	Action: func(cctx *cli.Context) error {
		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		ctx := lcli.DaemonContext(cctx)

		listen := cctx.String("listen")
		listenAddrPort := listen
		if host, port, err := net.SplitHostPort(listen); err == nil {
			if host == "" {
				host = "127.0.0.1"
			}
			if port == "" {
				return xerrors.Errorf("invalid listen address, no port: %s", listen)
			}
			listenAddrPort = net.JoinHostPort(host, port)
		}

		web, err := web.GetSrv(ctx, db, listen)
		if err != nil {
			return err
		}

		log.Infof("listening on %s; http://%s", listen, listenAddrPort)

		go web.ListenAndServe()

		<-ctx.Done()
		return web.Shutdown(ctx)
	},
}
