package main

import (
	"io"
	"mime"
	"net/http"
	"os"

	sectorbuilder "github.com/xjrwfilecoin/go-sectorbuilder"
	files "github.com/ipfs/go-ipfs-files"
	"golang.org/x/xerrors"
	"gopkg.in/cheggaaa/pb.v1"
	"path/filepath"

	"github.com/filecoin-project/lotus/lib/tarutil"
)

func (w *worker) fetch(typ string, sectorID uint64) error {
	return nil;
	outname := filepath.Join(w.repo, typ, w.sb.SectorName(sectorID))

	url := w.minerEndpoint + "/remote/" + typ + "/" + w.sb.SectorName(sectorID)
	log.Infof("Fetch %s %s", typ, url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return xerrors.Errorf("request: %w", err)
	}
	req.Header = w.auth

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return xerrors.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return xerrors.Errorf("non-200 code: %d", resp.StatusCode)
	}

	bar := pb.New64(resp.ContentLength)
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar.Units = pb.U_BYTES

	barreader := bar.NewProxyReader(resp.Body)

	bar.Start()
	defer bar.Finish()

	mediatype, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return xerrors.Errorf("parse media type: %w", err)
	}

	if err := os.RemoveAll(outname); err != nil {
		return xerrors.Errorf("removing dest: %w", err)
	}

	switch mediatype {
	case "application/x-tar":
		return tarutil.ExtractTar(barreader, outname)
	case "application/octet-stream":
		return files.WriteTo(files.NewReaderFile(barreader), outname)
	default:
		return xerrors.Errorf("unknown content type: '%s'", mediatype)
	}

}

func (w *worker) push(typ string, sectorID uint64) error {
	return nil
	filename := filepath.Join(w.repo, typ, w.sb.SectorName(sectorID))

	url := w.minerEndpoint + "/remote/" + typ + "/" + w.sb.SectorName(sectorID)
	log.Infof("Push %s %s", typ, url)

	stat, err := os.Stat(filename)
	if err != nil {
		return err
	}

	var r io.Reader
	if stat.IsDir() {
		r, err = tarutil.TarDirectory(filename)
	} else {
		r, err = os.OpenFile(filename, os.O_RDONLY, 0644)
	}
	if err != nil {
		return xerrors.Errorf("opening push reader: %w", err)
	}

	bar := pb.New64(0)
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar.ShowCounters = true
	bar.Units = pb.U_BYTES

	bar.Start()
	defer bar.Finish()
	//todo set content size

	header := w.auth

	if stat.IsDir() {
		header.Set("Content-Type", "application/x-tar")
	} else {
		header.Set("Content-Type", "application/octet-stream")
	}

	req, err := http.NewRequest("PUT", url, bar.NewProxyReader(r))
	if err != nil {
		return err
	}
	req.Header = header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return xerrors.Errorf("non-200 response: %d", resp.StatusCode)
	}

	if err := resp.Body.Close(); err != nil {
		return err
	}

	// TODO: keep files around for later stages of sealing
	return os.RemoveAll(filename)
}

func (w *worker) fetchSector(sectorID uint64, typ sectorbuilder.WorkerTaskType) error {
	var err error
	switch typ {
	case sectorbuilder.WorkerPreCommit:
		err = w.fetch("staging", sectorID)
	case sectorbuilder.WorkerCommit:
		err = w.fetch("sealed", sectorID)
		if err != nil {
			return xerrors.Errorf("fetch sealed: %w", err)
		}
		err = w.fetch("cache", sectorID)
	}
	if err != nil {
		return xerrors.Errorf("fetch failed: %w", err)
	}
	return nil
}
