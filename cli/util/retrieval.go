package cliutil

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
)

func ApiAddrToUrl(apiAddr string) (*url.URL, error) {
	ma, err := multiaddr.NewMultiaddr(apiAddr)
	if err == nil {
		_, addr, err := manet.DialArgs(ma)
		if err != nil {
			return nil, err
		}
		// todo: make cliutil helpers for this
		apiAddr = "http://" + addr
	}
	aa, err := url.Parse(apiAddr)
	if err != nil {
		return nil, xerrors.Errorf("parsing api address: %w", err)
	}
	switch aa.Scheme {
	case "ws":
		aa.Scheme = "http"
	case "wss":
		aa.Scheme = "https"
	}

	return aa, nil
}

func ClientExportStream(apiAddr string, apiAuth http.Header, eref api.ExportRef, car bool) (io.ReadCloser, error) {
	rj, err := json.Marshal(eref)
	if err != nil {
		return nil, xerrors.Errorf("marshaling export ref: %w", err)
	}

	aa, err := ApiAddrToUrl(apiAddr)
	if err != nil {
		return nil, err
	}

	aa.Path = path.Join(aa.Path, "rest/v0/export")
	req, err := http.NewRequest("GET", fmt.Sprintf("%s?car=%t&export=%s", aa, car, url.QueryEscape(string(rj))), nil)
	if err != nil {
		return nil, err
	}

	req.Header = apiAuth

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		em, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, xerrors.Errorf("reading error body: %w", err)
		}

		resp.Body.Close() // nolint
		return nil, xerrors.Errorf("getting root car: http %d: %s", resp.StatusCode, string(em))
	}

	return resp.Body, nil
}
