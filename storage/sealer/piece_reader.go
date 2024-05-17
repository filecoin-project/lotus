package sealer

import (
	"bufio"
	"context"
	"io"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ipfs/go-cid"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/metrics"
)

// For small read skips, it's faster to "burn" some bytes than to setup new sector reader.
// Assuming 1ms stream seek latency, and 1G/s stream rate, we're willing to discard up to 1 MiB.
var MaxPieceReaderBurnBytes int64 = 1 << 20 // 1M
var ReadBuf = 128 * (127 * 8)               // unpadded(128k)

var MinRandomReadSize = int64(4 << 10)

type pieceGetter func(offset, size uint64) (io.ReadCloser, error)

type pieceReader struct {
	getReader pieceGetter
	pieceCid  cid.Cid
	len       abi.UnpaddedPieceSize
	onClose   context.CancelFunc

	seqMCtx context.Context
	atMCtx  context.Context

	closed bool
	seqAt  int64 // next byte to be read by io.Reader

	// sequential reader
	seqMu sync.Mutex
	r     io.ReadCloser
	br    *bufio.Reader
	rAt   int64

	// random read cache
	remReads *lru.Cache[int64, []byte] // data start offset -> data
	// todo try carrying a "bytes read sequentially so far" counter with those
	//  cacahed byte buffers, increase buffer sizes when we see that we're doing
	//  a long sequential read
}

func (p *pieceReader) init(ctx context.Context) (_ *pieceReader, err error) {
	stats.Record(ctx, metrics.DagStorePRInitCount.M(1))

	p.seqMCtx, _ = tag.New(ctx, tag.Upsert(metrics.PRReadType, "seq"))
	p.atMCtx, _ = tag.New(ctx, tag.Upsert(metrics.PRReadType, "rand"))

	p.remReads, err = lru.New[int64, []byte](100)
	if err != nil {
		return nil, err
	}

	p.rAt = 0
	p.r, err = p.getReader(uint64(p.rAt), uint64(p.len))
	if err != nil {
		return nil, err
	}
	if p.r == nil {
		return nil, nil
	}

	p.br = bufio.NewReaderSize(p.r, ReadBuf)

	return p, nil
}

func (p *pieceReader) check() error {
	if p.closed {
		return xerrors.Errorf("reader closed")
	}

	return nil
}

func (p *pieceReader) Close() error {
	p.seqMu.Lock()
	defer p.seqMu.Unlock()

	if err := p.check(); err != nil {
		return err
	}

	if p.r != nil {
		if err := p.r.Close(); err != nil {
			return err
		}
		p.r = nil
	}

	p.onClose()

	p.closed = true

	return nil
}

func (p *pieceReader) Read(b []byte) (int, error) {
	p.seqMu.Lock()
	defer p.seqMu.Unlock()

	if err := p.check(); err != nil {
		return 0, err
	}

	n, err := p.readSeqReader(b)
	p.seqAt += int64(n)
	return n, err
}

func (p *pieceReader) Seek(offset int64, whence int) (int64, error) {
	p.seqMu.Lock()
	defer p.seqMu.Unlock()

	if err := p.check(); err != nil {
		return 0, err
	}

	switch whence {
	case io.SeekStart:
		p.seqAt = offset
	case io.SeekCurrent:
		p.seqAt += offset
	case io.SeekEnd:
		p.seqAt = int64(p.len) + offset
	default:
		return 0, xerrors.Errorf("bad whence")
	}

	return p.seqAt, nil
}

func (p *pieceReader) readSeqReader(b []byte) (n int, err error) {
	off := p.seqAt

	if err := p.check(); err != nil {
		return 0, err
	}

	stats.Record(p.seqMCtx, metrics.DagStorePRBytesRequested.M(int64(len(b))))

	// 1. Get the backing reader into the correct position

	// if the backing reader is ahead of the offset we want, or more than
	//  MaxPieceReaderBurnBytes behind, reset the reader
	if p.r == nil || p.rAt > off || p.rAt+MaxPieceReaderBurnBytes < off {
		if p.r != nil {
			if err := p.r.Close(); err != nil {
				return 0, xerrors.Errorf("closing backing reader: %w", err)
			}
			p.r = nil
			p.br = nil
		}

		log.Debugw("pieceReader new stream", "piece", p.pieceCid, "at", p.rAt, "off", off-p.rAt, "n", len(b))

		if off > p.rAt {
			stats.Record(p.seqMCtx, metrics.DagStorePRSeekForwardBytes.M(off-p.rAt), metrics.DagStorePRSeekForwardCount.M(1))
		} else {
			stats.Record(p.seqMCtx, metrics.DagStorePRSeekBackBytes.M(p.rAt-off), metrics.DagStorePRSeekBackCount.M(1))
		}

		p.rAt = off
		p.r, err = p.getReader(uint64(p.rAt), uint64(p.len))
		p.br = bufio.NewReaderSize(p.r, ReadBuf)
		if err != nil {
			return 0, xerrors.Errorf("getting backing reader: %w", err)
		}
	}

	// 2. Check if we need to burn some bytes
	if off > p.rAt {
		stats.Record(p.seqMCtx, metrics.DagStorePRBytesDiscarded.M(off-p.rAt), metrics.DagStorePRDiscardCount.M(1))

		n, err := io.CopyN(io.Discard, p.br, off-p.rAt)
		p.rAt += n
		if err != nil {
			return 0, xerrors.Errorf("discarding read gap: %w", err)
		}
	}

	// 3. Sanity check
	if off != p.rAt {
		return 0, xerrors.Errorf("bad reader offset; requested %d; at %d", off, p.rAt)
	}

	// 4. Read!
	n, err = io.ReadFull(p.br, b)
	if n < len(b) {
		log.Debugw("pieceReader short read", "piece", p.pieceCid, "at", p.rAt, "toEnd", int64(p.len)-p.rAt, "n", len(b), "read", n, "err", err)
	}
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}

	p.rAt += int64(n)
	return n, err
}

func (p *pieceReader) ReadAt(b []byte, off int64) (n int, err error) {
	stats.Record(p.atMCtx, metrics.DagStorePRBytesRequested.M(int64(len(b))))

	var filled int64

	// try to get a buf from lru
	data, ok := p.remReads.Get(off)
	if ok {
		n = copy(b, data)
		filled += int64(n)

		if n < len(data) {
			p.remReads.Add(off+int64(n), data[n:])

			// keep the header buffered
			if off != 0 {
				p.remReads.Remove(off)
			}
		}

		stats.Record(p.atMCtx, metrics.DagStorePRAtHitBytes.M(int64(n)), metrics.DagStorePRAtHitCount.M(1))
		// dagstore/pr_at_hit_bytes, dagstore/pr_at_hit_count
	}
	if filled == int64(len(b)) {
		// dagstore/pr_at_cache_fill_count
		stats.Record(p.atMCtx, metrics.DagStorePRAtCacheFillCount.M(1))
		return n, nil
	}

	readOff := off + filled
	readSize := int64(len(b)) - filled

	smallRead := readSize < MinRandomReadSize

	if smallRead {
		// read into small read buf
		readBuf := make([]byte, MinRandomReadSize)
		bn, err := p.readInto(readBuf, readOff)
		if err != nil && err != io.EOF {
			return int(filled), err
		}

		_ = stats.RecordWithTags(p.atMCtx, []tag.Mutator{tag.Insert(metrics.PRReadSize, "small")}, metrics.DagStorePRAtReadBytes.M(int64(bn)), metrics.DagStorePRAtReadCount.M(1))

		// reslice so that the slice is the data
		readBuf = readBuf[:bn]

		// fill user data
		used := copy(b[filled:], readBuf[:])
		filled += int64(used)
		readBuf = readBuf[used:]

		// cache the rest
		if len(readBuf) > 0 {
			p.remReads.Add(readOff+int64(used), readBuf)
		}
	} else {
		// read into user buf
		bn, err := p.readInto(b[filled:], readOff)
		if err != nil {
			return int(filled), err
		}
		filled += int64(bn)

		_ = stats.RecordWithTags(p.atMCtx, []tag.Mutator{tag.Insert(metrics.PRReadSize, "big")}, metrics.DagStorePRAtReadBytes.M(int64(bn)), metrics.DagStorePRAtReadCount.M(1))
	}

	if filled < int64(len(b)) {
		return int(filled), io.EOF
	}

	return int(filled), nil
}

func (p *pieceReader) readInto(b []byte, off int64) (n int, err error) {
	rd, err := p.getReader(uint64(off), uint64(len(b)))
	if err != nil {
		return 0, xerrors.Errorf("getting reader: %w", err)
	}

	n, err = io.ReadFull(rd, b)

	cerr := rd.Close()

	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}

	if err != nil {
		return n, err
	}

	return n, cerr
}
