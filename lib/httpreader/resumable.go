package httpreader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"
)

var log = logging.Logger("httpreader")

const (
	maxResumeAttempts = 10
	resumeBackoff     = time.Second
	maxResumeBackoff  = 30 * time.Second
	idleTimeout       = time.Minute
)

var errIdle = errors.New("no data received within the idle timeout")

type ResumableReader struct {
	ctx           context.Context
	initialURL    string
	finalURL      *string
	position      int64
	contentLength int64
	// validator is the strong ETag or Last-Modified of the first response,
	// sent as If-Range so a changed resource isn't resumed
	validator string
	client    *http.Client
	body      *body
	closed    bool

	backoff, maxBackoff, idleTimeout time.Duration
	// attempts counts resumes since the last read that made progress, and
	// cause is the error that last interrupted the download
	attempts int
	cause    error
	// reresolved is set while resuming through initialURL after finalURL
	// was refused
	reresolved bool
}

var _ io.ReadCloser = (*ResumableReader)(nil)

func NewResumableReader(ctx context.Context, url string) (*ResumableReader, error) {
	return newResumableReader(ctx, url, nil)
}

func newResumableReader(ctx context.Context, url string, transport http.RoundTripper) (*ResumableReader, error) {
	finalURL := ""

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			finalURL = req.URL.String()
			if len(via) >= 10 {
				return xerrors.New("stopped after 10 redirects")
			}
			return nil
		},
	}

	r := &ResumableReader{
		ctx:         ctx,
		initialURL:  url,
		finalURL:    &finalURL,
		position:    0,
		client:      client,
		backoff:     resumeBackoff,
		maxBackoff:  maxResumeBackoff,
		idleTimeout: idleTimeout,
	}

	resp, b, err := r.get(url, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_ = b.close()
		return nil, fmt.Errorf("failed to fetch resource, status code: %d", resp.StatusCode)
	}

	contentLength, parseErr := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if parseErr != nil {
		if closeErr := b.close(); closeErr != nil {
			return nil, multierr.Append(parseErr, closeErr)
		}
		return nil, parseErr
	}

	r.contentLength = contentLength
	r.body = b
	if etag := resp.Header.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		r.validator = etag
	} else {
		r.validator = resp.Header.Get("Last-Modified")
	}

	return r, nil
}

func (r *ResumableReader) ContentLength() int64 {
	return r.contentLength
}

func (r *ResumableReader) Close() error {
	r.closed = true
	if r.body == nil {
		return nil
	}
	err := r.body.close()
	r.body = nil
	return err
}

// Read resumes the download from the current position after any read error
// other than context cancellation. It gives up after maxResumeAttempts
// consecutive resumes that read nothing.
func (r *ResumableReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, os.ErrClosed
	}
	for {
		if r.position >= r.contentLength {
			r.closeBody()
			return 0, io.EOF
		}
		if r.body == nil {
			if err := r.resume(); err != nil {
				return 0, err
			}
		}

		n, err := r.body.read(p, r.idleTimeout)
		r.position += int64(n)
		if n > 0 {
			r.attempts = 0
			r.reresolved = false
		}
		if err == nil {
			return n, nil
		}

		r.closeBody()
		if r.position >= r.contentLength {
			return n, io.EOF
		}
		if r.ctx.Err() != nil {
			return n, err
		}
		r.cause = err
		if n > 0 {
			return n, nil
		}
	}
}

func (r *ResumableReader) closeBody() {
	if r.body == nil {
		return
	}
	if err := r.body.close(); err != nil {
		log.Warnf("error closing reader: %+v", err)
	}
	r.body = nil
}

// resume requests the rest of the resource from the current position, with
// backoff between attempts.
func (r *ResumableReader) resume() error {
	for {
		r.attempts++
		if r.attempts > maxResumeAttempts {
			return xerrors.Errorf("giving up at byte %d of %d after %d attempts to resume: %w",
				r.position, r.contentLength, maxResumeAttempts, r.cause)
		}
		delay := min(r.backoff<<(r.attempts-1), r.maxBackoff)
		log.Warnf("resuming at byte %d of %d in %s, attempt %d of %d: %s",
			r.position, r.contentLength, delay, r.attempts, maxResumeAttempts, r.cause)
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		case <-time.After(delay):
		}

		b, retry, err := r.requestRange()
		if err == nil {
			r.body = b
			return nil
		}
		if !retry {
			return err
		}
		r.cause = err
	}
}

// requestRange requests the resource from the current position, and reports
// whether a failure is worth retrying.
func (r *ResumableReader) requestRange() (*body, bool, error) {
	reqURL := r.initialURL
	redirected := *r.finalURL != ""
	if redirected {
		reqURL = *r.finalURL
	}

	header := http.Header{}
	header.Set("Range", fmt.Sprintf("bytes=%d-", r.position))
	if r.validator != "" {
		header.Set("If-Range", r.validator)
	}
	resp, b, err := r.get(reqURL, header)
	if err != nil {
		return nil, r.ctx.Err() == nil, err
	}

	switch {
	case resp.StatusCode == http.StatusPartialContent:
		if err := r.checkContentRange(resp.Header.Get("Content-Range")); err != nil {
			_ = b.close()
			return nil, false, err
		}
		return b, false, nil
	case resp.StatusCode == http.StatusOK && r.position == 0 && resp.ContentLength == r.contentLength:
		return b, false, nil
	case resp.StatusCode == http.StatusOK:
		_ = b.close()
		return nil, false, fmt.Errorf("server sent the whole resource instead of resuming at byte %d: it ignored the range, or the resource changed", r.position)
	}

	_ = b.close()
	err = fmt.Errorf("non-resumable status code: %d", resp.StatusCode)
	switch {
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return nil, true, err
	case resp.StatusCode >= 400 && redirected && !r.reresolved:
		// A redirect target, such as a presigned URL, may have expired;
		// resolve it again from the initial URL
		*r.finalURL = ""
		r.reresolved = true
		return nil, true, err
	default:
		return nil, false, err
	}
}

// checkContentRange checks a "bytes start-end/total" Content-Range resumes at
// the current position of the same resource.
func (r *ResumableReader) checkContentRange(cr string) error {
	var start, end, total int64
	if _, err := fmt.Sscanf(cr, "bytes %d-%d/%d", &start, &end, &total); err != nil {
		return xerrors.Errorf("resuming at byte %d, invalid Content-Range %q: %w", r.position, cr, err)
	}
	if start != r.position || total != r.contentLength {
		return fmt.Errorf("resuming at byte %d of %d, got Content-Range %q", r.position, r.contentLength, cr)
	}
	return nil
}

// get sends a GET under a context that is cancelled when the request, or a
// read of its body, waits longer than idleTimeout for data.
func (r *ResumableReader) get(url string, header http.Header) (*http.Response, *body, error) {
	ctx, cancel := context.WithCancelCause(r.ctx)
	idle := time.AfterFunc(r.idleTimeout, func() { cancel(errIdle) })
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		idle.Stop()
		cancel(nil)
		return nil, nil, err
	}
	if header != nil {
		req.Header = header
	}
	resp, err := r.client.Do(req)
	idle.Stop()
	if err != nil {
		if context.Cause(ctx) == errIdle {
			err = errIdle
		}
		cancel(nil)
		return nil, nil, err
	}
	return resp, &body{ReadCloser: resp.Body, ctx: ctx, idle: idle, cancel: cancel}, nil
}

// body is a response body whose reads are bounded by an idle timeout.
type body struct {
	io.ReadCloser
	ctx    context.Context
	idle   *time.Timer
	cancel context.CancelCauseFunc
}

func (b *body) read(p []byte, timeout time.Duration) (int, error) {
	b.idle.Reset(timeout)
	n, err := b.Read(p)
	b.idle.Stop()
	if err != nil && context.Cause(b.ctx) == errIdle {
		err = errIdle
	}
	return n, err
}

func (b *body) close() error {
	b.idle.Stop()
	b.cancel(nil)
	return b.Close()
}
