package main

import (
	"context"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	rice "github.com/GeertJohan/go.rice"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	verifregtypes9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	lcli "github.com/filecoin-project/lotus/cli"
)

var log = logging.Logger("main")

func main() {
	_ = logging.SetLogLevel("*", "INFO")

	log.Info("Starting fountain")

	local := []*cli.Command{
		runCmd,
	}

	app := &cli.App{
		Name:    "lotus-fountain",
		Usage:   "Devnet token distribution utility",
		Version: string(build.NodeUserVersion()),
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
		&cli.StringFlag{
			Name:    "amount",
			EnvVars: []string{"LOTUS_FOUNTAIN_AMOUNT"},
			Value:   "50",
		},
		&cli.Uint64Flag{
			Name:    "data-cap",
			EnvVars: []string{"LOTUS_DATACAP_AMOUNT"},
			Value:   verifregtypes9.MinVerifiedDealSize.Uint64(),
		},
		&cli.Float64Flag{
			Name:  "captcha-threshold",
			Value: 0.5,
		},
		&cli.StringFlag{
			Name:  "http-server-timeout",
			Value: "30s",
		},
	},
	Action: func(cctx *cli.Context) error {
		sendPerRequest, err := types.ParseFIL(cctx.String("amount"))
		if err != nil {
			return err
		}

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

		log.Infof("Remote version: %s", v.Version)

		from, err := address.NewFromString(cctx.String("from"))
		if err != nil {
			return xerrors.Errorf("parsing source address (provide correct --from flag!): %w", err)
		}

		box := rice.MustFindBox("site")

		h := &handler{
			ctx:            ctx,
			api:            nodeApi,
			from:           from,
			allowance:      types.NewInt(cctx.Uint64("data-cap")),
			sendPerRequest: sendPerRequest,
			limiter: NewLimiter(LimiterConfig{
				TotalRate:   500 * time.Millisecond,
				TotalBurst:  buildconstants.BlockMessageLimit,
				IPRate:      10 * time.Minute,
				IPBurst:     5,
				WalletRate:  15 * time.Minute,
				WalletBurst: 2,
			}),
			recapThreshold: cctx.Float64("captcha-threshold"),
			box:            box,
		}
		http.Handle("/", http.FileServer(box.HTTPBox()))
		http.HandleFunc("/funds.html", prepFundsHtml(box))
		http.Handle("/send", h)
		http.HandleFunc("/datacap.html", prepDataCapHtml(box))
		http.Handle("/datacap", h)
		fmt.Printf("Open http://%s\n", cctx.String("front"))

		go func() {
			<-ctx.Done()
			os.Exit(0)
		}()

		timeout, err := time.ParseDuration(cctx.String("http-server-timeout"))
		if err != nil {
			return xerrors.Errorf("invalid time string %s: %x", cctx.String("http-server-timeout"), err)
		}

		server := &http.Server{
			Addr:              cctx.String("front"),
			ReadHeaderTimeout: timeout,
		}

		return server.ListenAndServe()
	},
}

func prepFundsHtml(box *rice.Box) http.HandlerFunc {
	tmpl := template.Must(template.New("funds").Parse(box.MustString("funds.html")))
	return func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.Execute(w, os.Getenv("RECAPTCHA_SITE_KEY"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
}

func prepDataCapHtml(box *rice.Box) http.HandlerFunc {
	tmpl := template.Must(template.New("datacaps").Parse(box.MustString("datacap.html")))
	return func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.Execute(w, os.Getenv("RECAPTCHA_SITE_KEY"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
}

type handler struct {
	ctx context.Context
	api v0api.FullNode

	from           address.Address
	sendPerRequest types.FIL
	allowance      types.BigInt

	limiter        *Limiter
	recapThreshold float64
	box            *rice.Box
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "only POST is allowed", http.StatusBadRequest)
		return
	}

	reqIP := r.Header.Get("X-Real-IP")
	if reqIP == "" {
		h, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			log.Errorf("could not get ip from: %s, err: %s", r.RemoteAddr, err)
		}
		reqIP = h
	}

	capResp, err := VerifyToken(r.FormValue("g-recaptcha-response"), reqIP)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	if !capResp.Success || capResp.Score < h.recapThreshold {
		log.Infow("spam", "capResp", capResp)
		http.Error(w, "spam protection", http.StatusUnprocessableEntity)
		return
	}

	addressInput := r.FormValue("address")

	var filecoinAddress address.Address
	var decodeError error

	if strings.HasPrefix(addressInput, "0x") {
		ethAddress, err := ethtypes.ParseEthAddress(addressInput)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		filecoinAddress, decodeError = ethAddress.ToFilecoinAddress()
	} else {
		filecoinAddress, decodeError = address.NewFromString(addressInput)
	}

	if decodeError != nil {
		http.Error(w, decodeError.Error(), http.StatusBadRequest)
		return
	}
	if filecoinAddress == address.Undef {
		http.Error(w, "empty address", http.StatusBadRequest)
		return
	}

	// Limit based on wallet address
	limiter := h.limiter.GetWalletLimiter(filecoinAddress.String())
	if !limiter.Allow() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests)+": wallet limit\nYou can request tFIL and DataCap every 2 hours.", http.StatusTooManyRequests)
		return
	}

	// Limit based on IP
	if i := net.ParseIP(reqIP); i != nil && i.IsLoopback() {
		log.Errorf("rate limiting localhost: %s", reqIP)
	}

	limiter = h.limiter.GetIPLimiter(reqIP)
	if !limiter.Allow() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests)+": IP limit", http.StatusTooManyRequests)
		return
	}

	// General limiter to allow throttling all messages that can make it into the mpool
	if !h.limiter.Allow() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests)+": global limit", http.StatusTooManyRequests)
		return
	}

	var smsg *types.SignedMessage
	if r.RequestURI == "/send" {
		smsg, err = h.api.MpoolPushMessage(
			h.ctx, &types.Message{
				Value: types.BigInt(h.sendPerRequest),
				From:  h.from,
				To:    filecoinAddress,
			}, nil)
	} else if r.RequestURI == "/datacap" {
		var params []byte
		params, err = actors.SerializeParams(
			&verifregtypes9.AddVerifiedClientParams{
				Address:   filecoinAddress,
				Allowance: h.allowance,
			})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		smsg, err = h.api.MpoolPushMessage(
			h.ctx, &types.Message{
				Params: params,
				From:   h.from,
				To:     verifreg.Address,
				Method: verifreg.Methods.AddVerifiedClient,
			}, nil)
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Render the success HTML template
	if r.RequestURI == "/send" {
		tmpl, err := template.New("send_success").Parse(h.box.MustString("send_success.html"))
		if err != nil {
			http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		if err := tmpl.Execute(w, map[string]string{"CID": smsg.Cid().String()}); err != nil {
			http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		_, _ = w.Write([]byte(smsg.Cid().String()))
	}
}
