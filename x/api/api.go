package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/x/conf"
	"github.com/filecoin-project/lotus/x/micro"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

func initHeader(token string) http.Header {
	header := http.Header{}
	header.Add("Content-Type", "application/json")
	if len(token) > 0 {
		header.Set("Authorization", "Bearer "+token)
	}
	return header
}

func GetEndpointUrl(ep multiaddr.Multiaddr) (string, error) {
	_, addr, err := manet.DialArgs(ep)
	if err != nil {
		return "", err
	}
	return GetAddressUrl(addr), nil
}

func GetAddressUrl(addr string) string {
	proto := "https"
	if conf.X.Proto == "http" {
		proto = "http"
	}
	return fmt.Sprintf("%v://%v/rpc/v0", proto, addr)
}

func GetMinerApi(ctx context.Context) (v0api.StorageMiner, jsonrpc.ClientCloser, error) {
	dict, err := micro.Selects("lotus.x.miner")
	if err != nil {
		return nil, nil, err
	}
	if len(dict) == 0 {
		return nil, nil, errors.New("lotus.x.miner instance not found")
	}
	for _, url := range dict {
		return client.NewStorageMinerRPCV0(ctx, url, initHeader(conf.X.Token))
	}
	return nil, nil, err
}
