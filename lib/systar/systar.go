package systar

import (
	"io"
	"os/exec"
	"path/filepath"
)

func ExtractTar(body io.Reader, dest string) error {
	cmd := exec.Command("tar", "-xS", "-C", dest)
	cmd.Stdin = body
	return cmd.Run()
}

func TarDirectory(file string) (io.ReadCloser, error) {
	// use system builtin tar, golang one doesn't support sparse files

	dir := filepath.Dir(file)
	base := filepath.Base(file)

	i, o := io.Pipe()

	// don't bother with compression, it's mostly random data
	cmd := exec.Command("tar", "-cSf", "-", "-C", dir, base)
	cmd.Stdout = o
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &struct {
		io.Reader
		io.Closer
	}{
		Reader: i,
		Closer: closer(func() error {
			e1 := i.Close()
			if err := cmd.Wait(); err != nil {
				return err
			}

			return e1
		}),
	}, nil
}

type closer func() error
func (cl closer) Close() error {
	return cl()
}
