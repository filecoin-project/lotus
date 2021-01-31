//go:generate go run ./gen

package dagspliter

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/go-units"
	"github.com/filecoin-project/lotus/lib/ipfsbstore"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	mdag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

// WeakCID is a cid which doesn't generate CBOR tag when marshalled
type WeakCID []byte

type Prefix struct {
	Version  uint64
	Codec    uint64
	MhType   uint64
	MhLength int64
}

type BoxedNode struct {
	Prefix Prefix  // original CID prefix
	Raw    cid.Cid // link to the node with raw multicodec
}

type Edge struct {
	Box   WeakCID
	Links WeakCID // always links to Box-es
}

// Box contains part of a DAG, boxing of some edges
type Box struct {
	// Nodes with external links
	// root always 0-th; nodes don't have to depend on root; can be empty
	Nodes []*BoxedNode

	// Subgraphs of boxed nodes which fit here
	// if no Nodes above, root is 0-th; can be empty
	Internal []cid.Cid

	// References to external subgraphs
	// if no Nodes nor Internal above, root is 0-th; can be empty
	External []*Edge
}

type edgeTemplate struct {
	box   int
	links WeakCID
}

type boxTemplate struct {
	nodes    []*BoxedNode
	internal []cid.Cid
	external []edgeTemplate

	used uint64
}

type builder struct {
	st   cbor.IpldStore
	serv ipld.DAGService

	chunk uint64

	boxes []*boxTemplate // box 0 = root
}

func (b *builder) getSize(nd ipld.Node) (uint64, error) {
	switch n := nd.(type) {
	case *mdag.RawNode:
		return uint64(len(n.RawData())), nil

	case *mdag.ProtoNode:
		fsNode, err := unixfs.FSNodeFromBytes(n.Data())
		if err != nil {
			return 0, xerrors.Errorf("loading unixfs node: %w", err)
		}

		switch fsNode.Type() {
		case unixfs.TFile, unixfs.TRaw:
			return fsNode.FileSize(), nil
		case unixfs.TDirectory, unixfs.THAMTShard:
			var out uint64
			for _, link := range n.Links() {
				out += link.Size
			}
			return out, nil
		case unixfs.TMetadata:
			/*if len(n.Links()) == 0 {
				return nil, xerrors.New("incorrectly formatted metadata object")
			}
			child, err := n.Links()[0].GetNode(ctx, b.serv)
			if err != nil {
				return nil, err
			}

			childpb, ok := child.(*mdag.ProtoNode)
			if !ok {
				return nil, mdag.ErrNotProtobuf
			}*/

			return 0, xerrors.Errorf("metadata object support todo")
		case unixfs.TSymlink:
			return 0, xerrors.Errorf("symlink object support todo")
		default:
			return 0, unixfs.ErrUnrecognizedType
		}
	default:
		return 0, uio.ErrUnkownNodeType
	}
}

func (b *builder) getBox(data uint64) int {
	if len(b.boxes) == 0 {
		b.boxes = append(b.boxes, new(boxTemplate))
	}

	last := len(b.boxes) - 1

	// if the data can fit in one chunk, but not the current one, start a new one
	if data < b.chunk && b.boxes[last].used+data > b.chunk {
		b.boxes = append(b.boxes, new(boxTemplate))
		last++
	}

	// todo if full add

	return last
}

const mib = 1 << 20

func (b *builder) add(ctx context.Context, head cid.Cid, headNd ipld.Node, level string) (int, error) { // returns box id with head
	headSize, err := b.getSize(headNd)
	if err != nil {
		return 0, err
	}

	bid := b.getBox(headSize)
	_, _ = fmt.Fprintf(os.Stderr, level+"put %s into box %d\n", head, bid)

	if headSize+b.boxes[bid].used > b.chunk { // too big for this box, need more boxes
		p := head.Prefix()

		b.boxes[bid].nodes = append(b.boxes[bid].nodes, &BoxedNode{
			Prefix: Prefix{
				Version:  p.Version,
				Codec:    p.Codec,
				MhType:   p.MhType,
				MhLength: int64(p.MhLength),
			},
			Raw: cid.NewCidV1(cid.Raw, head.Hash()),
		})

		{
			// TODO: wasn't the ipfs blockstore supposed to operate on multihashes?
			rn, err := mdag.NewRawNodeWPrefix(headNd.RawData(), &cid.V1Builder{
				Codec:    cid.Raw,
				MhType:   p.MhType,
				MhLength: p.MhLength,
			})
			if err != nil {
				return 0, xerrors.Errorf("adding raw boxed node: %w", err)
			}

			if err := b.serv.Add(ctx, rn); err != nil {
				return 0, xerrors.Errorf("storing raw boxed node: %w", err)
			}
		}

		_, _ = fmt.Fprintf(os.Stderr, level+"^ make %s boxed\n", head)

		for l, subnd := range headNd.Links() { // todo maybe iterate in reverse
			subNd, err := b.serv.Get(ctx, subnd.Cid)
			if err != nil {
				return 0, xerrors.Errorf("getting subnode: %w", err)
			}

			subSize, err := b.getSize(subNd)
			if err != nil {
				return 0, xerrors.Errorf("getting subnode size: %w", err)
			}

			if subSize+b.boxes[bid].used > b.chunk { // sub node too big
				subBox, err := b.add(ctx, subnd.Cid, subNd, level+" ")
				if err != nil {
					return 0, xerrors.Errorf("processing subnode: %w", err)
				}

				if subBox != bid {
					_, _ = fmt.Fprintf(os.Stderr, level+"^ link %s/%s (link %d) external ref %s in box %d\n", head, subnd.Name, l, subnd.Cid, subBox)
					b.boxes[bid].external = append(b.boxes[bid].external, edgeTemplate{
						box:   subBox,
						links: subnd.Cid.Bytes(),
					})
				}
			} else { // fits here
				_, _ = fmt.Fprintf(os.Stderr, level+"^ link %s/%s (link %d) INTERNAL ref %s\n", head, subnd.Name, l, subnd.Cid)
				b.boxes[bid].used += subSize
				b.boxes[bid].internal = append(b.boxes[bid].internal, subnd.Cid)
			}
		}

	} else { // current box is big enough
		_, _ = fmt.Fprintf(os.Stderr, level+"^ make internal, %dmib\n", (headSize+b.boxes[bid].used)/mib)
		b.boxes[bid].used += headSize
		b.boxes[bid].internal = append(b.boxes[bid].internal, head)
	}

	return bid, nil
}

var Cmd = &cli.Command{
	Name:      "dagsplit",
	Usage:     "Cid command",
	ArgsUsage: "[root] [chunk size]",
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		bs, err := ipfsbstore.NewIpfsBstore(ctx, true)
		if err != nil {
			return xerrors.Errorf("getting ipfs bstore: %w", err)
		}

		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("expected 2 args, root, and chuck size")
		}

		root, err := cid.Parse(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("parsing root cid: %w", err)
		}

		chunk, err := units.RAMInBytes(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing chunk size: %w", err)
		}

		dag := mdag.NewDAGService(blockservice.New(bs, nil))

		cst := cbor.NewCborStore(bs)

		bb := builder{
			st:   cst,
			serv: dag,

			chunk: uint64(chunk),
		}

		rootNd, err := dag.Get(ctx, root)
		if err != nil {
			return xerrors.Errorf("getting head node: %w", err)
		}

		_, err = bb.add(ctx, root, rootNd, "")
		if err != nil {
			return err
		}

		boxCids := make([]cid.Cid, len(bb.boxes))
		for i := range bb.boxes {
			i = len(bb.boxes) - i - 1

			edges := make([]*Edge, len(bb.boxes[i].external))
			for j, template := range bb.boxes[i].external {
				edges[j] = &Edge{
					Box:   boxCids[template.box].Bytes(),
					Links: template.links,
				}
			}

			box := &Box{
				Nodes:    bb.boxes[i].nodes,
				Internal: bb.boxes[i].internal,
				External: edges,
			}

			boxCids[i], err = cst.Put(ctx, box)
			if err != nil {
				return xerrors.Errorf("putting box %d: %w", i, err)
			}

			fmt.Printf("%s\n", boxCids[i])
		}

		return nil
	},
}
