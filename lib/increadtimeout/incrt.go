package incrt

import (
	"io"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("incrt")

var now = time.Now

type ReaderDeadline interface {
	Read([]byte) (int, error)
	SetReadDeadline(time.Time) error
}

type incrt struct {
	rd ReaderDeadline

	waitPerByte time.Duration
	wait        time.Duration
	maxWait     time.Duration
}

// New creates an Incremental Reader Timeout, with minimum sustained speed of
// minSpeed bytes per second and with maximum wait of maxWait
func New(rd ReaderDeadline, minSpeed int64, maxWait time.Duration) io.Reader {
	return &incrt{
		rd:          rd,
		waitPerByte: time.Second / time.Duration(minSpeed),
		wait:        maxWait,
		maxWait:     maxWait,
	}
}

func (crt *incrt) Read(buf []byte) (n int, err error) {
	start := now()
	err = crt.rd.SetReadDeadline(start.Add(crt.wait))
	if err != nil {
		return 0, err
	}

	defer func() {
		crt.rd.SetReadDeadline(time.Time{})
		if err == nil {
			dur := now().Sub(start)
			crt.wait -= dur
			crt.wait += time.Duration(n) * crt.waitPerByte
			if crt.wait < 0 {
				crt.wait = 0
			}
			if crt.wait > crt.maxWait {
				crt.wait = crt.maxWait
			}
		}
	}()

	n, err = crt.rd.Read(buf)
	return n, err
}
