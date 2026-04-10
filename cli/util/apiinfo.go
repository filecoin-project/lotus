package cliutil

import (
	"net/http"
	"net/url"
	"strings"

	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

var log = logging.Logger("cliutil")

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

		return "ws://" + addr + "/rpc/" + version, nil
	}

	_, err = url.Parse(a.Addr)
	if err != nil {
		return "", err
	}
	return a.Addr + "/rpc/" + version, nil
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
