package main

import (
	"crypto/rand"
	"fmt"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/filecoin-project/go-lotus/node/repo"

	"golang.org/x/xerrors"
)

type api struct {
	cmds      int32
	running   map[int32]runningNode
	runningLk sync.Mutex
	genesis   string
}

type nodeInfo struct {
	Repo    string
	ID      int32
	ApiPort int32

	FullNode string // only for storage nodes
	Storage  bool
}

func (api *api) Spawn() (nodeInfo, error) {
	dir, err := ioutil.TempDir(os.TempDir(), "lotus-")
	if err != nil {
		return nodeInfo{}, err
	}

	genParam := "--genesis=" + api.genesis
	id := atomic.AddInt32(&api.cmds, 1)
	if id == 1 {
		// make genesis
		genf, err := ioutil.TempFile(os.TempDir(), "lotus-genesis-")
		if err != nil {
			return nodeInfo{}, err
		}

		api.genesis = genf.Name()
		genParam = "--lotus-make-random-genesis=" + api.genesis

		if err := genf.Close(); err != nil {
			return nodeInfo{}, err
		}

	}

	errlogfile, err := os.OpenFile(dir+".err.log", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nodeInfo{}, err
	}
	logfile, err := os.OpenFile(dir+".out.log", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nodeInfo{}, err
	}

	mux := newWsMux()

	cmd := exec.Command("./lotus", "daemon", genParam, "--api", fmt.Sprintf("%d", 2500+id))
	cmd.Stderr = io.MultiWriter(os.Stderr, errlogfile, mux.errpw)
	cmd.Stdout = io.MultiWriter(os.Stdout, logfile, mux.outpw)
	cmd.Env = []string{"LOTUS_PATH=" + dir}
	if err := cmd.Start(); err != nil {
		return nodeInfo{}, err
	}

	info := nodeInfo{
		Repo:    dir,
		ID:      id,
		ApiPort: 2500 + id,
	}

	api.runningLk.Lock()
	api.running[id] = runningNode{
		cmd:  cmd,
		meta: info,

		mux: mux,
		stop: func() {
			defer close(mux.stop)
			defer errlogfile.Close()
			defer logfile.Close()
		},
	}
	api.runningLk.Unlock()

	time.Sleep(time.Millisecond * 750) // TODO: Something less terrible

	return info, nil
}

func (api *api) Nodes() []nodeInfo {
	api.runningLk.Lock()
	out := make([]nodeInfo, 0, len(api.running))
	for _, node := range api.running {
		out = append(out, node.meta)
	}

	api.runningLk.Unlock()

	return out
}

func (api *api) TokenFor(id int32) (string, error) {
	api.runningLk.Lock()
	defer api.runningLk.Unlock()

	rnd, ok := api.running[id]
	if !ok {
		return "", xerrors.New("no running node with this ID")
	}

	r, err := repo.NewFS(rnd.meta.Repo)
	if err != nil {
		return "", err
	}

	t, err := r.APIToken()
	if err != nil {
		return "", err
	}

	return string(t), nil
}

func (api *api) SpawnStorage(fullNodeRepo string) (nodeInfo, error) {
	dir, err := ioutil.TempDir(os.TempDir(), "lotus-storage-")
	if err != nil {
		return nodeInfo{}, err
	}

	errlogfile, err := os.OpenFile(dir+".err.log", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nodeInfo{}, err
	}
	logfile, err := os.OpenFile(dir+".out.log", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nodeInfo{}, err
	}

	initArgs := []string{"init"}
	if fullNodeRepo == api.running[1].meta.Repo {
		initArgs = []string{"init", "--actor=t0101", "--genesis-miner"}
	}

	id := atomic.AddInt32(&api.cmds, 1)
	cmd := exec.Command("./lotus-storage-miner", initArgs...)
	cmd.Stderr = io.MultiWriter(os.Stderr, errlogfile)
	cmd.Stdout = io.MultiWriter(os.Stdout, logfile)
	cmd.Env = []string{"LOTUS_STORAGE_PATH=" + dir, "LOTUS_PATH=" + fullNodeRepo}
	if err := cmd.Run(); err != nil {
		return nodeInfo{}, err
	}

	time.Sleep(time.Millisecond * 300)

	mux := newWsMux()

	cmd = exec.Command("./lotus-storage-miner", "run", "--api", fmt.Sprintf("%d", 2500+id))
	cmd.Stderr = io.MultiWriter(os.Stderr, errlogfile, mux.errpw)
	cmd.Stdout = io.MultiWriter(os.Stdout, logfile, mux.outpw)
	cmd.Env = []string{"LOTUS_STORAGE_PATH=" + dir, "LOTUS_PATH=" + fullNodeRepo}
	if err := cmd.Start(); err != nil {
		return nodeInfo{}, err
	}

	info := nodeInfo{
		Repo:    dir,
		ID:      id,
		ApiPort: 2500 + id,

		FullNode: fullNodeRepo,
		Storage:  true,
	}

	api.runningLk.Lock()
	api.running[id] = runningNode{
		cmd:  cmd,
		meta: info,

		mux: mux,
		stop: func() {
			defer close(mux.stop)
			defer errlogfile.Close()
			defer logfile.Close()
		},
	}
	api.runningLk.Unlock()

	time.Sleep(time.Millisecond * 750) // TODO: Something less terrible

	return info, nil
}

func (api *api) CreateRandomFile(size int64) (string, error) {
	tf, err := ioutil.TempFile(os.TempDir(), "pond-random-")
	if err != nil {
		return "", err
	}

	_, err = io.CopyN(tf, rand.Reader, size)
	if err != nil {
		return "", err
	}

	if err := tf.Close(); err != nil {
		return "", err
	}

	return tf.Name(), nil
}

type client struct {
	Nodes func() []nodeInfo
}

func apiClient() (*client, error) {
	c := &client{}
	if _, err := jsonrpc.NewClient("ws://"+listenAddr+"/rpc/v0", "Pond", c, nil); err != nil {
		return nil, err
	}
	return c, nil
}
