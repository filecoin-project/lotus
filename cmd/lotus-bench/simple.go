package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/docker/go-units"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
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
		simpleWindowPost,
		simpleWinningPost,
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

		pieces, err := ParsePieceInfos(cctx)
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

func ParsePieceInfos(cctx *cli.Context) ([]abi.PieceInfo, error) {
	// supports only one for now

	c, err := cid.Parse(cctx.Args().Get(3))
	if err != nil {
		return nil, xerrors.Errorf("parse piece cid: %w", err)
	}

	psize, err := strconv.ParseUint(cctx.Args().Get(4), 10, 64)
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
