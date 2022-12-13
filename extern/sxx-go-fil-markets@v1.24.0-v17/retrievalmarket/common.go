package retrievalmarket

import (
	"bytes"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	cbg "github.com/whyrusleeping/cbor-gen"
)

// DecodeNode validates and computes a decoded ipld.Node selector from the
// provided cbor-encoded selector
func DecodeNode(defnode *cbg.Deferred) (ipld.Node, error) {
	reader := bytes.NewReader(defnode.Raw)
	nb := basicnode.Prototype.Any.NewBuilder()
	err := dagcbor.Decode(nb, reader)
	if err != nil {
		return nil, err
	}
	return nb.Build(), nil
}
