package sectorblocks

import (
	"context"
	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	ipld "github.com/ipfs/go-ipld-format"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	files "github.com/ipfs/go-ipfs-files"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-lotus/storage/sector"
)

type SealSerialization uint8

const (
	SerializationUnixfs0 SealSerialization = 'u'
)

var dsPrefix = datastore.NewKey("/sealedblocks")

type SectorBlocks struct {
	*sector.Store

	unsealed *unsealedBlocks
	keys     datastore.Batching
	keyLk    sync.Mutex
}

func NewSectorBlocks(sectst *sector.Store, ds dtypes.MetadataDS, sb *sectorbuilder.SectorBuilder) *SectorBlocks {
	sbc := &SectorBlocks{
		Store: sectst,
		keys:  namespace.Wrap(ds, dsPrefix),
	}

	unsealed := &unsealedBlocks{ // TODO: untangle this
		sb:        sb,
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
	blockReader UnixfsReader
	writeRef    func(cid cid.Cid, offset uint64, size uint32) error

	pieceRef  string
	remaining []byte
}

func (st *SectorBlocks) writeRef(cid cid.Cid, offset uint64, size uint32) error {
	st.keyLk.Lock() // TODO: make this multithreaded
	defer st.keyLk.Unlock()

	v, err := st.keys.Get(dshelp.CidToDsKey(cid))
	if err == datastore.ErrNotFound {
		err = nil
	}
	if err != nil {
		return err
	}

	var refs []api.SealedRef
	if len(v) > 0 {
		if err := cbor.DecodeInto(v, &refs); err != nil {
			return err
		}
	}

	refs = append(refs, api.SealedRef{
		Piece:  string(SerializationUnixfs0) + cid.String(),
		Offset: offset,
		Size:   size,
	})

	newRef, err := cbor.DumpObject(&refs)
	if err != nil {
		return err
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
			return 0, err
		}

		if len(data) == 0 {
			panic("Handle intermediate nodes") // TODO: !
		}

		if err := r.writeRef(nd.Cid(), offset, uint32(len(data))); err != nil {
			return 0, err
		}

		read := copy(p, data)
		if read < len(data) {
			r.remaining = data[read:]
		}
		// TODO: read multiple
		return read, nil
	}
}

func (st *SectorBlocks) AddUnixfsPiece(ref cid.Cid, r UnixfsReader, keepAtLeast uint64) (sectorID uint64, err error) {
	size, err := r.Size()
	if err != nil {
		return 0, err
	}

	refst := &refStorer{blockReader: r, pieceRef: string(SerializationUnixfs0) + ref.String(), writeRef: st.writeRef}

	return st.Store.AddPiece(refst.pieceRef, uint64(size), refst)
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

		var refs []api.SealedRef
		if err := cbor.DecodeInto(ent.Value, &refs); err != nil {
			return nil, err
		}

		out[refCid] = refs
	}

	return out, nil
}

func (st *SectorBlocks) GetRefs(k cid.Cid) ([]api.SealedRef, error) { // TODO: track local sectors
	ent, err := st.keys.Get(dshelp.CidToDsKey(k))
	if err != nil {
		return nil, err
	}

	var refs []api.SealedRef
	if err := cbor.DecodeInto(ent, &refs); err != nil {
		return nil, err
	}

	return refs, nil
}

func (st *SectorBlocks) Has(k cid.Cid) (bool, error) {
	// TODO: ensure sector is still there
	return st.keys.Has(dshelp.CidToDsKey(k))
}

func (st *SectorBlocks) SealedBlockstore(approveUnseal func() error) *SectorBlockStore {
	return &SectorBlockStore{
		//local:         nil, // TODO: Pass staging
		sectorBlocks:  st,
		approveUnseal: approveUnseal,
	}
}
