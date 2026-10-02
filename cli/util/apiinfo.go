package cliutil

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"golang.org/x/xerrors"
)

var log = logging.Logger("cliutil")

var rpcPathSuffix = regexp.MustCompile(`/rpc/v\d+/?$`)

type APIInfo struct {
	Addr  string
	Token []byte
}

func ParseApiInfo(s string) APIInfo {
	var tok []byte

	// Format is "TOKEN:ADDRESS". Skip the split when the address is a
	// bare multiaddr (leading "/") or a URL scheme (":" followed by "//").
	if !strings.HasPrefix(s, "/") {
		if idx := strings.Index(s, ":"); idx > 0 && !strings.HasPrefix(s[idx+1:], "//") {
			tok = []byte(s[:idx])
			s = s[idx+1:]
		}
	}

	return APIInfo{
		Addr:  s,
		Token: tok,
	}
}

func ParseApiInfoMulti(s string) []APIInfo {
	var apiInfos []APIInfo

	allAddrs := strings.SplitN(s, ",", -1)

	for _, addr := range allAddrs {
		apiInfos = append(apiInfos, ParseApiInfo(addr))
	}

	return apiInfos
}

func (a APIInfo) DialArgs(version string) (string, error) {
	ma, err := multiaddr.NewMultiaddr(a.Addr)
	if err == nil {
		_, addr, err := manet.DialArgs(ma)
		if err != nil {
			return "", err
		}

		scheme := "ws"
		multiaddr.ForEach(ma, func(c multiaddr.Component) bool {
			switch c.Protocol().Code {
			case multiaddr.P_WSS, multiaddr.P_TLS:
				scheme = "wss"
			}
			return true
		})

		return url.JoinPath(scheme+"://"+addr, "rpc", version)
	}

	u, err := url.Parse(a.Addr)
	if err != nil {
		return "", err
	}
	if rpcPath := rpcPathSuffix.FindString(u.Path); rpcPath != "" {
		return "", xerrors.Errorf("API address %q ends with %q; remove it, Lotus adds /rpc/%s itself",
			a.Addr, strings.TrimSuffix(rpcPath, "/"), version)
	}
	return url.JoinPath(a.Addr, "rpc", version)
}

func (a APIInfo) Host() (string, error) {
	ma, err := multiaddr.NewMultiaddr(a.Addr)
	if err == nil {
		_, addr, err := manet.DialArgs(ma)
		if err != nil {
			return "", err
		}

		return addr, nil
	}

	spec, err := url.Parse(a.Addr)
	if err != nil {
		return "", err
	}
	return spec.Host, nil
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
