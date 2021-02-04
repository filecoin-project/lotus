//go:generate go run ./gen

package dagspliter

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ipld/go-car"
	"io/ioutil"
	"os"
	"path/filepath"

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

// FIXME: Rethink the name. What is the difference between an edge and a link
//  besides belonging to different abstraction layers. I'd rather have a
//  BoxLink than an Edge.
type Edge struct {
	Box WeakCID
	// FIXME: Rename to singular.
	Links WeakCID // always links to Box-es
}

// Box contains part of a DAG, boxing of some edges
// Wrapper around nodes to group into a fixed size. Alternative to actual
// re-chunking.
type Box struct {
	// FIXME: Rename to ROOTS. We only care about top of (sub-)graphs, which
	//  is what we need to generate CAR files.
	Nodes []cid.Cid

	// References to external subgraphs
	// if no Nodes nor Internal above, root is 0-th; can be empty
	// FIXME: Maybe replace by array of boxes CID.
	//  From magik: "Or we may not need that at all if we write the boxed/raw
	//  nodes back into the CAR files with the correct codec"
	External []*Edge
}

func (box *Box) isExternal(link *ipld.Link) bool {
	// Boxes we're working on are likely to be in L2/L3 cache, and comparing bytes
	// is really fast, so it may not even make sense to optimize this, at least
	// unless it shows up in traces.
	for _, edge := range box.External {
		if bytes.Equal(edge.Links, link.Cid.Bytes()) {
			return true
		}
	}
	return false
}

type edgeTemplate struct {
	// FIXME: Not used at the moment but may be useful when unpacking many CAR
	//  files together (we will now in which box/CAR file we can find this node).
	//  Retaining it in the meanwhile.
	box BoxID

	links WeakCID
}

type boxTemplate struct {
	roots    []cid.Cid
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

	boxes []*boxTemplate // box 0 = root
}

func getSingleNodeSize(node ipld.Node) uint64 {
	// FIXME: How to check the size of the parent node without taking into
	//  account the children? The Node interface doesn't seem to account for
	//  that so we are going directly to the Block interface for now.
	//  We can probably get away with not accounting non-file data well, and
	//  just have some % overhead when accounting space (obviously that will
	//  break horribly with small files, but it should be good enough in the
	//  average case).
	return uint64(len(node.RawData()))
}

func (b *builder) getTreeSize(nd ipld.Node) (uint64, error) {
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
const kib = 1 << 10

// Get current box we are packing into. By definition now this is always the
// last created box.
func (b *builder) boxID() BoxID {
	return BoxID(len(b.boxes) - 1)
}

// Get current box we are packing into.
// FIXME: Make sure from the construction of the builder that there is always one.
func (b *builder) box() *boxTemplate {
	return b.boxes[b.boxID()]
}

func (b *builder) newBox() {
	b.boxes = append(b.boxes, new(boxTemplate))
}

// Remaining size in the current box.
func (b *builder) boxRemainingSize() uint64 {
	// FIXME: Assert this is always `0 <= ret <= max_size`.
	return b.chunk - b.used()
}

func (b *builder) used() uint64 {
	return b.box().used
}

func (b *builder) emptyBox() bool {
	// FIXME: Assert this is always `0 <= ret <= max_size`.
	return b.used() == 0
}

// Check this size fits in the current box.
func (b *builder) fits(size uint64) bool {
	return size <= b.boxRemainingSize()
}

func (b *builder) addSize(size uint64) {
	// FIXME: Maybe assert size (`fits`).
	b.box().used += size
}

func (b *builder) packRoot(c cid.Cid) {
	b.box().roots = append(b.box().roots, c)
}

func (b *builder) addExternalLink(node ipld.Node) {
	b.box().external = append(b.box().external, edgeTemplate{
		box:   b.boxID(),
		links: node.Cid().Bytes(),
	})
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

func (b *builder) add(ctx context.Context, initialRoot ipld.Node) error {
	// LIFO queue with the roots that need to be scanned and boxed.
	// LIFO(-ish, node links pushed in reverse) should result in slightly better
	// data layout (less fragmentation in leaves) than FIFO.
	rootsToPack := []ipld.Node{initialRoot}

	for len(rootsToPack) > 0 {
		// Pick one root node from the queue.
		root := rootsToPack[len(rootsToPack)-1]
		rootsToPack = rootsToPack[:len(rootsToPack)-1]
		b.packRoot(root.Cid())

		prevNumberOfRoots := len(rootsToPack)
		err := mdag.Walk(ctx,
			// FIXME: Check if this is the standard way of fetching links.
			func(ctx context.Context, c cid.Cid) ([]*ipld.Link, error) {
				return ipld.GetLinks(ctx, b.serv, c)
			},
			root.Cid(),
			// FIXME: The `Visit` function can't return errors, which seems odd
			//  given it should be the function that does the core of the walking
			//  logic (besides signaling if we want to continue with the walk or
			//  not). For now everything is a panic here.
			// FIXME: Check for repeated nodes? How do they count in the CAR file?
			func(nodeCid cid.Cid) bool {
				node, err := b.serv.Get(ctx, nodeCid)
				if err != nil {
					panic(fmt.Sprintf("getting head node: %w", err))
				}

				treeSize, err := b.getTreeSize(node)
				if err != nil {
					panic(fmt.Sprintf("getting tree size: %w", err))
				}

				_, _ = fmt.Fprintf(os.Stderr, "checking node %s (tree size %d) (box %d)\n",
					node.String(), treeSize, b.boxID())

				if b.fits(treeSize) {
					b.addSize(treeSize)

					_, _ = fmt.Fprintf(os.Stderr, "entire tree fits in box (cumulative %dkib)\n", b.used()/kib)

					// The entire (sub-)graph fits so no need to keep walking it.
					return false
				}

				// Too big for the current box. We need to split parent
				// and sub-graphs (from the child nodes) and inspect their
				// sizes separately.

				// First check the size of the parent node alone.
				parentSize := getSingleNodeSize(node)
				fmt.Fprintf(os.Stderr, "tree too big, single node size: %d\n",
					parentSize)

				if b.fits(parentSize) || b.emptyBox() {
					b.addSize(parentSize)
					// Even if the node doesn't fit but this is an empty box we
					// should add it nonetheless. It means it doesn't fit in *any*
					// box so at least make sure it has its own dedicated one.
					fmt.Fprintf(os.Stderr, "added node to box (cumulative %dkib)\n",
						b.used()/kib)
					// Added the parent to the box, now process its children in the
					// next `Walk()` calls.
					return true
				}

				// Doesn't fit: process this node in the next box as a root.
				rootsToPack = append(rootsToPack, node)
				b.addExternalLink(node)
				fmt.Fprintf(os.Stderr, "node too big, adding as root for another box\n")
				// No need to visit children as not even the parent fits.
				return false
			},
			// FIXME: We're probably not ready for any type of concurrency at this point.
			mdag.Concurrency(0),
		)
		if err != nil {
			return xerrors.Errorf("error walking dag: %w", err)
		}

		if len(rootsToPack) > prevNumberOfRoots {
			// We have added internal nodes as "new" roots which means we'll
			// need a new box to put them in.
			fmt.Fprintf(os.Stderr, "***CREATING NEW BOX %d*** (previous one used %d kib)\n",
				b.boxID()+1, b.used()/kib)
			b.newBox()
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

		err = bb.add(ctx, rootNd)
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
				Nodes:    bb.boxes[i].roots,
				External: edges,
			}

			boxCids[i], err = cst.Put(ctx, box)
			if err != nil {
				return xerrors.Errorf("putting box %d: %w", i, err)
			}

			fmt.Printf("%s\n", boxCids[i])
		}

		// =====================
		// CAR generation logic.
		// =====================
		// FIXME: Should be decoupled from the above (probably in its own
		//  separate command).

		CAR_OUT_DIR := "dagsplitter-car-files"
		if _, err := os.Stat(CAR_OUT_DIR); os.IsNotExist(err) {
			os.Mkdir(CAR_OUT_DIR, os.ModePerm)
		}

		for _, boxCid := range boxCids {
			box := Box{}
			if err := cst.Get(ctx, boxCid, &box); err != nil {
				// FIXME: We could just retain the created boxes but trying
				//  to make it more decoupled (we might not have them available
				//  by the time this is called).
				return xerrors.Errorf("failed retrieving box: %w", err)
			}

			//_, _ = fmt.Fprintf(os.Stderr, "Creating car with roots: %v\n", box.Nodes)

			out := new(bytes.Buffer)
			if err := car.WriteCarWithWalker(context.TODO(), dag, box.Nodes, out, BoxCarWalkFunc(&box)); err != nil {
				return xerrors.Errorf("write car failed: %w", err)
			}

			if err := ioutil.WriteFile(filepath.Join(CAR_OUT_DIR, boxCid.String()+".car"),
				out.Bytes(), 0644); err != nil {
				return xerrors.Errorf("write file failed: %w", err)
			}
		}

		return nil
	},
}

func BoxCarWalkFunc(box *Box) func(nd ipld.Node) (out []*ipld.Link, err error) {
	return func(nd ipld.Node) (out []*ipld.Link, err error) {
		for _, link := range nd.Links() {

			// Do not walk into nodes external to the current `box`.
			if box.isExternal(link) {
				//_, _ = fmt.Fprintf(os.Stderr, "Found external link, skipping from CAR generation: %s\n", link.Cid.String())
				continue
			}

			// Taken from the original `gen.CarWalkFunc`:
			//  Filecoin sector commitment CIDs (CommD (padded/truncated sha256
			//  binary tree), CommR (basically magic tree)). Those are linked
			//  directly in the chain state, so this avoids trying to accidentally
			//  walk over a few exabytes of data.
			// FIXME: Avoid duplicating this code from the original.
			pref := link.Cid.Prefix()
			if pref.Codec == cid.FilCommitmentSealed || pref.Codec == cid.FilCommitmentUnsealed {
				continue
			}

			out = append(out, link)
		}

		return out, nil
	}
}

// FIXME: Add real test. For now check that the output matches across refactors.
// ```bash
// ./lotus-shed dagsplit QmRLzQZ5efau2kJLfZRm9Guo1DxiBp3xCAVf6EuPCqKdsB 1M`
// # bafy2bzacedqfsq3jggpowtmjhsseflp6tu56gnkoyrgnobb64oxtmzf2uzrei
// # bafy2bzacecuyghj5wmna3xhkhkpon4lioaj5utcva7xqljz6vqaslze3wv7wo
// ```
