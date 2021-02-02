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

// FIXME: Replace by RawNode (or a thin wrapper of it).
type BoxedNode struct {
	Prefix Prefix  // original CID prefix
	Raw    cid.Cid // link to the node with raw multicodec
}

type Edge struct {
	Box   WeakCID
	Links WeakCID // always links to Box-es
}

// Box contains part of a DAG, boxing of some edges
// Wrapper around nodes to group into a fixed size. Alternative to actual
// re-chunking.
type Box struct {
	// Nodes with external links
	// root always 0-th; nodes don't have to depend on root; can be empty
	// FIXME: What are the >1th nodes? Children? Maybe we could decouple that.
	Nodes []cid.Cid

	// References to external subgraphs
	// if no Nodes nor Internal above, root is 0-th; can be empty
	// FIXME: Replace by array of boxes CID. Now it is a WIP until committed
	//  at the end of the command trhough boxTemplate.
	External []*Edge
}

type edgeTemplate struct {
	box   BoxID
	links WeakCID
}

type boxTemplate struct {
	nodes    []cid.Cid
	external []edgeTemplate

	// FIXME: We likely want to keep this in the final Box.
	used uint64
}

type BoxID int

// FIXME: Document, at least where do each come from. Where is the store provided?
type builder struct {
	st   cbor.IpldStore
	serv ipld.DAGService

	chunk uint64

	// FIXME: Abstract alongside Box ID (`bid`) to avoid direct access to array.
	boxes []*boxTemplate // box 0 = root
}

func (b *builder) getTotalSize(nd ipld.Node) (uint64, error) {
	switch n := nd.(type) {
	case *mdag.RawNode:
		return uint64(len(n.RawData())), nil

	case *mdag.ProtoNode:
		fsNode, err := unixfs.FSNodeFromBytes(n.Data())
		if err != nil {
			return 0, xerrors.Errorf("loading unixfs node: %w", err)
		}

		switch fsNode.Type() {
		case unixfs.TFile, unixfs.TRaw, unixfs.TDirectory, unixfs.THAMTShard:
			return n.Size()
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

// FIXME: Isn't this in go-units?
const mib = 1 << 20

// Get current box we are packing into.
// FIXME: Make sure from the construction of the builder that there is always
func (b *builder) boxID() BoxID {
	return BoxID(len(b.boxes) - 1)
}

// Get current box we are packing into.
// FIXME: Make sure from the construction of the builder that there is always
func (b *builder) box() *boxTemplate {
	return b.boxes[b.boxID()]
}

func (b *builder) newBox()  {
	b.boxes = append(b.boxes, new(boxTemplate))
}

// Remaining size in the current box.
// a box available.
func (b *builder) boxRemainingSize() uint64 {
	// FIXME: Assert this is always `0 <= ret <= max_size`.
	return b.chunk - b.box().used
}

// Check this size fits in the current box..
func (b *builder) fits(size uint64) bool {
	return size <= b.boxRemainingSize()
}

// Pack the entire DAG in this box.
// FIXME: What is a link exactly in this context? A generic node, a child?
func (b *builder) packLink(root cid.Cid, size uint64) {
	// FIXME: Maybe assert size (`fits`).
	b.box().nodes = append(b.box().nodes, root)
	b.box().used += size
}

func (b *builder) extractIntoRawNode(ctx context.Context, root cid.Cid, rootNode ipld.Node) (ipld.Node, error) {
	p := root.Prefix()

	// TODO: wasn't the ipfs blockstore supposed to operate on multihashes?
	rn, err := mdag.NewRawNodeWPrefix(rootNode.RawData(), &cid.V1Builder{
		Codec:    cid.Raw,
		MhType:   p.MhType,
		MhLength: p.MhLength,
	})
	if err != nil {
		return nil, xerrors.Errorf("adding raw boxed node: %w", err)
	}

	if err := b.serv.Add(ctx, rn); err != nil {
		return nil, xerrors.Errorf("storing raw boxed node: %w", err)
	}

	return rn, nil
}

func (b *builder) add(ctx context.Context, head cid.Cid, headNd ipld.Node, level string) error { // returns box id with head
	// FIXME: Rename to cumulative/total size. Head should track just the top node size.
	headSize, err := b.getTotalSize(headNd)
	if err != nil {
		return xerrors.Errorf("getting node total size: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stderr, level+"put %s into box %d\n", head, b.boxID())

	if b.fits(headSize) {
		_, _ = fmt.Fprintf(os.Stderr, level+"^ make internal, %dmib\n", (headSize+b.box().used)/mib)
		b.packLink(head, headSize)
		return nil
	}

	// Too big for the current box. We need to examine its links to see
	// how to partition it.

	// First the parent node.
	// FIXME: How to check the size of the parent node without taking into
	//  account the children? The Node interface doesn't seem to account for
	//  that so we are going directly to the Block interface for now.
	//  (If this is indeed the correct way reuse the raw data for the extract
	//   call next.)
	if !b.fits(uint64(len(headNd.RawData()))) {
		b.newBox()
	}
	// It may be the case that the parent node taken individually is still
	// larger than the box size but this is still the best effort possible:
	// packing a very big node in its own box separate from the rest.
	rawNode, err := b.extractIntoRawNode(ctx, head, headNd)
	// FIXME: Check if we really need to repackage the root into a raw node.
	if err != nil {
		return xerrors.Errorf("extracting raw node: %w", err)
	}
	rawNodeSize, err := rawNode.Size()
	// FIXME: This call can probably be avoided.
	if err != nil {
		return xerrors.Errorf("getting raw node size: %w", err)
	}
	b.packLink(rawNode.Cid(), rawNodeSize)

	_, _ = fmt.Fprintf(os.Stderr, level+"^ make %s boxed\n", head)

	// Now check child nodes.
	currentBox := b.box()
	// Track the current box we stored the root node before as the child nodes
	// will start to create new boxes.
	// FIXME: This will be cleaned up once we do a proper tracking of box IDs.
	for _, subnd := range headNd.Links() { // todo maybe iterate in reverse (why?)
		subNd, err := b.serv.Get(ctx, subnd.Cid)
		if err != nil {
			return xerrors.Errorf("getting subnode: %w", err)
		}
		err = b.add(ctx, subnd.Cid, subNd, level+" ")
		if err != nil {
			return xerrors.Errorf("processing subnode: %w", err)
		}
		if b.box() != currentBox {
			// The child nodes have been stored in new boxes.
			currentBox.external = append(currentBox.external, edgeTemplate{
				box:   b.boxID(),
				links: subnd.Cid.Bytes(),
			})
		}
	}

	return nil
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
			boxes: make([]*boxTemplate, 0),
		}
		bb.newBox() // FIXME: Encapsulate in a constructor.

		rootNd, err := dag.Get(ctx, root)
		if err != nil {
			return xerrors.Errorf("getting head node: %w", err)
		}

		err = bb.add(ctx, root, rootNd, "")
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

// FIXME: Add real test. For now check that the output matches across refactors.
// ```bash
// ./lotus-shed dagsplit QmRLzQZ5efau2kJLfZRm9Guo1DxiBp3xCAVf6EuPCqKdsB 1M`
// # bafy2bzacebucjp2d22m5mrdfkv2udcz3iabb4vtzodzr3jptihemcxkz2cfau
// # bafy2bzaceaisuwk563a3zcbtn2kyi7klpq3q3secedrxnxax7l54c3rksnqpm
// ```
