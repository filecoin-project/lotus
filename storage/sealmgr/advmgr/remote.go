package advmgr

import (
	"context"
	"net/http"

	"github.com/filecoin-project/specs-actors/actors/abi"
	storage2 "github.com/filecoin-project/specs-storage/storage"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
)

type remote struct {
	api.WorkerApi
}

func (r *remote) AddPiece(ctx context.Context, sector abi.SectorNumber, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storage2.Data) (abi.PieceInfo, error) {
	return abi.PieceInfo{},xerrors.New("unsupported")
}

func ConnectRemote(ctx context.Context, fa api.FullNode, url string) (*remote, error) {
	token, err := fa.AuthNew(ctx, []api.Permission{"admin"})
	if err != nil {
		return nil, xerrors.Errorf("creating auth token for remote connection: %w", err)
	}

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(token))

	wapi, close, err := client.NewWorkerRPC(url, headers)
	if err != nil {
		return nil, xerrors.Errorf("creating jsonrpc client: %w", err)
	}
	_ = close // TODO

	return &remote{wapi}, nil
}

var _ Worker = &remote{}
