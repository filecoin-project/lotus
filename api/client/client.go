package client

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/lib/rpcenc"
)

// NewCommonRPCV0 creates a new http jsonrpc client.
func NewCommonRPCV0(ctx context.Context, addr string, requestHeader http.Header) (api.Common, jsonrpc.ClientCloser, error) {
	var res v0api.CommonStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res), requestHeader, jsonrpc.WithErrors(api.RPCErrors))

	return &res, closer, err
}

// NewFullNodeRPCV0 creates a new http jsonrpc client for the /v0 API.
func NewFullNodeRPCV0(ctx context.Context, addr string, requestHeader http.Header) (v0api.FullNode, jsonrpc.ClientCloser, error) {
	var res v0api.FullNodeStruct

	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res), requestHeader, jsonrpc.WithErrors(api.RPCErrors))

	return &res, closer, err
}

// NewFullNodeRPCV1 creates a new http jsonrpc client for the /v1 API.
func NewFullNodeRPCV1(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (api.FullNode, jsonrpc.ClientCloser, error) {
	var res v1api.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res), requestHeader, append([]jsonrpc.Option{jsonrpc.WithErrors(api.RPCErrors)}, opts...)...)

	return &res, closer, err
}

// NewFullNodeRPCV2 creates a new http jsonrpc client for the /v2 API.
func NewFullNodeRPCV2(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (v2api.FullNode, jsonrpc.ClientCloser, error) {
	var res v2api.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res), requestHeader, append([]jsonrpc.Option{jsonrpc.WithErrors(api.RPCErrors)}, opts...)...)

	return &res, closer, err
}

func getPushUrl(addr string) (string, error) {
	pushUrl, err := url.Parse(addr)
	if err != nil {
		return "", err
	}
	switch pushUrl.Scheme {
	case "ws":
		pushUrl.Scheme = "http"
	case "wss":
		pushUrl.Scheme = "https"
	}
	///rpc/v0 -> /rpc/streams/v0/push

	pushUrl.Path = path.Join(pushUrl.Path, "../streams/v0/push")
	return pushUrl.String(), nil
}

// NewStorageMinerRPCV0 creates a new http jsonrpc client for miner
func NewStorageMinerRPCV0(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (v0api.StorageMiner, jsonrpc.ClientCloser, error) {
	pushUrl, err := getPushUrl(addr)
	if err != nil {
		return nil, nil, err
	}

	var res v0api.StorageMinerStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res), requestHeader,
		append([]jsonrpc.Option{
			rpcenc.ReaderParamEncoder(pushUrl),
			jsonrpc.WithErrors(api.RPCErrors),
		}, opts...)...)

	return &res, closer, err
}

func NewWorkerRPCV0(ctx context.Context, addr string, requestHeader http.Header) (v0api.Worker, jsonrpc.ClientCloser, error) {
	pushUrl, err := getPushUrl(addr)
	if err != nil {
		return nil, nil, err
	}

	var res api.WorkerStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res),
		requestHeader,
		rpcenc.ReaderParamEncoder(pushUrl),
		jsonrpc.WithNoReconnect(),
		jsonrpc.WithTimeout(30*time.Second),
		jsonrpc.WithErrors(api.RPCErrors),
	)

	return &res, closer, err
}

// NewGatewayRPCV1 creates a new http jsonrpc client for a gateway node.
func NewGatewayRPCV1(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (api.Gateway, jsonrpc.ClientCloser, error) {
	var res api.GatewayStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res),
		requestHeader,
		append(opts, jsonrpc.WithErrors(api.RPCErrors))...,
	)

	return &res, closer, err
}

// NewGatewayRPCV2 creates a new http jsonrpc client for a gateway node.
func NewGatewayRPCV2(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (v2api.Gateway, jsonrpc.ClientCloser, error) {
	var res v2api.GatewayStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res),
		requestHeader,
		append(opts, jsonrpc.WithErrors(api.RPCErrors))...,
	)

	return &res, closer, err
}

// NewGatewayRPCV0 creates a new http jsonrpc client for a gateway node.
func NewGatewayRPCV0(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (v0api.Gateway, jsonrpc.ClientCloser, error) {
	var res v0api.GatewayStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res),
		requestHeader,
		append(opts, jsonrpc.WithErrors(api.RPCErrors))...,
	)

	return &res, closer, err
}

func NewWalletRPCV0(ctx context.Context, addr string, requestHeader http.Header) (api.Wallet, jsonrpc.ClientCloser, error) {
	var res api.WalletStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res),
		requestHeader,
		jsonrpc.WithErrors(api.RPCErrors),
	)

	return &res, closer, err
}
