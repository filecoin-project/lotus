package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/lib/jsonrpc"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("cli")

const (
	metadataTraceConetxt = "traceContext"
)

// ApiConnector returns API instance
type ApiConnector func() api.FullNode

type APIInfo struct {
	Addr  multiaddr.Multiaddr
	Token []byte
}

func (a APIInfo) DialArgs() (string, error) {
	_, addr, err := manet.DialArgs(a.Addr)

	return "ws://" + addr + "/rpc/v0", err
}

func (a APIInfo) AuthHeader() http.Header {
	if len(a.Token) != 0 {
		headers := http.Header{}
		headers.Add("Authorization", "Bearer "+string(a.Token))
		return headers
	}
	log.Warn("API Token not set and requested, capabilities might be limited.")
	return nil
}

func flagForRepo(t repo.RepoType) string {
	switch t {
	case repo.FullNode:
		return "repo"
	case repo.StorageMiner:
		return "storagerepo"
	default:
		panic(fmt.Sprintf("Unknown repo type: %v", t))
	}
}

func envForRepo(t repo.RepoType) string {
	switch t {
	case repo.FullNode:
		return "FULLNODE_API_INFO"
	case repo.StorageMiner:
		return "STORAGE_API_INFO"
	default:
		panic(fmt.Sprintf("Unknown repo type: %v", t))
	}
}

func GetAPIInfo(ctx *cli.Context, t repo.RepoType) (APIInfo, error) {
	if env, ok := os.LookupEnv(envForRepo(t)); ok {
		sp := strings.SplitN(env, ":", 2)
		if len(sp) != 2 {
			log.Warnf("invalid env(%s) value, missing token or address", envForRepo(t))
		} else {
			ma, err := multiaddr.NewMultiaddr(sp[1])
			if err != nil {
				return APIInfo{}, xerrors.Errorf("could not parse multiaddr from env(%s): %w", envForRepo(t), err)
			}
			return APIInfo{
				Addr:  ma,
				Token: []byte(sp[0]),
			}, nil
		}
	}

	repoFlag := flagForRepo(t)

	p, err := homedir.Expand(ctx.String(repoFlag))
	if err != nil {
		return APIInfo{}, xerrors.Errorf("cound not exand home dir (%s): %w", repoFlag, err)
	}

	r, err := repo.NewFS(p)
	if err != nil {
		return APIInfo{}, xerrors.Errorf("could not open repo at path: %s; %w", p, err)
	}

	ma, err := r.APIEndpoint()
	if err != nil {
		return APIInfo{}, xerrors.Errorf("could not get api enpoint: %w", err)
	}

	token, err := r.APIToken()
	if err != nil {
		log.Warnf("Couldn't load CLI token, capabilities may be limited: %v", err)
	}

	return APIInfo{
		Addr:  ma,
		Token: token,
	}, nil
}

func GetRawAPI(ctx *cli.Context, t repo.RepoType) (string, http.Header, error) {

	ainfo, err := GetAPIInfo(ctx, t)
	if err != nil {
		return "", nil, xerrors.Errorf("could not get API info: %w", err)
	}

	addr, err := ainfo.DialArgs()
	if err != nil {
		return "", nil, xerrors.Errorf("could not get DialArgs: %w", err)
	}

	return addr, ainfo.AuthHeader(), nil
}

func GetAPI(ctx *cli.Context) (api.Common, jsonrpc.ClientCloser, error) {
	ti, ok := ctx.App.Metadata["repoType"]
	if !ok {
		log.Errorf("unknown repo type, are you sure you want to use GetAPI?")
		ti = repo.FullNode
	}
	t, ok := ti.(repo.RepoType)
	if !ok {
		log.Errorf("repoType type does not match the type of repo.RepoType")
	}

	addr, headers, err := GetRawAPI(ctx, t)
	if err != nil {
		return nil, nil, err
	}

	return client.NewCommonRPC(addr, headers)
}

func GetFullNodeAPI(ctx *cli.Context) (api.FullNode, jsonrpc.ClientCloser, error) {
	addr, headers, err := GetRawAPI(ctx, repo.FullNode)
	if err != nil {
		return nil, nil, err
	}

	return client.NewFullNodeRPC(addr, headers)
}

func GetStorageMinerAPI(ctx *cli.Context) (api.StorageMiner, jsonrpc.ClientCloser, error) {
	addr, headers, err := GetRawAPI(ctx, repo.StorageMiner)
	if err != nil {
		return nil, nil, err
	}

	return client.NewStorageMinerRPC(addr, headers)
}

func DaemonContext(cctx *cli.Context) context.Context {
	if mtCtx, ok := cctx.App.Metadata[metadataTraceConetxt]; ok {
		return mtCtx.(context.Context)
	}

	return context.Background()
}

// ReqContext returns context for cli execution. Calling it for the first time
// installs SIGTERM handler that will close returned context.
// Not safe for concurrent execution.
func ReqContext(cctx *cli.Context) context.Context {
	tCtx := DaemonContext(cctx)

	ctx, done := context.WithCancel(tCtx)
	sigChan := make(chan os.Signal, 2)
	go func() {
		<-sigChan
		done()
	}()
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	return ctx
}

var Commands = []*cli.Command{
	authCmd,
	chainCmd,
	clientCmd,
	fetchParamCmd,
	mpoolCmd,
	netCmd,
	paychCmd,
	sendCmd,
	stateCmd,
	syncCmd,
	versionCmd,
	walletCmd,
}
