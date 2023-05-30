package splitstore

import (
	"bufio"
	"io"
	"os"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

type Checkpoint struct {
	file *os.File
	buf  *bufio.Writer
}

func NewCheckpoint(path string) (*Checkpoint, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_SYNC, 0644)
	if err != nil {
		return nil, xerrors.Errorf("error creating checkpoint: %w", err)
	}
	buf := bufio.NewWriter(file)

	return &Checkpoint{
		file: file,
		buf:  buf,
	}, nil
}

func OpenCheckpoint(path string) (*Checkpoint, cid.Cid, error) {
	filein, err := os.Open(path)
	if err != nil {
		return nil, cid.Undef, xerrors.Errorf("error opening checkpoint for reading: %w", err)
	}
	defer filein.Close() //nolint:errcheck

	bufin := bufio.NewReader(filein)
	start, err := readRawCid(bufin, nil)
	if err != nil && err != io.EOF {
		return nil, cid.Undef, xerrors.Errorf("error reading cid from checkpoint: %w", err)
	}

	fileout, err := os.OpenFile(path, os.O_WRONLY|os.O_SYNC, 0644)
	if err != nil {
		return nil, cid.Undef, xerrors.Errorf("error opening checkpoint for writing: %w", err)
	}
	bufout := bufio.NewWriter(fileout)

	return &Checkpoint{
		file: fileout,
		buf:  bufout,
	}, start, nil
}

func (cp *Checkpoint) Set(c cid.Cid) error {
	if _, err := cp.file.Seek(0, io.SeekStart); err != nil {
		return xerrors.Errorf("error seeking beginning of checkpoint: %w", err)
	}

	if err := writeRawCid(cp.buf, c, true); err != nil {
		return xerrors.Errorf("error writing cid to checkpoint: %w", err)
	}

	return nil
}

func (cp *Checkpoint) Close() error {
	if cp.file == nil {
		return nil
	}

	err := cp.file.Close()
	cp.file = nil
	cp.buf = nil

	return err
}

func readRawCid(buf *bufio.Reader, hbuf []byte) (cid.Cid, error) {
	sz, err := buf.ReadByte()
	if err != nil {
		return cid.Undef, err // don't wrap EOF as it is not an error here
	}

	if hbuf == nil {
		hbuf = make([]byte, int(sz))
	} else {
		hbuf = hbuf[:int(sz)]
	}

	if _, err := io.ReadFull(buf, hbuf); err != nil {
		return cid.Undef, xerrors.Errorf("error reading hash: %w", err) // wrap EOF, it's corrupt
	}

	hash, err := mh.Cast(hbuf)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error casting multihash: %w", err)
	}

	return cid.NewCidV1(cid.Raw, hash), nil
}

func writeRawCid(buf *bufio.Writer, c cid.Cid, flush bool) error {
	hash := c.Hash()
	if err := buf.WriteByte(byte(len(hash))); err != nil {
		return err
	}
	if _, err := buf.Write(hash); err != nil {
		return err
	}
	if flush {
		return buf.Flush()
	}

	return nil
}
