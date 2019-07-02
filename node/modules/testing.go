package modules

import (
	"crypto/rand"
	"io"
	"io/ioutil"

	"github.com/libp2p/go-libp2p-core/peer"
	mh "github.com/multiformats/go-multihash"
)

// RandomPeerID generates random peer id
func RandomPeerID() (peer.ID, error) {
	b, err := ioutil.ReadAll(io.LimitReader(rand.Reader, 32))
	if err != nil {
		return "", err
	}
	hash, err := mh.Sum(b, mh.SHA2_256, -1)
	if err != nil {
		return "", err
	}
	return peer.ID(hash), nil
}
