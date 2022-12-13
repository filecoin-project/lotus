package stores

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-car/util"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/index"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/multiformats/go-varint"
	"github.com/petar/GoLLRB/llrb"
	cborg "github.com/whyrusleeping/cbor/go"
	"golang.org/x/exp/mmap"
)

/*

	This file contains extracted parts of CARv2 blockstore, modified to allow
	storage of arbitrary data indexed by ID CIDs.

	This was allowed by go-car prior to v2.1.0, but newer go-car releases
	require that data matches the multihash, which means that the library can
	no longer be exploited as a KV store as is done in filestore.go.

	We duplicate the code here temporarily, as an alternative to breaking
	existing nodes, or adding an option to go-car which would break the CAR spec
	(it also contains this hack to a single repo).

	Ideally we should migrate to a real KV store, but even for that we'll still
	need this code for the migration process.

*/

// Modified vs go-car/v2
func isIdentity(cid.Cid) (digest []byte, ok bool, err error) {
	/*
		dmh, err := multihash.Decode(key.Hash())
		if err != nil {
			return nil, false, err
		}
		ok = dmh.Code == multihash.IDENTITY
		digest = dmh.Digest
		return digest, ok, nil
	*/

	// This is the hack filestore datastore needs to use CARs as a KV store
	return nil, false, err
}

// Code below was copied from go-car/v2

var (
	_ io.ReaderAt   = (*OffsetReadSeeker)(nil)
	_ io.ReadSeeker = (*OffsetReadSeeker)(nil)
)

// OffsetReadSeeker implements Read, and ReadAt on a section
// of an underlying io.ReaderAt.
// The main difference between io.SectionReader and OffsetReadSeeker is that
// NewOffsetReadSeeker does not require the user to know the number of readable bytes.
//
// It also partially implements Seek, where the implementation panics if io.SeekEnd is passed.
// This is because, OffsetReadSeeker does not know the end of the file therefore cannot seek relative
// to it.
type OffsetReadSeeker struct {
	r    io.ReaderAt
	base int64
	off  int64
}

// NewOffsetReadSeeker returns an OffsetReadSeeker that reads from r
// starting offset offset off and stops with io.EOF when r reaches its end.
// The Seek function will panic if whence io.SeekEnd is passed.
func NewOffsetReadSeeker(r io.ReaderAt, off int64) *OffsetReadSeeker {
	return &OffsetReadSeeker{r, off, off}
}

func (o *OffsetReadSeeker) Read(p []byte) (n int, err error) {
	n, err = o.r.ReadAt(p, o.off)
	o.off += int64(n)
	return
}

func (o *OffsetReadSeeker) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, io.EOF
	}
	off += o.base
	return o.r.ReadAt(p, off)
}

func (o *OffsetReadSeeker) ReadByte() (byte, error) {
	b := []byte{0}
	_, err := o.Read(b)
	return b[0], err
}

func (o *OffsetReadSeeker) Offset() int64 {
	return o.off
}

func (o *OffsetReadSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		o.off = offset + o.base
	case io.SeekCurrent:
		o.off += offset
	case io.SeekEnd:
		panic("unsupported whence: SeekEnd")
	}
	return o.Position(), nil
}

// Position returns the current position of this reader relative to the initial offset.
func (o *OffsetReadSeeker) Position() int64 {
	return o.off - o.base
}

var (
	_ io.Writer      = (*OffsetWriteSeeker)(nil)
	_ io.WriteSeeker = (*OffsetWriteSeeker)(nil)
)

type OffsetWriteSeeker struct {
	w      io.WriterAt
	base   int64
	offset int64
}

func NewOffsetWriter(w io.WriterAt, off int64) *OffsetWriteSeeker {
	return &OffsetWriteSeeker{w, off, off}
}

func (ow *OffsetWriteSeeker) Write(b []byte) (n int, err error) {
	n, err = ow.w.WriteAt(b, ow.offset)
	ow.offset += int64(n)
	return
}

func (ow *OffsetWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		ow.offset = offset + ow.base
	case io.SeekCurrent:
		ow.offset += offset
	case io.SeekEnd:
		panic("unsupported whence: SeekEnd")
	}
	return ow.Position(), nil
}

// Position returns the current position of this writer relative to the initial offset, i.e. the number of bytes written.
func (ow *OffsetWriteSeeker) Position() int64 {
	return ow.offset - ow.base
}

type BytesReader interface {
	io.Reader
	io.ByteReader
}

func ReadNode(r io.Reader, zeroLenAsEOF bool) (cid.Cid, []byte, error) {
	data, err := LdRead(r, zeroLenAsEOF)
	if err != nil {
		return cid.Cid{}, nil, err
	}

	n, c, err := cid.CidFromBytes(data)
	if err != nil {
		return cid.Cid{}, nil, err
	}

	return c, data[n:], nil
}

func LdWrite(w io.Writer, d ...[]byte) error {
	var sum uint64
	for _, s := range d {
		sum += uint64(len(s))
	}

	buf := make([]byte, 8)
	n := varint.PutUvarint(buf, sum)
	_, err := w.Write(buf[:n])
	if err != nil {
		return err
	}

	for _, s := range d {
		_, err = w.Write(s)
		if err != nil {
			return err
		}
	}

	return nil
}

func LdSize(d ...[]byte) uint64 {
	var sum uint64
	for _, s := range d {
		sum += uint64(len(s))
	}
	s := varint.UvarintSize(sum)
	return sum + uint64(s)
}

func LdRead(r io.Reader, zeroLenAsEOF bool) ([]byte, error) {
	l, err := varint.ReadUvarint(ToByteReader(r))
	if err != nil {
		// If the length of bytes read is non-zero when the error is EOF then signal an unclean EOF.
		if l > 0 && err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	} else if l == 0 && zeroLenAsEOF {
		return nil, io.EOF
	}

	buf := make([]byte, l)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	return buf, nil
}

var (
	_ io.ByteReader = (*readerPlusByte)(nil)
	_ io.ByteReader = (*readSeekerPlusByte)(nil)
	_ io.ByteReader = (*discardingReadSeekerPlusByte)(nil)
	_ io.ReadSeeker = (*discardingReadSeekerPlusByte)(nil)
	_ io.ReaderAt   = (*readSeekerAt)(nil)
)

type (
	readerPlusByte struct {
		io.Reader

		byteBuf [1]byte // escapes via io.Reader.Read; preallocate
	}

	readSeekerPlusByte struct {
		io.ReadSeeker

		byteBuf [1]byte // escapes via io.Reader.Read; preallocate
	}

	discardingReadSeekerPlusByte struct {
		io.Reader
		offset int64

		byteBuf [1]byte // escapes via io.Reader.Read; preallocate
	}

	ByteReadSeeker interface {
		io.ReadSeeker
		io.ByteReader
	}

	readSeekerAt struct {
		rs io.ReadSeeker
		mu sync.Mutex
	}
)

func ToByteReader(r io.Reader) io.ByteReader {
	if br, ok := r.(io.ByteReader); ok {
		return br
	}
	return &readerPlusByte{Reader: r}
}

func ToByteReadSeeker(r io.Reader) ByteReadSeeker {
	if brs, ok := r.(ByteReadSeeker); ok {
		return brs
	}
	if rs, ok := r.(io.ReadSeeker); ok {
		return &readSeekerPlusByte{ReadSeeker: rs}
	}
	return &discardingReadSeekerPlusByte{Reader: r}
}

func ToReaderAt(rs io.ReadSeeker) io.ReaderAt {
	if ra, ok := rs.(io.ReaderAt); ok {
		return ra
	}
	return &readSeekerAt{rs: rs}
}

func (rb *readerPlusByte) ReadByte() (byte, error) {
	_, err := io.ReadFull(rb, rb.byteBuf[:])
	return rb.byteBuf[0], err
}

func (rsb *readSeekerPlusByte) ReadByte() (byte, error) {
	_, err := io.ReadFull(rsb, rsb.byteBuf[:])
	return rsb.byteBuf[0], err
}

func (drsb *discardingReadSeekerPlusByte) ReadByte() (byte, error) {
	_, err := io.ReadFull(drsb, drsb.byteBuf[:])
	return drsb.byteBuf[0], err
}

func (drsb *discardingReadSeekerPlusByte) Read(p []byte) (read int, err error) {
	read, err = drsb.Reader.Read(p)
	drsb.offset += int64(read)
	return
}

func (drsb *discardingReadSeekerPlusByte) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		n := offset - drsb.offset
		if n < 0 {
			panic("unsupported rewind via whence: io.SeekStart")
		}
		_, err := io.CopyN(ioutil.Discard, drsb, n)
		return drsb.offset, err
	case io.SeekCurrent:
		_, err := io.CopyN(ioutil.Discard, drsb, offset)
		return drsb.offset, err
	default:
		panic("unsupported whence: io.SeekEnd")
	}
}

func (rsa *readSeekerAt) ReadAt(p []byte, off int64) (n int, err error) {
	rsa.mu.Lock()
	defer rsa.mu.Unlock()
	if _, err := rsa.rs.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return rsa.rs.Read(p)
}

func init() {
	cbor.RegisterCborType(CarHeader{})
}

type Store interface {
	Put(blocks.Block) error
}

type ReadStore interface {
	Get(cid.Cid) (blocks.Block, error)
}

type CarHeader struct {
	Roots   []cid.Cid
	Version uint64
}

type carWriter struct {
	ds format.NodeGetter
	w  io.Writer
}

func WriteCar(ctx context.Context, ds format.NodeGetter, roots []cid.Cid, w io.Writer) error {
	h := &CarHeader{
		Roots:   roots,
		Version: 1,
	}

	if err := WriteHeader(h, w); err != nil {
		return fmt.Errorf("failed to write car header: %s", err)
	}

	cw := &carWriter{ds: ds, w: w}
	seen := cid.NewSet()
	for _, r := range roots {
		if err := merkledag.Walk(ctx, cw.enumGetLinks, r, seen.Visit); err != nil {
			return err
		}
	}
	return nil
}

func ReadHeader(r io.Reader) (*CarHeader, error) {
	hb, err := LdRead(r, false)
	if err != nil {
		return nil, err
	}

	var ch CarHeader
	if err := cbor.DecodeInto(hb, &ch); err != nil {
		return nil, fmt.Errorf("invalid header: %v", err)
	}

	return &ch, nil
}

func WriteHeader(h *CarHeader, w io.Writer) error {
	hb, err := cbor.DumpObject(h)
	if err != nil {
		return err
	}

	return util.LdWrite(w, hb)
}

func HeaderSize(h *CarHeader) (uint64, error) {
	hb, err := cbor.DumpObject(h)
	if err != nil {
		return 0, err
	}

	return util.LdSize(hb), nil
}

func (cw *carWriter) enumGetLinks(ctx context.Context, c cid.Cid) ([]*format.Link, error) {
	nd, err := cw.ds.Get(ctx, c)
	if err != nil {
		return nil, err
	}

	if err := cw.writeNode(ctx, nd); err != nil {
		return nil, err
	}

	return nd.Links(), nil
}

func (cw *carWriter) writeNode(ctx context.Context, nd format.Node) error {
	return util.LdWrite(cw.w, nd.Cid().Bytes(), nd.RawData())
}

type CarReader struct {
	r            io.Reader
	Header       *CarHeader
	zeroLenAsEOF bool
}

func NewCarReaderWithZeroLengthSectionAsEOF(r io.Reader) (*CarReader, error) {
	return newCarReader(r, true)
}

func NewCarReader(r io.Reader) (*CarReader, error) {
	return newCarReader(r, false)
}

func newCarReader(r io.Reader, zeroLenAsEOF bool) (*CarReader, error) {
	ch, err := ReadHeader(r)
	if err != nil {
		return nil, err
	}

	if ch.Version != 1 {
		return nil, fmt.Errorf("invalid car version: %d", ch.Version)
	}

	if len(ch.Roots) == 0 {
		return nil, fmt.Errorf("empty car, no roots")
	}

	return &CarReader{
		r:            r,
		Header:       ch,
		zeroLenAsEOF: zeroLenAsEOF,
	}, nil
}

func (cr *CarReader) Next() (blocks.Block, error) {
	c, data, err := ReadNode(cr.r, cr.zeroLenAsEOF)
	if err != nil {
		return nil, err
	}

	hashed, err := c.Prefix().Sum(data)
	if err != nil {
		return nil, err
	}

	if !hashed.Equals(c) {
		return nil, fmt.Errorf("mismatch in content integrity, name: %s, data: %s", c, hashed)
	}

	return blocks.NewBlockWithCid(data, c)
}

type batchStore interface {
	PutMany([]blocks.Block) error
}

func LoadCar(s Store, r io.Reader) (*CarHeader, error) {
	cr, err := NewCarReader(r)
	if err != nil {
		return nil, err
	}

	if bs, ok := s.(batchStore); ok {
		return loadCarFast(bs, cr)
	}

	return loadCarSlow(s, cr)
}

func loadCarFast(s batchStore, cr *CarReader) (*CarHeader, error) {
	var buf []blocks.Block
	for {
		blk, err := cr.Next()
		if err != nil {
			if err == io.EOF {
				if len(buf) > 0 {
					if err := s.PutMany(buf); err != nil {
						return nil, err
					}
				}
				return cr.Header, nil
			}
			return nil, err
		}

		buf = append(buf, blk)

		if len(buf) > 1000 {
			if err := s.PutMany(buf); err != nil {
				return nil, err
			}
			buf = buf[:0]
		}
	}
}

func loadCarSlow(s Store, cr *CarReader) (*CarHeader, error) {
	for {
		blk, err := cr.Next()
		if err != nil {
			if err == io.EOF {
				return cr.Header, nil
			}
			return nil, err
		}

		if err := s.Put(blk); err != nil {
			return nil, err
		}
	}
}

// Matches checks whether two headers match.
// Two headers are considered matching if:
//   1. They have the same version number, and
//   2. They contain the same root CIDs in any order.
// Note, this function explicitly ignores the order of roots.
// If order of roots matter use reflect.DeepEqual instead.
func (h CarHeader) Matches(other CarHeader) bool {
	if h.Version != other.Version {
		return false
	}
	thisLen := len(h.Roots)
	if thisLen != len(other.Roots) {
		return false
	}
	// Headers with a single root are popular.
	// Implement a fast execution path for popular cases.
	if thisLen == 1 {
		return h.Roots[0].Equals(other.Roots[0])
	}

	// Check other contains all roots.
	// TODO: should this be optimised for cases where the number of roots are large since it has O(N^2) complexity?
	for _, r := range h.Roots {
		if !other.containsRoot(r) {
			return false
		}
	}
	return true
}

func (h *CarHeader) containsRoot(root cid.Cid) bool {
	for _, r := range h.Roots {
		if r.Equals(root) {
			return true
		}
	}
	return false
}

var _ blockstore.Blockstore = (*ReadOnly)(nil)

var (
	errZeroLengthSection = fmt.Errorf("zero-length carv2 section not allowed by default; see WithZeroLengthSectionAsEOF option")
	errReadOnly          = fmt.Errorf("called write method on a read-only carv2 blockstore")
	errClosed            = fmt.Errorf("cannot use a carv2 blockstore after closing")
)

// ReadOnly provides a read-only CAR Block Store.
type ReadOnly struct {
	// mu allows ReadWrite to be safe for concurrent use.
	// It's in ReadOnly so that read operations also grab read locks,
	// given that ReadWrite embeds ReadOnly for methods like Get and Has.
	//
	// The main fields guarded by the mutex are the index and the underlying writers.
	// For simplicity, the entirety of the blockstore methods grab the mutex.
	mu sync.RWMutex

	// When true, the blockstore has been closed via Close, Discard, or
	// Finalize, and must not be used. Any further blockstore method calls
	// will return errClosed to avoid panics or broken behavior.
	closed bool

	// The backing containing the data payload in CARv1 format.
	backing io.ReaderAt
	// The CARv1 content index.
	idx index.Index

	// If we called carv2.NewReaderMmap, remember to close it too.
	carv2Closer io.Closer

	opts carv2.Options
}

type contextKey string

const asyncErrHandlerKey contextKey = "asyncErrorHandlerKey"

// UseWholeCIDs is a read option which makes a CAR blockstore identify blocks by
// whole CIDs, and not just their multihashes. The default is to use
// multihashes, which matches the current semantics of go-ipfs-blockstore v1.
//
// Enabling this option affects a number of methods, including read-only ones:
//
// • Get, Has, and HasSize will only return a block
// only if the entire CID is present in the CAR file.
//
// • AllKeysChan will return the original whole CIDs, instead of with their
// multicodec set to "raw" to just provide multihashes.
//
// • If AllowDuplicatePuts isn't set,
// Put and PutMany will deduplicate by the whole CID,
// allowing different CIDs with equal multihashes.
//
// Note that this option only affects the blockstore, and is ignored by the root
// go-car/v2 package.
func UseWholeCIDs(enable bool) carv2.Option {
	return func(o *carv2.Options) {
		o.BlockstoreUseWholeCIDs = enable
	}
}

// NewReadOnly creates a new ReadOnly blockstore from the backing with a optional index as idx.
// This function accepts both CARv1 and CARv2 backing.
// The blockstore is instantiated with the given index if it is not nil.
//
// Otherwise:
// * For a CARv1 backing an index is generated.
// * For a CARv2 backing an index is only generated if Header.HasIndex returns false.
//
// There is no need to call ReadOnly.Close on instances returned by this function.
func NewReadOnly(backing io.ReaderAt, idx index.Index, opts ...carv2.Option) (*ReadOnly, error) {
	b := &ReadOnly{
		opts: carv2.ApplyOptions(opts...),
	}

	version, err := readVersion(backing)
	if err != nil {
		return nil, err
	}
	switch version {
	case 1:
		if idx == nil {
			if idx, err = generateIndex(backing, opts...); err != nil {
				return nil, err
			}
		}
		b.backing = backing
		b.idx = idx
		return b, nil
	case 2:
		v2r, err := carv2.NewReader(backing, opts...)
		if err != nil {
			return nil, err
		}
		if idx == nil {
			if v2r.Header.HasIndex() {
				r, err := v2r.IndexReader()
				if err != nil {
					return nil, err
				}
				idx, err = index.ReadFrom(r)
				if err != nil {
					return nil, err
				}
			} else {
				r, err := v2r.DataReader()
				if err != nil {
					return nil, err
				}
				idx, err = generateIndex(r, opts...)
				if err != nil {
					return nil, err
				}
			}
		}
		drBacking, err := v2r.DataReader()
		if err != nil {
			return nil, err
		}
		b.backing = drBacking
		b.idx = idx
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported car version: %v", version)
	}
}

func readVersion(at io.ReaderAt) (uint64, error) {
	var rr io.Reader
	switch r := at.(type) {
	case io.Reader:
		rr = r
	default:
		rr = NewOffsetReadSeeker(r, 0)
	}
	return carv2.ReadVersion(rr)
}

func generateIndex(at io.ReaderAt, opts ...carv2.Option) (index.Index, error) {
	var rs io.ReadSeeker
	switch r := at.(type) {
	case io.ReadSeeker:
		rs = r
	default:
		rs = NewOffsetReadSeeker(r, 0)
	}

	// Note, we do not set any write options so that all write options fall back onto defaults.
	return carv2.GenerateIndex(rs, opts...)
}

// OpenReadOnly opens a read-only blockstore from a CAR file (either v1 or v2), generating an index if it does not exist.
// Note, the generated index if the index does not exist is ephemeral and only stored in memory.
// See car.GenerateIndex and Index.Attach for persisting index onto a CAR file.
func OpenReadOnly(path string, opts ...carv2.Option) (*ReadOnly, error) {
	f, err := mmap.Open(path)
	if err != nil {
		return nil, err
	}

	robs, err := NewReadOnly(f, nil, opts...)
	if err != nil {
		return nil, err
	}
	robs.carv2Closer = f

	return robs, nil
}

func (b *ReadOnly) readBlock(idx int64) (cid.Cid, []byte, error) {
	bcid, data, err := ReadNode(NewOffsetReadSeeker(b.backing, idx), b.opts.ZeroLengthSectionAsEOF)
	return bcid, data, err
}

// DeleteBlock is unsupported and always errors.
func (b *ReadOnly) DeleteBlock(_ context.Context, _ cid.Cid) error {
	return errReadOnly
}

// Has indicates if the store contains a block that corresponds to the given key.
// This function always returns true for any given key with multihash.IDENTITY code.
func (b *ReadOnly) Has(ctx context.Context, key cid.Cid) (bool, error) {
	// Check if the given CID has multihash.IDENTITY code
	// Note, we do this without locking, since there is no shared information to lock for in order to perform the check.
	if _, ok, err := isIdentity(key); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return false, errClosed
	}

	var fnFound bool
	var fnErr error
	err := b.idx.GetAll(key, func(offset uint64) bool {
		uar := NewOffsetReadSeeker(b.backing, int64(offset))
		var err error
		_, err = varint.ReadUvarint(uar)
		if err != nil {
			fnErr = err
			return false
		}
		_, readCid, err := cid.CidFromReader(uar)
		if err != nil {
			fnErr = err
			return false
		}
		if b.opts.BlockstoreUseWholeCIDs {
			fnFound = readCid.Equals(key)
			return !fnFound // continue looking if we haven't found it
		} else {
			fnFound = bytes.Equal(readCid.Hash(), key.Hash())
			return false
		}
	})
	if errors.Is(err, index.ErrNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return fnFound, fnErr
}

// Get gets a block corresponding to the given key.
// This API will always return true if the given key has multihash.IDENTITY code.
func (b *ReadOnly) Get(ctx context.Context, key cid.Cid) (blocks.Block, error) {
	// Check if the given CID has multihash.IDENTITY code
	// Note, we do this without locking, since there is no shared information to lock for in order to perform the check.
	if digest, ok, err := isIdentity(key); err != nil {
		return nil, err
	} else if ok {
		return blocks.NewBlockWithCid(digest, key)
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, errClosed
	}

	var fnData []byte
	var fnErr error
	err := b.idx.GetAll(key, func(offset uint64) bool {
		readCid, data, err := b.readBlock(int64(offset))
		if err != nil {
			fnErr = err
			return false
		}
		if b.opts.BlockstoreUseWholeCIDs {
			if readCid.Equals(key) {
				fnData = data
				return false
			} else {
				return true // continue looking
			}
		} else {
			if bytes.Equal(readCid.Hash(), key.Hash()) {
				fnData = data
			}
			return false
		}
	})
	if errors.Is(err, index.ErrNotFound) {
		return nil, format.ErrNotFound{Cid: key}
	} else if err != nil {
		return nil, err
	} else if fnErr != nil {
		return nil, fnErr
	}
	if fnData == nil {
		return nil, format.ErrNotFound{Cid: key}
	}
	return blocks.NewBlockWithCid(fnData, key)
}

// GetSize gets the size of an item corresponding to the given key.
func (b *ReadOnly) GetSize(ctx context.Context, key cid.Cid) (int, error) {
	// Check if the given CID has multihash.IDENTITY code
	// Note, we do this without locking, since there is no shared information to lock for in order to perform the check.
	if digest, ok, err := isIdentity(key); err != nil {
		return 0, err
	} else if ok {
		return len(digest), nil
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return 0, errClosed
	}

	fnSize := -1
	var fnErr error
	err := b.idx.GetAll(key, func(offset uint64) bool {
		rdr := NewOffsetReadSeeker(b.backing, int64(offset))
		sectionLen, err := varint.ReadUvarint(rdr)
		if err != nil {
			fnErr = err
			return false
		}
		cidLen, readCid, err := cid.CidFromReader(rdr)
		if err != nil {
			fnErr = err
			return false
		}
		if b.opts.BlockstoreUseWholeCIDs {
			if readCid.Equals(key) {
				fnSize = int(sectionLen) - cidLen
				return false
			} else {
				return true // continue looking
			}
		} else {
			if bytes.Equal(readCid.Hash(), key.Hash()) {
				fnSize = int(sectionLen) - cidLen
			}
			return false
		}
	})
	if errors.Is(err, index.ErrNotFound) {
		return -1, format.ErrNotFound{Cid: key}
	} else if err != nil {
		return -1, err
	} else if fnErr != nil {
		return -1, fnErr
	}
	if fnSize == -1 {
		return -1, format.ErrNotFound{Cid: key}
	}
	return fnSize, nil
}

// Put is not supported and always returns an error.
func (b *ReadOnly) Put(context.Context, blocks.Block) error {
	return errReadOnly
}

// PutMany is not supported and always returns an error.
func (b *ReadOnly) PutMany(context.Context, []blocks.Block) error {
	return errReadOnly
}

// WithAsyncErrorHandler returns a context with async error handling set to the given errHandler.
// Any errors that occur during asynchronous operations of AllKeysChan will be passed to the given
// handler.
func WithAsyncErrorHandler(ctx context.Context, errHandler func(error)) context.Context {
	return context.WithValue(ctx, asyncErrHandlerKey, errHandler)
}

// AllKeysChan returns the list of keys in the CAR data payload.
// If the ctx is constructed using WithAsyncErrorHandler any errors that occur during asynchronous
// retrieval of CIDs will be passed to the error handler function set in context.
// Otherwise, errors will terminate the asynchronous operation silently.
//
// See WithAsyncErrorHandler
func (b *ReadOnly) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	// We release the lock when the channel-sending goroutine stops.
	// Note that we can't use a deferred unlock here,
	// because if we return a nil error,
	// we only want to unlock once the async goroutine has stopped.
	b.mu.RLock()

	if b.closed {
		b.mu.RUnlock() // don't hold the mutex forever
		return nil, errClosed
	}

	// TODO we may use this walk for populating the index, and we need to be able to iterate keys in this way somewhere for index generation. In general though, when it's asked for all keys from a blockstore with an index, we should iterate through the index when possible rather than linear reads through the full car.
	rdr := NewOffsetReadSeeker(b.backing, 0)
	header, err := ReadHeader(rdr)
	if err != nil {
		b.mu.RUnlock() // don't hold the mutex forever
		return nil, fmt.Errorf("error reading car header: %w", err)
	}
	headerSize, err := HeaderSize(header)
	if err != nil {
		b.mu.RUnlock() // don't hold the mutex forever
		return nil, err
	}

	// TODO: document this choice of 5, or use simpler buffering like 0 or 1.
	ch := make(chan cid.Cid, 5)

	// Seek to the end of header.
	if _, err = rdr.Seek(int64(headerSize), io.SeekStart); err != nil {
		b.mu.RUnlock() // don't hold the mutex forever
		return nil, err
	}

	go func() {
		defer b.mu.RUnlock()
		defer close(ch)

		for {
			length, err := varint.ReadUvarint(rdr)
			if err != nil {
				if err != io.EOF {
					maybeReportError(ctx, err)
				}
				return
			}

			// Null padding; by default it's an error.
			if length == 0 {
				if b.opts.ZeroLengthSectionAsEOF {
					break
				} else {
					maybeReportError(ctx, errZeroLengthSection)
					return
				}
			}

			thisItemForNxt := rdr.Offset()
			_, c, err := cid.CidFromReader(rdr)
			if err != nil {
				maybeReportError(ctx, err)
				return
			}
			if _, err := rdr.Seek(thisItemForNxt+int64(length), io.SeekStart); err != nil {
				maybeReportError(ctx, err)
				return
			}

			// If we're just using multihashes, flatten to the "raw" codec.
			if !b.opts.BlockstoreUseWholeCIDs {
				c = cid.NewCidV1(cid.Raw, c.Hash())
			}

			select {
			case ch <- c:
			case <-ctx.Done():
				maybeReportError(ctx, ctx.Err())
				return
			}
		}
	}()
	return ch, nil
}

// maybeReportError checks if an error handler is present in context associated to the key
// asyncErrHandlerKey, and if preset it will pass the error to it.
func maybeReportError(ctx context.Context, err error) {
	value := ctx.Value(asyncErrHandlerKey)
	if eh, _ := value.(func(error)); eh != nil {
		eh(err)
	}
}

// HashOnRead is currently unimplemented; hashing on reads never happens.
func (b *ReadOnly) HashOnRead(bool) {
	// TODO: implement before the final release?
}

// Roots returns the root CIDs of the backing CAR.
func (b *ReadOnly) Roots() ([]cid.Cid, error) {
	header, err := ReadHeader(NewOffsetReadSeeker(b.backing, 0))
	if err != nil {
		return nil, fmt.Errorf("error reading car header: %w", err)
	}
	return header.Roots, nil
}

// Close closes the underlying reader if it was opened by OpenReadOnly.
// After this call, the blockstore can no longer be used.
//
// Note that this call may block if any blockstore operations are currently in
// progress, including an AllKeysChan that hasn't been fully consumed or cancelled.
func (b *ReadOnly) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.closeWithoutMutex()
}

func (b *ReadOnly) closeWithoutMutex() error {
	b.closed = true
	if b.carv2Closer != nil {
		return b.carv2Closer.Close()
	}
	return nil
}

var (
	errUnsupported      = errors.New("not supported")
	insertionIndexCodec = multicodec.Code(0x300003)
)

type (
	insertionIndex struct {
		items llrb.LLRB
	}

	recordDigest struct {
		digest []byte
		index.Record
	}
)

func (r recordDigest) Less(than llrb.Item) bool {
	other, ok := than.(recordDigest)
	if !ok {
		return false
	}
	return bytes.Compare(r.digest, other.digest) < 0
}

func newRecordDigest(r index.Record) recordDigest {
	d, err := multihash.Decode(r.Hash())
	if err != nil {
		panic(err)
	}

	return recordDigest{d.Digest, r}
}

func newRecordFromCid(c cid.Cid, at uint64) recordDigest {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		panic(err)
	}

	return recordDigest{d.Digest, index.Record{Cid: c, Offset: at}}
}

func (ii *insertionIndex) insertNoReplace(key cid.Cid, n uint64) {
	ii.items.InsertNoReplace(newRecordFromCid(key, n))
}

func (ii *insertionIndex) Get(c cid.Cid) (uint64, error) {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		return 0, err
	}
	entry := recordDigest{digest: d.Digest}
	e := ii.items.Get(entry)
	if e == nil {
		return 0, index.ErrNotFound
	}
	r, ok := e.(recordDigest)
	if !ok {
		return 0, errUnsupported
	}

	return r.Record.Offset, nil
}

func (ii *insertionIndex) GetAll(c cid.Cid, fn func(uint64) bool) error {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		return err
	}
	entry := recordDigest{digest: d.Digest}

	any := false
	iter := func(i llrb.Item) bool {
		existing := i.(recordDigest)
		if !bytes.Equal(existing.digest, entry.digest) {
			// We've already looked at all entries with matching digests.
			return false
		}
		any = true
		return fn(existing.Record.Offset)
	}
	ii.items.AscendGreaterOrEqual(entry, iter)
	if !any {
		return index.ErrNotFound
	}
	return nil
}

func (ii *insertionIndex) Marshal(w io.Writer) (uint64, error) {
	l := uint64(0)
	if err := binary.Write(w, binary.LittleEndian, int64(ii.items.Len())); err != nil {
		return l, err
	}

	l += 8
	var err error
	iter := func(i llrb.Item) bool {
		if err = cborg.Encode(w, i.(recordDigest).Record); err != nil {
			return false
		}
		return true
	}
	ii.items.AscendGreaterOrEqual(ii.items.Min(), iter)
	return l, err
}

func (ii *insertionIndex) Unmarshal(r io.Reader) error {
	var length int64
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return err
	}
	d := cborg.NewDecoder(r)
	for i := int64(0); i < length; i++ {
		var rec index.Record
		if err := d.Decode(&rec); err != nil {
			return err
		}
		ii.items.InsertNoReplace(newRecordDigest(rec))
	}
	return nil
}

func (ii *insertionIndex) Codec() multicodec.Code {
	return insertionIndexCodec
}

func (ii *insertionIndex) Load(rs []index.Record) error {
	for _, r := range rs {
		rec := newRecordDigest(r)
		if rec.digest == nil {
			return fmt.Errorf("invalid entry: %v", r)
		}
		ii.items.InsertNoReplace(rec)
	}
	return nil
}

func newInsertionIndex() *insertionIndex {
	return &insertionIndex{}
}

// flatten returns a formatted index in the given codec for more efficient subsequent loading.
func (ii *insertionIndex) flatten(codec multicodec.Code) (index.Index, error) {
	si, err := index.New(codec)
	if err != nil {
		return nil, err
	}
	rcrds := make([]index.Record, ii.items.Len())

	idx := 0
	iter := func(i llrb.Item) bool {
		rcrds[idx] = i.(recordDigest).Record
		idx++
		return true
	}
	ii.items.AscendGreaterOrEqual(ii.items.Min(), iter)

	if err := si.Load(rcrds); err != nil {
		return nil, err
	}
	return si, nil
}

// note that hasExactCID is very similar to GetAll,
// but it's separate as it allows us to compare Record.Cid directly,
// whereas GetAll just provides Record.Offset.

func (ii *insertionIndex) hasExactCID(c cid.Cid) bool {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		panic(err)
	}
	entry := recordDigest{digest: d.Digest}

	found := false
	iter := func(i llrb.Item) bool {
		existing := i.(recordDigest)
		if !bytes.Equal(existing.digest, entry.digest) {
			// We've already looked at all entries with matching digests.
			return false
		}
		if existing.Record.Cid == c {
			// We found an exact match.
			found = true
			return false
		}
		// Continue looking in ascending order.
		return true
	}
	ii.items.AscendGreaterOrEqual(entry, iter)
	return found
}

var _ blockstore.Blockstore = (*ReadWrite)(nil)

// ReadWrite implements a blockstore that stores blocks in CARv2 format.
// Blocks put into the blockstore can be read back once they are successfully written.
// This implementation is preferable for a write-heavy workload.
// The blocks are written immediately on Put and PutAll calls, while the index is stored in memory
// and updated incrementally.
//
// The Finalize function must be called once the putting blocks are finished.
// Upon calling Finalize header is finalized and index is written out.
// Once finalized, all read and write calls to this blockstore will result in errors.
type ReadWrite struct {
	ronly ReadOnly

	f          *os.File
	dataWriter *OffsetWriteSeeker
	idx        *insertionIndex
	header     carv2.Header

	opts carv2.Options
}

// AllowDuplicatePuts is a write option which makes a CAR blockstore not
// deduplicate blocks in Put and PutMany. The default is to deduplicate,
// which matches the current semantics of go-ipfs-blockstore v1.
//
// Note that this option only affects the blockstore, and is ignored by the root
// go-car/v2 package.
func AllowDuplicatePuts(allow bool) carv2.Option {
	return func(o *carv2.Options) {
		o.BlockstoreAllowDuplicatePuts = allow
	}
}

// OpenReadWrite creates a new ReadWrite at the given path with a provided set of root CIDs and options.
//
// ReadWrite.Finalize must be called once putting and reading blocks are no longer needed.
// Upon calling ReadWrite.Finalize the CARv2 header and index are written out onto the file and the
// backing file is closed. Once finalized, all read and write calls to this blockstore will result
// in errors. Note, ReadWrite.Finalize must be called on an open instance regardless of whether any
// blocks were put or not.
//
// If a file at given path does not exist, the instantiation will write car.Pragma and data payload
// header (i.e. the inner CARv1 header) onto the file before returning.
//
// When the given path already exists, the blockstore will attempt to resume from it.
// On resumption the existing data sections in file are re-indexed, allowing the caller to continue
// putting any remaining blocks without having to re-ingest blocks for which previous ReadWrite.Put
// returned successfully.
//
// Resumption only works on files that were created by a previous instance of a ReadWrite
// blockstore. This means a file created as a result of a successful call to OpenReadWrite can be
// resumed from as long as write operations such as ReadWrite.Put, ReadWrite.PutMany returned
// successfully. On resumption the roots argument and WithDataPadding option must match the
// previous instantiation of ReadWrite blockstore that created the file. More explicitly, the file
// resuming from must:
//   1. start with a complete CARv2 car.Pragma.
//   2. contain a complete CARv1 data header with root CIDs matching the CIDs passed to the
//      constructor, starting at offset optionally padded by WithDataPadding, followed by zero or
//      more complete data sections. If any corrupt data sections are present the resumption will fail.
//      Note, if set previously, the blockstore must use the same WithDataPadding option as before,
//      since this option is used to locate the CARv1 data payload.
//
// Note, resumption should be used with WithCidDeduplication, so that blocks that are successfully
// written into the file are not re-written. Unless, the user explicitly wants duplicate blocks.
//
// Resuming from finalized files is allowed. However, resumption will regenerate the index
// regardless by scanning every existing block in file.
func OpenReadWrite(path string, roots []cid.Cid, opts ...carv2.Option) (*ReadWrite, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o666) // TODO: Should the user be able to configure FileMode permissions?
	if err != nil {
		return nil, fmt.Errorf("could not open read/write file: %w", err)
	}
	stat, err := f.Stat()
	if err != nil {
		// Note, we should not get a an os.ErrNotExist here because the flags used to open file includes os.O_CREATE
		return nil, err
	}
	// Try and resume by default if the file size is non-zero.
	resume := stat.Size() != 0
	// If construction of blockstore fails, make sure to close off the open file.
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	// Instantiate block store.
	// Set the header fileld before applying options since padding options may modify header.
	rwbs := &ReadWrite{
		f:      f,
		idx:    newInsertionIndex(),
		header: carv2.NewHeader(0),
		opts:   carv2.ApplyOptions(opts...),
	}
	rwbs.ronly.opts = rwbs.opts

	if p := rwbs.opts.DataPadding; p > 0 {
		rwbs.header = rwbs.header.WithDataPadding(p)
	}
	if p := rwbs.opts.IndexPadding; p > 0 {
		rwbs.header = rwbs.header.WithIndexPadding(p)
	}

	rwbs.dataWriter = NewOffsetWriter(rwbs.f, int64(rwbs.header.DataOffset))
	v1r := NewOffsetReadSeeker(rwbs.f, int64(rwbs.header.DataOffset))
	rwbs.ronly.backing = v1r
	rwbs.ronly.idx = rwbs.idx
	rwbs.ronly.carv2Closer = rwbs.f

	if resume {
		if err = rwbs.resumeWithRoots(roots); err != nil {
			return nil, err
		}
	} else {
		if err = rwbs.initWithRoots(roots); err != nil {
			return nil, err
		}
	}

	return rwbs, nil
}

func (b *ReadWrite) initWithRoots(roots []cid.Cid) error {
	if _, err := b.f.WriteAt(carv2.Pragma, 0); err != nil {
		return err
	}
	return WriteHeader(&CarHeader{Roots: roots, Version: 1}, b.dataWriter)
}

func (b *ReadWrite) resumeWithRoots(roots []cid.Cid) error {
	// On resumption it is expected that the CARv2 Pragma, and the CARv1 header is successfully written.
	// Otherwise we cannot resume from the file.
	// Read pragma to assert if b.f is indeed a CARv2.
	version, err := carv2.ReadVersion(b.f)
	if err != nil {
		// The file is not a valid CAR file and cannot resume from it.
		// Or the write must have failed before pragma was written.
		return err
	}
	if version != 2 {
		// The file is not a CARv2 and we cannot resume from it.
		return fmt.Errorf("cannot resume on CAR file with version %v", version)
	}

	// Check if file was finalized by trying to read the CARv2 header.
	// We check because if finalized the CARv1 reader behaviour needs to be adjusted since
	// EOF will not signify end of CARv1 payload. i.e. index is most likely present.
	var headerInFile carv2.Header
	_, err = headerInFile.ReadFrom(NewOffsetReadSeeker(b.f, carv2.PragmaSize))

	// If reading CARv2 header succeeded, and CARv1 offset in header is not zero then the file is
	// most-likely finalized. Check padding and truncate the file to remove index.
	// Otherwise, carry on reading the v1 payload at offset determined from b.header.
	if err == nil && headerInFile.DataOffset != 0 {
		if headerInFile.DataOffset != b.header.DataOffset {
			// Assert that the padding on file matches the given WithDataPadding option.
			wantPadding := headerInFile.DataOffset - carv2.PragmaSize - carv2.HeaderSize
			gotPadding := b.header.DataOffset - carv2.PragmaSize - carv2.HeaderSize
			return fmt.Errorf(
				"cannot resume from file with mismatched CARv1 offset; "+
					"`WithDataPadding` option must match the padding on file. "+
					"Expected padding value of %v but got %v", wantPadding, gotPadding,
			)
		} else if headerInFile.DataSize == 0 {
			// If CARv1 size is zero, since CARv1 offset wasn't, then the CARv2 header was
			// most-likely partially written. Since we write the header last in Finalize then the
			// file most-likely contains the index and we cannot know where it starts, therefore
			// can't resume.
			return errors.New("corrupt CARv2 header; cannot resume from file")
		}
	}

	// Use the given CARv1 padding to instantiate the CARv1 reader on file.
	v1r := NewOffsetReadSeeker(b.ronly.backing, 0)
	header, err := ReadHeader(v1r)
	if err != nil {
		// Cannot read the CARv1 header; the file is most likely corrupt.
		return fmt.Errorf("error reading car header: %w", err)
	}
	if !header.Matches(CarHeader{Roots: roots, Version: 1}) {
		// Cannot resume if version and root does not match.
		return errors.New("cannot resume on file with mismatching data header")
	}

	if headerInFile.DataOffset != 0 {
		// If header in file contains the size of car v1, then the index is most likely present.
		// Since we will need to re-generate the index, as the one in file is flattened, truncate
		// the file so that the Readonly.backing has the right set of bytes to deal with.
		// This effectively means resuming from a finalized file will wipe its index even if there
		// are no blocks put unless the user calls finalize.
		if err := b.f.Truncate(int64(headerInFile.DataOffset + headerInFile.DataSize)); err != nil {
			return err
		}
	}
	// Now that CARv2 header is present on file, clear it to avoid incorrect size and offset in
	// header in case blocksotre is closed without finalization and is resumed from.
	if err := b.unfinalize(); err != nil {
		return fmt.Errorf("could not un-finalize: %w", err)
	}

	// TODO See how we can reduce duplicate code here.
	// The code here comes from car.GenerateIndex.
	// Copied because we need to populate an insertindex, not a sorted index.
	// Producing a sorted index via generate, then converting it to insertindex is not possible.
	// Because Index interface does not expose internal records.
	// This may be done as part of https://github.com/ipld/go-car/issues/95

	offset, err := HeaderSize(header)
	if err != nil {
		return err
	}
	sectionOffset := int64(0)
	if sectionOffset, err = v1r.Seek(int64(offset), io.SeekStart); err != nil {
		return err
	}

	for {
		// Grab the length of the section.
		// Note that ReadUvarint wants a ByteReader.
		length, err := varint.ReadUvarint(v1r)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		// Null padding; by default it's an error.
		if length == 0 {
			if b.ronly.opts.ZeroLengthSectionAsEOF {
				break
			} else {
				return fmt.Errorf("carv1 null padding not allowed by default; see WithZeroLegthSectionAsEOF")
			}
		}

		// Grab the CID.
		n, c, err := cid.CidFromReader(v1r)
		if err != nil {
			return err
		}
		b.idx.insertNoReplace(c, uint64(sectionOffset))

		// Seek to the next section by skipping the block.
		// The section length includes the CID, so subtract it.
		if sectionOffset, err = v1r.Seek(int64(length)-int64(n), io.SeekCurrent); err != nil {
			return err
		}
	}
	// Seek to the end of last skipped block where the writer should resume writing.
	_, err = b.dataWriter.Seek(sectionOffset, io.SeekStart)
	return err
}

func (b *ReadWrite) unfinalize() error {
	_, err := new(carv2.Header).WriteTo(NewOffsetWriter(b.f, carv2.PragmaSize))
	return err
}

// Put puts a given block to the underlying datastore
func (b *ReadWrite) Put(ctx context.Context, blk blocks.Block) error {
	// PutMany already checks b.ronly.closed.
	return b.PutMany(ctx, []blocks.Block{blk})
}

// PutMany puts a slice of blocks at the same time using batching
// capabilities of the underlying datastore whenever possible.
func (b *ReadWrite) PutMany(ctx context.Context, blks []blocks.Block) error {
	b.ronly.mu.Lock()
	defer b.ronly.mu.Unlock()

	if b.ronly.closed {
		return errClosed
	}

	for _, bl := range blks {
		c := bl.Cid()

		// If StoreIdentityCIDs option is disabled then treat IDENTITY CIDs like IdStore.
		if !b.opts.StoreIdentityCIDs {
			// Check for IDENTITY CID. If IDENTITY, ignore and move to the next block.
			if _, ok, err := isIdentity(c); err != nil {
				return err
			} else if ok {
				continue
			}
		}

		// Check if its size is too big.
		// If larger than maximum allowed size, return error.
		// Note, we need to check this regardless of whether we have IDENTITY CID or not.
		// Since multhihash codes other than IDENTITY can result in large digests.
		cSize := uint64(len(c.Bytes()))
		if cSize > b.opts.MaxIndexCidSize {
			return &carv2.ErrCidTooLarge{MaxSize: b.opts.MaxIndexCidSize, CurrentSize: cSize}
		}

		if !b.opts.BlockstoreAllowDuplicatePuts {
			if b.ronly.opts.BlockstoreUseWholeCIDs && b.idx.hasExactCID(c) {
				continue // deduplicated by CID
			}
			if !b.ronly.opts.BlockstoreUseWholeCIDs {
				_, err := b.idx.Get(c)
				if err == nil {
					continue // deduplicated by hash
				}
			}
		}

		n := uint64(b.dataWriter.Position())
		if err := util.LdWrite(b.dataWriter, c.Bytes(), bl.RawData()); err != nil {
			return err
		}
		b.idx.insertNoReplace(c, n)
	}
	return nil
}

// Discard closes this blockstore without finalizing its header and index.
// After this call, the blockstore can no longer be used.
//
// Note that this call may block if any blockstore operations are currently in
// progress, including an AllKeysChan that hasn't been fully consumed or cancelled.
func (b *ReadWrite) Discard() {
	// Same semantics as ReadOnly.Close, including allowing duplicate calls.
	// The only difference is that our method is called Discard,
	// to further clarify that we're not properly finalizing and writing a
	// CARv2 file.
	b.ronly.Close()
}

// Finalize finalizes this blockstore by writing the CARv2 header, along with flattened index
// for more efficient subsequent read.
// After this call, the blockstore can no longer be used.
func (b *ReadWrite) Finalize() error {
	b.ronly.mu.Lock()
	defer b.ronly.mu.Unlock()

	if b.ronly.closed {
		// Allow duplicate Finalize calls, just like Close.
		// Still error, just like ReadOnly.Close; it should be discarded.
		return fmt.Errorf("called Finalize on a closed blockstore")
	}

	// TODO check if add index option is set and don't write the index then set index offset to zero.
	b.header = b.header.WithDataSize(uint64(b.dataWriter.Position()))
	b.header.Characteristics.SetFullyIndexed(b.opts.StoreIdentityCIDs)

	// Note that we can't use b.Close here, as that tries to grab the same
	// mutex we're holding here.
	defer b.ronly.closeWithoutMutex()

	// TODO if index not needed don't bother flattening it.
	fi, err := b.idx.flatten(b.opts.IndexCodec)
	if err != nil {
		return err
	}
	if _, err := index.WriteTo(fi, NewOffsetWriter(b.f, int64(b.header.IndexOffset))); err != nil {
		return err
	}
	if _, err := b.header.WriteTo(NewOffsetWriter(b.f, carv2.PragmaSize)); err != nil {
		return err
	}

	if err := b.ronly.closeWithoutMutex(); err != nil {
		return err
	}
	return nil
}

func (b *ReadWrite) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return b.ronly.AllKeysChan(ctx)
}

func (b *ReadWrite) Has(ctx context.Context, key cid.Cid) (bool, error) {
	return b.ronly.Has(ctx, key)
}

func (b *ReadWrite) Get(ctx context.Context, key cid.Cid) (blocks.Block, error) {
	return b.ronly.Get(ctx, key)
}

func (b *ReadWrite) GetSize(ctx context.Context, key cid.Cid) (int, error) {
	return b.ronly.GetSize(ctx, key)
}

func (b *ReadWrite) DeleteBlock(_ context.Context, _ cid.Cid) error {
	return fmt.Errorf("ReadWrite blockstore does not support deleting blocks")
}

func (b *ReadWrite) HashOnRead(enable bool) {
	b.ronly.HashOnRead(enable)
}

func (b *ReadWrite) Roots() ([]cid.Cid, error) {
	return b.ronly.Roots()
}
