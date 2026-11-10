package impl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/lib/backupds"
)

func TestBackupBasePath(t *testing.T) {
	ctx := context.Background()

	root := t.TempDir()
	base := filepath.Join(root, "backups")
	sibling := filepath.Join(root, "backups-old")
	require.NoError(t, os.Mkdir(base, 0755))
	require.NoError(t, os.Mkdir(sibling, 0755))
	t.Setenv("LOTUS_BACKUP_BASE_PATH", base)

	bds, err := backupds.Wrap(datastore.NewMapDatastore(), backupds.NoLogdir)
	require.NoError(t, err)

	inside := filepath.Join(base, "node.bak")
	require.NoError(t, backup(ctx, bds, inside))
	require.FileExists(t, inside)

	// Sharing the base path's string prefix does not put a path inside it.
	for _, outside := range []string{
		filepath.Join(sibling, "node.bak"),
		base + ".bak",
		filepath.Join(base, "..", "node.bak"),
	} {
		require.ErrorContains(t, backup(ctx, bds, outside), "must be inside base path", outside)
		require.NoFileExists(t, outside)
	}
}
