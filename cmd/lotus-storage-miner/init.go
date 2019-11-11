package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p-core/crypto"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
)

var initCmd = &cli.Command{
	Name:  "init",
	Usage: "Initialize a lotus storage miner repo",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "actor",
			Usage: "specify the address of an already created miner actor",
		},
		&cli.BoolFlag{
			Name:   "genesis-miner",
			Usage:  "enable genesis mining (DON'T USE ON BOOTSTRAPPED NETWORK)",
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:  "create-worker-key",
			Usage: "create separate worker key",
		},
		&cli.StringFlag{
			Name:    "worker",
			Aliases: []string{"w"},
			Usage:   "worker key to use (overrides --create-worker-key)",
		},
		&cli.StringFlag{
			Name:    "owner",
			Aliases: []string{"o"},
			Usage:   "owner key to use",
		},
		&cli.Uint64Flag{
			Name:  "sector-size",
			Usage: "specify sector size to use",
			Value: build.SectorSizes[0],
		},
	},
	Action: func(cctx *cli.Context) error {
		log.Info("Initializing lotus storage miner")

		log.Info("Checking proof parameters")
		if err := build.GetParams(true); err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		log.Info("Checking if repo exists")

		repoPath := cctx.String(FlagStorageRepo)
		r, err := repo.NewFS(repoPath)
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}
		if ok {
			return xerrors.Errorf("repo at '%s' is already initialized", cctx.String(FlagStorageRepo))
		}

		log.Info("Trying to connect to full node RPC")

		api, closer, err := lcli.GetFullNodeAPI(cctx) // TODO: consider storing full node address in config
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		log.Info("Checking full node version")

		v, err := api.Version(ctx)
		if err != nil {
			return err
		}

		if v.APIVersion&build.MinorMask != build.APIVersion&build.MinorMask {
			return xerrors.Errorf("Remote API version didn't match (local %x, remote %x)", build.APIVersion, v.APIVersion)
		}

		log.Info("Initializing repo")

		if err := r.Init(repo.RepoStorageMiner); err != nil {
			return err
		}

		if err := storageMinerInit(ctx, cctx, api, r); err != nil {
			fmt.Printf("ERROR: failed to initialize lotus-storage-miner: %s\n", err)
			fmt.Println("Cleaning up after attempt...")
			if err := os.RemoveAll(repoPath); err != nil {
				fmt.Println("ERROR: failed to clean up failed storage repo: ", err)
			}
			return fmt.Errorf("storage-miner init failed")
		}

		// TODO: Point to setting storage price, maybe do it interactively or something
		log.Info("Storage miner successfully created, you can now start it with 'lotus-storage-miner run'")

		return nil
	},
}

func storageMinerInit(ctx context.Context, cctx *cli.Context, api api.FullNode, r repo.Repo) error {
	lr, err := r.Lock(repo.RepoStorageMiner)
	if err != nil {
		return err
	}
	defer lr.Close()

	log.Info("Initializing libp2p identity")

	p2pSk, err := makeHostKey(lr)
	if err != nil {
		return err
	}

	peerid, err := peer.IDFromPrivateKey(p2pSk)
	if err != nil {
		return err
	}

	var addr address.Address
	if act := cctx.String("actor"); act != "" {
		a, err := address.NewFromString(act)
		if err != nil {
			return err
		}

		if err := configureStorageMiner(ctx, api, a, peerid, cctx.Bool("genesis-miner")); err != nil {
			return xerrors.Errorf("failed to configure storage miner: %w", err)
		}

		addr = a
	} else {
		a, err := createStorageMiner(ctx, api, peerid, cctx)
		if err != nil {
			return err
		}

		addr = a
	}

	log.Infof("Created new storage miner: %s", addr)

	ds, err := lr.Datastore("/metadata")
	if err != nil {
		return err
	}
	if err := ds.Put(datastore.NewKey("miner-address"), addr.Bytes()); err != nil {
		return err
	}

	return nil
}

func makeHostKey(lr repo.LockedRepo) (crypto.PrivKey, error) {
	pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, err
	}

	kbytes, err := pk.Bytes()
	if err != nil {
		return nil, err
	}

	if err := ks.Put("libp2p-host", types.KeyInfo{
		Type:       "libp2p-host",
		PrivateKey: kbytes,
	}); err != nil {
		return nil, err
	}

	return pk, nil
}

func configureStorageMiner(ctx context.Context, api api.FullNode, addr address.Address, peerid peer.ID, genmine bool) error {
	if genmine {
		log.Warn("Starting genesis mining. This shouldn't happen when connecting to the real network.")
		// We may be one of genesis miners, start mining before trying to do any chain operations
		// (otherwise our messages won't be mined)
		if err := api.MinerRegister(ctx, addr); err != nil {
			return err
		}

		defer func() {
			if err := api.MinerUnregister(ctx, addr); err != nil {
				log.Errorf("failed to call api.MinerUnregister: %s", err)
			}
		}()
	}

	// This really just needs to be an api call at this point...
	recp, err := api.StateCall(ctx, &types.Message{
		To:     addr,
		From:   addr,
		Method: actors.MAMethods.GetWorkerAddr,
	}, nil)
	if err != nil {
		return xerrors.Errorf("failed to get worker address: %w", err)
	}

	if recp.ExitCode != 0 {
		return xerrors.Errorf("getWorkerAddr returned exit code %d", recp.ExitCode)
	}

	waddr, err := address.NewFromBytes(recp.Return)
	if err != nil {
		return xerrors.Errorf("getWorkerAddr returned bad address: %w", err)
	}

	enc, err := actors.SerializeParams(&actors.UpdatePeerIDParams{PeerID: peerid})
	if err != nil {
		return err
	}

	msg := &types.Message{
		To:       addr,
		From:     waddr,
		Method:   actors.MAMethods.UpdatePeerID,
		Params:   enc,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(100000000),
	}

	smsg, err := api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return err
	}

	log.Info("Waiting for message: ", smsg.Cid())
	ret, err := api.StateWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return err
	}

	if ret.Receipt.ExitCode != 0 {
		return xerrors.Errorf("update peer id message failed with exit code %d", ret.Receipt.ExitCode)
	}

	return nil
}

func createStorageMiner(ctx context.Context, api api.FullNode, peerid peer.ID, cctx *cli.Context) (addr address.Address, err error) {
	log.Info("Creating StorageMarket.CreateStorageMiner message")

	var owner address.Address
	if cctx.String("owner") != "" {
		owner, err = address.NewFromString(cctx.String("owner"))
	} else {
		owner, err = api.WalletDefaultAddress(ctx)
	}
	if err != nil {
		return address.Undef, err
	}

	ssize := cctx.Uint64("sector-size")

	worker := owner
	if cctx.String("worker") != "" {
		worker, err = address.NewFromString(cctx.String("worker"))
	} else if cctx.Bool("create-worker-key") { // TODO: Do we need to force this if owner is Secpk?
		worker, err = api.WalletNew(ctx, types.KTBLS)
	}
	// TODO: Transfer some initial funds to worker
	if err != nil {
		return address.Undef, err
	}

	collateral, err := api.StatePledgeCollateral(ctx, nil)
	if err != nil {
		return address.Undef, err
	}

	params, err := actors.SerializeParams(&actors.CreateStorageMinerParams{
		Owner:      owner,
		Worker:     worker,
		SectorSize: ssize,
		PeerID:     peerid,
	})
	if err != nil {
		return address.Undef, err
	}

	createStorageMinerMsg := &types.Message{
		To:    actors.StoragePowerAddress,
		From:  owner,
		Value: collateral,

		Method: actors.SPAMethods.CreateStorageMiner,
		Params: params,

		GasLimit: types.NewInt(10000000),
		GasPrice: types.NewInt(0),
	}

	signed, err := api.MpoolPushMessage(ctx, createStorageMinerMsg)
	if err != nil {
		return address.Undef, err
	}

	log.Infof("Pushed StorageMarket.CreateStorageMiner, %s to Mpool", signed.Cid())
	log.Infof("Waiting for confirmation")

	mw, err := api.StateWaitMsg(ctx, signed.Cid())
	if err != nil {
		return address.Undef, err
	}

	if mw.Receipt.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("create storage miner failed: exit code %d", mw.Receipt.ExitCode)
	}

	addr, err = address.NewFromBytes(mw.Receipt.Return)
	if err != nil {
		return address.Undef, err
	}

	log.Infof("New storage miners address is: %s", addr)
	return addr, nil
}
