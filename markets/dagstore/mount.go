package dagstore

import (
	"context"
	"net/url"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore/mount"
)

const lotusScheme = "lotus"

var _ mount.Mount = (*LotusMount)(nil)

// mountTemplate returns a templated LotusMount containing the supplied API.
//
// It is called when registering a mount type with the mount registry
// of the DAG store. It is used to reinstantiate mounts after a restart.
//
// When the registry needs to deserialize a mount it clones the template then
// calls Deserialize on the cloned instance, which will have a reference to the
// lotus mount API supplied here.
func mountTemplate(api MinerAPI) *LotusMount {
	return &LotusMount{API: api}
}

// LotusMount is a DAGStore mount implementation that fetches deal data
// from a PieceCID.
type LotusMount struct {
	API      MinerAPI
	PieceCid cid.Cid
}

func NewLotusMount(pieceCid cid.Cid, api MinerAPI) (*LotusMount, error) {
	return &LotusMount{
		PieceCid: pieceCid,
		API:      api,
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
	return l.API.FetchUnsealedPiece(ctx, l.PieceCid)
}

func (l *LotusMount) Info() mount.Info {
	return mount.Info{
		Kind:             mount.KindRemote,
		AccessSequential: true,
		AccessSeek:       true,
		AccessRandom:     true,
	}
}

func (l *LotusMount) Close() error {
	return nil
}

func (l *LotusMount) Stat(ctx context.Context) (mount.Stat, error) {
	size, err := l.API.GetUnpaddedCARSize(ctx, l.PieceCid)
	if err != nil {
		return mount.Stat{}, xerrors.Errorf("failed to fetch piece size for piece %s: %w", l.PieceCid, err)
	}
	isUnsealed, err := l.API.IsUnsealed(ctx, l.PieceCid)
	if err != nil {
		return mount.Stat{}, xerrors.Errorf("failed to verify if we have the unsealed piece %s: %w", l.PieceCid, err)
	}

	// TODO Mark false when storage deal expires.
	return mount.Stat{
		Exists: true,
		Size:   int64(size),
		Ready:  isUnsealed,
	}, nil
}
