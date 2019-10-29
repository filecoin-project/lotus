package build

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	rice "github.com/GeertJohan/go.rice"
	logging "github.com/ipfs/go-log"
	"github.com/minio/blake2b-simd"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"
	pb "gopkg.in/cheggaaa/pb.v1"
)

var log = logging.Logger("build")

//const gateway = "http://198.211.99.118/ipfs/"
const gateway = "https://ipfs.io/ipfs/"
const paramdir = "/var/tmp/filecoin-proof-parameters"

type paramFile struct {
	Cid        string `json:"cid"`
	Digest     string `json:"digest"`
	SectorSize uint64 `json:"sector_size"`
}

type fetch struct {
	wg      sync.WaitGroup
	fetchLk sync.Mutex

	errs []error
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

	ft := &fetch{}

	for name, info := range params {
		if !SupportedSectorSize(info.SectorSize) {
			continue
		}
		if !storage && strings.HasSuffix(name, ".params") {
			continue
		}

		ft.maybeFetchAsync(name, info)
	}

	return ft.wait()
}

func (ft *fetch) maybeFetchAsync(name string, info paramFile) {
	ft.wg.Add(1)

	go func() {
		defer ft.wg.Done()

		path := filepath.Join(paramdir, name)

		err := ft.checkFile(path, info)
		if !os.IsNotExist(err) && err != nil {
			log.Warn(err)
		}
		if err == nil {
			return
		}

		ft.fetchLk.Lock()
		defer ft.fetchLk.Unlock()

		if err := doFetch(path, info); err != nil {
			ft.errs = append(ft.errs, xerrors.Errorf("fetching file %s: %w", path, err))
		}
	}()
}

func (ft *fetch) checkFile(path string, info paramFile) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
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

	return xerrors.Errorf("checksum mismatch in param file %s, %s != %s", path, strSum, info.Digest)
}

func (ft *fetch) wait() error {
	ft.wg.Wait()
	return multierr.Combine(ft.errs...)
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
