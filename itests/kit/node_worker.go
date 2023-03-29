package kit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// TestWorker represents a worker enrolled in an Ensemble.
type TestWorker struct {
	api.Worker

	t *testing.T

	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node
	ListenAddr multiaddr.Multiaddr

	Stop func(context.Context) error

	FetchHandler   http.HandlerFunc
	MinerNode      *TestMiner
	RemoteListener net.Listener

	options nodeOpts
}

func (tm *TestWorker) AddStorage(ctx context.Context, t *testing.T, conf func(*storiface.LocalStorageMeta)) storiface.ID {
	p := t.TempDir()

	if err := os.MkdirAll(p, 0755); err != nil {
		if !os.IsExist(err) {
			require.NoError(t, err)
		}
	}

	_, err := os.Stat(filepath.Join(p, metaFile))
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}

	cfg := &storiface.LocalStorageMeta{
		ID:       storiface.ID(uuid.New().String()),
		Weight:   10,
		CanSeal:  false,
		CanStore: false,
	}

	conf(cfg)

	if !(cfg.CanStore || cfg.CanSeal) {
		t.Fatal("must specify at least one of CanStore or cfg.CanSeal")
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(p, metaFile), b, 0644)
	require.NoError(t, err)

	err = tm.StorageAddLocal(ctx, p)
	require.NoError(t, err)

	return cfg.ID
}
