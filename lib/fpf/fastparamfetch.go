package fastparamfetch

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	fslock "github.com/ipfs/go-fs-lock"
	logging "github.com/ipfs/go-log/v2"
	"github.com/minio/blake2b-simd"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"
)

var log = logging.Logger("paramfetch")

// const gateway = "http://198.211.99.118/ipfs/"
const gateway = "https://proofs.filecoin.io/ipfs/"
const paramdir = "/var/tmp/filecoin-proof-parameters"
const dirEnv = "FIL_PROOFS_PARAMETER_CACHE"
const lockFile = "fetch.lock"
const lockRetry = time.Second * 10

var checked = map[string]struct{}{}
var checkedLk sync.Mutex

type paramFile struct {
	Cid        string `json:"cid"`
	Digest     string `json:"digest"`
	SectorSize uint64 `json:"sector_size"`
}

type fetch struct {
	wg sync.WaitGroup

	errs []error

	fsLockRelease func()
	fsLockOnce    sync.Once
	lockFail      bool // true if we failed to acquire the lock at least once, meaning that is was claimed by another process
}

func getParamDir() string {
	if os.Getenv(dirEnv) == "" {
		return paramdir
	}
	return os.Getenv(dirEnv)
}

func GetParams(ctx context.Context, paramBytes []byte, srsBytes []byte, storageSize uint64) error {
	if err := os.Mkdir(getParamDir(), 0755); err != nil && !os.IsExist(err) {
		return err
	}

	var params map[string]paramFile

	if err := json.Unmarshal(paramBytes, &params); err != nil {
		return err
	}

	ft := &fetch{}

	defer func() {
		if ft.fsLockRelease != nil {
			ft.fsLockRelease()
		}
	}()

	for name, info := range params {
		if storageSize != info.SectorSize && strings.HasSuffix(name, ".params") {
			continue
		}

		ft.maybeFetchAsync(ctx, name, info)
	}

	var srs map[string]paramFile

	if err := json.Unmarshal(srsBytes, &srs); err != nil {
		return err
	}

	for name, info := range srs {
		ft.maybeFetchAsync(ctx, name, info)
	}

	return ft.wait(ctx)
}

// getFsLock tries to acquire the filesystem lock. If it fails, it will retry until it succeeds. Returns whether
// there was lock contention (true if we needed more than one try)
func (ft *fetch) getFsLock() bool {
	ft.fsLockOnce.Do(func() {
		for {
			unlocker, err := fslock.Lock(getParamDir(), lockFile)
			if err == nil {
				ft.fsLockRelease = func() {
					err := unlocker.Close()
					if err != nil {
						log.Errorw("unlock fs lock", "error", err)
					}
				}
				return
			}

			ft.lockFail = true

			le := fslock.LockedError("")
			if errors.As(err, &le) {
				log.Warnf("acquiring filesystem fetch lock: %s; will retry in %s", err, lockRetry)
				time.Sleep(lockRetry)
				continue
			}
			return
		}
	})

	return ft.lockFail
}

func (ft *fetch) maybeFetchAsync(ctx context.Context, name string, info paramFile) {
	ft.wg.Add(1)

	go func() {
		defer ft.wg.Done()

		path := filepath.Join(getParamDir(), name)

		err := ft.checkFile(path, info)
		if !os.IsNotExist(err) && err != nil {
			log.Warn(err)
		}
		if err == nil {
			// all good, no need to fetch
			return
		}

		if ft.getFsLock() {
			// we've managed to get the lock, but we need to re-check file contents - maybe it's fetched now
			ft.maybeFetchAsync(ctx, name, info)
			return
		}

		if err := doFetch(ctx, path, info); err != nil {
			ft.errs = append(ft.errs, xerrors.Errorf("fetching file %s failed: %w", path, err))
			return
		}
		err = ft.checkFile(path, info)
		if err != nil {
			log.Errorf("sanity checking fetched file failed, removing and retrying: %+v", err)
			// remove and retry once more
			err := os.Remove(path)
			if err != nil && !os.IsNotExist(err) {
				ft.errs = append(ft.errs, xerrors.Errorf("remove file %s failed: %w", path, err))
				return
			}

			if err := doFetch(ctx, path, info); err != nil {
				ft.errs = append(ft.errs, xerrors.Errorf("fetching file %s failed: %w", path, err))
				return
			}

			err = ft.checkFile(path, info)
			if err != nil {
				ft.errs = append(ft.errs, xerrors.Errorf("re-checking file %s failed: %w", path, err))
				err := os.Remove(path)
				if err != nil && !os.IsNotExist(err) {
					ft.errs = append(ft.errs, xerrors.Errorf("remove file %s failed: %w", path, err))
				}
			}
		}
	}()
}

func hasTrustableExtension(path string) bool {
	// known extensions include "vk", "srs", and "params"
	// expected to only treat "params" ext as trustable
	// via allowlist
	return strings.HasSuffix(path, "params")
}

func (ft *fetch) checkFile(path string, info paramFile) error {
	isSnapParam := strings.HasPrefix(filepath.Base(path), "v28-empty-sector-update")

	if !isSnapParam && os.Getenv("TRUST_PARAMS") == "1" && hasTrustableExtension(path) {
		log.Debugf("Skipping param check: %s", path)
		log.Warn("Assuming parameter files are ok. DO NOT USE IN PRODUCTION")
		return nil
	}

	checkedLk.Lock()
	_, ok := checked[path]
	checkedLk.Unlock()
	if ok {
		return nil
	}

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
		log.Infof("Parameter file %s is ok", path)

		checkedLk.Lock()
		checked[path] = struct{}{}
		checkedLk.Unlock()

		return nil
	}

	return xerrors.Errorf("checksum mismatch in param file %s, %s != %s", path, strSum, info.Digest)
}

func (ft *fetch) wait(ctx context.Context) error {
	waitChan := make(chan struct{}, 1)

	go func() {
		defer close(waitChan)
		ft.wg.Wait()
	}()

	select {
	case <-ctx.Done():
		log.Infof("context closed... shutting down")
	case <-waitChan:
		log.Infof("parameter and key-fetching complete")
	}

	return multierr.Combine(ft.errs...)
}

func doFetch(ctx context.Context, out string, info paramFile) error {
	gw := os.Getenv("IPFS_GATEWAY")
	if gw == "" {
		gw = gateway
	}
	log.Infof("Fetching %s from %s", out, gw)

	url, err := url.Parse(gw + info.Cid)
	if err != nil {
		return err
	}
	log.Infof("GET %s", url)

	// Try aria2c first
	if err := fetchWithAria2c(ctx, out, url.String()); err == nil {
		return nil
	} else {
		log.Warnf("aria2c fetch failed: %s", err)
	}

	outf, err := os.OpenFile(out, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	defer outf.Close()

	fStat, err := outf.Stat()
	if err != nil {
		return err
	}
	header := http.Header{}
	header.Set("Range", "bytes="+strconv.FormatInt(fStat.Size(), 10)+"-")

	req, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
	if err != nil {
		return err
	}
	req.Close = true
	req.Header = header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(outf, resp.Body)

	return err
}

func fetchWithAria2c(ctx context.Context, out, url string) error {
	aria2cPath, err := exec.LookPath("aria2c")
	if err != nil {
		return xerrors.New("aria2c not found in PATH")
	}

	cmd := exec.CommandContext(ctx, aria2cPath, "--continue", "-x16", "-s16", "--dir", filepath.Dir(out), "-o", filepath.Base(out), url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}
