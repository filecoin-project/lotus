package cliutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/node/repo"
)

const (
	metadataTraceContext = "traceContext"
)

// flagsForAPI returns flags passed on the command line with the listen address
// of the API server (only used by the tests), in the order of precedence they
// should be applied for the requested kind of node.
func flagsForAPI(t repo.RepoType) []string {
	switch t {
	case repo.FullNode:
		return []string{"api-url"}
	case repo.StorageMiner:
		return []string{"miner-api-url"}
	case repo.Worker:
		return []string{"worker-api-url"}
	case repo.Markets:
		// support split markets-miner and monolith deployments.
		return []string{"markets-api-url", "miner-api-url"}
	default:
		panic(fmt.Sprintf("Unknown repo type: %v", t))
	}
}

func flagsForRepo(t repo.RepoType) []string {
	switch t {
	case repo.FullNode:
		return []string{"repo"}
	case repo.StorageMiner:
		return []string{"miner-repo"}
	case repo.Worker:
		return []string{"worker-repo"}
	case repo.Markets:
		// support split markets-miner and monolith deployments.
		return []string{"markets-repo", "miner-repo"}
	default:
		panic(fmt.Sprintf("Unknown repo type: %v", t))
	}
}

// EnvsForAPIInfos returns the environment variables to use in order of precedence
// to determine the API endpoint of the specified node type.
//
// It returns the current variables and deprecated ones separately, so that
// the user can log a warning when deprecated ones are found to be in use.
func EnvsForAPIInfos(t repo.RepoType) (primary string, fallbacks []string, deprecated []string) {
	switch t {
	case repo.FullNode:
		return "FULLNODE_API_INFO", nil, nil
	case repo.StorageMiner:
		// TODO remove deprecated deprecation period
		return "MINER_API_INFO", nil, []string{"STORAGE_API_INFO"}
	case repo.Worker:
		return "WORKER_API_INFO", nil, nil
	case repo.Markets:
		// support split markets-miner and monolith deployments.
		return "MARKETS_API_INFO", []string{"MINER_API_INFO"}, nil
	default:
		panic(fmt.Sprintf("Unknown repo type: %v", t))
	}
}

// GetAPIInfo returns the API endpoint to use for the specified kind of repo.
//
// The order of precedence is as follows:
//
//  1. *-api-url command line flags.
//  2. *_API_INFO environment variables
//  3. deprecated *_API_INFO environment variables
//  4. *-repo command line flags.
func GetAPIInfo(ctx *cli.Context, t repo.RepoType) (APIInfo, error) {
	// Check if there was a flag passed with the listen address of the API
	// server (only used by the tests)
	apiFlags := flagsForAPI(t)
	for _, f := range apiFlags {
		if !ctx.IsSet(f) {
			continue
		}
		strma := ctx.String(f)
		strma = strings.TrimSpace(strma)

		return APIInfo{Addr: strma}, nil
	}

	//
	// Note: it is not correct/intuitive to prefer environment variables over
	// CLI flags (repo flags below).
	//
	primaryEnv, fallbacksEnvs, deprecatedEnvs := EnvsForAPIInfos(t)
	env, ok := os.LookupEnv(primaryEnv)
	if ok {
		return ParseApiInfo(env), nil
	}

	for _, env := range deprecatedEnvs {
		env, ok := os.LookupEnv(env)
		if ok {
			log.Warnf("Using deprecated env(%s) value, please use env(%s) instead.", env, primaryEnv)
			return ParseApiInfo(env), nil
		}
	}

	repoFlags := flagsForRepo(t)
	for _, f := range repoFlags {
		// cannot use ctx.IsSet because it ignores default values
		path := ctx.String(f)
		if path == "" {
			continue
		}

		p, err := homedir.Expand(path)
		if err != nil {
			return APIInfo{}, xerrors.Errorf("could not expand home dir (%s): %w", f, err)
		}

		r, err := repo.NewFS(p)
		if err != nil {
			return APIInfo{}, xerrors.Errorf("could not open repo at path: %s; %w", p, err)
		}

		exists, err := r.Exists()
		if err != nil {
			return APIInfo{}, xerrors.Errorf("repo.Exists returned an error: %w", err)
		}

		if !exists {
			return APIInfo{}, errors.New("repo directory does not exist. Make sure your configuration is correct")
		}

		ma, err := r.APIEndpoint()
		if err != nil {
			return APIInfo{}, xerrors.Errorf("could not get api endpoint: %w", err)
		}

		token, err := r.APIToken()
		if err != nil {
			log.Warnf("Couldn't load CLI token, capabilities may be limited: %v", err)
		}

		return APIInfo{
			Addr:  ma.String(),
			Token: token,
		}, nil
	}

	for _, env := range fallbacksEnvs {
		env, ok := os.LookupEnv(env)
		if ok {
			return ParseApiInfo(env), nil
		}
	}

	return APIInfo{}, fmt.Errorf("could not determine API endpoint for node type: %v", t)
}

func GetRawAPI(ctx *cli.Context, t repo.RepoType, version string) (string, http.Header, error) {
	ainfo, err := GetAPIInfo(ctx, t)
	if err != nil {
		return "", nil, xerrors.Errorf("could not get API info for %s: %w", t, err)
	}

	addr, err := ainfo.DialArgs(version)
	if err != nil {
		return "", nil, xerrors.Errorf("could not get DialArgs: %w", err)
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintf(ctx.App.Writer, "using raw API %s endpoint: %s\n", version, addr)
	}

	return addr, ainfo.AuthHeader(), nil
}

func GetCommonAPI(ctx *cli.Context) (api.CommonNet, jsonrpc.ClientCloser, error) {
	ti, ok := ctx.App.Metadata["repoType"]
	if !ok {
		log.Errorf("unknown repo type, are you sure you want to use GetCommonAPI?")
		ti = repo.FullNode
	}
	t, ok := ti.(repo.RepoType)
	if !ok {
		log.Errorf("repoType type does not match the type of repo.RepoType")
	}

	if tn, ok := ctx.App.Metadata["testnode-storage"]; ok {
		return tn.(api.StorageMiner), func() {}, nil
	}
	if tn, ok := ctx.App.Metadata["testnode-full"]; ok {
		return tn.(api.FullNode), func() {}, nil
	}

	addr, headers, err := GetRawAPI(ctx, t, "v0")
	if err != nil {
		return nil, nil, err
	}

	return client.NewCommonRPCV0(ctx.Context, addr, headers)
}

func GetFullNodeAPI(ctx *cli.Context) (v0api.FullNode, jsonrpc.ClientCloser, error) {
	if tn, ok := ctx.App.Metadata["testnode-full"]; ok {
		return &v0api.WrapperV1Full{FullNode: tn.(v1api.FullNode)}, func() {}, nil
	}

	addr, headers, err := GetRawAPI(ctx, repo.FullNode, "v0")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using full node API v0 endpoint:", addr)
	}

	return client.NewFullNodeRPCV0(ctx.Context, addr, headers)
}

func GetFullNodeAPIV1(ctx *cli.Context) (v1api.FullNode, jsonrpc.ClientCloser, error) {
	if tn, ok := ctx.App.Metadata["testnode-full"]; ok {
		return tn.(v1api.FullNode), func() {}, nil
	}

	addr, headers, err := GetRawAPI(ctx, repo.FullNode, "v1")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using full node API v1 endpoint:", addr)
	}

	v1API, closer, err := client.NewFullNodeRPCV1(ctx.Context, addr, headers)
	if err != nil {
		return nil, nil, err
	}

	v, err := v1API.Version(ctx.Context)
	if err != nil {
		return nil, nil, err
	}
	if !v.APIVersion.EqMajorMinor(api.FullAPIVersion1) {
		return nil, nil, xerrors.Errorf("Remote API version didn't match (expected %s, remote %s)", api.FullAPIVersion1, v.APIVersion)
	}
	return v1API, closer, nil
}

type GetStorageMinerOptions struct {
	PreferHttp bool
}

type GetStorageMinerOption func(*GetStorageMinerOptions)

func StorageMinerUseHttp(opts *GetStorageMinerOptions) {
	opts.PreferHttp = true
}

func GetStorageMinerAPI(ctx *cli.Context, opts ...GetStorageMinerOption) (api.StorageMiner, jsonrpc.ClientCloser, error) {
	var options GetStorageMinerOptions
	for _, opt := range opts {
		opt(&options)
	}

	if tn, ok := ctx.App.Metadata["testnode-storage"]; ok {
		return tn.(api.StorageMiner), func() {}, nil
	}

	addr, headers, err := GetRawAPI(ctx, repo.StorageMiner, "v0")
	if err != nil {
		return nil, nil, err
	}

	if options.PreferHttp {
		u, err := url.Parse(addr)
		if err != nil {
			return nil, nil, xerrors.Errorf("parsing miner api URL: %w", err)
		}

		switch u.Scheme {
		case "ws":
			u.Scheme = "http"
		case "wss":
			u.Scheme = "https"
		}

		addr = u.String()
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using miner API v0 endpoint:", addr)
	}

	return client.NewStorageMinerRPCV0(ctx.Context, addr, headers)
}

func GetWorkerAPI(ctx *cli.Context) (api.Worker, jsonrpc.ClientCloser, error) {
	addr, headers, err := GetRawAPI(ctx, repo.Worker, "v0")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using worker API v0 endpoint:", addr)
	}

	return client.NewWorkerRPCV0(ctx.Context, addr, headers)
}

func GetMarketsAPI(ctx *cli.Context) (api.StorageMiner, jsonrpc.ClientCloser, error) {
	// to support lotus-miner cli tests.
	if tn, ok := ctx.App.Metadata["testnode-storage"]; ok {
		return tn.(api.StorageMiner), func() {}, nil
	}

	addr, headers, err := GetRawAPI(ctx, repo.Markets, "v0")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using markets API v0 endpoint:", addr)
	}

	// the markets node is a specialised miner's node, supporting only the
	// markets API, which is a subset of the miner API. All non-markets
	// operations will error out with "unsupported".
	return client.NewStorageMinerRPCV0(ctx.Context, addr, headers)
}

func GetGatewayAPI(ctx *cli.Context) (api.Gateway, jsonrpc.ClientCloser, error) {
	addr, headers, err := GetRawAPI(ctx, repo.FullNode, "v1")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using gateway API v1 endpoint:", addr)
	}

	return client.NewGatewayRPCV1(ctx.Context, addr, headers)
}

func GetGatewayAPIV0(ctx *cli.Context) (v0api.Gateway, jsonrpc.ClientCloser, error) {
	addr, headers, err := GetRawAPI(ctx, repo.FullNode, "v0")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using gateway API v0 endpoint:", addr)
	}

	return client.NewGatewayRPCV0(ctx.Context, addr, headers)
}

func DaemonContext(cctx *cli.Context) context.Context {
	if mtCtx, ok := cctx.App.Metadata[metadataTraceContext]; ok {
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
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	return ctx
}
