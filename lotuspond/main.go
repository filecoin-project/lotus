package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
	"github.com/filecoin-project/go-lotus/node/repo"
)

const listenAddr = "127.0.0.1:2222"

type runningNode struct {
	cmd  *exec.Cmd
	meta nodeInfo

	stop func()
}

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

	errlogfile, err := os.OpenFile(dir + ".err.log", os.O_CREATE | os.O_WRONLY, 0644)
	if err != nil {
		return nodeInfo{}, err
	}
	logfile, err := os.OpenFile(dir + ".out.log", os.O_CREATE | os.O_WRONLY, 0644)
	if err != nil {
		return nodeInfo{}, err
	}

	cmd := exec.Command("./lotus", "daemon", genParam, "--api", fmt.Sprintf("%d", 2500+id))
	cmd.Stderr = io.MultiWriter(os.Stderr, errlogfile)
	cmd.Stdout = io.MultiWriter(os.Stdout, logfile)
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

		stop: func() {
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
		return "", errors.New("no running node with this ID")
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

func main() {
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Pond", &api{running: map[int32]runningNode{}})

	http.Handle("/", http.FileServer(http.Dir("lotuspond/front/build")))
	http.Handle("/rpc/v0", rpcServer)

	fmt.Printf("Listening on http://%s\n", listenAddr)
	http.ListenAndServe(listenAddr, nil)
}
