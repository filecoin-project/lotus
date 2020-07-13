package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	rice "github.com/GeertJohan/go.rice"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

var log = logging.Logger("main")

var supportedSectors struct {
	SectorSizes []struct {
		Name    string
		Value   uint64
		Default bool
	}
}

func init() {
	for supportedSector, _ := range miner.SupportedProofTypes {
		sectorSize, err := supportedSector.SectorSize()
		if err != nil {
			panic(err)
		}

		supportedSectors.SectorSizes = append(supportedSectors.SectorSizes, struct {
			Name    string
			Value   uint64
			Default bool
		}{
			Name:    sectorSize.ShortString(),
			Value:   uint64(sectorSize),
			Default: false,
		})

	}

	sort.Slice(supportedSectors.SectorSizes[:], func(i, j int) bool {
		return supportedSectors.SectorSizes[i].Value < supportedSectors.SectorSizes[j].Value
	})

	supportedSectors.SectorSizes[0].Default = true
}

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting fountain")

	local := []*cli.Command{
		runCmd,
	}

	app := &cli.App{
		Name:    "lotus-fountain",
		Usage:   "Devnet token distribution utility",
		Version: build.UserVersion(),
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

		log.Info("Remote version: %s", v.Version)

		from, err := address.NewFromString(cctx.String("from"))
		if err != nil {
			return xerrors.Errorf("parsing source address (provide correct --from flag!): %w", err)
		}

		defaultMinerPeer, err := peer.Decode("12D3KooWJpBNhwgvoZ15EB1JwRTRpxgM9D2fwq6eEktrJJG74aP6")
		if err != nil {
			return err
		}

		h := &handler{
			ctx:            ctx,
			api:            nodeApi,
			from:           from,
			sendPerRequest: sendPerRequest,
			limiter: NewLimiter(LimiterConfig{
				TotalRate:   time.Second,
				TotalBurst:  256,
				IPRate:      time.Minute,
				IPBurst:     5,
				WalletRate:  15 * time.Minute,
				WalletBurst: 2,
			}),
			minerLimiter: NewLimiter(LimiterConfig{
				TotalRate:   time.Second,
				TotalBurst:  256,
				IPRate:      10 * time.Minute,
				IPBurst:     2,
				WalletRate:  1 * time.Hour,
				WalletBurst: 2,
			}),
			defaultMinerPeer: defaultMinerPeer,
		}

		http.Handle("/", http.FileServer(rice.MustFindBox("site").HTTPBox()))
		http.HandleFunc("/miner.html", h.minerhtml)
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

	from           address.Address
	sendPerRequest types.FIL

	limiter      *Limiter
	minerLimiter *Limiter

	defaultMinerPeer peer.ID
}

func (h *handler) minerhtml(w http.ResponseWriter, r *http.Request) {
	f, err := rice.MustFindBox("site").Open("_miner.html")
	if err != nil {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	tmpl, err := ioutil.ReadAll(f)
	if err != nil {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	var executedTmpl bytes.Buffer

	t, err := template.New("miner.html").Parse(string(tmpl))
	if err := t.Execute(&executedTmpl, supportedSectors); err != nil {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if _, err := io.Copy(w, &executedTmpl); err != nil {
		log.Errorf("failed to write template to string %s", err)
	}

	return
}

func (h *handler) send(w http.ResponseWriter, r *http.Request) {
	to, err := address.NewFromString(r.FormValue("address"))
	if err != nil {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(err.Error()))
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
		Value: types.BigInt(h.sendPerRequest),
		From:  h.from,
		To:    to,

		GasPrice: types.NewInt(0),
		GasLimit: 10000,
	})
	if err != nil {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	_, _ = w.Write([]byte(smsg.Cid().String()))
}

func (h *handler) mkminer(w http.ResponseWriter, r *http.Request) {
	owner, err := address.NewFromString(r.FormValue("address"))
	if err != nil {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if owner.Protocol() != address.BLS {
		w.WriteHeader(400)
		_, _ = w.Write([]byte("Miner address must use BLS. A BLS address starts with the prefix 't3'."))
		_, _ = w.Write([]byte("Please create a BLS address by running \"lotus wallet new bls\" while connected to a Lotus node."))
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
	reqIP := r.Header.Get("X-Real-IP")
	if reqIP == "" {
		h, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			log.Errorf("could not get ip from: %s, err: %s", r.RemoteAddr, err)
		}
		reqIP = h
	}
	limiter = h.minerLimiter.GetIPLimiter(reqIP)
	if !limiter.Allow() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests)+": IP limit", http.StatusTooManyRequests)
		return
	}

	// General limiter owner allow throttling all messages that can make it into the mpool
	if !h.minerLimiter.Allow() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests)+": global limit", http.StatusTooManyRequests)
		return
	}

	collateral, err := h.api.StatePledgeCollateral(r.Context(), types.EmptyTSK)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	smsg, err := h.api.MpoolPushMessage(h.ctx, &types.Message{
		Value: types.BigInt(h.sendPerRequest),
		From:  h.from,
		To:    owner,

		GasPrice: types.NewInt(0),
		GasLimit: 10000,
	})
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte("pushfunds: " + err.Error()))
		return
	}
	log.Infof("%s: push funds %s", owner, smsg.Cid())

	spt, err := ffiwrapper.SealProofTypeFromSectorSize(abi.SectorSize(ssize))
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte("sealprooftype: " + err.Error()))
		return
	}

	params, err := actors.SerializeParams(&power.CreateMinerParams{
		Owner:         owner,
		Worker:        owner,
		SealProofType: spt,
		Peer:          abi.PeerID(h.defaultMinerPeer),
	})
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	createStorageMinerMsg := &types.Message{
		To:    builtin.StoragePowerActorAddr,
		From:  h.from,
		Value: types.BigAdd(collateral, types.BigDiv(collateral, types.NewInt(100))),

		Method: builtin.MethodsPower.CreateMiner,
		Params: params,

		GasLimit: 10000000,
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

	mw, err := h.api.StateWaitMsg(r.Context(), c, build.MessageConfidence)
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

	mw, err := h.api.StateWaitMsg(r.Context(), c, build.MessageConfidence)
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

	var ma power.CreateMinerReturn
	if err := ma.UnmarshalCBOR(bytes.NewReader(mw.Receipt.Return)); err != nil {
		log.Errorf("%w", err)
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	fmt.Fprintf(w, "{\"addr\": \"%s\"}", ma.IDAddress)
}
