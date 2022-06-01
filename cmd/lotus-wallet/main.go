package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/filecoin-project/lotus/api/v0api"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	ledgerwallet "github.com/filecoin-project/lotus/chain/wallet/ledger"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("main")

const FlagWalletRepo = "wallet-repo"

type jwtPayload struct {
	Allow []auth.Permission

	// Token creation time. Can be used to revoke older tokens
	Created time.Time

	// Rules limit actions which can be performed with a tokes. By default, when
	//  rules are nil, the token can perform all actions
	Rules *Rule
}

func main() {
	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		runCmd,
		getApiKeyCmd,
	}

	app := &cli.App{
		Name:    "lotus-wallet",
		Usage:   "Basic external wallet",
		Version: build.UserVersion(),
		Description: `
lotus-wallet provides a remote wallet service for lotus.

To configure your lotus node to use a remote wallet:
* Run 'lotus-wallet get-api-key' to generate API key
* Start lotus-wallet using 'lotus-wallet run' (see --help for additional flags)
* Edit lotus config (~/.lotus/config.toml)
  * Find the '[Wallet]' section
  * Set 'RemoteBackend' to '[api key]:http://[wallet ip]:[wallet port]'
    (the default port is 1777)
* Start (or restart) the lotus daemon

RULES:
lotus-wallet api tokens can carry filtering rules, limiting what actions can be
performed with the token.

Rule is a json object (map) with a single entry, where the key is specifying an
action, and the value contains action parameter. Some actions may take
additional sub-rules as parameters.

Rule execution can halt with 'Accept' or 'Reject' or finish without either of 
those things happening, in which case the filter will accept or reject based on
the value of the --rule-must-accept flag.

Special rules - Rules valid anywhere 
- {"Accept":{}} - Finish rule execution, accept action
- {"Reject":{}} - Finish rule execution, reject action
- [Rule...], {"First": [Rule...]} - Accepts when at least one sub-rule
             accepts; Stops on first Accept or Reject

Action rules - Rules matching actions
- {"New": Rule} - If WalletNew is called, execute the sub-rule
- {"Sign": Rule} - If WalletSign is called, execute the sub-rule

Sign rules - Rules matching signature parameters - can only be called as a 
                  sub-rule of the "Sign" rule
- {"Signer": {"Addr":["fXXX",..], "Next": Rule}} - Check that the signer address
  is one of the specified addresses
- {"Message": Rule} - WalletSign signing a chain message
- {"Block": Rule} - WalletSign signing a block header
- {"DealProposal": Rule} - WalletSign signing a deal proposal

Message rules - Rules matching message fields - can only be called as sub-rules
                of the "Message" rule.
- {"Source": {"Addr":["fXXX",..], "Next": Rule}} - Check that the source address
  is one of the specified addresses
- {"Destination": {"Addr":["fXXX",..], "Next": Rule}} - Check that the
  destination address is one of the specified addresses
- {"Method": {"Method":[number..], "Next": Rule}} - Check that the method number
  is one of the specified numbers
- {"Value":{("LessThan"|"MoreThan"): "[fil]", "Next": Rule}} - Check that
  message value is less/more that the specified value
- {"MaxFee":{("LessThan"|"MoreThan"): "[fil]", "Next": Rule}} - Check that
  maximum message fees are less/more that the specified value.

Block rules - Rules matching block header fields - can only be called as
              sub-rules of the "Block" rule.
- {"Miner": {"Addr":["fXXX",..], "Next": Rule}} - Check that the block miner
  address is one of the specified addresses

Example rules:

- Only allow signing messages from f3aaa
	[
		{"Sign": {"Signer":{"Addr":["f3aaa"], "Next": {"Accept":{}}}}},
		{"Reject": {}}
	]

- Disallow ExtendSectorExpiration, TerminateSectors, DeclareFaults,
  DeclareFaultsRecovered (Methods 8, 9, 10, 11)
	{"Sign": {"Message": {"Method":
		{"Method": [8,9,10,11], "Next":{"Reject":{}}}
	}}}

- Allow PoSt messages from f3aaa and f3bbb, and block mining from f3aaa on
  miner f01000
	[
		{"Sign": {"Signer":{"Addr":["f3aaa"], "Next": {"Block": {"Miner":
			{"Addr":["f01000"], "Next": {"Accept": {}}}
		}}}}},
		{"Sign": {"Signer":{"Addr":["f3aaa", "f3bbb"], "Next":
			{"Message": [
				{"Method": [5, 10, 11], "Next": {"Destination":
					{"Addr":["f01000"], "Next": {"Accept": {}}}
				}}
			]}
		}}},
		{"Reject": {}}
	]
`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    FlagWalletRepo,
				EnvVars: []string{"WALLET_PATH"},
				Value:   "~/.lotuswallet", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus",
			},
		},

		Commands: local,
	}
	app.Setup()

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		return
	}
}

var getApiKeyCmd = &cli.Command{
	Name:  "get-api-key",
	Usage: "Generate API Key",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "rules",
			Usage: "filtering rule object (see 'lotus-wallet --help')",
		},
		&cli.PathFlag{
			Name:  "rules-file",
			Usage: "path to filtering rules json file",
		},
	},
	Action: func(cctx *cli.Context) error {
		lr, ks, err := openRepo(cctx)
		if err != nil {
			return err
		}
		defer lr.Close() // nolint

		if cctx.IsSet("rules") && cctx.IsSet("rules-file") {
			return xerrors.Errorf("only one of --rules or --rules-file can be set")
		}

		var rules Rule
		if cctx.IsSet("rules") || cctx.IsSet("rules-file") {
			rb := []byte(cctx.String("rules"))
			if cctx.IsSet("rules-file") {
				rb, err = ioutil.ReadFile(cctx.Path("rules-file"))
				if err != nil {
					return xerrors.Errorf("reading rules file: %w", err)
				}
			}

			var r interface{}
			if err := json.Unmarshal(rb, &r); err != nil {
				return xerrors.Errorf("unmarshalling rules: %w", err)
			}

			_, err := ParseRule(cctx.Context, r)
			if err != nil {
				return xerrors.Errorf("parsing rules: %w", err)
			}

			rules = &r
		}

		p := jwtPayload{
			Allow:   []auth.Permission{api.PermAdmin},
			Created: time.Now(),
			Rules:   &rules,
		}

		authKey, err := modules.APISecret(ks, lr)
		if err != nil {
			return xerrors.Errorf("setting up api secret: %w", err)
		}

		k, err := jwt.Sign(&p, (*jwt.HMACSHA)(authKey))
		if err != nil {
			return xerrors.Errorf("jwt sign: %w", err)
		}

		fmt.Println(string(k))
		return nil
	},
}

type jwtTok struct{}

var tokenKey jwtTok

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start lotus wallet",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Usage: "host address and port the wallet api will listen on",
			Value: "0.0.0.0:1777",
		},
		&cli.BoolFlag{
			Name:  "ledger",
			Usage: "use a ledger device instead of an on-disk wallet",
		},
		&cli.BoolFlag{
			Name:  "interactive",
			Usage: "prompt before performing actions (DO NOT USE FOR MINER WORKER ADDRESS)",
		},
		&cli.BoolFlag{
			Name:  "offline",
			Usage: "don't query chain state in interactive mode",
		},
		&cli.BoolFlag{
			Name:   "disable-auth",
			Usage:  "(insecure) disable api auth",
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:  "rule-must-accept",
			Usage: "require all operations to be accepted by rule filters",
		},
	},
	Description: "Needs FULLNODE_API_INFO env-var to be set before running (see lotus-wallet --help for setup instructions)",
	Action: func(cctx *cli.Context) error {
		log.Info("Starting lotus wallet")

		ctx := lcli.ReqContext(cctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Register all metric views
		if err := view.Register(
			metrics.DefaultViews...,
		); err != nil {
			log.Fatalf("Cannot register the view: %v", err)
		}

		lr, ks, err := openRepo(cctx)
		if err != nil {
			return err
		}
		defer lr.Close() // nolint

		lw, err := wallet.NewWallet(ks)
		if err != nil {
			return err
		}

		var w api.Wallet = lw
		if cctx.Bool("ledger") {
			ds, err := lr.Datastore(context.Background(), "/metadata")
			if err != nil {
				return err
			}

			w = wallet.MultiWallet{
				Local:  lw,
				Ledger: ledgerwallet.NewWallet(ds),
			}
		}

		w = &FilteredWallet{
			under:      w,
			mustAccept: cctx.Bool("rule-must-accept"),
		}

		address := cctx.String("listen")
		mux := mux.NewRouter()

		log.Info("Setting up API endpoint at " + address)

		if cctx.Bool("interactive") {
			var ag func() (v0api.FullNode, jsonrpc.ClientCloser, error)

			if !cctx.Bool("offline") {
				ag = func() (v0api.FullNode, jsonrpc.ClientCloser, error) {
					return lcli.GetFullNodeAPI(cctx)
				}
			}

			w = &InteractiveWallet{
				under:     w,
				apiGetter: ag,
			}
		} else {
			w = &LoggedWallet{under: w}
		}

		rpcApi := proxy.MetricedWalletAPI(w)
		if !cctx.Bool("disable-auth") {
			rpcApi = api.PermissionedWalletAPI(rpcApi)
		}

		rpcServer := jsonrpc.NewServer()
		rpcServer.Register("Filecoin", rpcApi)

		mux.Handle("/rpc/v0", rpcServer)
		mux.PathPrefix("/").Handler(http.DefaultServeMux) // pprof

		var handler http.Handler = mux

		if !cctx.Bool("disable-auth") {
			authKey, err := modules.APISecret(ks, lr)
			if err != nil {
				return xerrors.Errorf("setting up api secret: %w", err)
			}

			authVerify := func(ctx context.Context, token string) ([]auth.Permission, error) {
				payload, ok := ctx.Value(tokenKey).(jwtPayload)
				if !ok {
					return nil, xerrors.Errorf("jwt payload not set on request context")
				}

				return payload.Allow, nil
			}

			log.Info("API auth enabled, use 'lotus-wallet get-api-key' to get API key")
			ah := &auth.Handler{
				Verify: authVerify,
				Next:   mux.ServeHTTP,
			}

			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				token := r.Header.Get("Authorization")
				if token == "" {
					token = r.FormValue("token")
					if token != "" {
						token = "Bearer " + token
					}
				}

				if token != "" {
					if !strings.HasPrefix(token, "Bearer ") {
						log.Warn("missing Bearer prefix in auth header")
						w.WriteHeader(401)
						return
					}
					token = strings.TrimPrefix(token, "Bearer ")

					var payload jwtPayload
					if _, err := jwt.Verify([]byte(token), (*jwt.HMACSHA)(authKey), &payload); err != nil {
						http.Error(w, fmt.Sprintf("JWT Verification failed: %s", err), http.StatusForbidden)
					}

					r = r.Clone(context.WithValue(r.Context(), tokenKey, payload))
				}

				ah.ServeHTTP(w, r)
			})
		}

		srv := &http.Server{
			Handler: handler,
			BaseContext: func(listener net.Listener) context.Context {
				ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.APIInterface, "lotus-wallet"))
				return ctx
			},
		}

		go func() {
			<-ctx.Done()
			log.Warn("Shutting down...")
			if err := srv.Shutdown(context.TODO()); err != nil {
				log.Errorf("shutting down RPC server failed: %s", err)
			}
			log.Warn("Graceful shutdown successful")
		}()

		nl, err := net.Listen("tcp", address)
		if err != nil {
			return err
		}

		return srv.Serve(nl)
	},
}

func openRepo(cctx *cli.Context) (repo.LockedRepo, types.KeyStore, error) {
	repoPath := cctx.String(FlagWalletRepo)
	r, err := repo.NewFS(repoPath)
	if err != nil {
		return nil, nil, err
	}

	ok, err := r.Exists()
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		if err := r.Init(repo.Wallet); err != nil {
			return nil, nil, err
		}
	}

	lr, err := r.Lock(repo.Wallet)
	if err != nil {
		return nil, nil, err
	}

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, nil, err
	}

	return lr, ks, nil
}
