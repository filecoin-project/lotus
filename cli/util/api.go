package cliutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/lib/retry"
	"github.com/filecoin-project/lotus/node/repo"
)

const (
	metadataTraceContext = "traceContext"
)

// GetAPIInfoMulti returns the API endpoints to use for the specified kind of repo.
//
// The order of precedence is as follows:
//
//  1. *-api-url command line flags.
//  2. *_API_INFO environment variables
//  3. deprecated *_API_INFO environment variables
//  4. *-repo command line flags.
func GetAPIInfoMulti(ctx *cli.Context, t repo.RepoType) ([]APIInfo, error) {
	// Check if there was a flag passed with the listen address of the API
	// server (only used by the tests)
	for _, f := range t.APIFlags() {
		if !ctx.IsSet(f) {
			continue
		}
		strma := ctx.String(f)
		strma = strings.TrimSpace(strma)

		return []APIInfo{{Addr: strma}}, nil
	}

	//
	// Note: it is not correct/intuitive to prefer environment variables over
	// CLI flags (repo flags below).
	//
	primaryEnv, fallbacksEnvs, deprecatedEnvs := t.APIInfoEnvVars()
	env, ok := os.LookupEnv(primaryEnv)
	if ok {
		return ParseApiInfoMulti(env), nil
	}

	for _, env := range deprecatedEnvs {
		env, ok := os.LookupEnv(env)
		if ok {
			log.Warnf("Using deprecated env(%s) value, please use env(%s) instead.", env, primaryEnv)
			return ParseApiInfoMulti(env), nil
		}
	}

	for _, f := range t.RepoFlags() {
		// cannot use ctx.IsSet because it ignores default values
		path := ctx.String(f)
		if path == "" {
			continue
		}
		return GetAPIInfoFromRepoPath(path, t)
	}
	for _, env := range fallbacksEnvs {
		env, ok := os.LookupEnv(env)
		if ok {
			return ParseApiInfoMulti(env), nil
		}
	}

	return []APIInfo{}, fmt.Errorf("could not determine API endpoint for node type: %v. Try setting environment variable: %s", t.Type(), primaryEnv)
}

func GetAPIInfoFromRepoPath(path string, t repo.RepoType) ([]APIInfo, error) {
	p, err := homedir.Expand(path)
	if err != nil {
		return []APIInfo{}, xerrors.Errorf("could not expand home dir (%s): %w", path, err)
	}

	r, err := repo.NewFS(p)
	if err != nil {
		return []APIInfo{}, xerrors.Errorf("could not open repo at path: %s; %w", p, err)
	}

	exists, err := r.Exists()
	if err != nil {
		return []APIInfo{}, xerrors.Errorf("repo.Exists returned an error: %w", err)
	}

	if !exists {
		return []APIInfo{}, errors.New("repo directory does not exist. Make sure your configuration is correct")
	}

	ma, err := r.APIEndpoint()
	if err != nil {
		return []APIInfo{}, xerrors.Errorf("could not get api endpoint: %w", err)
	}

	token, err := r.APIToken()
	if err != nil {
		log.Warnf("Couldn't load CLI token, capabilities may be limited: %v", err)
	}

	return []APIInfo{{
		Addr:  ma.String(),
		Token: token,
	}}, nil
}

func GetAPIInfo(ctx *cli.Context, t repo.RepoType) (APIInfo, error) {
	ainfos, err := GetAPIInfoMulti(ctx, t)
	if err != nil || len(ainfos) == 0 {
		return APIInfo{}, err
	}

	if len(ainfos) > 1 {
		log.Warn("multiple API infos received when only one was expected")
	}

	return ainfos[0], nil

}

type HttpHead struct {
	addr   string
	header http.Header
}

func GetRawAPIMulti(ctx *cli.Context, t repo.RepoType, version string) ([]HttpHead, error) {

	var httpHeads []HttpHead
	ainfos, err := GetAPIInfoMulti(ctx, t)
	if err != nil || len(ainfos) == 0 {
		return httpHeads, xerrors.Errorf("could not get API info for %s: %w", t.Type(), err)
	}

	for _, ainfo := range ainfos {
		addr, err := ainfo.DialArgs(version)
		if err != nil {
			return httpHeads, xerrors.Errorf("could not get DialArgs: %w", err)
		}
		httpHeads = append(httpHeads, HttpHead{addr: addr, header: ainfo.AuthHeader()})
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintf(ctx.App.Writer, "using raw API %s endpoint: %s\n", version, httpHeads[0].addr)
	}

	return httpHeads, nil
}

func GetRawAPI(ctx *cli.Context, t repo.RepoType, version string) (string, http.Header, error) {
	heads, err := GetRawAPIMulti(ctx, t, version)
	if err != nil {
		return "", nil, err
	}

	if len(heads) > 1 {
		log.Warnf("More than 1 header received when expecting only one")
	}

	return heads[0].addr, heads[0].header, nil
}

func GetCommonAPI(ctx *cli.Context) (api.Common, jsonrpc.ClientCloser, error) {
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
	// use the mocked API in CLI unit tests, see cli/mocks_test.go for mock definition
	if mock, ok := ctx.App.Metadata["test-full-api"]; ok {
		return &v0api.WrapperV1Full{FullNode: mock.(v1api.FullNode)}, func() {}, nil
	}

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

type contextKey string

// OnSingleNode returns a modified context that, when passed to a method on a FullNodeProxy, will
// cause all calls to be directed at the same node when possible.
//
// Think "sticky sessions".
func OnSingleNode(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKey("retry-node"), new(atomic.Int32))
}

// makeNodeProxy creates a proxy that load balances across multiple nodes with automatic retry and failover
func makeNodeProxy[T any](ins []T, outs []interface{}) {
	if len(ins) == 0 {
		log.Error("makeNodeProxy called with empty node list")
		return
	}

	var rins []reflect.Value
	for _, in := range ins {
		rins = append(rins, reflect.ValueOf(in))
	}

	for _, out := range outs {
		rProxyInternal := reflect.ValueOf(out).Elem()

		for f := 0; f < rProxyInternal.NumField(); f++ {
			field := rProxyInternal.Type().Field(f)

			var fns []reflect.Value
			for _, rin := range rins {
				fns = append(fns, rin.MethodByName(field.Name))
			}

			rProxyInternal.Field(f).Set(reflect.MakeFunc(field.Type, func(args []reflect.Value) (results []reflect.Value) {
				errorsToRetry := []error{&jsonrpc.RPCConnectionError{}, &jsonrpc.ErrClient{}}
				initialBackoff, err := time.ParseDuration("1s")
				if err != nil {
					return nil
				}

				ctx := args[0].Interface().(context.Context)

				// for calls that need to be performed on the same node
				// primarily for miner when calling create block and submit block subsequently
				var curr *atomic.Int32
				if v, ok := ctx.Value(contextKey("retry-node")).(*atomic.Int32); ok {
					curr = v
				} else {
					curr = new(atomic.Int32)
				}

				total := int32(len(rins))
				result, rerr := retry.Retry(ctx, 5, initialBackoff, errorsToRetry, func() ([]reflect.Value, error) {
					idx := curr.Load()
					result := fns[idx].Call(args)
					if result[len(result)-1].IsNil() {
						return result, nil
					}
					// On failure, switch to the next node.
					//
					// We CAS instead of incrementing because this might have
					// already been incremented by a concurrent call if we have
					// a shared `curr` (we're sticky to a single node).
					curr.CompareAndSwap(idx, (idx+1)%total)

					e := result[len(result)-1].Interface().(error)
					return result, e
				})

				// If retry failed and we don't have a valid result, construct an error response
				if rerr != nil && len(result) != field.Type.NumOut() {
					log.Errorf("retry failed for %s, err: %s", field.Name, rerr)
					// Construct a response with zero values and the error
					result = make([]reflect.Value, field.Type.NumOut())
					for i := 0; i < field.Type.NumOut()-1; i++ {
						result[i] = reflect.Zero(field.Type.Out(i))
					}
					// Set the last element to the error
					result[field.Type.NumOut()-1] = reflect.ValueOf(rerr)
				}

				return result
			}))
		}
	}
}

func FullNodeProxy[T api.FullNode](ins []T, outstr *api.FullNodeStruct) {
	outs := api.GetInternalStructs(outstr)
	makeNodeProxy(ins, outs)
}

func FullNodeProxyV2[T v2api.FullNode](ins []T, outstr *v2api.FullNodeStruct) {
	outs := api.GetInternalStructs(outstr)
	makeNodeProxy(ins, outs)
}

func GetFullNodeAPIV1Single(ctx *cli.Context) (v1api.FullNode, jsonrpc.ClientCloser, error) {
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

type GetFullNodeOptions struct {
	EthSubHandler api.EthSubscriber
}

type GetFullNodeOption func(*GetFullNodeOptions)

func FullNodeWithEthSubscriptionHandler(sh api.EthSubscriber) GetFullNodeOption {
	return func(opts *GetFullNodeOptions) {
		opts.EthSubHandler = sh
	}
}

func GetFullNodeAPIV1(ctx *cli.Context, opts ...GetFullNodeOption) (v1api.FullNode, jsonrpc.ClientCloser, error) {
	if tn, ok := ctx.App.Metadata["testnode-full"]; ok {
		return tn.(v1api.FullNode), func() {}, nil
	}

	var options GetFullNodeOptions
	for _, opt := range opts {
		opt(&options)
	}

	var rpcOpts []jsonrpc.Option
	if options.EthSubHandler != nil {
		rpcOpts = append(rpcOpts, jsonrpc.WithClientHandler("Filecoin", options.EthSubHandler), jsonrpc.WithClientHandlerAlias("eth_subscription", "Filecoin.EthSubscription"))
	}

	heads, err := GetRawAPIMulti(ctx, repo.FullNode, "v1")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using full node API v1 endpoint:", heads[0].addr)
	}

	var fullNodes []api.FullNode
	var closers []jsonrpc.ClientCloser

	for _, head := range heads {
		v1api, closer, err := client.NewFullNodeRPCV1(ctx.Context, head.addr, head.header, rpcOpts...)
		if err != nil {
			log.Warnf("Not able to establish connection to node with addr: %s", head.addr)
			continue
		}
		fullNodes = append(fullNodes, v1api)
		closers = append(closers, closer)
	}

	// When running in cluster mode and trying to establish connections to multiple nodes, fail
	// if less than 2 lotus nodes are actually running
	if len(heads) > 1 && len(fullNodes) < 2 {
		return nil, nil, xerrors.Errorf("Not able to establish connection to more than a single node")
	}

	finalCloser := func() {
		for _, c := range closers {
			c()
		}
	}

	var v1API api.FullNodeStruct
	FullNodeProxy(fullNodes, &v1API)

	v, err := v1API.Version(ctx.Context)
	if err != nil {
		return nil, nil, err
	}
	if !v.APIVersion.EqMajorMinor(api.FullAPIVersion1) {
		return nil, nil, xerrors.Errorf("Remote API version didn't match (expected %s, remote %s)", api.FullAPIVersion1, v.APIVersion)
	}
	return &v1API, finalCloser, nil
}

func GetFullNodeAPIV2(ctx *cli.Context, opts ...GetFullNodeOption) (v2api.FullNode, jsonrpc.ClientCloser, error) {
	if tn, ok := ctx.App.Metadata["testnode-full"]; ok {
		return tn.(v2api.FullNode), func() {}, nil
	}

	var options GetFullNodeOptions
	for _, opt := range opts {
		opt(&options)
	}

	var rpcOpts []jsonrpc.Option
	if options.EthSubHandler != nil {
		rpcOpts = append(rpcOpts, jsonrpc.WithClientHandler("Filecoin", options.EthSubHandler), jsonrpc.WithClientHandlerAlias("eth_subscription", "Filecoin.EthSubscription"))
	}

	heads, err := GetRawAPIMulti(ctx, repo.FullNode, "v2")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using full node API v2 endpoint:", heads[0].addr)
	}

	var fullNodes []v2api.FullNode
	var closers []jsonrpc.ClientCloser

	for _, head := range heads {
		v2, closer, err := client.NewFullNodeRPCV2(ctx.Context, head.addr, head.header, rpcOpts...)
		if err != nil {
			log.Warnf("Not able to establish connection to node with addr: %s", head.addr)
			continue
		}
		fullNodes = append(fullNodes, v2)
		closers = append(closers, closer)
	}

	// When running in cluster mode and trying to establish connections to multiple nodes, fail
	// if less than 2 lotus nodes are actually running
	if len(heads) > 1 && len(fullNodes) < 2 {
		return nil, nil, xerrors.Errorf("Not able to establish connection to more than a single node")
	}

	finalCloser := func() {
		for _, c := range closers {
			c()
		}
	}

	var v2API v2api.FullNodeStruct
	FullNodeProxyV2(fullNodes, &v2API)

	// TODO check major version of v2 API similar to v1 once there is a Version API in v2
	return &v2API, finalCloser, nil
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

func GetGatewayAPIV1(ctx *cli.Context) (api.Gateway, jsonrpc.ClientCloser, error) {
	addr, headers, err := GetRawAPI(ctx, repo.FullNode, "v1")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using gateway API v1 endpoint:", addr)
	}

	return client.NewGatewayRPCV1(ctx.Context, addr, headers)
}

func GetGatewayAPIV2(ctx *cli.Context) (v2api.Gateway, jsonrpc.ClientCloser, error) {
	addr, headers, err := GetRawAPI(ctx, repo.FullNode, "v2")
	if err != nil {
		return nil, nil, err
	}

	if IsVeryVerbose {
		_, _ = fmt.Fprintln(ctx.App.Writer, "using gateway API v2 endpoint:", addr)
	}

	return client.NewGatewayRPCV2(ctx.Context, addr, headers)
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
