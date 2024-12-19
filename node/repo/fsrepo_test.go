package repo

import (
	"testing"
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
