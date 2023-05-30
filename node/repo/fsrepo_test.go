// stm: #unit
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
	//stm: @NODE_FS_REPO_LOCK_001,@NODE_FS_REPO_LOCK_002,@NODE_FS_REPO_UNLOCK_001
	//stm: @NODE_FS_REPO_SET_API_ENDPOINT_001, @NODE_FS_REPO_GET_API_ENDPOINT_001
	//stm: @NODE_FS_REPO_GET_CONFIG_001, @NODE_FS_REPO_SET_CONFIG_001
	//stm: @NODE_FS_REPO_LIST_KEYS_001, @NODE_FS_REPO_PUT_KEY_001
	//stm: @NODE_FS_REPO_GET_KEY_001, NODE_FS_REPO_DELETE_KEY_001
	repo := genFsRepo(t)
	basicTest(t, repo)
}
