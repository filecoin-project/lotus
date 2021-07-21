package dagstore

import (
	"context"
	"io"
	"net/url"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore/mount"
)

const lotusScheme = "lotus"

var _ mount.Mount = (*LotusMount)(nil)

// LotusMount is the Lotus implementation of a Sharded DAG Store Mount.
// A Filecoin Piece is treated as a Shard by this implementation.
type LotusMount struct {
	Api      LotusAccessor
	PieceCid cid.Cid
}

// This method is called when registering a mount with the DAG store registry.
// The DAG store registry receives an instance of the mount (a "template").
// When the registry needs to deserialize a mount it clones the template then
// calls Deserialize on the cloned instance, which will have a reference to the
// lotus mount API supplied here.
func NewLotusMountTemplate(api LotusAccessor) *LotusMount {
	return &LotusMount{Api: api}
}

func NewLotusMount(pieceCid cid.Cid, api LotusAccessor) (*LotusMount, error) {
	return &LotusMount{
		PieceCid: pieceCid,
		Api:      api,
	}, nil
}

func (l *LotusMount) Serialize() *url.URL {
	return &url.URL{
		Host: l.PieceCid.String(),
	}
}

func (l *LotusMount) Deserialize(u *url.URL) error {
	pieceCid, err := cid.Decode(u.Host)
	if err != nil {
		return xerrors.Errorf("failed to parse PieceCid from host '%s': %w", u.Host, err)
	}

	l.PieceCid = pieceCid
	return nil
}

func (l *LotusMount) Fetch(ctx context.Context) (mount.Reader, error) {
	r, err := l.Api.FetchUnsealedPiece(ctx, l.PieceCid)
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch unsealed piece %s: %w", l.PieceCid, err)
	}
	return &readCloser{r}, nil
}

func (l *LotusMount) Info() mount.Info {
	return mount.Info{
		Kind:             mount.KindRemote,
		AccessSequential: true,
		AccessSeek:       false,
		AccessRandom:     false,
	}
}

func (l *LotusMount) Close() error {
	return nil
}

func (l *LotusMount) Stat(_ context.Context) (mount.Stat, error) {
	size, err := l.Api.GetUnpaddedCARSize(l.PieceCid)
	if err != nil {
		return mount.Stat{}, xerrors.Errorf("failed to fetch piece size for piece %s: %w", l.PieceCid, err)
	}

	// TODO Mark false when storage deal expires.
	return mount.Stat{
		Exists: true,
		Size:   int64(size),
	}, nil
}

type readCloser struct {
	io.ReadCloser
}

var _ mount.Reader = (*readCloser)(nil)

func (r *readCloser) ReadAt(p []byte, off int64) (n int, err error) {
	return 0, xerrors.Errorf("ReadAt called but not implemented")
}

func (r *readCloser) Seek(offset int64, whence int) (int64, error) {
	return 0, xerrors.Errorf("Seek called but not implemented")
}
