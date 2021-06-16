package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	genesis2 "github.com/filecoin-project/lotus/chain/gen/genesis"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/genesis"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
}

func (api *api) Spawn() (nodeInfo, error) {
	dir, err := ioutil.TempDir(os.TempDir(), "lotus-")
	if err != nil {
		return nodeInfo{}, err
	}

	params := []string{"daemon", "--bootstrap=false"}
	genParam := "--genesis=" + api.genesis

	id := atomic.AddInt32(&api.cmds, 1)
	if id == 1 {
		// preseal

		genMiner, err := address.NewIDAddress(genesis2.MinerStart)
		if err != nil {
			return nodeInfo{}, err
		}

		sbroot := filepath.Join(dir, "preseal")
		genm, ki, err := seed.PreSeal(genMiner, abi.RegisteredSealProof_StackedDrg2KiBV1, 0, 2, sbroot, []byte("8"), nil, false)
		if err != nil {
			return nodeInfo{}, xerrors.Errorf("preseal failed: %w", err)
		}

		if err := seed.WriteGenesisMiner(genMiner, sbroot, genm, ki); err != nil {
			return nodeInfo{}, xerrors.Errorf("failed to write genminer info: %w", err)
		}
		params = append(params, "--import-key="+filepath.Join(dir, "preseal", "pre-seal-t01000.key"))
		params = append(params, "--genesis-template="+filepath.Join(dir, "preseal", "genesis-template.json"))

		// Create template

		var template genesis.Template
		template.Miners = append(template.Miners, *genm)
		template.Accounts = append(template.Accounts, genesis.Actor{
			Type:    genesis.TAccount,
			Balance: types.FromFil(5000000),
			Meta:    (&genesis.AccountMeta{Owner: genm.Owner}).ActorMeta(),
		})
		template.VerifregRootKey = gen.DefaultVerifregRootkeyActor
		template.RemainderAccount = gen.DefaultRemainderAccountActor
		template.NetworkName = "pond-" + uuid.New().String()

		tb, err := json.Marshal(&template)
		if err != nil {
			return nodeInfo{}, xerrors.Errorf("marshal genesis template: %w", err)
		}

		if err := ioutil.WriteFile(filepath.Join(dir, "preseal", "genesis-template.json"), tb, 0664); err != nil {
			return nodeInfo{}, xerrors.Errorf("write genesis template: %w", err)
		}

		// make genesis
		genf, err := ioutil.TempFile(os.TempDir(), "lotus-genesis-")
		if err != nil {
			return nodeInfo{}, err
		}

		api.genesis = genf.Name()
		genParam = "--lotus-make-genesis=" + api.genesis

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
	confStr := fmt.Sprintf("[API]\nListenAddress = \"/ip4/127.0.0.1/tcp/%d/http\"\n", 2500+id)

	err = ioutil.WriteFile(filepath.Join(dir, "config.toml"), []byte(confStr), 0700)
	if err != nil {
		return nodeInfo{}, err
	}

	cmd := exec.Command("./lotus", append(params, genParam)...)

	cmd.Stderr = io.MultiWriter(os.Stderr, errlogfile, mux.errpw)
	cmd.Stdout = io.MultiWriter(os.Stdout, logfile, mux.outpw)
	cmd.Env = append(os.Environ(), "LOTUS_PATH="+dir)
	if err := cmd.Start(); err != nil {
		return nodeInfo{}, err
	}

	info := nodeInfo{
		Repo:    dir,
		ID:      id,
		APIPort: 2500 + id,
		State:   NodeRunning,
	}

	api.runningLk.Lock()
	api.running[id] = &runningNode{
		cmd:  cmd,
		meta: info,

		mux: mux,
		stop: func() {
			cmd.Process.Signal(os.Interrupt)
			cmd.Process.Wait()

			api.runningLk.Lock()
			api.running[id].meta.State = NodeStopped
			api.runningLk.Unlock()
		},
	}
	api.runningLk.Unlock()

	time.Sleep(time.Millisecond * 750) // TODO: Something less terrible

	return info, nil
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

	initArgs := []string{"init", "--nosync"}
	if fullNodeRepo == api.running[1].meta.Repo {
		presealPrefix := filepath.Join(fullNodeRepo, "preseal")
		initArgs = []string{"init", "--actor=t01000", "--genesis-miner", "--pre-sealed-sectors=" + presealPrefix, "--pre-sealed-metadata=" + filepath.Join(presealPrefix, "pre-seal-t01000.json")}
	}

	id := atomic.AddInt32(&api.cmds, 1)
	cmd := exec.Command("./lotus-miner", initArgs...)
	cmd.Stderr = io.MultiWriter(os.Stderr, errlogfile)
	cmd.Stdout = io.MultiWriter(os.Stdout, logfile)
	cmd.Env = append(os.Environ(), "LOTUS_MINER_PATH="+dir, "LOTUS_PATH="+fullNodeRepo)
	if err := cmd.Run(); err != nil {
		return nodeInfo{}, err
	}

	time.Sleep(time.Millisecond * 300)

	mux := newWsMux()

	cmd = exec.Command("./lotus-miner", "run", "--miner-api", fmt.Sprintf("%d", 2500+id), "--nosync")
	cmd.Stderr = io.MultiWriter(os.Stderr, errlogfile, mux.errpw)
	cmd.Stdout = io.MultiWriter(os.Stdout, logfile, mux.outpw)
	cmd.Env = append(os.Environ(), "LOTUS_MINER_PATH="+dir, "LOTUS_PATH="+fullNodeRepo)
	if err := cmd.Start(); err != nil {
		return nodeInfo{}, err
	}

	info := nodeInfo{
		Repo:    dir,
		ID:      id,
		APIPort: 2500 + id,
		State:   NodeRunning,

		FullNode: fullNodeRepo,
		Storage:  true,
	}

	api.runningLk.Lock()
	api.running[id] = &runningNode{
		cmd:  cmd,
		meta: info,

		mux: mux,
		stop: func() {
			cmd.Process.Signal(os.Interrupt)
			cmd.Process.Wait()

			api.runningLk.Lock()
			api.running[id].meta.State = NodeStopped
			api.runningLk.Unlock()
		},
	}
	api.runningLk.Unlock()

	time.Sleep(time.Millisecond * 750) // TODO: Something less terrible

	return info, nil
}

func (api *api) RestartNode(id int32) (nodeInfo, error) {
	api.runningLk.Lock()
	defer api.runningLk.Unlock()
	nd, ok := api.running[id]

	if !ok {
		return nodeInfo{}, xerrors.New("node not found")
	}

	if nd.meta.State != NodeStopped {
		return nodeInfo{}, xerrors.New("node not stopped")
	}

	var cmd *exec.Cmd
	if nd.meta.Storage {
		cmd = exec.Command("./lotus-miner", "run", "--miner-api", fmt.Sprintf("%d", 2500+id), "--nosync")
	} else {
		cmd = exec.Command("./lotus", "daemon", "--api", fmt.Sprintf("%d", 2500+id))
	}

	cmd.Stderr = nd.cmd.Stderr // recycle old vars
	cmd.Stdout = nd.cmd.Stdout
	cmd.Env = nd.cmd.Env

	if err := cmd.Start(); err != nil {
		return nodeInfo{}, err
	}

	nd.cmd = cmd

	nd.stop = func() {
		cmd.Process.Signal(os.Interrupt)
		cmd.Process.Wait()

		api.runningLk.Lock()
		api.running[id].meta.State = NodeStopped
		api.runningLk.Unlock()
	}

	nd.meta.State = NodeRunning

	time.Sleep(time.Millisecond * 750) // TODO: Something less terrible

	return nd.meta, nil
}
