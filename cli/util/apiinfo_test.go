package cliutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseApiInfo(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantToken string
		wantAddr  string
	}{
		{
			name:      "jwt token + multiaddr",
			input:     "eyJhbG.eyJBbGxvdy.qpZpDnaPmjz:/ip4/127.0.0.1/tcp/1234",
			wantToken: "eyJhbG.eyJBbGxvdy.qpZpDnaPmjz",
			wantAddr:  "/ip4/127.0.0.1/tcp/1234",
		},
		{
			name:      "jwt token + multiaddr with /http",
			input:     "eyJhbG.eyJBbGxvdy.qpZpDnaPmjz:/ip4/127.0.0.1/tcp/1234/http",
			wantToken: "eyJhbG.eyJBbGxvdy.qpZpDnaPmjz",
			wantAddr:  "/ip4/127.0.0.1/tcp/1234/http",
		},
		{
			name:      "opaque token + dns multiaddr with /wss",
			input:     "opaque-api-key:/dns/host/tcp/443/wss",
			wantToken: "opaque-api-key",
			wantAddr:  "/dns/host/tcp/443/wss",
		},
		{
			name:      "opaque token + wss URL",
			input:     "opaque-api-key:wss://host",
			wantToken: "opaque-api-key",
			wantAddr:  "wss://host",
		},
		{
			name:      "opaque token + https URL (Glif-style)",
			input:     "GLIFAPIKEY:https://api.node.glif.io/rpc/v1",
			wantToken: "GLIFAPIKEY",
			wantAddr:  "https://api.node.glif.io/rpc/v1",
		},
		{
			name:      "token + ipv6 multiaddr",
			input:     "tok:/ip6/::1/tcp/1234",
			wantToken: "tok",
			wantAddr:  "/ip6/::1/tcp/1234",
		},
		{
			name:     "bare multiaddr",
			input:    "/ip4/127.0.0.1/tcp/1234",
			wantAddr: "/ip4/127.0.0.1/tcp/1234",
		},
		{
			name:     "bare ipv6 multiaddr with shorthand",
			input:    "/ip6/::1/tcp/1234",
			wantAddr: "/ip6/::1/tcp/1234",
		},
		{
			name:     "bare ipv6 multiaddr with full form",
			input:    "/ip6/2001:db8::1/tcp/1234",
			wantAddr: "/ip6/2001:db8::1/tcp/1234",
		},
		{
			name:     "bare wss URL",
			input:    "wss://host",
			wantAddr: "wss://host",
		},
		{
			name:     "bare ws URL with port and path",
			input:    "ws://host:1234/rpc/v1",
			wantAddr: "ws://host:1234/rpc/v1",
		},
		{
			name:     "bare https URL",
			input:    "https://api.node.glif.io/rpc/v1",
			wantAddr: "https://api.node.glif.io/rpc/v1",
		},
		{
			name:     "URL with userinfo treated as bare URL",
			input:    "http://user:pass@host:1234/path",
			wantAddr: "http://user:pass@host:1234/path",
		},
		{
			name:     "empty string",
			input:    "",
			wantAddr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseApiInfo(tt.input)
			assert.Equal(t, tt.wantToken, string(got.Token))
			assert.Equal(t, tt.wantAddr, got.Addr)
		})
	}
}

func TestAPIInfoDialArgs(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{
			name: "multiaddr with /http stays ws",
			addr: "/ip4/127.0.0.1/tcp/1234/http",
			want: "ws://127.0.0.1:1234/rpc/v1",
		},
		{
			name: "multiaddr with /ws stays ws",
			addr: "/ip4/127.0.0.1/tcp/1234/ws",
			want: "ws://127.0.0.1:1234/rpc/v1",
		},
		{
			name: "multiaddr with /wss upgrades to wss",
			addr: "/dns4/host.example/tcp/443/wss",
			want: "wss://host.example:443/rpc/v1",
		},
		{
			name: "multiaddr with /tls/ws upgrades to wss",
			addr: "/dns4/host.example/tcp/443/tls/ws",
			want: "wss://host.example:443/rpc/v1",
		},
		{
			name: "http URL passes through",
			addr: "http://host:1234",
			want: "http://host:1234/rpc/v1",
		},
		{
			name: "https URL passes through",
			addr: "https://api.node.glif.io",
			want: "https://api.node.glif.io/rpc/v1",
		},
		{
			name: "wss URL passes through",
			addr: "wss://host.example",
			want: "wss://host.example/rpc/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := APIInfo{Addr: tt.addr}.DialArgs("v1")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
