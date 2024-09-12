package build

import (
	"embed"
	"io"
	"path"

	logging "github.com/ipfs/go-log/v2"
	"github.com/klauspost/compress/zstd"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

// moved from now-defunct build/paramfetch.go
var log = logging.Logger("build")

//go:embed genesis/*.car.zst
var genesisCars embed.FS

func MaybeGenesis() []byte {
	file, err := genesisCars.Open(path.Join("genesis", buildconstants.GenesisFile))
	if err != nil {
		log.Warnf("opening built-in genesis: %s", err)
		return nil
	}
	defer file.Close() //nolint

	decoder, err := zstd.NewReader(file)
	if err != nil {
		log.Warnf("creating zstd decoder: %s", err)
		return nil
	}
	defer decoder.Close() //nolint

	decompressedBytes, err := io.ReadAll(decoder)
	if err != nil {
		log.Warnf("reading decompressed genesis file: %s", err)
		return nil
	}

	return decompressedBytes
}
