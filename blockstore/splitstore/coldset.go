package splitstore

import (
	"bufio"
	"io"
	"os"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

type ColdSetWriter struct {
	file *os.File
	buf  *bufio.Writer
}

type ColdSetReader struct {
	file *os.File
	buf  *bufio.Reader
}

func NewColdSetWriter(path string) (*ColdSetWriter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return nil, xerrors.Errorf("error creating coldset: %w", err)
	}
	buf := bufio.NewWriter(file)

	return &ColdSetWriter{
		file: file,
		buf:  buf,
	}, nil
}

func NewColdSetReader(path string) (*ColdSetReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("error opening coldset: %w", err)
	}
	buf := bufio.NewReader(file)

	return &ColdSetReader{
		file: file,
		buf:  buf,
	}, nil
}

func (s *ColdSetWriter) Write(c cid.Cid) error {
	return writeRawCid(s.buf, c, false)
}

func (s *ColdSetWriter) Close() error {
	if s.file == nil {
		return nil
	}

	err1 := s.buf.Flush()
	err2 := s.file.Close()
	s.buf = nil
	s.file = nil

	if err1 != nil {
		return err1
	}
	return err2
}

func (s *ColdSetReader) ForEach(f func(cid.Cid) error) error {
	hbuf := make([]byte, 256)
	for {
		next, err := readRawCid(s.buf, hbuf)
		if err != nil {
			if err == io.EOF {
				return nil
			}

			return xerrors.Errorf("error reading coldset: %w", err)
		}

		if err := f(next); err != nil {
			return err
		}
	}
}

func (s *ColdSetReader) Reset() error {
	_, err := s.file.Seek(0, io.SeekStart)
	return err
}

func (s *ColdSetReader) Close() error {
	if s.file == nil {
		return nil
	}

	err := s.file.Close()
	s.file = nil
	s.buf = nil

	return err
}
