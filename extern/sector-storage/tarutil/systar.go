package tarutil

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("tarutil") // nolint

func ExtractTar(body io.Reader, dir string, buf []byte) error {
	if err := os.MkdirAll(dir, 0755); err != nil { // nolint
		return xerrors.Errorf("mkdir: %w", err)
	}

	tr := tar.NewReader(body)
	for {
		header, err := tr.Next()
		switch err {
		default:
			return err
		case io.EOF:
			return nil

		case nil:
		}

		f, err := os.Create(filepath.Join(dir, header.Name))
		if err != nil {
			return xerrors.Errorf("creating file %s: %w", filepath.Join(dir, header.Name), err)
		}

		// This data is coming from a trusted source, no need to check the size.
		//nolint:gosec
		if _, err := io.CopyBuffer(f, tr, buf); err != nil {
			return err
		}

		if err := f.Close(); err != nil {
			return err
		}
	}
}

func TarDirectory(dir string, w io.Writer, buf []byte) error {
	tw := tar.NewWriter(w)

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, file := range files {
		h, err := tar.FileInfoHeader(file, "")
		if err != nil {
			return xerrors.Errorf("getting header for file %s: %w", file.Name(), err)
		}

		if err := tw.WriteHeader(h); err != nil {
			return xerrors.Errorf("wiritng header for file %s: %w", file.Name(), err)
		}

		f, err := os.OpenFile(filepath.Join(dir, file.Name()), os.O_RDONLY, 644) // nolint
		if err != nil {
			return xerrors.Errorf("opening %s for reading: %w", file.Name(), err)
		}

		if _, err := io.CopyBuffer(tw, f, buf); err != nil {
			return xerrors.Errorf("copy data for file %s: %w", file.Name(), err)
		}

		if err := f.Close(); err != nil {
			return err
		}

	}

	return nil
}
