// Package idxmsg is a copy of the message and codec from go-legs v0.3.0. It is
// copied here because having a dependency on go-legs v0.3.0 brings other
// incompatible dependencies.  The code here was copied from:
//
// https://github.com/filecoin-project/go-legs/tree/main/dtsync
package idxmsg

import (
	"errors"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multiaddr"
)

var ErrBadEncoding = errors.New("invalid message encoding")

type Message struct {
	Cid       cid.Cid
	Addrs     [][]byte
	ExtraData []byte `json:",omitempty"`
}

func (m *Message) SetAddrs(addrs []multiaddr.Multiaddr) {
	m.Addrs = make([][]byte, 0, len(addrs))
	for _, a := range addrs {
		m.Addrs = append(m.Addrs, a.Bytes())
	}
}

func (m *Message) GetAddrs() ([]multiaddr.Multiaddr, error) {
	addrs := make([]multiaddr.Multiaddr, 0, len(m.Addrs))
	for _, a := range m.Addrs {
		p, err := multiaddr.NewMultiaddrBytes(a)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, p)
	}
	return addrs, nil
}
