package sectorstorage

import (
	"context"
	"net/http"

	"github.com/filecoin-project/specs-actors/actors/abi"
	storage2 "github.com/filecoin-project/specs-storage/storage"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/lib/jsonrpc"
)

type remote struct {
	api.WorkerApi
	closer jsonrpc.ClientCloser
}

func (r *remote) NewSector(ctx context.Context, sector abi.SectorID) error {
	return xerrors.New("unsupported")
}

func (r *remote) AddPiece(ctx context.Context, sector abi.SectorID, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storage2.Data) (abi.PieceInfo, error) {
	return abi.PieceInfo{}, xerrors.New("unsupported")
}

func ConnectRemote(ctx context.Context, fa api.Common, url string) (*remote, error) {
	token, err := fa.AuthNew(ctx, []api.Permission{"admin"})
	if err != nil {
		return nil, xerrors.Errorf("creating auth token for remote connection: %w", err)
	}

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(token))

	wapi, closer, err := client.NewWorkerRPC(url, headers)
	if err != nil {
		return nil, xerrors.Errorf("creating jsonrpc client: %w", err)
	}

	return &remote{wapi, closer}, nil
}

func (r *remote) Close() error {
	r.closer()
	return nil
}

var _ Worker = &remote{}
