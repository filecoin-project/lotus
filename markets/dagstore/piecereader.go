package dagstore

import (
	"context"
	"github.com/filecoin-project/lotus/metrics"
	"go.opencensus.io/stats"
	"io"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore/mount"
	"github.com/filecoin-project/go-state-types/abi"
)

// For small read skips, it's faster to "burn" some bytes than to setup new sector reader.
// Assuming 1ms stream seek latency, and 1G/s stream rate, we're willing to discard up to 1 MiB.
var MaxPieceReaderBurnBytes int64 = 1 << 20 // 1M

type pieceReader struct {
	ctx      context.Context
	api      MinerAPI
	pieceCid cid.Cid
	len      abi.UnpaddedPieceSize

	closed bool
	seqAt  int64 // next byte to be read by io.Reader

	r   io.ReadCloser
	rAt int64
}

func (p *pieceReader) init() (_ *pieceReader, err error) {
	stats.Record(p.ctx, metrics.DagStorePRInitCount.M(1))

	p.rAt = 0
	p.r, p.len, err = p.api.FetchUnsealedPiece(p.ctx, p.pieceCid, uint64(p.rAt))
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *pieceReader) check() error {
	if p.closed {
		return xerrors.Errorf("reader closed")
	}

	return nil
}

func (p *pieceReader) Close() error {
	if err := p.check(); err != nil {
		return err
	}

	if p.r != nil {
		if err := p.r.Close(); err != nil {
			return err
		}
		p.r = nil
	}

	return nil
}

func (p *pieceReader) Read(b []byte) (int, error) {
	if err := p.check(); err != nil {
		return 0, err
	}

	n, err := p.ReadAt(b, p.seqAt)
	p.seqAt += int64(n)
	return n, err
}

func (p *pieceReader) Seek(offset int64, whence int) (int64, error) {
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

func (p *pieceReader) ReadAt(b []byte, off int64) (n int, err error) {
	if err := p.check(); err != nil {
		return 0, err
	}

	stats.Record(p.ctx, metrics.DagStorePRBytesRequested.M(int64(len(b))))

	// 1. Get the backing reader into the correct position

	// if the backing reader is ahead of the offset we want, or more than
	//  MaxPieceReaderBurnBytes behind, reset the reader
	if p.r == nil || p.rAt > off || p.rAt+MaxPieceReaderBurnBytes < off {
		if p.r != nil {
			if err := p.r.Close(); err != nil {
				return 0, xerrors.Errorf("closing backing reader: %w", err)
			}
			p.r = nil
		}

		log.Debugw("pieceReader new stream", "piece", p.pieceCid, "at", p.rAt, "off", off-p.rAt)

		if off > p.rAt {
			stats.Record(p.ctx, metrics.DagStorePRSeekForwardBytes.M(off-p.rAt), metrics.DagStorePRSeekForwardCount.M(1))
		} else {
			stats.Record(p.ctx, metrics.DagStorePRSeekBackBytes.M(p.rAt-off), metrics.DagStorePRSeekBackCount.M(1))
		}

		p.rAt = off
		p.r, _, err = p.api.FetchUnsealedPiece(p.ctx, p.pieceCid, uint64(p.rAt))
		if err != nil {
			return 0, xerrors.Errorf("getting backing reader: %w", err)
		}
	}

	// 2. Check if we need to burn some bytes
	if off > p.rAt {
		stats.Record(p.ctx, metrics.DagStorePRBytesDiscarded.M(off-p.rAt), metrics.DagStorePRDiscardCount.M(1))

		n, err := io.CopyN(io.Discard, p.r, off-p.rAt)
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
	n, err = p.r.Read(b)
	p.rAt += int64(n)
	return n, err
}

var _ mount.Reader = (*pieceReader)(nil)
