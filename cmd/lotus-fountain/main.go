package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	rice "github.com/GeertJohan/go.rice"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p-peer"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var log = logging.Logger("main")

var sendPerRequest, _ = types.ParseFIL("0.005")

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting fountain")

	local := []*cli.Command{
		runCmd,
	}

	app := &cli.App{
		Name:    "lotus-fountain",
		Usage:   "Devnet token distribution utility",
		Version: build.UserVersion,
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
			limiter: NewLimiter(LimiterConfig{
				TotalRate:   time.Second,
				TotalBurst:  20,
				IPRate:      time.Minute,
				IPBurst:     5,
				WalletRate:  15 * time.Minute,
				WalletBurst: 2,
			}),
			minerLimiter: NewLimiter(LimiterConfig{
				TotalRate:   time.Second,
				TotalBurst:  20,
				IPRate:      10 * time.Minute,
				IPBurst:     2,
				WalletRate:  1 * time.Hour,
				WalletBurst: 2,
			}),
		}

		http.Handle("/", http.FileServer(rice.MustFindBox("site").HTTPBox()))
		http.HandleFunc("/send", h.send)
		http.HandleFunc("/mkminer", h.mkminer)
		http.HandleFunc("/msgwait", h.msgwait)
		http.HandleFunc("/msgwaitaddr", h.msgwaitaddr)

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

	limiter      *Limiter
	minerLimiter *Limiter
}

func (h *handler) send(w http.ResponseWriter, r *http.Request) {
	to, err := address.NewFromString(r.FormValue("address"))
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	// Limit based on wallet address
	limiter := h.limiter.GetWalletLimiter(to.String())
	if !limiter.Allow() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests)+": wallet limit", http.StatusTooManyRequests)
		return
	}

	// Limit based on IP

	reqIP := r.Header.Get("X-Real-IP")
	if reqIP == "" {
		h, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			log.Errorf("could not get ip from: %s, err: %s", r.RemoteAddr, err)
		}
		reqIP = h
	}
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

	smsg, err := h.api.MpoolPushMessage(h.ctx, &types.Message{
		Value: types.BigInt(sendPerRequest),
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
	owner, err := address.NewFromString(r.FormValue("address"))
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	if owner.Protocol() != address.BLS {
		w.WriteHeader(400)
		w.Write([]byte("Miner address must use BLS"))
		return
	}

	ssize, err := strconv.ParseInt(r.FormValue("sectorSize"), 10, 64)
	if err != nil {
		return
	}

	log.Infof("%s: create actor start", owner)

	// Limit based on wallet address
	limiter := h.minerLimiter.GetWalletLimiter(owner.String())
	if !limiter.Allow() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests)+": wallet limit", http.StatusTooManyRequests)
		return
	}

	// Limit based on IP
	limiter = h.minerLimiter.GetIPLimiter(r.RemoteAddr)
	if !limiter.Allow() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests)+": IP limit", http.StatusTooManyRequests)
		return
	}

	// General limiter owner allow throttling all messages that can make it into the mpool
	if !h.minerLimiter.Allow() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests)+": global limit", http.StatusTooManyRequests)
		return
	}

	collateral, err := h.api.StatePledgeCollateral(r.Context(), nil)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	smsg, err := h.api.MpoolPushMessage(h.ctx, &types.Message{
		Value: types.BigInt(sendPerRequest),
		From:  h.from,
		To:    owner,

		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000),
	})
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte("pushfunds: " + err.Error()))
		return
	}
	log.Infof("%s: push funds %s", owner, smsg.Cid())

	params, err := actors.SerializeParams(&actors.CreateStorageMinerParams{
		Owner:      owner,
		Worker:     owner,
		SectorSize: uint64(ssize),
		PeerID:     peer.ID("SETME"),
	})
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	createStorageMinerMsg := &types.Message{
		To:    actors.StoragePowerAddress,
		From:  h.from,
		Value: collateral,

		Method: actors.SPAMethods.CreateStorageMiner,
		Params: params,

		GasLimit: types.NewInt(10000000),
		GasPrice: types.NewInt(0),
	}

	signed, err := h.api.MpoolPushMessage(r.Context(), createStorageMinerMsg)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	log.Infof("%s: create miner msg: %s", owner, signed.Cid())

	http.Redirect(w, r, fmt.Sprintf("/wait.html?f=%s&m=%s&o=%s", signed.Cid(), smsg.Cid(), owner), 303)
}

func (h *handler) msgwait(w http.ResponseWriter, r *http.Request) {
	c, err := cid.Parse(r.FormValue("cid"))
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	mw, err := h.api.StateWaitMsg(r.Context(), c)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	if mw.Receipt.ExitCode != 0 {
		w.WriteHeader(400)
		w.Write([]byte(xerrors.Errorf("create storage miner failed: exit code %d", mw.Receipt.ExitCode).Error()))
		return
	}
	w.WriteHeader(200)
}

func (h *handler) msgwaitaddr(w http.ResponseWriter, r *http.Request) {
	c, err := cid.Parse(r.FormValue("cid"))
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	mw, err := h.api.StateWaitMsg(r.Context(), c)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	if mw.Receipt.ExitCode != 0 {
		w.WriteHeader(400)
		w.Write([]byte(xerrors.Errorf("create storage miner failed: exit code %d", mw.Receipt.ExitCode).Error()))
		return
	}
	w.WriteHeader(200)

	addr, err := address.NewFromBytes(mw.Receipt.Return)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	fmt.Fprintf(w, "{\"addr\": \"%s\"}", addr)
}
