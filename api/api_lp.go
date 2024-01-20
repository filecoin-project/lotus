package api

import (
	"context"
	"net/http"
	"net/url"

	"github.com/filecoin-project/go-address"
)

type LotusProvider interface {
	Version(context.Context) (Version, error) //perm:admin

	AllocatePieceToSector(ctx context.Context, maddr address.Address, piece PieceDealInfo, rawSize int64, source url.URL, header http.Header) (SectorOffset, error) //perm:write

	// Trigger shutdown
	Shutdown(context.Context) error //perm:admin
}
