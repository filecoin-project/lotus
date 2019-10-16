package build

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	rice "github.com/GeertJohan/go.rice"
	logging "github.com/ipfs/go-log"
	"github.com/minio/blake2b-simd"
	pb "gopkg.in/cheggaaa/pb.v1"
)

var log = logging.Logger("build")

const gateway = "http://198.211.99.118/ipfs/"
const paramdir = "/var/tmp/filecoin-proof-parameters"

type paramFile struct {
	Cid        string `json:"cid"`
	Digest     string `json:"digest"`
	SectorSize uint64 `json:"sector_size"`
}

func GetParams(storage bool) error {
	if err := os.Mkdir(paramdir, 0755); err != nil && !os.IsExist(err) {
		return err
	}

	var params map[string]paramFile

	paramBytes := rice.MustFindBox("proof-params").MustBytes("parameters.json")
	if err := json.Unmarshal(paramBytes, &params); err != nil {
		return err
	}

	for name, info := range params {
		if !SupportedSectorSize(info.SectorSize) {
			continue
		}
		if !storage && strings.HasSuffix(name, ".params") {
			continue
		}

		if err := maybeFetch(name, info); err != nil {
			return err
		}
	}

	return nil
}

func maybeFetch(name string, info paramFile) error {
	path := filepath.Join(paramdir, name)
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()

		h := blake2b.New512()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}

		sum := h.Sum(nil)
		strSum := hex.EncodeToString(sum[:16])
		if strSum == info.Digest {
			return nil
		}

		log.Warnf("Checksum mismatch in param file %s, %s != %s", name, strSum, info.Digest)
	}

	return doFetch(path, info)
}

func doFetch(out string, info paramFile) error {
	log.Infof("Fetching %s", out)

	resp, err := http.Get(gateway + info.Cid)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	outf, err := os.Create(out)
	if err != nil {
		return err
	}
	defer outf.Close()

	bar := pb.New64(resp.ContentLength)
	bar.Units = pb.U_BYTES
	bar.ShowSpeed = true
	bar.Start()

	_, err = io.Copy(outf, bar.NewProxyReader(resp.Body))

	bar.Finish()

	return err
}
