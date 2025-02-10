package build

import (
	"bytes"
	"embed"
	"io"
	"path"

	logging "github.com/ipfs/go-log/v2"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

// moved from now-defunct build/paramfetch.go
var log = logging.Logger("build")

//go:embed genesis/*.car.zst
var genesisCars embed.FS

var zstdHeader = []byte{0x28, 0xb5, 0x2f, 0xfd}

func MaybeGenesis() []byte {
	file, err := genesisCars.Open(path.Join("genesis", buildconstants.GenesisFile))
	if err != nil {
		log.Warnf("opening built-in genesis: %s", err)
		return nil
	}
	defer file.Close() //nolint
	decompressed, err := DecompressAsZstd(file)
	if err != nil {
		log.Warnf("decompressing genesis: %s", err)
		return nil
	}
	return decompressed
}

func DecompressAsZstd(target io.Reader) ([]byte, error) {
	decoder, err := zstd.NewReader(target)
	if err != nil {
		return nil, xerrors.Errorf("creating zstd decoder: %w", err)
	}
	defer decoder.Close() //nolint

	decompressed, err := io.ReadAll(decoder)
	if err != nil {
		return nil, xerrors.Errorf("reading decompressed genesis file: %w", err)
	}
	return decompressed, nil
}

func IsZstdCompressed(file io.ReadSeeker) (_ bool, _err error) {
	pos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, xerrors.Errorf("getting current position: %w", err)
	}
	defer func() {
		_, err := file.Seek(pos, io.SeekStart)
		if _err == nil && err != nil {
			_err = xerrors.Errorf("seeking back to original offset: %w", err)
		}
	}()
	header := make([]byte, 4)
	if _, err := file.Read(header); err != nil {
		return false, xerrors.Errorf("failed to read file header: %w", err)
	}
	return bytes.Equal(header, zstdHeader), nil
}
