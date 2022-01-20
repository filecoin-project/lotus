package lp2p

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/fx"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/network"
	rcmgr "github.com/libp2p/go-libp2p-resource-manager"

	"github.com/filecoin-project/lotus/node/repo"
)

func ResourceManager(lc fx.Lifecycle, repo repo.LockedRepo) (network.ResourceManager, error) {
	var limiter *rcmgr.BasicLimiter
	var opts []rcmgr.Option

	repoPath := repo.Path()

	// create limiter -- parse $repo/limits.json if exists
	limitsFile := filepath.Join(repoPath, "limits.json")
	limitsIn, err := os.Open(limitsFile)
	switch {
	case err == nil:
		defer limitsIn.Close() //nolint:errcheck
		limiter, err = rcmgr.NewDefaultLimiterFromJSON(limitsIn)
		if err != nil {
			return nil, fmt.Errorf("error parsing limit file: %w", err)
		}

	case errors.Is(err, os.ErrNotExist):
		limiter = rcmgr.NewDefaultLimiter()

	default:
		return nil, err
	}

	// TODO: also set appropriate default limits for lotus protocols
	libp2p.SetDefaultServiceLimits(limiter)

	if os.Getenv("LOTUS_DEBUG_RCMGR") != "" {
		debugPath := filepath.Join(repoPath, "debug")
		if err := os.MkdirAll(debugPath, 0755); err != nil {
			return nil, fmt.Errorf("error creating debug directory: %w", err)
		}
		traceFile := filepath.Join(debugPath, "rcmgr.json.gz")
		opts = append(opts, rcmgr.WithTrace(traceFile))
	}

	mgr, err := rcmgr.NewResourceManager(limiter, opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating resource manager: %w", err)
	}

	return mgr, nil
}

func ResourceManagerOption(mgr network.ResourceManager) Libp2pOpts {
	return Libp2pOpts{
		Opts: []libp2p.Option{libp2p.ResourceManager(mgr)},
	}
}
