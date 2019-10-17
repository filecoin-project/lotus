package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	rice "github.com/GeertJohan/go.rice"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	lcli "github.com/filecoin-project/go-lotus/cli"
)

var log = logging.Logger("main")

var sendPerRequest = types.NewInt(500_000_000)

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting fountain")

	local := []*cli.Command{
		runCmd,
	}

	app := &cli.App{
		Name:    "lotus-fountain",
		Usage:   "Devnet token distribution utility",
		Version: build.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
		},

		Commands: local,
	}

	if err := app.Run(os.Args); err != nil {
		log.Warn(err)
		return
	}
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start lotus fountain",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "front",
			Value: "127.0.0.1:7777",
		},
		&cli.StringFlag{
			Name: "from",
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		v, err := nodeApi.Version(ctx)
		if err != nil {
			return err
		}

		log.Info("Remote version: %s", v.Version)

		from, err := address.NewFromString(cctx.String("from"))
		if err != nil {
			return xerrors.Errorf("parsing source address (provide correct --from flag!): %w", err)
		}

		h := &handler{
			ctx:  ctx,
			api:  nodeApi,
			from: from,
		}

		http.Handle("/", http.FileServer(rice.MustFindBox("site").HTTPBox()))
		http.HandleFunc("/send", h.send)
		http.HandleFunc("/mkminer", h.mkminer)

		fmt.Printf("Open http://%s\n", cctx.String("front"))

		go func() {
			<-ctx.Done()
			os.Exit(0)
		}()

		return http.ListenAndServe(cctx.String("front"), nil)
	},
}

type handler struct {
	ctx context.Context
	api api.FullNode

	from address.Address
}

func (h *handler) send(w http.ResponseWriter, r *http.Request) {
	to, err := address.NewFromString(r.FormValue("address"))
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	smsg, err := h.api.MpoolPushMessage(h.ctx, &types.Message{
		Value: sendPerRequest,
		From:  h.from,
		To:    to,

		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000),
	})
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	w.Write([]byte(smsg.Cid().String()))
}

func (h *handler) mkminer(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(400)
	// todo
}
