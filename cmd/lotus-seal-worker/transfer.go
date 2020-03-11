package main


import (
	"mime"
	"net/http"
	"os"

	files "github.com/ipfs/go-ipfs-files"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/lib/tarutil"
)

func (w *workerStorage) fetch(url, outname string) error {
	log.Infof("Fetch %s", url)

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

	/*bar := pb.New64(w.sizeForType(typ))
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar.Units = pb.U_BYTES

	barreader := bar.NewProxyReader(resp.Body)

	bar.Start()
	defer bar.Finish()*/

	mediatype, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return xerrors.Errorf("parse media type: %w", err)
	}

	if err := os.RemoveAll(outname); err != nil {
		return xerrors.Errorf("removing dest: %w", err)
	}

	switch mediatype {
	case "application/x-tar":
		return tarutil.ExtractTar(resp.Body, outname)
	case "application/octet-stream":
		return files.WriteTo(files.NewReaderFile(resp.Body), outname)
	default:
		return xerrors.Errorf("unknown content type: '%s'", mediatype)
	}

}
