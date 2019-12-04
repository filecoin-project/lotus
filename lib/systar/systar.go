package systar

import (
	"golang.org/x/xerrors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("systar")

func ExtractTar(body io.Reader, dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return xerrors.Errorf("creating dest directory: %w", err)
	}

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

	go func() {
		if err := o.CloseWithError(cmd.Wait()); err != nil {
			log.Error(err)
		}
	}()

	return i, nil
}
