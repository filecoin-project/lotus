package repo

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func genFsRepo(t *testing.T) *FsRepo {
	path := t.TempDir()

	repo, err := NewFS(path)
	if err != nil {
		t.Fatal(err)
	}

	err = repo.Init(FullNode)
	if err != ErrRepoExists && err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestFsBasic(t *testing.T) {
	repo := genFsRepo(t)
	basicTest(t, repo)
}

func TestFullNodeConfigRequiresIndexerRetention(t *testing.T) {
	repo := genFsRepo(t)
	lr, err := repo.Lock(FullNode)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, lr.Close()) })

	_, err = lr.Config()
	require.NoError(t, err)

	cfg := "[Chainstore]\n  EnableSplitstore = true\n[ChainIndexer]\n  EnableIndexer = true\n"
	require.NoError(t, os.WriteFile(repo.configPath, []byte(cfg), 0644))
	_, err = lr.Config()
	require.ErrorContains(t, err, "without ChainIndexer.GCRetentionEpochs")
	require.ErrorContains(t, lr.SetConfig(func(interface{}) {}), "without ChainIndexer.GCRetentionEpochs")
}
