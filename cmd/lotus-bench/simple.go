package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/docker/go-units"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	prf "github.com/filecoin-project/specs-actors/actors/runtime/proof"
	"github.com/filecoin-project/specs-storage/storage"
)

var simpleCmd = &cli.Command{
	Name:  "simple",
	Usage: "Run basic sector operations",
	Subcommands: []*cli.Command{
		simpleAddPiece,
		simplePreCommit1,
		simplePreCommit2,
		simpleCommit1,
		simpleCommit2,
		simpleWindowPost,
		simpleWinningPost,
		simpleReplicaUpdate,
		simpleProveReplicaUpdate1,
		simpleProveReplicaUpdate2,
	},
}

type benchSectorProvider map[storiface.SectorFileType]string

func (b benchSectorProvider) AcquireSector(ctx context.Context, id storage.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, ptype storiface.PathType) (storiface.SectorPaths, func(), error) {
	out := storiface.SectorPaths{
		ID:          id.ID,
		Unsealed:    b[storiface.FTUnsealed],
		Sealed:      b[storiface.FTSealed],
		Cache:       b[storiface.FTCache],
		Update:      b[storiface.FTUpdate],
		UpdateCache: b[storiface.FTUpdateCache],
	}
	return out, func() {}, nil
}

var _ ffiwrapper.SectorProvider = &benchSectorProvider{}

var simpleAddPiece = &cli.Command{
	Name:      "addpiece",
	ArgsUsage: "[data] [unsealed]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "512MiB",
			Usage: "size of the sectors in bytes, i.e. 32GiB",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		maddr, err := address.NewFromString(cctx.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		pp := benchSectorProvider{
			storiface.FTUnsealed: cctx.Args().Get(1),
		}
		sealer, err := ffiwrapper.New(pp)
		if err != nil {
			return err
		}

		sr := storage.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize),
		}

		data, err := os.Open(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("open data file: %w", err)
		}

		start := time.Now()

		pi, err := sealer.AddPiece(ctx, sr, []abi.UnpaddedPieceSize{}, abi.PaddedPieceSize(sectorSize).Unpadded(), data)
		if err != nil {
			return xerrors.Errorf("add piece: %w", err)
		}

		took := time.Now().Sub(start)

		fmt.Printf("AddPiece %s (%s)\n", took, bps(abi.SectorSize(pi.Size), 1, took))
		fmt.Printf("%s %d\n", pi.PieceCID, pi.Size)

		return nil
	},
}

var simplePreCommit1 = &cli.Command{
	Name: "precommit1",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "512MiB",
			Usage: "size of the sectors in bytes, i.e. 32GiB",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	ArgsUsage: "[unsealed] [sealed] [cache] [[piece cid] [piece size]]...",
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		maddr, err := address.NewFromString(cctx.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		pp := benchSectorProvider{
			storiface.FTUnsealed: cctx.Args().Get(0),
			storiface.FTSealed:   cctx.Args().Get(1),
			storiface.FTCache:    cctx.Args().Get(2),
		}
		sealer, err := ffiwrapper.New(pp)
		if err != nil {
			return err
		}

		sr := storage.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize),
		}

		var ticket [32]byte // all zero

		pieces, err := ParsePieceInfos(cctx, 3)
		if err != nil {
			return err
		}

		start := time.Now()

		p1o, err := sealer.SealPreCommit1(ctx, sr, ticket[:], pieces)
		if err != nil {
			return xerrors.Errorf("precommit1: %w", err)
		}

		took := time.Now().Sub(start)

		fmt.Printf("PreCommit1 %s (%s)\n", took, bps(sectorSize, 1, took))
		fmt.Println(base64.StdEncoding.EncodeToString(p1o))
		return nil
	},
}

var simplePreCommit2 = &cli.Command{
	Name: "precommit2",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "512MiB",
			Usage: "size of the sectors in bytes, i.e. 32GiB",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	ArgsUsage: "[sealed] [cache] [pc1 out]",
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		maddr, err := address.NewFromString(cctx.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		pp := benchSectorProvider{
			storiface.FTSealed: cctx.Args().Get(0),
			storiface.FTCache:  cctx.Args().Get(1),
		}
		sealer, err := ffiwrapper.New(pp)
		if err != nil {
			return err
		}

		p1o, err := base64.StdEncoding.DecodeString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		sr := storage.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize),
		}

		start := time.Now()

		p2o, err := sealer.SealPreCommit2(ctx, sr, p1o)
		if err != nil {
			return xerrors.Errorf("precommit2: %w", err)
		}

		took := time.Now().Sub(start)

		fmt.Printf("PreCommit2 %s (%s)\n", took, bps(sectorSize, 1, took))
		fmt.Printf("d:%s r:%s\n", p2o.Unsealed, p2o.Sealed)
		return nil
	},
}

var simpleCommit1 = &cli.Command{
	Name: "commit1",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "512MiB",
			Usage: "size of the sectors in bytes, i.e. 32GiB",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	ArgsUsage: "[sealed] [cache] [comm D] [comm R] [c1out.json]",
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		maddr, err := address.NewFromString(cctx.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		pp := benchSectorProvider{
			storiface.FTSealed: cctx.Args().Get(0),
			storiface.FTCache:  cctx.Args().Get(1),
		}
		sealer, err := ffiwrapper.New(pp)
		if err != nil {
			return err
		}

		sr := storage.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize),
		}

		start := time.Now()

		var ticket, seed [32]byte // all zero

		commd, err := cid.Parse(cctx.Args().Get(2))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		commr, err := cid.Parse(cctx.Args().Get(3))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		c1o, err := sealer.SealCommit1(ctx, sr, ticket[:], seed[:], []abi.PieceInfo{
			{
				Size:     abi.PaddedPieceSize(sectorSize),
				PieceCID: commd,
			},
		}, storage.SectorCids{
			Unsealed: commd,
			Sealed:   commr,
		})
		if err != nil {
			return xerrors.Errorf("commit1: %w", err)
		}

		took := time.Now().Sub(start)

		fmt.Printf("Commit1 %s (%s)\n", took, bps(sectorSize, 1, took))

		c2in := Commit2In{
			SectorNum:  int64(1),
			Phase1Out:  c1o,
			SectorSize: uint64(sectorSize),
		}

		b, err := json.Marshal(&c2in)
		if err != nil {
			return err
		}

		if err := ioutil.WriteFile(cctx.Args().Get(4), b, 0664); err != nil {
			log.Warnf("%+v", err)
		}

		return nil
	},
}

var simpleCommit2 = &cli.Command{
	Name:      "commit2",
	ArgsUsage: "[c1out.json]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-gpu",
			Usage: "disable gpu usage for the benchmark run",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	Action: func(c *cli.Context) error {
		if c.Bool("no-gpu") {
			err := os.Setenv("BELLMAN_NO_GPU", "1")
			if err != nil {
				return xerrors.Errorf("setting no-gpu flag: %w", err)
			}
		}

		if !c.Args().Present() {
			return xerrors.Errorf("Usage: lotus-bench prove [input.json]")
		}

		inb, err := ioutil.ReadFile(c.Args().First())
		if err != nil {
			return xerrors.Errorf("reading input file: %w", err)
		}

		var c2in Commit2In
		if err := json.Unmarshal(inb, &c2in); err != nil {
			return xerrors.Errorf("unmarshalling input file: %w", err)
		}

		if err := paramfetch.GetParams(lcli.ReqContext(c), build.ParametersJSON(), build.SrsJSON(), c2in.SectorSize); err != nil {
			return xerrors.Errorf("getting params: %w", err)
		}

		maddr, err := address.NewFromString(c.String("miner-addr"))
		if err != nil {
			return err
		}
		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}

		sb, err := ffiwrapper.New(nil)
		if err != nil {
			return err
		}

		ref := storage.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(c2in.SectorNum),
			},
			ProofType: spt(abi.SectorSize(c2in.SectorSize)),
		}

		start := time.Now()

		proof, err := sb.SealCommit2(context.TODO(), ref, c2in.Phase1Out)
		if err != nil {
			return err
		}

		sealCommit2 := time.Now()
		dur := sealCommit2.Sub(start)

		fmt.Printf("Commit2: %s (%s)\n", dur, bps(abi.SectorSize(c2in.SectorSize), 1, dur))
		fmt.Printf("proof: %x\n", proof)
		return nil
	},
}

var simpleWindowPost = &cli.Command{
	Name: "window-post",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "512MiB",
			Usage: "size of the sectors in bytes, i.e. 32GiB",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	ArgsUsage: "[sealed] [cache] [comm R] [sector num]",
	Action: func(cctx *cli.Context) error {
		maddr, err := address.NewFromString(cctx.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		var rand [32]byte // all zero

		commr, err := cid.Parse(cctx.Args().Get(2))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		wpt, err := spt(sectorSize).RegisteredWindowPoStProof()
		if err != nil {
			return err
		}

		snum, err := strconv.ParseUint(cctx.Args().Get(3), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing sector num: %w", err)
		}
		sn := abi.SectorNumber(snum)

		ch, err := ffi.GeneratePoStFallbackSectorChallenges(wpt, mid, rand[:], []abi.SectorNumber{sn})
		if err != nil {
			return xerrors.Errorf("generating challenges: %w", err)
		}

		start := time.Now()

		vp, err := ffi.GenerateSingleVanillaProof(ffi.PrivateSectorInfo{
			SectorInfo: prf.SectorInfo{
				SealProof:    spt(sectorSize),
				SectorNumber: sn,
				SealedCID:    commr,
			},
			CacheDirPath:     cctx.Args().Get(1),
			PoStProofType:    wpt,
			SealedSectorPath: cctx.Args().Get(0),
		}, ch.Challenges[sn])
		if err != nil {
			return err
		}

		challenge := time.Now()

		proof, err := ffi.GenerateSinglePartitionWindowPoStWithVanilla(wpt, mid, rand[:], [][]byte{vp}, 0)
		if err != nil {
			return xerrors.Errorf("generate post: %w", err)
		}

		end := time.Now()

		fmt.Printf("Vanilla %s (%s)\n", challenge.Sub(start), bps(sectorSize, 1, challenge.Sub(start)))
		fmt.Printf("Proof %s (%s)\n", end.Sub(challenge), bps(sectorSize, 1, end.Sub(challenge)))
		fmt.Println(base64.StdEncoding.EncodeToString(proof.ProofBytes))
		return nil
	},
}

var simpleWinningPost = &cli.Command{
	Name: "winning-post",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "512MiB",
			Usage: "size of the sectors in bytes, i.e. 32GiB",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	ArgsUsage: "[sealed] [cache] [comm R] [sector num]",
	Action: func(cctx *cli.Context) error {
		maddr, err := address.NewFromString(cctx.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		var rand [32]byte // all zero

		commr, err := cid.Parse(cctx.Args().Get(2))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		wpt, err := spt(sectorSize).RegisteredWinningPoStProof()
		if err != nil {
			return err
		}

		snum, err := strconv.ParseUint(cctx.Args().Get(3), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing sector num: %w", err)
		}
		sn := abi.SectorNumber(snum)

		ch, err := ffi.GeneratePoStFallbackSectorChallenges(wpt, mid, rand[:], []abi.SectorNumber{sn})
		if err != nil {
			return xerrors.Errorf("generating challenges: %w", err)
		}

		start := time.Now()

		vp, err := ffi.GenerateSingleVanillaProof(ffi.PrivateSectorInfo{
			SectorInfo: prf.SectorInfo{
				SealProof:    spt(sectorSize),
				SectorNumber: sn,
				SealedCID:    commr,
			},
			CacheDirPath:     cctx.Args().Get(1),
			PoStProofType:    wpt,
			SealedSectorPath: cctx.Args().Get(0),
		}, ch.Challenges[sn])
		if err != nil {
			return err
		}

		challenge := time.Now()

		proof, err := ffi.GenerateWinningPoStWithVanilla(wpt, mid, rand[:], [][]byte{vp})
		if err != nil {
			return xerrors.Errorf("generate post: %w", err)
		}

		end := time.Now()

		fmt.Printf("Vanilla %s (%s)\n", challenge.Sub(start), bps(sectorSize, 1, challenge.Sub(start)))
		fmt.Printf("Proof %s (%s)\n", end.Sub(challenge), bps(sectorSize, 1, end.Sub(challenge)))
		fmt.Println(base64.StdEncoding.EncodeToString(proof[0].ProofBytes))
		return nil
	},
}

var simpleReplicaUpdate = &cli.Command{
	Name: "replicaupdate",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "512MiB",
			Usage: "size of the sectors in bytes, i.e. 32GiB",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	ArgsUsage: "[sealed] [cache] [unsealed] [update] [updatecache] [[piece cid] [piece size]]...",
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		maddr, err := address.NewFromString(cctx.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		pp := benchSectorProvider{
			storiface.FTSealed:      cctx.Args().Get(0),
			storiface.FTCache:       cctx.Args().Get(1),
			storiface.FTUnsealed:    cctx.Args().Get(2),
			storiface.FTUpdate:      cctx.Args().Get(3),
			storiface.FTUpdateCache: cctx.Args().Get(4),
		}
		sealer, err := ffiwrapper.New(pp)
		if err != nil {
			return err
		}

		pieces, err := ParsePieceInfos(cctx, 5)
		if err != nil {
			return err
		}
		sr := storage.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize),
		}

		start := time.Now()

		ruo, err := sealer.ReplicaUpdate(ctx, sr, pieces)
		if err != nil {
			return xerrors.Errorf("replica update: %w", err)
		}

		took := time.Now().Sub(start)

		fmt.Printf("ReplicaUpdate %s (%s)\n", took, bps(sectorSize, 1, took))
		fmt.Printf("d:%s r:%s\n", ruo.NewUnsealed, ruo.NewSealed)
		return nil
	},
}

var simpleProveReplicaUpdate1 = &cli.Command{
	Name: "provereplicaupdate1",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "512MiB",
			Usage: "size of the sectors in bytes, i.e. 32GiB",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	ArgsUsage: "[sealed] [cache] [update] [updatecache] [sectorKey] [newSealed] [newUnsealed] [vproofs.json]",
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		maddr, err := address.NewFromString(cctx.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		pp := benchSectorProvider{
			storiface.FTSealed:      cctx.Args().Get(0),
			storiface.FTCache:       cctx.Args().Get(1),
			storiface.FTUpdate:      cctx.Args().Get(2),
			storiface.FTUpdateCache: cctx.Args().Get(3),
		}
		sealer, err := ffiwrapper.New(pp)
		if err != nil {
			return err
		}

		sr := storage.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize),
		}

		start := time.Now()

		oldcommr, err := cid.Parse(cctx.Args().Get(4))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		commr, err := cid.Parse(cctx.Args().Get(5))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		commd, err := cid.Parse(cctx.Args().Get(6))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		rvp, err := sealer.ProveReplicaUpdate1(ctx, sr, oldcommr, commr, commd)
		if err != nil {
			return xerrors.Errorf("replica update: %w", err)
		}

		took := time.Now().Sub(start)

		fmt.Printf("ProveReplicaUpdate1 %s (%s)\n", took, bps(sectorSize, 1, took))

		vpjb, err := json.Marshal(&rvp)
		if err != nil {
			return xerrors.Errorf("json marshal vanillla proofs: %w", err)
		}

		if err := ioutil.WriteFile(cctx.Args().Get(7), vpjb, 0666); err != nil {
			return xerrors.Errorf("writing vanilla proofs file: %w", err)
		}

		return nil
	},
}

var simpleProveReplicaUpdate2 = &cli.Command{
	Name: "provereplicaupdate2",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "512MiB",
			Usage: "size of the sectors in bytes, i.e. 32GiB",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	ArgsUsage: "[sectorKey] [newSealed] [newUnsealed] [vproofs.json]",
	Action: func(cctx *cli.Context) error {
		ctx := cctx.Context

		maddr, err := address.NewFromString(cctx.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		pp := benchSectorProvider{}
		sealer, err := ffiwrapper.New(pp)
		if err != nil {
			return err
		}

		sr := storage.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize),
		}

		start := time.Now()

		oldcommr, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		commr, err := cid.Parse(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		commd, err := cid.Parse(cctx.Args().Get(2))
		if err != nil {
			return xerrors.Errorf("parse commr: %w", err)
		}

		vpb, err := ioutil.ReadFile(cctx.Args().Get(3))
		if err != nil {
			return xerrors.Errorf("reading valilla proof file: %w", err)
		}

		var vp storage.ReplicaVanillaProofs
		if err := json.Unmarshal(vpb, &vp); err != nil {
			return xerrors.Errorf("unmarshalling vanilla proofs: %w", err)
		}

		p, err := sealer.ProveReplicaUpdate2(ctx, sr, oldcommr, commr, commd, vp)
		if err != nil {
			return xerrors.Errorf("prove replica update2: %w", err)
		}

		took := time.Now().Sub(start)

		fmt.Printf("ProveReplicaUpdate2 %s (%s)\n", took, bps(sectorSize, 1, took))
		fmt.Println("p:", base64.StdEncoding.EncodeToString(p))

		return nil
	},
}

func ParsePieceInfos(cctx *cli.Context, firstArg int) ([]abi.PieceInfo, error) {
	// supports only one for now

	c, err := cid.Parse(cctx.Args().Get(firstArg))
	if err != nil {
		return nil, xerrors.Errorf("parse piece cid: %w", err)
	}

	psize, err := strconv.ParseUint(cctx.Args().Get(firstArg+1), 10, 64)
	if err != nil {
		return nil, xerrors.Errorf("parse piece size: %w", err)
	}

	return []abi.PieceInfo{
		{
			Size:     abi.PaddedPieceSize(psize),
			PieceCID: c,
		},
	}, nil
}
