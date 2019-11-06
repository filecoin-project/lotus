package sectorblocks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-unixfs"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/sector"
)

type SealSerialization uint8

const (
	SerializationUnixfs0 SealSerialization = 'u'
)

var dsPrefix = datastore.NewKey("/sealedblocks")
var imBlocksPrefix = datastore.NewKey("/intermediate")

var ErrNotFound = errors.New("not found")

type SectorBlocks struct {
	*sector.Store

	intermediate blockstore.Blockstore // holds intermediate nodes TODO: consider combining with the staging blockstore

	unsealed *unsealedBlocks
	keys     datastore.Batching
	keyLk    sync.Mutex
}

func NewSectorBlocks(sectst *sector.Store, ds dtypes.MetadataDS, sb *sectorbuilder.SectorBuilder) *SectorBlocks {
	sbc := &SectorBlocks{
		Store: sectst,

		intermediate: blockstore.NewBlockstore(namespace.Wrap(ds, imBlocksPrefix)),

		keys: namespace.Wrap(ds, dsPrefix),
	}

	unsealed := &unsealedBlocks{ // TODO: untangle this
		sb: sb,

		unsealed:  map[string][]byte{},
		unsealing: map[string]chan struct{}{},
	}

	sbc.unsealed = unsealed
	return sbc
}

type UnixfsReader interface {
	files.File

	// ReadBlock reads data from a single unixfs block. Data is nil
	// for intermediate nodes
	ReadBlock(context.Context) (data []byte, offset uint64, nd ipld.Node, err error)
}

type refStorer struct {
	blockReader  UnixfsReader
	writeRef     func(cid cid.Cid, pieceRef string, offset uint64, size uint64) error
	intermediate blockstore.Blockstore

	pieceRef  string
	remaining []byte
}

func (st *SectorBlocks) writeRef(cid cid.Cid, pieceRef string, offset uint64, size uint64) error {
	st.keyLk.Lock() // TODO: make this multithreaded
	defer st.keyLk.Unlock()

	v, err := st.keys.Get(dshelp.CidToDsKey(cid))
	if err == datastore.ErrNotFound {
		err = nil
	}
	if err != nil {
		return xerrors.Errorf("getting existing refs: %w", err)
	}

	var refs api.SealedRefs
	if len(v) > 0 {
		if err := cborrpc.ReadCborRPC(bytes.NewReader(v), &refs); err != nil {
			return xerrors.Errorf("decoding existing refs: %w", err)
		}
	}

	refs.Refs = append(refs.Refs, api.SealedRef{
		Piece:  pieceRef,
		Offset: offset,
		Size:   size,
	})

	newRef, err := cborrpc.Dump(&refs)
	if err != nil {
		return xerrors.Errorf("serializing refs: %w", err)
	}
	return st.keys.Put(dshelp.CidToDsKey(cid), newRef) // TODO: batch somehow
}

func (r *refStorer) Read(p []byte) (n int, err error) {
	offset := 0
	if len(r.remaining) > 0 {
		offset += len(r.remaining)
		read := copy(p, r.remaining)
		if read == len(r.remaining) {
			r.remaining = nil
		} else {
			r.remaining = r.remaining[read:]
		}
		return read, nil
	}

	for {
		data, offset, nd, err := r.blockReader.ReadBlock(context.TODO())
		if err != nil {
			if err == io.EOF {
				return 0, io.EOF
			}
			return 0, xerrors.Errorf("reading block: %w", err)
		}

		if len(data) == 0 {
			// TODO: batch
			// TODO: GC
			if err := r.intermediate.Put(nd); err != nil {
				return 0, xerrors.Errorf("storing intermediate node: %w", err)
			}
			continue
		}

		if err := r.writeRef(nd.Cid(), r.pieceRef, offset, uint64(len(data))); err != nil {
			return 0, xerrors.Errorf("writing ref: %w", err)
		}

		read := copy(p, data)
		if read < len(data) {
			r.remaining = data[read:]
		}
		// TODO: read multiple
		return read, nil
	}
}

func (st *SectorBlocks) AddUnixfsPiece(ref cid.Cid, r UnixfsReader, dealID uint64) (sectorID uint64, err error) {
	size, err := r.Size()
	if err != nil {
		return 0, err
	}

	refst := &refStorer{
		blockReader:  r,
		pieceRef:     string(SerializationUnixfs0) + ref.String(),
		writeRef:     st.writeRef,
		intermediate: st.intermediate,
	}

	return st.Store.AddPiece(refst.pieceRef, uint64(size), refst, dealID)
}

func (st *SectorBlocks) List() (map[cid.Cid][]api.SealedRef, error) {
	res, err := st.keys.Query(query.Query{})
	if err != nil {
		return nil, err
	}

	ents, err := res.Rest()
	if err != nil {
		return nil, err
	}

	out := map[cid.Cid][]api.SealedRef{}
	for _, ent := range ents {
		refCid, err := dshelp.DsKeyToCid(datastore.RawKey(ent.Key))
		if err != nil {
			return nil, err
		}

		var refs api.SealedRefs
		if err := cborrpc.ReadCborRPC(bytes.NewReader(ent.Value), &refs); err != nil {
			return nil, err
		}

		out[refCid] = refs.Refs
	}

	return out, nil
}

func (st *SectorBlocks) GetRefs(k cid.Cid) ([]api.SealedRef, error) { // TODO: track local sectors
	ent, err := st.keys.Get(dshelp.CidToDsKey(k))
	if err == datastore.ErrNotFound {
		err = ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var refs api.SealedRefs
	if err := cborrpc.ReadCborRPC(bytes.NewReader(ent), &refs); err != nil {
		return nil, err
	}

	return refs.Refs, nil
}

func (st *SectorBlocks) GetSize(k cid.Cid) (uint64, error) {
	blk, err := st.intermediate.Get(k)
	if err == blockstore.ErrNotFound {
		refs, err := st.GetRefs(k)
		if err != nil {
			return 0, err
		}

		return uint64(refs[0].Size), nil
	}
	if err != nil {
		return 0, err
	}

	nd, err := ipld.Decode(blk)
	if err != nil {
		return 0, err
	}

	fsn, err := unixfs.ExtractFSNode(nd)
	if err != nil {
		return 0, err
	}

	return fsn.FileSize(), nil
}

func (st *SectorBlocks) Has(k cid.Cid) (bool, error) {
	// TODO: ensure sector is still there
	return st.keys.Has(dshelp.CidToDsKey(k))
}

func (st *SectorBlocks) SealedBlockstore(approveUnseal func() error) *SectorBlockStore {
	return &SectorBlockStore{
		intermediate:  st.intermediate,
		sectorBlocks:  st,
		approveUnseal: approveUnseal,
	}
}
