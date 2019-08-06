// +build !windows
// TODO: windows now has pipes, verify if it maybe works

// TODO: extract from filecoin

package bytesink

import (
	"fmt"
	"io/ioutil"
	"os"
	"syscall"

	"github.com/pkg/errors"
)

// FifoByteSink is not safe for concurrent access, as writes to underlying pipe are atomic only
// if len(buf) is less than the OS-specific PIPE_BUF value.
type FifoByteSink struct {
	file *os.File
	path string
}

// Open prepares the sink for writing by opening the backing FIFO file. Open
// will block until someone opens the FIFO file for reading.
func (s *FifoByteSink) Open() error {
	file, err := os.OpenFile(s.path, os.O_WRONLY, os.ModeNamedPipe)
	if err != nil {
		return errors.Wrap(err, "failed to open pipe")
	}

	s.file = file

	return nil
}

// Write writes the provided buffer to the underlying pipe. Write will block
// until the provided buffer's bytes have been read from the read end of the
// pipe.
//
// Warning: Writes are atomic only if len(buf) is less than the OS-specific
// PIPE_BUF value. For more information, see:
//
// http://pubs.opengroup.org/onlinepubs/9699919799/functions/write.html
func (s *FifoByteSink) Write(buf []byte) (int, error) {
	return s.file.Write(buf)
}

// Close ensures that the underlying file is closed and removed.
func (s *FifoByteSink) Close() (retErr error) {
	cerr := s.file.Close()
	if cerr != nil {
		return cerr
	}

	defer func() {
		rerr := os.Remove(s.path)
		if retErr == nil {
			retErr = rerr
		}
	}()

	return
}

// ID produces a string-identifier for this byte sink. For now, this is just the
// path of the FIFO file. This string may get more structured in the future.
func (s *FifoByteSink) ID() string {
	return s.path
}

// NewFifo creates a FIFO pipe and returns a pointer to a FifoByteSink, which
// satisfies the ByteSink interface. The FIFO pipe is used to stream bytes to
// rust-fil-proofs from Go during the piece-adding flow. Writes to the pipe are
// buffered automatically by the OS; the size of the buffer varies.
func NewFifo() (*FifoByteSink, error) {
	path, err := createTmpFifoPath()
	if err != nil {
		return nil, errors.Wrap(err, "creating FIFO path failed")
	}

	err = syscall.Mkfifo(path, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "mkfifo failed")
	}

	return &FifoByteSink{
		path: path,
	}, nil
}

// createTmpFifoPath creates a path with which a temporary FIFO file may be
// created.
func createTmpFifoPath() (string, error) {
	file, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}

	err = file.Close()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s.fifo", file.Name()), nil
}
