package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/exchange/offline"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-commp-utils/v2"
	"github.com/filecoin-project/go-hamt-ipld/v3"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	v10 "github.com/filecoin-project/go-state-types/builtin/v10"
	v11 "github.com/filecoin-project/go-state-types/builtin/v11"
	v12 "github.com/filecoin-project/go-state-types/builtin/v12"
	v13 "github.com/filecoin-project/go-state-types/builtin/v13"
	market13 "github.com/filecoin-project/go-state-types/builtin/v13/market"
	adt13 "github.com/filecoin-project/go-state-types/builtin/v13/util/adt"
	v14 "github.com/filecoin-project/go-state-types/builtin/v14"
	v15 "github.com/filecoin-project/go-state-types/builtin/v15"
	v16 "github.com/filecoin-project/go-state-types/builtin/v16"
	v17 "github.com/filecoin-project/go-state-types/builtin/v17"
	v18 "github.com/filecoin-project/go-state-types/builtin/v18"
	market8 "github.com/filecoin-project/go-state-types/builtin/v8/market"
	adt8 "github.com/filecoin-project/go-state-types/builtin/v8/util/adt"
	v9 "github.com/filecoin-project/go-state-types/builtin/v9"
	market9 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	adt9 "github.com/filecoin-project/go-state-types/builtin/v9/util/adt"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/manifest"
	mutil "github.com/filecoin-project/go-state-types/migration"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-actors/v7/actors/migration/nv15"

	"github.com/filecoin-project/lotus/blockstore"
	badgerbs "github.com/filecoin-project/lotus/blockstore/badger"
	"github.com/filecoin-project/lotus/blockstore/splitstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	lbuiltin "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	proofsffi "github.com/filecoin-project/lotus/chain/proofs/ffi"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/node/repo"
)

var migrationsCmd = &cli.Command{
	Name:        "migrate-state",
	Description: "Run a network upgrade migration",
	ArgsUsage:   `<network version or fix specifier (e.g. "tock-fix")> <CID of block to look back from>`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.BoolFlag{
			Name: "skip-pre-migration",
		},
		&cli.BoolFlag{
			Name: "check-invariants",
		},
		&cli.StringFlag{
			Name: "export-bad-migration",
		},
	},
	Action: func(cctx *cli.Context) error {
		fmt.Println("REMINDER: If you are running this, you likely want to ALSO run the continuity testing tool!")
		ctx := context.TODO()

		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}

		var (
			nv                   network.Version
			upgradeActorsFunc    UpgradeActorsFunc
			preUpgradeActorsFunc PreUpgradeActorsFunc
			checkInvariantsFunc  CheckInvariantsFunc
		)

		if nvi, err := strconv.ParseUint(cctx.Args().Get(0), 10, 32); err != nil {
			// not a specific network version number, treat it as a string and check predefined strings
			nv, upgradeActorsFunc, preUpgradeActorsFunc, checkInvariantsFunc, err = getMigrationFuncsForString(cctx.Args().Get(0))
			if err != nil {
				return fmt.Errorf("failed to parse network version or string: %w", err)
			}
		} else {
			nv = network.Version(nvi)
			upgradeActorsFunc, preUpgradeActorsFunc, checkInvariantsFunc, err = getMigrationFuncsForNetwork(nv)
			if err != nil {
				return err
			}
		}

		blkCid, err := cid.Decode(cctx.Args().Get(1))
		if err != nil {
			return fmt.Errorf("failed to parse input: %w", err)
		}

		fsrepo, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lkrepo, err := fsrepo.Lock(repo.FullNode)
		if err != nil {
			return xerrors.Errorf("failed to lock repo: %w", err)
		}

		defer lkrepo.Close() //nolint:errcheck

		cold, err := lkrepo.Blockstore(ctx, repo.UniversalBlockstore)
		if err != nil {
			return fmt.Errorf("failed to open universal blockstore %w", err)
		}

		path, err := lkrepo.SplitstorePath()
		if err != nil {
			return err
		}

		path = filepath.Join(path, "hot.badger")
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}

		opts, err := repo.BadgerBlockstoreOptions(repo.HotBlockstore, path, lkrepo.Readonly())
		if err != nil {
			return err
		}

		hot, err := badgerbs.Open(opts)
		if err != nil {
			return err
		}

		mds, err := lkrepo.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		cfg := &splitstore.Config{
			MarkSetType:       "map",
			DiscardColdBlocks: true,
		}
		ss, err := splitstore.Open(path, mds, hot, cold, cfg)
		if err != nil {
			return err
		}
		defer func() {
			if err := ss.Close(); err != nil {
				log.Warnf("failed to close blockstore: %s", err)

			}
		}()
		bs := ss

		cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		// Note: we use a map datastore for the metadata to avoid writing / using cached migration results in the metadata store
		sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(proofsffi.ProofVerifier), filcns.DefaultUpgradeSchedule(), nil,
			datastore.NewMapDatastore(), nil)
		if err != nil {
			return err
		}

		blk, err := cs.GetBlock(ctx, blkCid)
		if err != nil {
			return err
		}

		migrationTs, err := cs.LoadTipSet(ctx, types.NewTipSetKey(blk.Parents...))
		if err != nil {
			return err
		}

		startTime := time.Now()

		newCid2, err := upgradeActorsFunc(ctx, sm, nv15.NewMemMigrationCache(), nil, blk.ParentStateRoot, blk.Height-1, migrationTs)
		if err != nil {
			return err
		}

		uncachedMigrationTime := time.Since(startTime)

		fmt.Println("migration height ", blk.Height-1)
		fmt.Println("old cid ", blk.ParentStateRoot)
		fmt.Println("new cid ", newCid2)
		fmt.Println("completed round actual (without cache), took ", uncachedMigrationTime)

		if !cctx.IsSet("skip-pre-migration") && preUpgradeActorsFunc != nil {
			cache := mutil.NewMemMigrationCache()

			ts1, err := cs.GetTipsetByHeight(ctx, blk.Height-60, migrationTs, false)
			if err != nil {
				return err
			}

			startTime = time.Now()

			err = preUpgradeActorsFunc(ctx, sm, cache, ts1.ParentState(), ts1.Height()-1, ts1)
			if err != nil {
				return err
			}

			preMigrationTime := time.Since(startTime)
			fmt.Println("completed premigration, took ", preMigrationTime)

			startTime = time.Now()

			newCid1, err := upgradeActorsFunc(ctx, sm, cache, nil, blk.ParentStateRoot, blk.Height-1, migrationTs)
			if err != nil {
				return err
			}

			cachedMigrationTime := time.Since(startTime)

			if newCid1 != newCid2 {
				{
					if err := printStateDiff(ctx, nv, newCid2, newCid1, bs); err != nil {
						fmt.Println("failed to print state diff: ", err)
					}
				}

				if cctx.IsSet("export-bad-migration") {
					fi, err := os.Create(cctx.String("export-bad-migration"))
					if err != nil {
						return xerrors.Errorf("opening the output file: %w", err)
					}

					defer fi.Close() //nolint:errcheck

					roots := []cid.Cid{newCid1, newCid2}

					dag := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
					err = car.WriteCarWithWalker(ctx, dag, roots, fi, carWalkFunc)
					if err != nil {
						return err
					}

					fmt.Println("exported bad migration to ", cctx.String("export-bad-migration"))
				}

				return xerrors.Errorf("got different results with and without the cache: %s, %s", newCid1,
					newCid2)
			}

			fmt.Println("completed round actual (with cache), took ", cachedMigrationTime)
		} else if !cctx.IsSet("skip-pre-migration") {
			fmt.Println("skipping pre-migration, no pre-migration function")
		}

		if cctx.Bool("check-invariants") {
			if checkInvariantsFunc == nil {
				return xerrors.Errorf("check invariants not implemented for nv%d", nv)
			}
			err = checkInvariantsFunc(ctx, blk.ParentStateRoot, newCid2, bs, blk.Height-1)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

func getMigrationFuncsForNetwork(nv network.Version) (UpgradeActorsFunc, PreUpgradeActorsFunc, CheckInvariantsFunc, error) {
	switch nv {
	case network.Version17:
		return filcns.UpgradeActorsV9, filcns.PreUpgradeActorsV9, checkNv17Invariants, nil
	case network.Version18:
		return filcns.UpgradeActorsV10, filcns.PreUpgradeActorsV10, checkNv18Invariants, nil
	case network.Version19:
		return filcns.UpgradeActorsV11, filcns.PreUpgradeActorsV11, checkNv19Invariants, nil
	case network.Version21:
		return filcns.UpgradeActorsV12, filcns.PreUpgradeActorsV12, checkNv21Invariants, nil
	case network.Version22:
		return filcns.UpgradeActorsV13, filcns.PreUpgradeActorsV13, checkNv22Invariants, nil
	case network.Version23:
		return filcns.UpgradeActorsV14, filcns.PreUpgradeActorsV14, checkNv23Invariants, nil
	case network.Version24:
		return filcns.UpgradeActorsV15, filcns.PreUpgradeActorsV15, checkNv24Invariants, nil
	case network.Version25:
		return filcns.UpgradeActorsV16, filcns.PreUpgradeActorsV16, checkNv25Invariants, nil
	case network.Version27:
		return filcns.UpgradeActorsV17, filcns.PreUpgradeActorsV17, checkNv27Invariants, nil
	case network.Version28:
		return filcns.UpgradeActorsV18, filcns.PreUpgradeActorsV18, checkNv28Invariants, nil
	default:
		return nil, nil, nil, xerrors.Errorf("migration not implemented for nv%d", nv)
	}
}

func getMigrationFuncsForString(input string) (network.Version, UpgradeActorsFunc, PreUpgradeActorsFunc, CheckInvariantsFunc, error) {
	switch input {
	case "tock-fix":
		return network.Version27, filcns.UpgradeActorsV16Fix, nil, checkNv25Invariants, nil
	default:
		return 0, nil, nil, nil, xerrors.Errorf("migration not implemented for string: %s", input)
	}
}

type UpgradeActorsFunc = func(context.Context, *stmgr.StateManager, stmgr.MigrationCache, stmgr.ExecMonitor, cid.Cid, abi.ChainEpoch, *types.TipSet) (cid.Cid, error)
type PreUpgradeActorsFunc = func(context.Context, *stmgr.StateManager, stmgr.MigrationCache, cid.Cid, abi.ChainEpoch, *types.TipSet) error
type CheckInvariantsFunc = func(context.Context, cid.Cid, cid.Cid, blockstore.Blockstore, abi.ChainEpoch) error

func printStateDiff(ctx context.Context, nv network.Version, newCid1, newCid2 cid.Cid, bs blockstore.Blockstore) error {
	// migration diff
	var sra, srb types.StateRoot
	cst := cbornode.NewCborStore(bs)

	if err := cst.Get(ctx, newCid1, &sra); err != nil {
		return err
	}
	if err := cst.Get(ctx, newCid2, &srb); err != nil {
		return err
	}

	if sra.Version != srb.Version {
		fmt.Println("state root versions do not match: ", sra.Version, srb.Version)
	}
	if sra.Info != srb.Info {
		fmt.Println("state root infos do not match: ", sra.Info, srb.Info)
	}
	if sra.Actors != srb.Actors {
		fmt.Println("state root actors do not match: ", sra.Actors, srb.Actors)
		if err := printActorsDiff(ctx, cst, nv, sra.Actors, srb.Actors); err != nil {
			return err
		}
	}

	return nil
}

func printActorsDiff(ctx context.Context, cst *cbornode.BasicIpldStore, nv network.Version, a, b cid.Cid) error {
	// actor diff, a b are a hamt

	diffs, err := hamt.Diff(ctx, cst, cst, a, b, hamt.UseTreeBitWidth(builtin.DefaultHamtBitwidth))
	if err != nil {
		return err
	}

	keyParser := func(k string) (interface{}, error) {
		return address.NewFromBytes([]byte(k))
	}

	for _, d := range diffs {
		switch d.Type {
		case hamt.Add:
			color.Green("+ Add %v", must.One(keyParser(d.Key)))
		case hamt.Remove:
			color.Red("- Remove %v", must.One(keyParser(d.Key)))
		case hamt.Modify:
			addr := must.One(keyParser(d.Key)).(address.Address)
			color.Yellow("~ Modify %v", addr)
			var aa, bb types.ActorV5

			if err := aa.UnmarshalCBOR(bytes.NewReader(d.Before.Raw)); err != nil {
				return err
			}
			if err := bb.UnmarshalCBOR(bytes.NewReader(d.After.Raw)); err != nil {
				return err
			}

			if err := printActorDiff(ctx, cst, nv, addr, aa, bb); err != nil {
				return err
			}
		}
	}

	return nil
}

func printActorDiff(ctx context.Context, cst *cbornode.BasicIpldStore, nv network.Version, addr address.Address, a, b types.ActorV5) error {
	if a.Code != b.Code {
		fmt.Println("  Code: ", a.Code, b.Code)
	}
	if a.Head != b.Head {
		fmt.Println("  Head: ", a.Head, b.Head)
	}
	if a.Nonce != b.Nonce {
		fmt.Println("  Nonce: ", a.Nonce, b.Nonce)
	}
	if big.Cmp(a.Balance, b.Balance) == 0 {
		fmt.Println("  Balance: ", a.Balance, b.Balance)
	}

	switch addr.String() {
	case "f05":
		if err := printMarketActorDiff(ctx, cst, nv, a.Head, b.Head); err != nil {
			return err
		}
	default:
		fmt.Println("no logic to diff actor state for ", addr)
	}

	return nil
}

func printMarketActorDiff(ctx context.Context, cst *cbornode.BasicIpldStore, nv network.Version, a, b cid.Cid) error {
	if nv != network.Version22 {
		return xerrors.Errorf("market actor diff not implemented for nv%d", nv)
	}

	var ma, mb market13.State
	if err := cst.Get(ctx, a, &ma); err != nil {
		return err
	}
	if err := cst.Get(ctx, b, &mb); err != nil {
		return err
	}

	if ma.Proposals != mb.Proposals {
		fmt.Println("  Proposals: ", ma.Proposals, mb.Proposals)
	}
	if ma.States != mb.States {
		fmt.Println("  States: ", ma.States, mb.States)

		// diff the AMTs
		amtDiff, err := amt.Diff(ctx, cst, cst, ma.States, mb.States, amt.UseTreeBitWidth(market13.StatesAmtBitwidth))
		if err != nil {
			return err
		}

		proposalsArrA, err := adt13.AsArray(adt8.WrapStore(ctx, cst), ma.Proposals, market13.ProposalsAmtBitwidth)
		if err != nil {
			return err
		}
		proposalsArrB, err := adt13.AsArray(adt8.WrapStore(ctx, cst), mb.Proposals, market13.ProposalsAmtBitwidth)
		if err != nil {
			return err
		}

		for _, d := range amtDiff {
			switch d.Type {
			case amt.Add:
				color.Green("  state + Add %v", d.Key)
			case amt.Remove:
				color.Red("  state - Remove %v", d.Key)
			case amt.Modify:
				color.Yellow("  state ~ Modify %v", d.Key)

				var a, b market13.DealState
				if err := a.UnmarshalCBOR(bytes.NewReader(d.Before.Raw)); err != nil {
					return err
				}
				if err := b.UnmarshalCBOR(bytes.NewReader(d.After.Raw)); err != nil {
					return err
				}

				ja, err := json.Marshal(a)
				if err != nil {
					return err
				}
				jb, err := json.Marshal(b)
				if err != nil {
					return err
				}

				fmt.Println("   A: ", string(ja))
				fmt.Println("   B: ", string(jb))

				var propA, propB market13.DealProposal

				if _, err := proposalsArrA.Get(d.Key, &propA); err != nil {
					return err
				}
				if _, err := proposalsArrB.Get(d.Key, &propB); err != nil {
					return err
				}

				pab, err := json.Marshal(propA)
				if err != nil {
					return err
				}
				pbb, err := json.Marshal(propB)
				if err != nil {
					return err
				}
				if string(pab) != string(pbb) {
					fmt.Println("   PropA: ", string(pab))
					fmt.Println("   PropB: ", string(pbb))
				} else {
					fmt.Println("   Prop: ", string(pab))
				}

			}
		}
	}
	if ma.PendingProposals != mb.PendingProposals {
		fmt.Println("  PendingProposals: ", ma.PendingProposals, mb.PendingProposals)
	}
	if ma.EscrowTable != mb.EscrowTable {
		fmt.Println("  EscrowTable: ", ma.EscrowTable, mb.EscrowTable)
	}
	if ma.LockedTable != mb.LockedTable {
		fmt.Println("  LockedTable: ", ma.LockedTable, mb.LockedTable)
	}
	if ma.NextID != mb.NextID {
		fmt.Println("  NextID: ", ma.NextID, mb.NextID)
	}
	if ma.DealOpsByEpoch != mb.DealOpsByEpoch {
		fmt.Println("  DealOpsByEpoch: ", ma.DealOpsByEpoch, mb.DealOpsByEpoch)
	}
	if ma.LastCron != mb.LastCron {
		fmt.Println("  LastCron: ", ma.LastCron, mb.LastCron)
	}
	if ma.TotalClientLockedCollateral != mb.TotalClientLockedCollateral {
		fmt.Println("  TotalClientLockedCollateral: ", ma.TotalClientLockedCollateral, mb.TotalClientLockedCollateral)
	}
	if ma.TotalProviderLockedCollateral != mb.TotalProviderLockedCollateral {
		fmt.Println("  TotalProviderLockedCollateral: ", ma.TotalProviderLockedCollateral, mb.TotalProviderLockedCollateral)
	}
	if ma.TotalClientStorageFee != mb.TotalClientStorageFee {
		fmt.Println("  TotalClientStorageFee: ", ma.TotalClientStorageFee, mb.TotalClientStorageFee)
	}
	if ma.PendingDealAllocationIds != mb.PendingDealAllocationIds {
		fmt.Println("  PendingDealAllocationIds: ", ma.PendingDealAllocationIds, mb.PendingDealAllocationIds)
	}
	if ma.ProviderSectors != mb.ProviderSectors {
		fmt.Println("  ProviderSectors: ", ma.ProviderSectors, mb.ProviderSectors)

		// diff the HAMTs
		hamtDiff, err := hamt.Diff(ctx, cst, cst, ma.ProviderSectors, mb.ProviderSectors, hamt.UseTreeBitWidth(market13.ProviderSectorsHamtBitwidth))
		if err != nil {
			return err
		}

		for _, d := range hamtDiff {
			spIDk := must.One(abi.ParseUIntKey(d.Key))

			switch d.Type {
			case hamt.Add:
				color.Green("  ProviderSectors + Add f0%v", spIDk)

				var b cbg.CborCid
				if err := b.UnmarshalCBOR(bytes.NewReader(d.After.Raw)); err != nil {
					return err
				}

				fmt.Println("  |-B: ", cid.Cid(b).String())

				inner, err := adt13.AsMap(adt8.WrapStore(ctx, cst), cid.Cid(b), market13.ProviderSectorsHamtBitwidth)
				if err != nil {
					return err
				}

				var ids market13.SectorDealIDs
				err = inner.ForEach(&ids, func(k string) error {
					sectorNumber := must.One(abi.ParseUIntKey(k))

					color.Green("  |-- ProviderSectors + Add %v", sectorNumber)
					fmt.Printf("  |+: %v\n", ids)

					return nil
				})
				if err != nil {
					return err
				}
			case hamt.Remove:
				color.Red("  ProviderSectors - Remove f0%v", spIDk)
			case hamt.Modify:
				color.Yellow("  ProviderSectors ~ Modify f0%v", spIDk)

				var a, b cbg.CborCid
				if err := a.UnmarshalCBOR(bytes.NewReader(d.Before.Raw)); err != nil {
					return err
				}
				if err := b.UnmarshalCBOR(bytes.NewReader(d.After.Raw)); err != nil {
					return err
				}

				fmt.Println("  |-A: ", cid.Cid(b).String())
				fmt.Println("  |-B: ", cid.Cid(a).String())

				// diff the inner HAMTs
				innerHamtDiff, err := hamt.Diff(ctx, cst, cst, cid.Cid(a), cid.Cid(b), hamt.UseTreeBitWidth(market13.ProviderSectorsHamtBitwidth))
				if err != nil {
					return err
				}

				for _, d := range innerHamtDiff {
					sectorNumber := must.One(abi.ParseUIntKey(d.Key))

					switch d.Type {
					case hamt.Add:
						var b market13.SectorDealIDs

						if err := b.UnmarshalCBOR(bytes.NewReader(d.After.Raw)); err != nil {
							return err
						}

						color.Green("  |-- ProviderSectors + Add %v", sectorNumber)
						fmt.Printf("  |B: %v\n", b)
					case hamt.Remove:
						var a market13.SectorDealIDs

						if err := a.UnmarshalCBOR(bytes.NewReader(d.Before.Raw)); err != nil {
							return err
						}

						color.Red("  |-- ProviderSectors - Remove %v", sectorNumber)
						fmt.Printf("  |A: %v\n", a)
					case hamt.Modify:
						var a, b market13.SectorDealIDs

						if err := a.UnmarshalCBOR(bytes.NewReader(d.Before.Raw)); err != nil {
							return err
						}
						if err := b.UnmarshalCBOR(bytes.NewReader(d.After.Raw)); err != nil {
							return err
						}

						color.Yellow("  |-- ProviderSectors ~ Modify %v", sectorNumber)
						fmt.Printf("  |A: %v\n", a)
						fmt.Printf("  |B: %v\n", b)
					}
				}
			}
		}
	}

	return nil
}

func checkNv27Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {

	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version17)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v17.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}

func checkNv28Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {

	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version18)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v18.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}

func checkNv25Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {

	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version16)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v16.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}

func checkNv24Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {

	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version15)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v15.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}

func checkNv23Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {

	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version14)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v14.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}
func checkNv22Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {

	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version13)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v13.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}
func checkNv21Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {

	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version12)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v12.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}
func checkNv19Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {

	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version11)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v11.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}

func checkNv18Invariants(ctx context.Context, oldStateRootCid cid.Cid, newStateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {
	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	// Load the new state root.
	var newStateRoot types.StateRoot
	if err := actorStore.Get(ctx, newStateRootCid, &newStateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version10)
	if err != nil {
		return err
	}
	newActorTree, err := builtin.LoadTree(actorStore, newStateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v10.CheckStateInvariants(newActorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}

/// NV17 and earlier stuff

func checkNv17Invariants(ctx context.Context, v8StateRootCid cid.Cid, v9StateRootCid cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) error {
	actorStore := store.ActorStore(ctx, bs)
	startTime := time.Now()

	stateTreeV8, err := state.LoadStateTree(actorStore, v8StateRootCid)
	if err != nil {
		return err
	}

	stateTreeV9, err := state.LoadStateTree(actorStore, v9StateRootCid)
	if err != nil {
		return err
	}

	err = checkDatacaps(stateTreeV8, stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	err = checkPendingVerifiedDeals(stateTreeV8, stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	err = checkAllMinersUnsealedCID(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	// Load the state root.
	var v9stateRoot types.StateRoot
	if err := actorStore.Get(ctx, v9StateRootCid, &v9stateRoot); err != nil {
		return xerrors.Errorf("failed to decode state root: %w", err)
	}

	actorCodeCids, err := actors.GetActorCodeIDs(actorstypes.Version9)
	if err != nil {
		return err
	}
	v9actorTree, err := builtin.LoadTree(actorStore, v9stateRoot.Actors)
	if err != nil {
		return err
	}
	messages, err := v9.CheckStateInvariants(v9actorTree, epoch, actorCodeCids)
	if err != nil {
		return xerrors.Errorf("checking state invariants: %w", err)
	}

	for _, message := range messages.Messages() {
		fmt.Println("got the following error: ", message)
	}

	fmt.Println("completed invariant checks, took ", time.Since(startTime))

	return nil
}

func checkDatacaps(stateTreeV8 *state.StateTree, stateTreeV9 *state.StateTree, actorStore adt.Store) error {
	verifregDatacaps, err := getVerifreg8Datacaps(stateTreeV8, actorStore)
	if err != nil {
		return err
	}

	newDatacaps, err := getDatacap9Datacaps(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	// Should have all the v8 datacaps, plus the verifreg actor itself
	if len(verifregDatacaps)+1 != len(newDatacaps) {
		return xerrors.Errorf("size of datacap maps do not match. verifreg: %d, datacap: %d", len(verifregDatacaps), len(newDatacaps))
	}

	for addr, oldDcap := range verifregDatacaps {
		dcap, ok := newDatacaps[addr]
		if !ok {
			return xerrors.Errorf("datacap for address: %s not found in datacap state", addr)
		}
		if !dcap.Equals(oldDcap) {
			return xerrors.Errorf("datacap for address: %s do not match. verifreg: %d, datacap: %d", addr, oldDcap, dcap)
		}
	}

	return nil
}

func getVerifreg8Datacaps(stateTreeV8 *state.StateTree, actorStore adt.Store) (map[address.Address]abi.StoragePower, error) {
	verifregStateV8, err := getVerifregActorV8(stateTreeV8, actorStore)
	if err != nil {
		return nil, xerrors.Errorf("failed to get verifreg actor state: %w", err)
	}

	var verifregDatacaps = make(map[address.Address]abi.StoragePower)
	err = verifregStateV8.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
		verifregDatacaps[addr] = dcap
		return nil
	})
	if err != nil {
		return nil, err
	}

	return verifregDatacaps, nil
}

func getDatacap9Datacaps(stateTreeV9 *state.StateTree, actorStore adt.Store) (map[address.Address]abi.StoragePower, error) {
	datacapStateV9, err := getDatacapActorV9(stateTreeV9, actorStore)
	if err != nil {
		return nil, xerrors.Errorf("failed to get datacap actor state: %w", err)
	}

	var datacaps = make(map[address.Address]abi.StoragePower)
	err = datacapStateV9.ForEachClient(func(addr address.Address, dcap abi.StoragePower) error {
		datacaps[addr] = dcap
		return nil
	})
	if err != nil {
		return nil, err
	}

	return datacaps, nil
}

func checkPendingVerifiedDeals(stateTreeV8 *state.StateTree, stateTreeV9 *state.StateTree, actorStore adt.Store) error {
	marketActorV9, err := getMarketActorV9(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	verifregActorV9, err := getVerifregActorV9(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	verifregStateV9, err := getVerifregStateV9(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	marketStateV8, err := getMarketStateV8(stateTreeV8, actorStore)
	if err != nil {
		return err
	}

	marketStateV9, err := getMarketStateV9(stateTreeV9, actorStore)
	if err != nil {
		return err
	}

	pendingProposalsV8, err := adt8.AsSet(actorStore, marketStateV8.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return xerrors.Errorf("failed to load pending proposals: %w", err)
	}

	dealProposalsV8, err := market8.AsDealProposalArray(actorStore, marketStateV8.Proposals)
	if err != nil {
		return xerrors.Errorf("failed to get proposals: %w", err)
	}

	// We only want those pending deals that haven't been activated -- an activated deal has an entry in dealStates8
	dealStates8, err := adt9.AsArray(actorStore, marketStateV8.States, market8.StatesAmtBitwidth)
	if err != nil {
		return xerrors.Errorf("failed to load v8 states array: %w", err)
	}

	var numPendingVerifiedDeals = 0
	var proposal market8.DealProposal
	err = dealProposalsV8.ForEach(&proposal, func(dealID int64) error {
		// If not verified, do nothing
		if !proposal.VerifiedDeal {
			return nil
		}

		pcid, err := proposal.Cid()
		if err != nil {
			return err
		}

		isPending, err := pendingProposalsV8.Has(abi.CidKey(pcid))
		if err != nil {
			return xerrors.Errorf("failed to check pending: %w", err)
		}

		// Nothing to do for not-pending deals
		if !isPending {
			return nil
		}

		var _dealState8 market8.DealState
		found, err := dealStates8.Get(uint64(dealID), &_dealState8)
		if err != nil {
			return xerrors.Errorf("failed to lookup deal state: %w", err)
		}

		// the deal has an entry in deal states, which means it's already been allocated, nothing to do
		if found {
			return nil
		}

		numPendingVerifiedDeals++
		// Checks if allocation ID is in market map
		allocationId, err := marketActorV9.GetAllocationIdForPendingDeal(abi.DealID(dealID))
		if err != nil {
			return err
		}

		// Checks if allocation is in verifreg
		allocation, found, err := verifregActorV9.GetAllocation(proposal.Client, allocationId)
		if !found {
			return xerrors.Errorf("allocation %d not found for address %s", allocationId, proposal.Client)
		}
		if err != nil {
			return err
		}

		err = compareProposalToAllocation(proposal, *allocation)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("Pending Verified deals in market v8: %d\n", numPendingVerifiedDeals)

	numAllocationIds, err := countAllocationIds(actorStore, marketStateV9)
	if err != nil {
		return err
	}
	fmt.Printf("Allocation IDs in market v9: %d\n", numAllocationIds)

	if numAllocationIds != numPendingVerifiedDeals {
		return xerrors.Errorf("number of allocation IDsf: %d did not match the number of pending verified deals: %d", numAllocationIds, numPendingVerifiedDeals)
	}

	numAllocations, err := countAllocations(verifregStateV9, actorStore)
	if err != nil {
		return err
	}
	fmt.Printf("Allocations in verifreg v9: %d\n", numAllocations)

	if numAllocations != numPendingVerifiedDeals {
		return xerrors.Errorf("number of allocations: %d did not match the number of pending verified deals: %d", numAllocations, numPendingVerifiedDeals)
	}

	nextAllocationId := int(verifregStateV9.NextAllocationId)
	fmt.Printf("Next Allocation ID: %d\n", nextAllocationId)

	if numAllocations+1 != nextAllocationId {
		return xerrors.Errorf("number of allocations + 1: %d did not match the next allocation ID: %d", numAllocations+1, nextAllocationId)
	}

	return nil
}

func compareProposalToAllocation(prop market8.DealProposal, alloc verifreg9.Allocation) error {
	if prop.PieceCID != alloc.Data {
		return xerrors.Errorf("piece cid mismatch between proposal and allocation: %s, %s", prop.PieceCID, alloc.Data)
	}

	proposalClientID, err := address.IDFromAddress(prop.Client)
	if err != nil {
		return xerrors.Errorf("couldn't get ID from address")
	}
	if proposalClientID != uint64(alloc.Client) {
		return xerrors.Errorf("client id mismatch between proposal and allocation: %v, %v", proposalClientID, alloc.Client)
	}

	proposalProviderID, err := address.IDFromAddress(prop.Provider)
	if err != nil {
		return xerrors.Errorf("couldn't get ID from address")
	}
	if proposalProviderID != uint64(alloc.Provider) {
		return xerrors.Errorf("provider id mismatch between proposal and allocation: %v, %v", proposalProviderID, alloc.Provider)
	}

	if prop.PieceSize != alloc.Size {
		return xerrors.Errorf("piece size mismatch between proposal and allocation: %v, %v", prop.PieceSize, alloc.Size)
	}

	if alloc.TermMax != 540*builtin.EpochsInDay {
		return xerrors.Errorf("allocation term should be 540 days. Got %d epochs", alloc.TermMax)
	}

	if prop.EndEpoch-prop.StartEpoch != alloc.TermMin {
		return xerrors.Errorf("allocation term mismatch between proposal and allocation: %d, %d", prop.EndEpoch-prop.StartEpoch, alloc.TermMin)
	}

	return nil
}

func getMarketStateV8(stateTreeV8 *state.StateTree, actorStore adt.Store) (market8.State, error) {
	marketV8, err := stateTreeV8.GetActor(market.Address)
	if err != nil {
		return market8.State{}, err
	}

	var marketStateV8 market8.State
	if err = actorStore.Get(actorStore.Context(), marketV8.Head, &marketStateV8); err != nil {
		return market8.State{}, xerrors.Errorf("failed to get market actor state: %w", err)
	}

	return marketStateV8, nil
}

func getMarketStateV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (market9.State, error) {
	marketV9, err := stateTreeV9.GetActor(market.Address)
	if err != nil {
		return market9.State{}, err
	}

	var marketStateV9 market9.State
	if err = actorStore.Get(actorStore.Context(), marketV9.Head, &marketStateV9); err != nil {
		return market9.State{}, xerrors.Errorf("failed to get market actor state: %w", err)
	}

	return marketStateV9, nil
}

func getMarketActorV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (market.State, error) {
	marketV9, err := stateTreeV9.GetActor(market.Address)
	if err != nil {
		return nil, err
	}

	return market.Load(actorStore, marketV9)
}

func getVerifregActorV8(stateTreeV8 *state.StateTree, actorStore adt.Store) (verifreg.State, error) {
	verifregV8, err := stateTreeV8.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	return verifreg.Load(actorStore, verifregV8)
}

func getVerifregActorV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (verifreg.State, error) {
	verifregV9, err := stateTreeV9.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	return verifreg.Load(actorStore, verifregV9)
}

func getVerifregStateV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (verifreg9.State, error) {
	verifregV9, err := stateTreeV9.GetActor(verifreg.Address)
	if err != nil {
		return verifreg9.State{}, err
	}

	var verifregStateV9 verifreg9.State
	if err = actorStore.Get(actorStore.Context(), verifregV9.Head, &verifregStateV9); err != nil {
		return verifreg9.State{}, xerrors.Errorf("failed to get verifreg actor state: %w", err)
	}

	return verifregStateV9, nil
}

func getDatacapActorV9(stateTreeV9 *state.StateTree, actorStore adt.Store) (datacap.State, error) {
	datacapV9, err := stateTreeV9.GetActor(datacap.Address)
	if err != nil {
		return nil, err
	}

	return datacap.Load(actorStore, datacapV9)
}

func checkAllMinersUnsealedCID(stateTreeV9 *state.StateTree, store adt.Store) error {
	return stateTreeV9.ForEach(func(addr address.Address, actor *types.Actor) error {
		if !lbuiltin.IsStorageMinerActor(actor.Code) {
			return nil // no need to check
		}

		err := checkMinerUnsealedCID(actor, stateTreeV9, store)
		if err != nil {
			fmt.Println("failure for miner ", addr)
			return err
		}
		return nil
	})
}

func checkMinerUnsealedCID(act *types.Actor, stateTreeV9 *state.StateTree, store adt.Store) error {
	minerCodeCid, found := actors.GetActorCodeID(actorstypes.Version9, manifest.MinerKey)
	if !found {
		return xerrors.Errorf("could not find code cid for miner actor")
	}
	if minerCodeCid != act.Code {
		return nil // no need to check
	}

	marketActorV9, err := getMarketActorV9(stateTreeV9, store)
	if err != nil {
		return err
	}
	dealProposals, err := marketActorV9.Proposals()
	if err != nil {
		return err
	}

	m, err := miner.Load(store, act)
	if err != nil {
		return err
	}

	err = m.ForEachPrecommittedSector(func(info miner9.SectorPreCommitOnChainInfo) error {
		dealIDs := info.Info.DealIDs

		if len(dealIDs) == 0 {
			return nil // Nothing to check here
		}

		pieceCids := make([]abi.PieceInfo, len(dealIDs))
		for i, dealId := range dealIDs {
			dealProposal, found, err := dealProposals.Get(dealId)
			if err != nil {
				return err
			}
			if !found {
				return nil
			}

			pieceCids[i] = abi.PieceInfo{
				Size:     dealProposal.PieceSize,
				PieceCID: dealProposal.PieceCID,
			}
		}

		if len(pieceCids) == 0 {
			return nil
		}

		if info.Info.UnsealedCid == nil {
			return xerrors.Errorf("nil unsealed CID for sector with deals")
		}

		pieceCID, _, err := commp.PieceAggregateCommP(abi.RegisteredSealProof_StackedDrg64GiBV1_1, pieceCids)
		if err != nil {
			return err
		}

		if pieceCID != *info.Info.UnsealedCid {
			return xerrors.Errorf("calculated piece CID %s did not match unsealed CID in precommitted sector info: %s", pieceCID, *info.Info.UnsealedCid)
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func countAllocations(verifregState verifreg9.State, store adt.Store) (int, error) {
	var count = 0

	actorToHamtMap, err := adt9.AsMap(store, verifregState.Allocations, builtin.DefaultHamtBitwidth)
	if err != nil {
		return 0, xerrors.Errorf("couldn't get outer map: %x", err)
	}

	var innerHamtCid cbg.CborCid
	err = actorToHamtMap.ForEach(&innerHamtCid, func(key string) error {
		innerMap, err := adt9.AsMap(store, cid.Cid(innerHamtCid), builtin.DefaultHamtBitwidth)
		if err != nil {
			return xerrors.Errorf("couldn't get outer map: %x", err)
		}

		var allocation verifreg9.Allocation
		err = innerMap.ForEach(&allocation, func(key string) error {
			count++
			return nil
		})
		if err != nil {
			return xerrors.Errorf("couldn't iterate over inner map: %x", err)
		}

		return nil
	})
	if err != nil {
		return 0, xerrors.Errorf("couldn't iterate over outer map: %x", err)
	}

	return count, nil
}

func countAllocationIds(store adt.Store, marketState market9.State) (int, error) {
	allocationIds, err := adt9.AsMap(store, marketState.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
	if err != nil {
		return 0, err
	}

	var numAllocationIds int
	_ = allocationIds.ForEach(nil, func(key string) error {
		numAllocationIds++
		return nil
	})

	return numAllocationIds, nil
}
