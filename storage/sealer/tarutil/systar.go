package tarutil

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
)

var log = logging.Logger("tarutil") // nolint

var CacheFileConstraints = map[string]int64{
	"p_aux": 64,
	"t_aux": 10240,

	"sc-02-data-tree-r-last.dat": 10_000_000, // small sectors

	"sc-02-data-tree-r-last-0.dat": 10_000_000,
	"sc-02-data-tree-r-last-1.dat": 10_000_000,
	"sc-02-data-tree-r-last-2.dat": 10_000_000,
	"sc-02-data-tree-r-last-3.dat": 10_000_000,
	"sc-02-data-tree-r-last-4.dat": 10_000_000,
	"sc-02-data-tree-r-last-5.dat": 10_000_000,
	"sc-02-data-tree-r-last-6.dat": 10_000_000,
	"sc-02-data-tree-r-last-7.dat": 10_000_000,

	"sc-02-data-tree-r-last-8.dat":  10_000_000,
	"sc-02-data-tree-r-last-9.dat":  10_000_000,
	"sc-02-data-tree-r-last-10.dat": 10_000_000,
	"sc-02-data-tree-r-last-11.dat": 10_000_000,
	"sc-02-data-tree-r-last-12.dat": 10_000_000,
	"sc-02-data-tree-r-last-13.dat": 10_000_000,
	"sc-02-data-tree-r-last-14.dat": 10_000_000,
	"sc-02-data-tree-r-last-15.dat": 10_000_000,

	"sc-02-data-layer-1.dat":  65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-2.dat":  65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-3.dat":  65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-4.dat":  65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-5.dat":  65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-6.dat":  65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-7.dat":  65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-8.dat":  65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-9.dat":  65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-10.dat": 65 << 30, // 1x sector size + small buffer
	"sc-02-data-layer-11.dat": 65 << 30, // 1x sector size + small buffer

	"sc-02-data-tree-c-0.dat": 5 << 30, // ~4.6G
	"sc-02-data-tree-c-1.dat": 5 << 30,
	"sc-02-data-tree-c-2.dat": 5 << 30,
	"sc-02-data-tree-c-3.dat": 5 << 30,
	"sc-02-data-tree-c-4.dat": 5 << 30,
	"sc-02-data-tree-c-5.dat": 5 << 30,
	"sc-02-data-tree-c-6.dat": 5 << 30,
	"sc-02-data-tree-c-7.dat": 5 << 30,

	"sc-02-data-tree-c-8.dat":  5 << 30,
	"sc-02-data-tree-c-9.dat":  5 << 30,
	"sc-02-data-tree-c-10.dat": 5 << 30,
	"sc-02-data-tree-c-11.dat": 5 << 30,
	"sc-02-data-tree-c-12.dat": 5 << 30,
	"sc-02-data-tree-c-13.dat": 5 << 30,
	"sc-02-data-tree-c-14.dat": 5 << 30,
	"sc-02-data-tree-c-15.dat": 5 << 30,

	"sc-02-data-tree-d.dat": 130 << 30, // 2x sector size, ~130G accunting for small buffer on 64G sectors
}

func ExtractTar(body io.Reader, dir string, buf []byte) (int64, error) {
	if err := os.MkdirAll(dir, 0755); err != nil { // nolint
		return 0, xerrors.Errorf("mkdir: %w", err)
	}

	tr := tar.NewReader(body)
	var read int64
	for {
		header, err := tr.Next()
		switch err {
		default:
			return read, err
		case io.EOF:
			return read, nil

		case nil:
		}

		sz, found := CacheFileConstraints[header.Name]
		if !found {
			return read, xerrors.Errorf("tar file %#v isn't expected", header.Name)
		}
		if header.Size > sz {
			return read, xerrors.Errorf("tar file %#v is bigger than expected: %d > %d", header.Name, header.Size, sz)
		}

		out := filepath.Join(dir, header.Name) //nolint:gosec

		if !strings.HasPrefix(out, filepath.Clean(dir)) {
			return read, xerrors.Errorf("unsafe tar path %#v (must be within %#v)", out, filepath.Clean(dir))
		}

		f, err := os.Create(out)
		if err != nil {
			return read, xerrors.Errorf("creating file %s: %w", out, err)
		}

		ltr := io.LimitReader(tr, header.Size)

		r, err := io.CopyBuffer(f, ltr, buf)
		read += r
		if err != nil {
			return read, err
		}

		if err := f.Close(); err != nil {
			return read, err
		}
	}
}

func TarDirectory(dir string, w io.Writer, buf []byte) error {
	tw := tar.NewWriter(w)

	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			return xerrors.Errorf("getting file info for file %s: %w", file.Name(), err)
		}

		h, err := tar.FileInfoHeader(info, "")
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
