package shared_testutil

import (
	"github.com/ipfs/go-cid"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/fluent/qp"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/multiformats/go-multicodec"
)

// MakeIdentityCidWith will create either a dag-pb or dag-cbor identity CID
// containing the provided list of CIDs and optional byte blocks.
// The dag-cbor identity CID will be a simple list with all items appended.
// This can also be used recursively to increase the ridiculousness.
func MakeIdentityCidWith(cids []cid.Cid, codec multicodec.Code, padding ...[]byte) (cid.Cid, error) {
	var encoded []byte
	var node datamodel.Node
	var err error
	switch codec {
	case multicodec.DagPb:
		if len(padding) > 1 {
			panic("dag-pb only supports one bytes block")
		}
		node, err = qp.BuildMap(dagpb.Type.PBNode, 2, func(ma datamodel.MapAssembler) {
			if len(padding) > 0 {
				qp.MapEntry(ma, "Data", qp.Bytes(padding[0]))
			}
			qp.MapEntry(ma, "Links", qp.List(int64(len(cids)), func(la datamodel.ListAssembler) {
				for _, cid := range cids {
					qp.ListEntry(la, qp.Map(1, func(ma datamodel.MapAssembler) {
						qp.MapEntry(ma, "Hash", qp.Link(cidlink.Link{Cid: cid}))
					}))
				}
			}))
		})
		if err != nil {
			return cid.Undef, err
		}
		encoded, err = ipld.Encode(node, dagpb.Encode)
	case multicodec.DagCbor:
		// for dag-cbor we'll just build a list and push everything into it
		node, err = qp.BuildList(basicnode.Prototype.List, int64(len(cids)+len(padding)), func(la datamodel.ListAssembler) {
			for _, cid := range cids {
				qp.ListEntry(la, qp.Link(cidlink.Link{Cid: cid}))
			}
			for _, pad := range padding {
				qp.ListEntry(la, qp.Bytes(pad))
			}
		})
		if err != nil {
			return cid.Undef, err
		}
		encoded, err = ipld.Encode(node, dagcbor.Encode)
	default:
		panic("unsupported codec")
	}

	if err != nil {
		return cid.Undef, err
	}

	lp := cidlink.LinkPrototype{Prefix: cid.Prefix{Version: 1, Codec: uint64(codec), MhType: 0x0}}
	return lp.BuildLink(encoded).(cidlink.Link).Cid, nil
}
