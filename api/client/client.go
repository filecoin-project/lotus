package client

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/lib/rpcenc"
)

// NewCommonRPC creates a new http jsonrpc client.
func NewCommonRPC(ctx context.Context, addr string, requestHeader http.Header) (api.Common, jsonrpc.ClientCloser, error) {
	var res v0api.CommonStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		[]interface{}{
			&res.Internal,
		},
		requestHeader,
	)

	return &res, closer, err
}

// NewFullNodeRPC creates a new http jsonrpc client.
func NewFullNodeRPC(ctx context.Context, addr string, requestHeader http.Header) (api.FullNode, jsonrpc.ClientCloser, error) {
	var res v1api.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		[]interface{}{
			&res.CommonStruct.Internal,
			&res.Internal,
		}, requestHeader)

	return &res, closer, err
}

// NewStorageMinerRPC creates a new http jsonrpc client for miner
func NewStorageMinerRPC(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (v0api.StorageMiner, jsonrpc.ClientCloser, error) {
	var res v0api.StorageMinerStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		[]interface{}{
			&res.CommonStruct.Internal,
			&res.Internal,
		},
		requestHeader,
		opts...,
	)

	return &res, closer, err
}

func NewWorkerRPC(ctx context.Context, addr string, requestHeader http.Header) (api.Worker, jsonrpc.ClientCloser, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, nil, err
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	}
	///rpc/v0 -> /rpc/streams/v0/push

	u.Path = path.Join(u.Path, "../streams/v0/push")

	var res apistruct.WorkerStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		[]interface{}{
			&res.Internal,
		},
		requestHeader,
		rpcenc.ReaderParamEncoder(u.String()),
		jsonrpc.WithNoReconnect(),
		jsonrpc.WithTimeout(30*time.Second),
	)

	return &res, closer, err
}

// NewGatewayRPC creates a new http jsonrpc client for a gateway node.
func NewGatewayRPC(ctx context.Context, addr string, requestHeader http.Header, opts ...jsonrpc.Option) (api.Gateway, jsonrpc.ClientCloser, error) {
	var res apistruct.GatewayStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		[]interface{}{
			&res.Internal,
		},
		requestHeader,
		opts...,
	)

	return &res, closer, err
}

func NewWalletRPC(ctx context.Context, addr string, requestHeader http.Header) (api.Wallet, jsonrpc.ClientCloser, error) {
	var res apistruct.WalletStruct
	closer, err := jsonrpc.NewMergeClient(ctx, addr, "Filecoin",
		[]interface{}{
			&res.Internal,
		},
		requestHeader,
	)

	return &res, closer, err
}
