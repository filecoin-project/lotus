package sealedbstore

import (
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
	"github.com/ipfs/go-datastore/namespace"
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

type SealedRef struct {
	Serialization SealSerialization

	Piece  cid.Cid
	Offset uint64
	Size   uint32
}

type Sealedbstore struct {
	*sector.Store

	keys  datastore.Batching
	keyLk sync.Mutex
}

func NewSealedbstore(sectst *sector.Store, ds dtypes.MetadataDS) *Sealedbstore {
	return &Sealedbstore{
		Store: sectst,
		keys:  namespace.Wrap(ds, dsPrefix),
	}
}

type UnixfsReader interface {
	files.File

	// ReadBlock reads data from a single unixfs block. Data is nil
	// for intermediate nodes
	ReadBlock() (data []byte, offset uint64, cid cid.Cid, err error)
}

type refStorer struct {
	blockReader UnixfsReader
	writeRef    func(cid cid.Cid, offset uint64, size uint32) error

	pieceRef  string
	remaining []byte
}

func (st *Sealedbstore) writeRef(cid cid.Cid, offset uint64, size uint32) error {
	st.keyLk.Lock() // TODO: make this multithreaded
	defer st.keyLk.Unlock()

	v, err := st.keys.Get(dshelp.CidToDsKey(cid))
	if err == datastore.ErrNotFound {
		err = nil
	}
	if err != nil {
		return err
	}

	var refs []SealedRef
	if len(v) > 0 {
		if err := cbor.DecodeInto(v, &refs); err != nil {
			return err
		}
	}

	refs = append(refs, SealedRef{
		Serialization: SerializationUnixfs0,
		Piece:         cid,
		Offset:        offset,
		Size:          size,
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
		data, offset, cid, err := r.blockReader.ReadBlock()
		if err != nil {
			return 0, err
		}

		if err := r.writeRef(cid, offset, uint32(len(data))); err != nil {
			return 0, err
		}

	}
}

func (st *Sealedbstore) AddUnixfsPiece(ref cid.Cid, r UnixfsReader, keepAtLeast uint64) (sectorID uint64, err error) {
	size, err := r.Size()
	if err != nil {
		return 0, err
	}

	refst := &refStorer{blockReader: r, pieceRef: string(SerializationUnixfs0) + ref.String(), writeRef: st.writeRef}

	return st.Store.AddPiece(refst.pieceRef, uint64(size), refst)
}
