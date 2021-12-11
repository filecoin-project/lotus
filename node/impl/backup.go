package impl

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/lib/backupds"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func backup(ctx context.Context, mds dtypes.MetadataDS, fpath string) error {
	bb, ok := os.LookupEnv("LOTUS_BACKUP_BASE_PATH")
	if !ok {
		return xerrors.Errorf("LOTUS_BACKUP_BASE_PATH env var not set")
	}

	bds, ok := mds.(*backupds.Datastore)
	if !ok {
		return xerrors.Errorf("expected a backup datastore")
	}

	bb, err := homedir.Expand(bb)
	if err != nil {
		return xerrors.Errorf("expanding base path: %w", err)
	}

	bb, err = filepath.Abs(bb)
	if err != nil {
		return xerrors.Errorf("getting absolute base path: %w", err)
	}

	fpath, err = homedir.Expand(fpath)
	if err != nil {
		return xerrors.Errorf("expanding file path: %w", err)
	}

	fpath, err = filepath.Abs(fpath)
	if err != nil {
		return xerrors.Errorf("getting absolute file path: %w", err)
	}

	if !strings.HasPrefix(fpath, bb) {
		return xerrors.Errorf("backup file name (%s) must be inside base path (%s)", fpath, bb)
	}

	out, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return xerrors.Errorf("open %s: %w", fpath, err)
	}

	if err := bds.Backup(ctx, out); err != nil {
		if cerr := out.Close(); cerr != nil {
			log.Errorw("error closing backup file while handling backup error", "closeErr", cerr, "backupErr", err)
		}
		return xerrors.Errorf("backup error: %w", err)
	}

	if err := out.Close(); err != nil {
		return xerrors.Errorf("closing backup file: %w", err)
	}

	return nil
}
