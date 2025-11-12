package miner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/docker/go-units"
	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-statestore"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	power2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/power"
	power6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/power"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/journal/fsjournal"
	storageminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/paths"
	pipeline "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var initCmd = &cli.Command{
	Name:  "init",
	Usage: "Initialize a lotus miner repo",
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
		&cli.StringFlag{
			Name:  "sector-size",
			Usage: "specify sector size to use",
		},
		&cli.StringSliceFlag{
			Name:  "pre-sealed-sectors",
			Usage: "specify set of presealed sectors for starting as a genesis miner",
		},
		&cli.StringFlag{
			Name:  "pre-sealed-metadata",
			Usage: "specify the metadata file for the presealed sectors",
		},
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},
		&cli.BoolFlag{
			Name:  "symlink-imported-sectors",
			Usage: "attempt to symlink to presealed sectors instead of copying them into place",
		},
		&cli.BoolFlag{
			Name:  "no-local-storage",
			Usage: "don't use storageminer repo for sector storage",
		},
		&cli.StringFlag{
			Name:  "gas-premium",
			Usage: "set gas premium for initialization messages in AttoFIL",
			Value: "0",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "select which address to send actor creation message from",
		},
		&cli.Uint64Flag{
			Name:  "confidence",
			Usage: "number of block confirmations to wait for",
			Value: buildconstants.MessageConfidence,
		},
		&cli.Float64Flag{
			Name:  "deposit-margin-factor",
			Usage: "Multiplier (>=1.0) to scale the suggested deposit for on-chain variance (e.g. 1.01 adds 1%)",
			Value: 1.01,
		},
	},
	Subcommands: []*cli.Command{
		restoreCmd,
	},
	Action: func(cctx *cli.Context) error {
		log.Info("Initializing lotus miner")

		ssize, err := abi.RegisteredSealProof_StackedDrg32GiBV1.SectorSize()
		if err != nil {
			return xerrors.Errorf("failed to calculate default sector size: %w", err)
		}

		if cctx.IsSet("sector-size") {
			sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
			if err != nil {
				return err
			}
			ssize = abi.SectorSize(sectorSizeInt)
		}

		gasPrice, err := types.BigFromString(cctx.String("gas-premium"))
		if err != nil {
			return xerrors.Errorf("failed to parse gas-price flag: %s", err)
		}

		confidence := cctx.Uint64("confidence")

		symlink := cctx.Bool("symlink-imported-sectors")
		if symlink {
			log.Info("will attempt to symlink to imported sectors")
		}

		ctx := lcli.ReqContext(cctx)

		log.Info("Checking proof parameters")

		if err := paramfetch.GetParams(ctx, build.ParametersJSON(), build.SrsJSON(), uint64(ssize)); err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		log.Info("Trying to connect to full node RPC")

		if err := checkV1ApiSupport(ctx, cctx); err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPIV1(cctx) // TODO: consider storing full node address in config
		if err != nil {
			return err
		}
		defer closer()

		log.Info("Checking full node sync status")

		if !cctx.Bool("genesis-miner") && !cctx.Bool("nosync") {
			if err := lcli.SyncWait(ctx, &v0api.WrapperV1Full{FullNode: api}, false); err != nil {
				return xerrors.Errorf("sync wait: %w", err)
			}
		}

		log.Info("Checking if repo exists")

		repoPath := cctx.String(FlagMinerRepo)
		r, err := repo.NewFS(repoPath)
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}
		if ok {
			return xerrors.Errorf("repo at '%s' is already initialized", cctx.String(FlagMinerRepo))
		}

		log.Info("Checking full node version")

		v, err := api.Version(ctx)
		if err != nil {
			return err
		}

		if !v.APIVersion.EqMajorMinor(lapi.FullAPIVersion1) {
			return xerrors.Errorf("Remote API version didn't match (expected %s, remote %s)", lapi.FullAPIVersion1, v.APIVersion)
		}

		log.Info("Initializing repo")

		if err := r.Init(repo.StorageMiner); err != nil {
			return err
		}

		{
			lr, err := r.Lock(repo.StorageMiner)
			if err != nil {
				return err
			}

			var localPaths []storiface.LocalPath

			if pssb := cctx.StringSlice("pre-sealed-sectors"); len(pssb) != 0 {
				log.Infof("Setting up storage config with presealed sectors: %v", pssb)

				for _, psp := range pssb {
					psp, err := homedir.Expand(psp)
					if err != nil {
						return err
					}
					localPaths = append(localPaths, storiface.LocalPath{
						Path: psp,
					})
				}
			}

			if !cctx.Bool("no-local-storage") {
				b, err := json.MarshalIndent(&storiface.LocalStorageMeta{
					ID:       storiface.ID(uuid.New().String()),
					Weight:   10,
					CanSeal:  true,
					CanStore: true,
				}, "", "  ")
				if err != nil {
					return xerrors.Errorf("marshaling storage config: %w", err)
				}

				if err := os.WriteFile(filepath.Join(lr.Path(), "sectorstore.json"), b, 0644); err != nil {
					return xerrors.Errorf("persisting storage metadata (%s): %w", filepath.Join(lr.Path(), "sectorstore.json"), err)
				}

				localPaths = append(localPaths, storiface.LocalPath{
					Path: lr.Path(),
				})
			}

			if err := lr.SetStorage(func(sc *storiface.StorageConfig) {
				sc.StoragePaths = append(sc.StoragePaths, localPaths...)
			}); err != nil {
				return xerrors.Errorf("set storage config: %w", err)
			}

			if err := lr.Close(); err != nil {
				return err
			}
		}

		if err := storageMinerInit(ctx, cctx, api, r, ssize, gasPrice, confidence); err != nil {
			log.Errorf("Failed to initialize lotus-miner: %+v", err)
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
		log.Info("Miner successfully created, you can now start it with 'lotus-miner run'")

		return nil
	},
}

func migratePreSealMeta(ctx context.Context, api v1api.FullNode, metadata string, maddr address.Address, mds dtypes.MetadataDS) error {
	metadata, err := homedir.Expand(metadata)
	if err != nil {
		return xerrors.Errorf("expanding preseal dir: %w", err)
	}

	b, err := os.ReadFile(metadata)
	if err != nil {
		return xerrors.Errorf("reading preseal metadata: %w", err)
	}

	psm := map[string]genesis.Miner{}
	if err := json.Unmarshal(b, &psm); err != nil {
		return xerrors.Errorf("unmarshaling preseal metadata: %w", err)
	}

	meta, ok := psm[maddr.String()]
	if !ok {
		return xerrors.Errorf("preseal file didn't contain metadata for miner %s", maddr)
	}

	maxSectorID := abi.SectorNumber(0)
	for _, sector := range meta.Sectors {
		sectorKey := datastore.NewKey(pipeline.SectorStorePrefix).ChildString(fmt.Sprint(sector.SectorID))

		dealID, err := findMarketDealID(ctx, api, sector.Deal)
		if err != nil {
			return xerrors.Errorf("finding storage deal for pre-sealed sector %d: %w", sector.SectorID, err)
		}
		commD := sector.CommD
		commR := sector.CommR

		info := &pipeline.SectorInfo{
			State:        pipeline.Proving,
			SectorNumber: sector.SectorID,
			Pieces: []pipeline.SafeSectorPiece{
				pipeline.SafePiece(lapi.SectorPiece{
					Piece: abi.PieceInfo{
						Size:     abi.PaddedPieceSize(meta.SectorSize),
						PieceCID: commD,
					},
					DealInfo: &piece.PieceDealInfo{
						DealID:       dealID,
						DealProposal: &sector.Deal,
						DealSchedule: piece.DealSchedule{
							StartEpoch: sector.Deal.StartEpoch,
							EndEpoch:   sector.Deal.EndEpoch,
						},
					},
				}),
			},
			CommD:            &commD,
			CommR:            &commR,
			Proof:            nil,
			TicketValue:      abi.SealRandomness{},
			TicketEpoch:      0,
			PreCommitMessage: nil,
			SeedValue:        abi.InteractiveSealRandomness{},
			SeedEpoch:        0,
			CommitMessage:    nil,
		}

		b, err := cborutil.Dump(info)
		if err != nil {
			return err
		}

		if err := mds.Put(ctx, sectorKey, b); err != nil {
			return err
		}

		if sector.SectorID > maxSectorID {
			maxSectorID = sector.SectorID
		}

		/* // TODO: Import deals into market
		pnd, err := cborutil.AsIpld(sector.Deal)
		if err != nil {
			return err
		}

		dealKey := datastore.NewKey(deals.ProviderDsPrefix).ChildString(pnd.Cid().String())

		deal := &deals.MinerDeal{
			MinerDeal: storagemarket.MinerDeal{
				ClientDealProposal: sector.Deal,
				ProposalCid: pnd.Cid(),
				State:       storagemarket.StorageDealActive,
				Ref:         &storagemarket.DataRef{Root: proposalCid}, // TODO: This is super wrong, but there
				// are no params for CommP CIDs, we can't recover unixfs cid easily,
				// and this isn't even used after the deal enters Complete state
				DealID: dealID,
			},
		}

		b, err = cborutil.Dump(deal)
		if err != nil {
			return err
		}

		if err := mds.Put(dealKey, b); err != nil {
			return err
		}*/
	}

	buf := make([]byte, binary.MaxVarintLen64)
	size := binary.PutUvarint(buf, uint64(maxSectorID))
	return mds.Put(ctx, datastore.NewKey(pipeline.StorageCounterDSPrefix), buf[:size])
}

func findMarketDealID(ctx context.Context, api v1api.FullNode, deal markettypes.DealProposal) (abi.DealID, error) {
	// TODO: find a better way
	//  (this is only used by genesis miners)

	deals, err := api.StateMarketDeals(ctx, types.EmptyTSK)
	if err != nil {
		return 0, xerrors.Errorf("getting market deals: %w", err)
	}

	for k, v := range deals {
		if v.Proposal.PieceCID.Equals(deal.PieceCID) {
			id, err := strconv.ParseUint(k, 10, 64)
			return abi.DealID(id), err
		}
	}

	return 0, xerrors.New("deal not found")
}

func storageMinerInit(ctx context.Context, cctx *cli.Context, api v1api.FullNode, r repo.Repo, ssize abi.SectorSize, gasPrice types.BigInt, confidence uint64) error {
	lr, err := r.Lock(repo.StorageMiner)
	if err != nil {
		return err
	}
	defer lr.Close() //nolint:errcheck

	log.Info("Initializing libp2p identity")

	p2pSk, err := makeHostKey(lr)
	if err != nil {
		return xerrors.Errorf("make host key: %w", err)
	}

	peerid, err := peer.IDFromPrivateKey(p2pSk)
	if err != nil {
		return xerrors.Errorf("peer ID from private key: %w", err)
	}

	mds, err := lr.Datastore(ctx, "/metadata")
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
			if err := mds.Put(ctx, datastore.NewKey("miner-address"), a.Bytes()); err != nil {
				return err
			}

			mid, err := address.IDFromAddress(a)
			if err != nil {
				return xerrors.Errorf("getting id address: %w", err)
			}

			sa, err := modules.StorageAuth(ctx, api)
			if err != nil {
				return err
			}

			wsts := statestore.New(namespace.Wrap(mds, modules.WorkerCallsPrefix))
			smsts := statestore.New(namespace.Wrap(mds, modules.ManagerWorkPrefix))

			si := paths.NewMemIndex(nil)

			lstor, err := paths.NewLocal(ctx, lr, si, nil)
			if err != nil {
				return err
			}
			stor := paths.NewRemote(lstor, si, http.Header(sa), 10, &paths.DefaultPartialFileHandler{})

			smgr, err := sealer.New(ctx, lstor, stor, lr, si, config.SealerConfig{
				ParallelFetchLimit:       10,
				AllowAddPiece:            true,
				AllowPreCommit1:          true,
				AllowPreCommit2:          true,
				AllowCommit:              true,
				AllowUnseal:              true,
				AllowReplicaUpdate:       true,
				AllowProveReplicaUpdate2: true,
				AllowRegenSectorKey:      true,
			}, config.ProvingConfig{}, wsts, smsts)
			if err != nil {
				return err
			}
			epp, err := storage.NewWinningPoStProver(api, smgr, ffiwrapper.ProofVerifier, dtypes.MinerID(mid))
			if err != nil {
				return err
			}

			j, err := fsjournal.OpenFSJournal(lr, journal.EnvDisabledEvents())
			if err != nil {
				return fmt.Errorf("failed to open filesystem journal: %w", err)
			}

			m := storageminer.NewMiner(api, epp, a, slashfilter.New(mds), j)
			{
				if err := m.Start(ctx); err != nil {
					return xerrors.Errorf("failed to start up genesis miner: %w", err)
				}

				cerr := configureStorageMiner(ctx, api, a, peerid, gasPrice, confidence)

				if err := m.Stop(ctx); err != nil {
					log.Error("failed to shut down miner: ", err)
				}

				if cerr != nil {
					return xerrors.Errorf("failed to configure miner: %w", cerr)
				}
			}

			if pssb := cctx.String("pre-sealed-metadata"); pssb != "" {
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

		if pssb := cctx.String("pre-sealed-metadata"); pssb != "" {
			pssb, err := homedir.Expand(pssb)
			if err != nil {
				return err
			}

			log.Infof("Importing pre-sealed sector metadata for %s", a)

			if err := migratePreSealMeta(ctx, api, pssb, a, mds); err != nil {
				return xerrors.Errorf("migrating presealed sector metadata: %w", err)
			}
		}

		if err := configureStorageMiner(ctx, api, a, peerid, gasPrice, confidence); err != nil {
			return xerrors.Errorf("failed to configure miner: %w", err)
		}

		addr = a
	} else {
		a, err := createStorageMiner(ctx, api, ssize, peerid, gasPrice, confidence, cctx)
		if err != nil {
			return xerrors.Errorf("creating miner failed: %w", err)
		}

		addr = a
	}

	log.Infof("Created new miner: %s", addr)
	if err := mds.Put(ctx, datastore.NewKey("miner-address"), addr.Bytes()); err != nil {
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

	kbytes, err := crypto.MarshalPrivateKey(pk)
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

func configureStorageMiner(ctx context.Context, api v1api.FullNode, addr address.Address, peerid peer.ID, gasPrice types.BigInt, confidence uint64) error {
	mi, err := api.StateMinerInfo(ctx, addr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getWorkerAddr returned bad address: %w", err)
	}

	enc, err := actors.SerializeParams(&miner2.ChangePeerIDParams{NewID: abi.PeerID(peerid)})
	if err != nil {
		return err
	}

	msg := &types.Message{
		To:         addr,
		From:       mi.Worker,
		Method:     builtin.MethodsMiner.ChangePeerID,
		Params:     enc,
		Value:      types.NewInt(0),
		GasPremium: gasPrice,
	}

	smsg, err := api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return err
	}

	log.Info("Waiting for message: ", smsg.Cid())
	ret, err := api.StateWaitMsg(ctx, smsg.Cid(), confidence, lapi.LookbackNoLimit, true)
	if err != nil {
		return err
	}

	if ret.Receipt.ExitCode != 0 {
		return xerrors.Errorf("update peer id message failed with exit code %d", ret.Receipt.ExitCode)
	}

	return nil
}

func createStorageMiner(ctx context.Context, api v1api.FullNode, ssize abi.SectorSize, peerid peer.ID, gasPrice types.BigInt, confidence uint64, cctx *cli.Context) (address.Address, error) {
	var err error
	var owner address.Address
	if cctx.String("owner") != "" {
		owner, err = address.NewFromString(cctx.String("owner"))
	} else {
		owner, err = api.WalletDefaultAddress(ctx)
	}
	if err != nil {
		return address.Undef, err
	}

	worker := owner
	if cctx.String("worker") != "" {
		worker, err = address.NewFromString(cctx.String("worker"))
	} else if cctx.Bool("create-worker-key") { // TODO: Do we need to force this if owner is Secpk?
		worker, err = api.WalletNew(ctx, types.KTBLS)
	}
	if err != nil {
		return address.Address{}, err
	}

	sender := owner
	if fromstr := cctx.String("from"); fromstr != "" {
		faddr, err := address.NewFromString(fromstr)
		if err != nil {
			return address.Undef, fmt.Errorf("could not parse from address: %w", err)
		}
		sender = faddr
	}

	// make sure the sender account exists on chain
	_, err = api.StateLookupID(ctx, sender, types.EmptyTSK)
	if err != nil {
		return address.Undef, xerrors.Errorf("sender must exist on chain: %w", err)
	}

	// make sure the worker account exists on chain
	_, err = api.StateLookupID(ctx, worker, types.EmptyTSK)
	if err != nil {
		signed, err := api.MpoolPushMessage(ctx, &types.Message{
			From:  sender,
			To:    worker,
			Value: types.NewInt(0),
		}, nil)
		if err != nil {
			return address.Undef, xerrors.Errorf("push worker init: %w", err)
		}

		log.Infof("Initializing worker account %s, message: %s", worker, signed.Cid())
		log.Infof("Waiting for confirmation")

		mw, err := api.StateWaitMsg(ctx, signed.Cid(), confidence, lapi.LookbackNoLimit, true)
		if err != nil {
			return address.Undef, xerrors.Errorf("waiting for worker init: %w", err)
		}
		if mw.Receipt.ExitCode != 0 {
			return address.Undef, xerrors.Errorf("initializing worker account failed: exit code %d", mw.Receipt.ExitCode)
		}
	}

	// make sure the owner account exists on chain
	_, err = api.StateLookupID(ctx, owner, types.EmptyTSK)
	if err != nil {
		signed, err := api.MpoolPushMessage(ctx, &types.Message{
			From:  sender,
			To:    owner,
			Value: types.NewInt(0),
		}, nil)
		if err != nil {
			return address.Undef, xerrors.Errorf("push owner init: %w", err)
		}

		log.Infof("Initializing owner account %s, message: %s", worker, signed.Cid())
		log.Infof("Waiting for confirmation")

		mw, err := api.StateWaitMsg(ctx, signed.Cid(), confidence, lapi.LookbackNoLimit, true)
		if err != nil {
			return address.Undef, xerrors.Errorf("waiting for owner init: %w", err)
		}
		if mw.Receipt.ExitCode != 0 {
			return address.Undef, xerrors.Errorf("initializing owner account failed: exit code %d", mw.Receipt.ExitCode)
		}
	}

	// Note: the correct thing to do would be to call SealProofTypeFromSectorSize if actors version is v3 or later, but this still works
	nv, err := api.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to get network version: %w", err)
	}
	spt, err := miner.WindowPoStProofTypeFromSectorSize(ssize, nv)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting post proof type: %w", err)
	}

	params, err := actors.SerializeParams(&power6.CreateMinerParams{
		Owner:               owner,
		Worker:              worker,
		WindowPoStProofType: spt,
		Peer:                abi.PeerID(peerid),
	})
	if err != nil {
		return address.Undef, err
	}

	depositMarginFactor := cctx.Float64("deposit-margin-factor")
	if depositMarginFactor < 1 {
		return address.Undef, xerrors.Errorf("deposit margin factor must be greater than or equal to 1")
	}

	deposit, err := api.StateMinerCreationDeposit(ctx, types.EmptyTSK)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting miner creation deposit: %w", err)
	}

	scaledDeposit := types.BigDiv(types.BigMul(deposit, types.NewInt(uint64(depositMarginFactor*100))), types.NewInt(100))

	createStorageMinerMsg := &types.Message{
		To:    power.Address,
		From:  sender,
		Value: scaledDeposit,

		Method: power.Methods.CreateMiner,
		Params: params,

		GasLimit:   0,
		GasPremium: gasPrice,
	}

	signed, err := api.MpoolPushMessage(ctx, createStorageMinerMsg, nil)
	if err != nil {
		return address.Undef, xerrors.Errorf("pushing createMiner message: %w", err)
	}

	log.Infof("Pushed CreateMiner message: %s", signed.Cid())
	log.Infof("Waiting for confirmation")

	mw, err := api.StateWaitMsg(ctx, signed.Cid(), confidence, lapi.LookbackNoLimit, true)
	if err != nil {
		return address.Undef, xerrors.Errorf("waiting for createMiner message: %w", err)
	}

	if mw.Receipt.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("create miner failed: exit code %d", mw.Receipt.ExitCode)
	}

	var retval power2.CreateMinerReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(mw.Receipt.Return)); err != nil {
		return address.Undef, err
	}

	log.Infof("New miners address is: %s (%s)", retval.IDAddress, retval.RobustAddress)
	return retval.IDAddress, nil
}

// checkV1ApiSupport uses v0 api version to signal support for v1 API
// trying to query the v1 api on older lotus versions would get a 404, which can happen for any number of other reasons
func checkV1ApiSupport(ctx context.Context, cctx *cli.Context) error {
	// check v0 api version to make sure it supports v1 api
	api0, closer, err := lcli.GetFullNodeAPI(cctx)
	if err != nil {
		return err
	}

	v, err := api0.Version(ctx)
	closer()

	if err != nil {
		return err
	}

	if !v.APIVersion.EqMajorMinor(lapi.FullAPIVersion0) {
		return xerrors.Errorf("Remote API version didn't match (expected %s, remote %s)", lapi.FullAPIVersion0, v.APIVersion)
	}

	return nil
}
