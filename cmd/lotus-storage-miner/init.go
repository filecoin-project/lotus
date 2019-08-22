package main

import (
	"context"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	lcli "github.com/filecoin-project/go-lotus/cli"
	"github.com/filecoin-project/go-lotus/node/repo"
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
			Name:  "genesis-miner",
			Usage: "enable genesis mining (DON'T USE ON BOOTSTRAPPED NETWORK)",
		},
	},
	Action: func(cctx *cli.Context) error {
		log.Info("Initializing lotus storage miner")
		log.Info("Checking if repo exists")

		r, err := repo.NewFS(cctx.String(FlagStorageRepo))
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

		api, err := lcli.GetFullNodeAPI(cctx) // TODO: consider storing full node address in config
		if err != nil {
			return err
		}
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

		if err := r.Init(); err != nil {
			return err
		}

		lr, err := r.Lock()
		if err != nil {
			return err
		}
		defer lr.Close()

		log.Info("Initializing libp2p identity")

		p2pSk, err := lr.Libp2pIdentity()
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
			a, err := createStorageMiner(ctx, api, peerid)
			if err != nil {
				return err
			}

			addr = a
		}

		ds, err := lr.Datastore("/metadata")
		if err != nil {
			return err
		}
		if err := ds.Put(datastore.NewKey("miner-address"), addr.Bytes()); err != nil {
			return err
		}

		// TODO: Point to setting storage price, maybe do it interactively or something
		log.Info("Storage miner successfully created, you can now start it with 'lotus-storage-miner run'")

		return nil
	},
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
	recp, err := api.ChainCall(ctx, &types.Message{
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

	nonce, err := api.MpoolGetNonce(ctx, waddr)
	if err != nil {
		return err
	}

	msg := &types.Message{
		To:       addr,
		From:     waddr,
		Method:   actors.MAMethods.UpdatePeerID,
		Params:   enc,
		Nonce:    nonce,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000),
	}

	smsg, err := api.WalletSignMessage(ctx, waddr, msg)
	if err != nil {
		return err
	}

	if err := api.MpoolPush(ctx, smsg); err != nil {
		return err
	}

	log.Info("Waiting for message: ", smsg.Cid())
	ret, err := api.ChainWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return err
	}

	if ret.Receipt.ExitCode != 0 {
		return xerrors.Errorf("update peer id message failed with exit code %d", ret.Receipt.ExitCode)
	}

	return nil
}

func createStorageMiner(ctx context.Context, api api.FullNode, peerid peer.ID) (address.Address, error) {
	log.Info("Creating StorageMarket.CreateStorageMiner message")

	defOwner, err := api.WalletDefaultAddress(ctx)
	if err != nil {
		return address.Undef, err
	}

	nonce, err := api.MpoolGetNonce(ctx, defOwner)
	if err != nil {
		return address.Undef, err
	}

	k, err := api.WalletNew(ctx, types.KTBLS)
	if err != nil {
		return address.Undef, err
	}

	collateral := types.NewInt(1000) // TODO: Get this from params

	params, err := actors.SerializeParams(actors.CreateStorageMinerParams{
		Owner:      defOwner,
		Worker:     k,
		SectorSize: types.NewInt(actors.SectorSize),
		PeerID:     peerid,
	})
	if err != nil {
		return address.Undef, err
	}

	createStorageMinerMsg := types.Message{
		To:    actors.StorageMarketAddress,
		From:  defOwner,
		Nonce: nonce,
		Value: collateral,

		Method: actors.SMAMethods.CreateStorageMiner,
		Params: params,

		GasLimit: types.NewInt(10000),
		GasPrice: types.NewInt(0),
	}

	unsigned, err := createStorageMinerMsg.Serialize()
	if err != nil {
		return address.Undef, err
	}

	log.Info("Signing StorageMarket.CreateStorageMiner")

	sig, err := api.WalletSign(ctx, defOwner, unsigned)
	if err != nil {
		return address.Undef, err
	}

	signed := &types.SignedMessage{
		Message:   createStorageMinerMsg,
		Signature: *sig,
	}

	log.Infof("Pushing %s to Mpool", signed.Cid())

	err = api.MpoolPush(ctx, signed)
	if err != nil {
		return address.Undef, err
	}

	log.Infof("Waiting for confirmation")

	mw, err := api.ChainWaitMsg(ctx, signed.Cid())
	if err != nil {
		return address.Undef, err
	}

	addr, err := address.NewFromBytes(mw.Receipt.Return)
	if err != nil {
		return address.Undef, err
	}

	log.Infof("New storage miners address is: %s", addr)
	return addr, nil
}
