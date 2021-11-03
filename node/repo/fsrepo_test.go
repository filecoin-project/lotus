//stm: #unit
package repo

import (
	"io/ioutil"
	"os"
	"testing"
)

func genFsRepo(t *testing.T) (*FsRepo, func()) {
	path, err := ioutil.TempDir("", "lotus-repo-")
	if err != nil {
		t.Fatal(err)
	}

	//stm: @REPO_FS_001
	repo, err := NewFS(path)
	if err != nil {
		t.Fatal(err)
	}

	//stm: @REPO_FS_002
	err = repo.Init(FullNode)
	if err != ErrRepoExists && err != nil {
		t.Fatal(err)
	}
	return repo, func() {
		_ = os.RemoveAll(path)
	}
}

func TestFsBasic(t *testing.T) {
	repo, closer := genFsRepo(t)
	defer closer()
	//stm: @REPO_FS_003
	basicTest(t, repo)
}
