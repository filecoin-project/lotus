package lpweb

import (
	"embed"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
	"net"
	"net/http"
	"sync"
	"text/template"
)

var log = logging.Logger("lpweb")

//go:embed web/*
var templateFS embed.FS

type app struct {
	db *harmonydb.DB
	t  *template.Template

	rpcInfoLk  sync.Mutex
	rpcInfos   []rpcInfo
	workingApi v1api.FullNode

	actorInfoLk sync.Mutex
	actorInfos  []actorInfo
}

type rpcInfo struct {
	Address   string
	CLayers   []string
	Reachable bool
	SyncState string
	Version   string
}

type actorInfo struct {
	Address string
	CLayers []string

	QualityAdjustedPower string
	RawBytePower         string

	Deadlines []actorDeadline
}

type actorDeadline struct {
	Empty      bool
	Current    bool
	Proven     bool
	PartFaulty bool
	Faulty     bool
}

func (a *app) index(w http.ResponseWriter, r *http.Request) {
	var indexData struct {
		RPCInfos   []rpcInfo
		ActorInfos []actorInfo
	}

	a.rpcInfoLk.Lock()
	defer a.rpcInfoLk.Unlock()

	a.actorInfoLk.Lock()
	defer a.actorInfoLk.Unlock()

	indexData.RPCInfos = a.rpcInfos
	indexData.ActorInfos = a.actorInfos

	a.executeTemplate(w, "index", indexData)
}

func (a *app) chainRpc(w http.ResponseWriter, r *http.Request) {
	a.rpcInfoLk.Lock()
	defer a.rpcInfoLk.Unlock()

	a.executeTemplate(w, "chain_rpcs", a.rpcInfos)
}

func (a *app) executeTemplate(w http.ResponseWriter, name string, data interface{}) {
	if err := a.t.ExecuteTemplate(w, name, data); err != nil {
		log.Errorf("execute template %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func ServeWeb(listen string, db *harmonydb.DB) error {
	t := new(template.Template)
	t, err := t.ParseFS(templateFS, "web/*")
	if err != nil {
		return xerrors.Errorf("parse templates: %w", err)
	}

	a := &app{
		db: db,
		t:  t,
	}

	go a.watchRpc()
	go a.watchActor()

	m := mux.NewRouter()

	m.HandleFunc("/", a.index)
	m.HandleFunc("/index/chainrpc", a.chainRpc)

	http.Handle("/", m)

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

	log.Infof("listening on %s; http://%s", listen, listenAddrPort)
	if err := http.ListenAndServe(listen, nil); err != nil {
		return xerrors.Errorf("listen and serve: %w", err)
	}

	return nil
}
