package exchange

import (
	"io"
	"os"
	"time"
)

// progressChunk is the most written under one deadline.
const progressChunk = 64 << 10 // 64 KiB

type deadlineWriter interface {
	io.Writer
	SetWriteDeadline(time.Time) error
}

// progressWriter is the write-side counterpart of the client's incremental
// read timeout. Writes complete only as the peer reads (beyond whatever the
// transport buffers), so it requires the peer to keep up with ReadResMinSpeed,
// the slowest rate a client accepts, on average. A reader that falls behind
// spends its time budget and is cut off; the whole write is also bounded by
// WriteResDeadline.
type progressWriter struct {
	w   deadlineWriter
	now func() time.Time
	end time.Time // no write may run past this

	// budget is time banked by writing faster than ReadResMinSpeed, capped at
	// ReadResDeadline. It starts full, giving the peer time to start reading.
	budget time.Duration
}

func newProgressWriter(w deadlineWriter, now func() time.Time) *progressWriter {
	return &progressWriter{w: w, now: now, end: now().Add(WriteResDeadline), budget: ReadResDeadline}
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	var written int
	for len(p) > 0 {
		if pw.budget <= 0 {
			return written, os.ErrDeadlineExceeded
		}
		chunk := p[:min(len(p), progressChunk)]

		// This chunk may take its own transfer time at ReadResMinSpeed plus
		// whatever budget is banked.
		start := pw.now()
		deadline := start.Add(pw.budget + transferTime(len(chunk)))
		if deadline.After(pw.end) {
			deadline = pw.end
		}
		if err := pw.w.SetWriteDeadline(deadline); err != nil {
			return written, err
		}
		n, err := pw.w.Write(chunk)
		written += n
		if err != nil {
			return written, err
		}
		if n < len(chunk) {
			// A short write without an error violates io.Writer; retrying it
			// could spin.
			return written, io.ErrShortWrite
		}

		// Settle up: credit the transfer time of the bytes written, debit the
		// time actually taken. Faster than ReadResMinSpeed grows the budget,
		// slower shrinks it.
		pw.budget = min(pw.budget+transferTime(n)-pw.now().Sub(start), ReadResDeadline)
		p = p[n:]
	}
	return written, nil
}

func transferTime(n int) time.Duration {
	return time.Duration(n) * time.Second / ReadResMinSpeed
}
