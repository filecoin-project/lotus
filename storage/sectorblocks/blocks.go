package sectorblocks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
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

	"github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/padreader"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage"
)

type SealSerialization uint8

const (
	SerializationUnixfs0 SealSerialization = 'u'
)

var dsPrefix = datastore.NewKey("/sealedblocks")
var imBlocksPrefix = datastore.NewKey("/intermediate")

var ErrNotFound = errors.New("not found")

type SectorBlocks struct {
	*storage.Miner
	sb sectorbuilder.Interface

	intermediate blockstore.Blockstore // holds intermediate nodes TODO: consider combining with the staging blockstore

	keys  datastore.Batching
	keyLk sync.Mutex
}

func NewSectorBlocks(miner *storage.Miner, ds dtypes.MetadataDS, sb sectorbuilder.Interface) *SectorBlocks {
	sbc := &SectorBlocks{
		Miner: miner,
		sb:    sb,

		intermediate: blockstore.NewBlockstore(namespace.Wrap(ds, imBlocksPrefix)),

		keys: namespace.Wrap(ds, dsPrefix),
	}

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
	writeRef     func(cid cid.Cid, offset uint64, size uint64) error
	intermediate blockstore.Blockstore

	remaining []byte
}

func (st *SectorBlocks) writeRef(cid cid.Cid, sectorID uint64, offset uint64, size uint64) error {
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
		if err := cborutil.ReadCborRPC(bytes.NewReader(v), &refs); err != nil {
			return xerrors.Errorf("decoding existing refs: %w", err)
		}
	}

	refs.Refs = append(refs.Refs, api.SealedRef{
		SectorID: sectorID,
		Offset:   offset,
		Size:     size,
	})

	newRef, err := cborutil.Dump(&refs)
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

		if err := r.writeRef(nd.Cid(), offset, uint64(len(data))); err != nil {
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

func (st *SectorBlocks) AddUnixfsPiece(ctx context.Context, r UnixfsReader, dealID uint64) (sectorID uint64, err error) {
	size, err := r.Size()
	if err != nil {
		return 0, err
	}

	sectorID, pieceOffset, err := st.Miner.AllocatePiece(padreader.PaddedSize(uint64(size)))
	if err != nil {
		return 0, err
	}

	refst := &refStorer{
		blockReader: r,
		writeRef: func(cid cid.Cid, offset uint64, size uint64) error {
			offset += pieceOffset

			return st.writeRef(cid, sectorID, offset, size)
		},
		intermediate: st.intermediate,
	}

	pr, psize := padreader.New(refst, uint64(size))

	return sectorID, st.Miner.SealPiece(ctx, psize, pr, sectorID, dealID)
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
		if err := cborutil.ReadCborRPC(bytes.NewReader(ent.Value), &refs); err != nil {
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
	if err := cborutil.ReadCborRPC(bytes.NewReader(ent), &refs); err != nil {
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
