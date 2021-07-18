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
	"github.com/filecoin-project/lotus/lib/rpcenc"
)

// NewCommonRPCV0 creates a new http jsonrpc client.
func NewCommonRPCV0(ctx context.Context, addr string, requestHeader http.Header) (api.CommonNet, jsonrpc.ClientCloser, error) {
	var res v0api.CommonNetStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res), requestHeader)

	return &res, closer, err
}

// NewFullNodeRPCV0 creates a new http jsonrpc client.
func NewFullNodeRPCV0(ctx context.Context, addr string, requestHeader http.Header) (v0api.FullNode, jsonrpc.ClientCloser, error) {
	var res v0api.FullNodeStruct

	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res), requestHeader)

	return &res, closer, err
}

// NewFullNodeRPCV1 creates a new http jsonrpc client.
func NewFullNodeRPCV1(ctx context.Context, addr string, requestHeader http.Header) (api.FullNode, jsonrpc.ClientCloser, error) {
	var res v1api.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res), requestHeader)

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
	)

	return &res, closer, err
}

// NewGatewayRPCV1 creates a new http jsonrpc client for a gateway node.
func NewGatewayRPCV1(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (api.Gateway, jsonrpc.ClientCloser, error) {
	var res api.GatewayStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res),
		requestHeader,
		opts...,
	)

	return &res, closer, err
}

// NewGatewayRPCV0 creates a new http jsonrpc client for a gateway node.
func NewGatewayRPCV0(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (v0api.Gateway, jsonrpc.ClientCloser, error) {
	var res v0api.GatewayStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res),
		requestHeader,
		opts...,
	)

	return &res, closer, err
}

func NewWalletRPCV0(ctx context.Context, addr string, requestHeader http.Header) (api.Wallet, jsonrpc.ClientCloser, error) {
	var res api.WalletStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		api.GetInternalStructs(&res),
		requestHeader,
	)

	return &res, closer, err
}
