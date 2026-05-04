//go:build !nodaemon

package lotus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"

	"github.com/filecoin-project/lotus/node/repo"
)

func TestRemoveExistingChainRemovesF3Data(t *testing.T) {
	repoPath := t.TempDir()

	r, err := repo.NewFS(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Init(repo.FullNode); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	lr, err := r.Lock(repo.FullNode)
	if err != nil {
		t.Fatal(err)
	}
	mds, err := lr.Datastore(ctx, "/metadata")
	if err != nil {
		t.Fatal(err)
	}
	legacyF3Ds := namespace.Wrap(mds, datastore.NewKey("/f3"))
	if err := legacyF3Ds.Put(ctx, datastore.NewKey("/legacy"), []byte("stale")); err != nil {
		t.Fatal(err)
	}
	if err := mds.Put(ctx, datastore.NewKey("/keep"), []byte("keep")); err != nil {
		t.Fatal(err)
	}
	if err := lr.Close(); err != nil {
		t.Fatal(err)
	}

	removedDirs := []string{
		filepath.Join(repoPath, "datastore", "chain"),
		filepath.Join(repoPath, "datastore", "splitstore"),
		filepath.Join(repoPath, "datastore", "f3"),
		filepath.Join(repoPath, "f3"),
	}
	for _, dir := range removedDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "stale"), []byte("stale"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeExistingChain(nil, r); err != nil {
		t.Fatal(err)
	}

	for _, dir := range removedDirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, got err: %v", dir, err)
		}
	}

	lr, err = r.Lock(repo.FullNode)
	if err != nil {
		t.Fatal(err)
	}
	defer lr.Close() //nolint:errcheck

	mds, err = lr.Datastore(ctx, "/metadata")
	if err != nil {
		t.Fatal(err)
	}
	legacyF3Ds = namespace.Wrap(mds, datastore.NewKey("/f3"))

	if _, err := legacyF3Ds.Get(ctx, datastore.NewKey("/legacy")); !errors.Is(err, datastore.ErrNotFound) {
		t.Fatalf("expected legacy F3 metadata to be removed, got err: %v", err)
	}
	if _, err := mds.Get(ctx, datastore.NewKey("/keep")); err != nil {
		t.Fatalf("expected unrelated metadata to remain: %v", err)
	}
}
