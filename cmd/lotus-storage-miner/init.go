package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	deals "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sealing"
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
		&cli.StringFlag{
			Name:  "pre-sealed-sectors",
			Usage: "specify set of presealed sectors for starting as a genesis miner",
		},
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},
		&cli.BoolFlag{
			Name:  "symlink-imported-sectors",
			Usage: "attempt to symlink to presealed sectors instead of copying them into place",
		},
	},
	Action: func(cctx *cli.Context) error {
		log.Info("Initializing lotus storage miner")

		ssize := cctx.Uint64("sector-size")

		symlink := cctx.Bool("symlink-imported-sectors")
		if symlink {
			log.Info("will attempt to symlink to imported sectors")
		}

		log.Info("Checking proof parameters")
		if err := paramfetch.GetParams(build.ParametersJson(), ssize); err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		log.Info("Trying to connect to full node RPC")

		api, closer, err := lcli.GetFullNodeAPI(cctx) // TODO: consider storing full node address in config
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		log.Info("Checking full node sync status")

		if !cctx.Bool("genesis-miner") && !cctx.Bool("nosync") {
			if err := lcli.SyncWait(ctx, api); err != nil {
				return xerrors.Errorf("sync wait: %w", err)
			}
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

		log.Info("Checking full node version")

		v, err := api.Version(ctx)
		if err != nil {
			return err
		}

		if !v.APIVersion.EqMajorMinor(build.APIVersion) {
			return xerrors.Errorf("Remote API version didn't match (local %s, remote %s)", build.APIVersion, v.APIVersion)
		}

		log.Info("Initializing repo")

		if err := r.Init(repo.StorageMiner); err != nil {
			return err
		}

		if pssb := cctx.String("pre-sealed-sectors"); pssb != "" {
			pssb, err := homedir.Expand(pssb)
			if err != nil {
				return err
			}

			log.Infof("moving pre-sealed-sectors from %s into newly created storage miner repo", pssb)
			lr, err := r.Lock(repo.StorageMiner)
			if err != nil {
				return err
			}
			mds, err := lr.Datastore("/metadata")
			if err != nil {
				return err
			}

			bopts := badger.DefaultOptions
			bopts.ReadOnly = true
			oldmds, err := badger.NewDatastore(filepath.Join(pssb, "badger"), &bopts)
			if err != nil {
				return err
			}

			oldsb, err := sectorbuilder.New(&sectorbuilder.Config{
				SectorSize:    ssize,
				WorkerThreads: 2,
				Dir:           pssb,
			}, namespace.Wrap(oldmds, datastore.NewKey("/sectorbuilder")))
			if err != nil {
				return xerrors.Errorf("failed to open up preseal sectorbuilder: %w", err)
			}

			nsb, err := sectorbuilder.New(&sectorbuilder.Config{
				SectorSize:    ssize,
				WorkerThreads: 2,
				Dir:           lr.Path(),
			}, namespace.Wrap(mds, datastore.NewKey("/sectorbuilder")))
			if err != nil {
				return xerrors.Errorf("failed to open up sectorbuilder: %w", err)
			}

			if err := nsb.ImportFrom(oldsb, symlink); err != nil {
				return err
			}
			if err := lr.Close(); err != nil {
				return xerrors.Errorf("unlocking repo after preseal migration: %w", err)
			}
		}

		if err := storageMinerInit(ctx, cctx, api, r); err != nil {
			log.Errorf("Failed to initialize lotus-storage-miner: %+v", err)
			path, err := homedir.Expand(repoPath)
			if err != nil {
				return err
			}
			log.Infof("Cleaning up %s after attempt...", path)
			if err := os.RemoveAll(path); err != nil {
				log.Errorf("Failed to clean up failed storage repo: %s", err)
			}
			return xerrors.Errorf("Storage-miner init failed")
		}

		// TODO: Point to setting storage price, maybe do it interactively or something
		log.Info("Storage miner successfully created, you can now start it with 'lotus-storage-miner run'")

		return nil
	},
}

func migratePreSealMeta(ctx context.Context, api lapi.FullNode, presealDir string, maddr address.Address, mds dtypes.MetadataDS) error {
	presealDir, err := homedir.Expand(presealDir)
	if err != nil {
		return xerrors.Errorf("expanding preseal dir: %w", err)
	}

	b, err := ioutil.ReadFile(filepath.Join(presealDir, "pre-seal-"+maddr.String()+".json"))
	if err != nil {
		return xerrors.Errorf("reading preseal metadata: %w", err)
	}

	preseals := map[string]genesis.GenesisMiner{}

	if err := json.Unmarshal(b, &preseals); err != nil {
		return xerrors.Errorf("unmarshaling preseal metadata: %w", err)
	}

	meta, ok := preseals[maddr.String()]
	if !ok {
		return xerrors.New("got wrong preseal info")
	}

	for _, sector := range meta.Sectors {
		sectorKey := datastore.NewKey(sealing.SectorStorePrefix).ChildString(fmt.Sprint(sector.SectorID))

		dealID, err := findMarketDealID(ctx, api, sector.Deal)
		if err != nil {
			return xerrors.Errorf("finding storage deal for pre-sealed sector %d: %w", sector.SectorID, err)
		}

		info := &sealing.SectorInfo{
			State:    lapi.Proving,
			SectorID: sector.SectorID,
			Pieces: []sealing.Piece{
				{
					DealID: dealID,
					Size:   meta.SectorSize,
					CommP:  sector.CommD[:],
				},
			},
			CommD:            sector.CommD[:],
			CommR:            sector.CommR[:],
			Proof:            nil,
			Ticket:           sealing.SealTicket{},
			PreCommitMessage: nil,
			Seed:             sealing.SealSeed{},
			CommitMessage:    nil,
		}

		b, err := cborutil.Dump(info)
		if err != nil {
			return err
		}

		if err := mds.Put(sectorKey, b); err != nil {
			return err
		}

		proposalCid, err := sector.Deal.Cid()
		if err != nil {
			return err
		}

		dealKey := datastore.NewKey(deals.ProviderDsPrefix).ChildString(proposalCid.String())

		proposal, err := utils.ToSharedStorageDealProposal(&sector.Deal)
		if err != nil {
			return err
		}
		deal := &deals.MinerDeal{
			MinerDeal: storagemarket.MinerDeal{
				Proposal:    *proposal,
				ProposalCid: proposalCid,
				State:       lapi.DealComplete,
				Ref:         proposalCid, // TODO: This is super wrong, but there
				// are no params for CommP CIDs, we can't recover unixfs cid easily,
				// and this isn't even used after the deal enters Complete state
				DealID:   dealID,
				SectorID: sector.SectorID,
			},
		}

		b, err = cborutil.Dump(deal)
		if err != nil {
			return err
		}

		if err := mds.Put(dealKey, b); err != nil {
			return err
		}
	}

	return nil
}

func findMarketDealID(ctx context.Context, api lapi.FullNode, deal actors.StorageDealProposal) (uint64, error) {
	// TODO: find a better way
	//  (this is only used by genesis miners)

	deals, err := api.StateMarketDeals(ctx, nil)
	if err != nil {
		return 0, xerrors.Errorf("getting market deals: %w", err)
	}

	for k, v := range deals {
		if bytes.Equal(v.PieceRef, deal.PieceRef) {
			return strconv.ParseUint(k, 10, 64)
		}
	}

	return 0, xerrors.New("deal not found")
}

func storageMinerInit(ctx context.Context, cctx *cli.Context, api lapi.FullNode, r repo.Repo) error {
	lr, err := r.Lock(repo.StorageMiner)
	if err != nil {
		return err
	}
	defer lr.Close()

	log.Info("Initializing libp2p identity")

	p2pSk, err := makeHostKey(lr)
	if err != nil {
		return xerrors.Errorf("make host key: %w", err)
	}

	peerid, err := peer.IDFromPrivateKey(p2pSk)
	if err != nil {
		return xerrors.Errorf("peer ID from private key: %w", err)
	}

	mds, err := lr.Datastore("/metadata")
	if err != nil {
		return err
	}

	var addr address.Address
	if act := cctx.String("actor"); act != "" {
		a, err := address.NewFromString(act)
		if err != nil {
			return xerrors.Errorf("failed parsing actor flag value (%q): %w", act, err)
		}

		if cctx.Bool("genesis-miner") {
			if err := mds.Put(datastore.NewKey("miner-address"), a.Bytes()); err != nil {
				return err
			}

			sbcfg, err := modules.SectorBuilderConfig(lr.Path(), 2, false, false)(mds, api)
			if err != nil {
				return xerrors.Errorf("getting genesis miner sector builder config: %w", err)
			}
			sb, err := sectorbuilder.New(sbcfg, mds)
			if err != nil {
				return xerrors.Errorf("failed to set up sectorbuilder for genesis mining: %w", err)
			}
			epp := storage.NewElectionPoStProver(sb)

			m := miner.NewMiner(api, epp)
			{
				if err := m.Register(a); err != nil {
					return xerrors.Errorf("failed to start up genesis miner: %w", err)
				}

				defer func() {
					if err := m.Unregister(ctx, a); err != nil {
						log.Error("failed to shut down storage miner: ", err)
					}
				}()

				if err := configureStorageMiner(ctx, api, a, peerid); err != nil {
					return xerrors.Errorf("failed to configure storage miner: %w", err)
				}
			}

			if pssb := cctx.String("pre-sealed-sectors"); pssb != "" {
				pssb, err := homedir.Expand(pssb)
				if err != nil {
					return err
				}

				log.Infof("Importing pre-sealed sector metadata for %s", a)

				if err := migratePreSealMeta(ctx, api, pssb, a, mds); err != nil {
					return xerrors.Errorf("migrating presealed sector metadata: %w", err)
				}
			}

			return nil
		}

		if err := configureStorageMiner(ctx, api, a, peerid); err != nil {
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
	if err := mds.Put(datastore.NewKey("miner-address"), addr.Bytes()); err != nil {
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

func configureStorageMiner(ctx context.Context, api lapi.FullNode, addr address.Address, peerid peer.ID) error {
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

func createStorageMiner(ctx context.Context, api lapi.FullNode, peerid peer.ID, cctx *cli.Context) (addr address.Address, err error) {
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
		Value: types.BigAdd(collateral, types.BigDiv(collateral, types.NewInt(100))),

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
