package graphsyncimpl

import (
	"bytes"

	logging "github.com/ipfs/go-log"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/encoding/dagcbor"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
)

var log = logging.Logger("graphsync-impl")

// nodeAsBytes serializes an ipld.Node
func nodeAsBytes(node ipld.Node) ([]byte, error) {
	var buffer bytes.Buffer
	err := dagcbor.Encoder(node, &buffer)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// nodeFromBytes deserializes an ipld.Node
func nodeFromBytes(from []byte) (ipld.Node, error) {
	reader := bytes.NewReader(from)
	return dagcbor.Decoder(ipldfree.NodeBuilder(), reader)
}
