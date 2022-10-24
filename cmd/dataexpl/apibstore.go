package main

import (
	"context"
	"net/url"
	"path"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
)

type apiBstoreServer struct {
	remoteAddr *url.URL
	nd         api.FullNode

	stores map[uuid.UUID]blockstore.Blockstore

	lk sync.RWMutex
}

func (a *apiBstoreServer) MakeRemoteBstore(ctx context.Context, bs blockstore.Blockstore) (api.RemoteStoreID, error) {
	a.lk.Lock()
	defer a.lk.Unlock()

	id := uuid.New()
	a.stores[id] = bs

	au := *a.remoteAddr

	switch au.Scheme {
	case "http":
		au.Scheme = "ws"
	case "https":
		au.Scheme = "wss"
	}

	au.Path = path.Join(au.Path, "/rest/v0/store/"+id.String())

	conn, _, err := websocket.DefaultDialer.Dial(au.String(), nil)
	if err != nil {
		return api.RemoteStoreID{}, xerrors.Errorf("dial ws (%s): %w", au.String(), err)
	}

	_ = blockstore.HandleNetBstoreWS(ctx, bs, conn)

	return id, nil
}
