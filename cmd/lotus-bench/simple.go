package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	prf "github.com/filecoin-project/specs-actors/actors/runtime/proof"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var simpleCmd = &cli.Command{
	Name:  "simple",
	Usage: "Run basic sector operations",
	Description: `Example sealing steps:

> Create unsealed sector file

$ ./lotus-bench simple addpiece --sector-size 2K /dev/zero /tmp/unsealed
AddPiece 25.23225ms (79.26 KiB/s)
baga6ea4seaqpy7usqklokfx2vxuynmupslkeutzexe2uqurdg5vhtebhxqmpqmy 2048

> Run PreCommit1

$ ./lotus-bench simple precommit1 --sector-size 2k /tmp/unsealed /tmp/sealed /tmp/cache baga6ea4seaqpy7usqklokfx2vxuynmupslkeutzexe2uqurdg5vhtebhxqmpqmy 2048
PreCommit1 30.151666ms (66.33 KiB/s)
eyJfbG90dXNfU2VhbFJhbmRvbW5lc3MiOiJBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFB[...]==

> Run PreCommit2

$ ./lotus-bench simple precommit2 --sector-size 2k /tmp/sealed /tmp/cache eyJfbG90dXNfU2VhbFJhbmRvbW5lc3MiOiJBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFB[...]==
PreCommit2 75.320167ms (26.55 KiB/s)
d:baga6ea4seaqpy7usqklokfx2vxuynmupslkeutzexe2uqurdg5vhtebhxqmpqmy r:bagboea4b5abcbrshxgmmpaucffwp2elaofbcrvb7hmcu3653o4lsw2arlor4hn3c

> Run Commit1

$ ./lotus-bench simple commit1 --sector-size 2k /tmp/sealed /tmp/cache baga6ea4seaqpy7usqklokfx2vxuynmupslkeutzexe2uqurdg5vhtebhxqmpqmy bagboea4b5abcbrshxgmmpaucffwp2elaofbcrvb7hmcu3653o4lsw2arlor4hn3c /tmp/c1.json
Commit1 20.691875ms (96.66 KiB/s)

> Run Commit2

$ ./lotus-bench simple commit2 /tmp/c1.json
[...]
Commit2: 13.829147792s (148 B/s)
proof: 8b624a6a4b272a6196517f858d07205c586cfae77fc026e8e9340acefbb8fc1d5af25b33724756c0a4481a800e14ff1ea914c3ce20bf6e2932296ad8ffa32867989ceae62e50af1479ca56a1ea5228cc8acf5ca54bc0b8e452bf74194b758b2c12ece76599a8b93f6b3dd9f0b1bb2e023bf311e9a404c7d453aeddf284e46025b63b631610de6ff6621bc6f630a14dd3ad59edbe6e940fdebbca3d97bea2708fd21764ea929f4699ebc93d818037a74be3363bdb2e8cc29b3e386c6376ff98fa

----
Example PoSt steps:

> Try window-post

$ ./lotus-bench simple window-post --sector-size 2k /tmp/sealed /tmp/cache bagboea4b5abcbrshxgmmpaucffwp2elaofbcrvb7hmcu3653o4lsw2arlor4hn3c 1
Vanilla 14.192625ms (140.9 KiB/s)
Proof 387.819333ms (5.156 KiB/s)
mI6TdveK9wMqHwVsRlVa90q44yGEIsNqLpTQLB...

> Try winning-post

$ ./lotus-bench simple winning-post --sector-size 2k /tmp/sealed /tmp/cache bagboea4b5abcbrshxgmmpaucffwp2elaofbcrvb7hmcu3653o4lsw2arlor4hn3c 1
Vanilla 19.266625ms (103.8 KiB/s)
Proof 1.234634708s (1.619 KiB/s)
o4VBUf2wBHuvmm58XY8wgCC/1xBqfujlgmNs...

----
Example SnapDeals steps:

> Create unsealed update file

$ ./lotus-bench simple addpiece --sector-size 2K /dev/random /tmp/new-unsealed
AddPiece 23.845958ms (83.87 KiB/s)
baga6ea4seaqkt24j5gbf2ye2wual5gn7a5yl2tqb52v2sk4nvur4bdy7lg76cdy 2048

> Create updated sealed file

$ ./lotus-bench simple replicaupdate --sector-size 2K /tmp/sealed /tmp/cache /tmp/new-unsealed /tmp/update /tmp/update-cache baga6ea4seaqkt24j5gbf2ye2wual5gn7a5yl2tqb52v2sk4nvur4bdy7lg76cdy 2048
ReplicaUpdate 63.0815ms (31.7 KiB/s)
d:baga6ea4seaqkt24j5gbf2ye2wual5gn7a5yl2tqb52v2sk4nvur4bdy7lg76cdy r:bagboea4b5abcaydcwlbtdx5dph2a3efpqt42emxpn3be76iu4e4lx3ltrpmpi7af

> Run ProveReplicaUpdate1

$ ./lotus-bench simple provereplicaupdate1 --sector-size 2K /tmp/sealed /tmp/cache /tmp/update /tmp/update-cache bagboea4b5abcbrshxgmmpaucffwp2elaofbcrvb7hmcu3653o4lsw2arlor4hn3c bagboea4b5abcaydcwlbtdx5dph2a3efpqt42emxpn3be76iu4e4lx3ltrpmpi7af baga6ea4seaqkt24j5gbf2ye2wual5gn7a5yl2tqb52v2sk4nvur4bdy7lg76cdy /tmp/pr1.json
ProveReplicaUpdate1 18.373375ms (108.9 KiB/s)

> Run ProveReplicaUpdate2

$ ./lotus-bench simple provereplicaupdate2 --sector-size 2K bagboea4b5abcbrshxgmmpaucffwp2elaofbcrvb7hmcu3653o4lsw2arlor4hn3c bagboea4b5abcaydcwlbtdx5dph2a3efpqt42emxpn3be76iu4e4lx3ltrpmpi7af baga6ea4seaqkt24j5gbf2ye2wual5gn7a5yl2tqb52v2sk4nvur4bdy7lg76cdy /tmp/pr1.json
ProveReplicaUpdate2 7.339033459s (279 B/s)
p: pvC0JBrEyUqtIIUvB2UUx/2a24c3Cvnu6AZ0D3IMBYAu...
`,
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

func (b benchSectorProvider) AcquireSectorCopy(ctx context.Context, id storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, ptype storiface.PathType) (storiface.SectorPaths, func(), error) {
	// there's no copying in this context
	return b.AcquireSector(ctx, id, existing, allocate, ptype)
}

func (b benchSectorProvider) AcquireSector(ctx context.Context, id storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, ptype storiface.PathType) (storiface.SectorPaths, func(), error) {
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

		sr := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize, miner.SealProofVariant_Standard),
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

		took := time.Since(start)

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
		&cli.BoolFlag{
			Name:  "synthetic",
			Usage: "generate synthetic PoRep proofs",
		},
		&cli.BoolFlag{
			Name:  "non-interactive",
			Usage: "generate NI-PoRep proofs",
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

		variant, err := variantFromArgs(cctx)
		if err != nil {
			return err
		}

		sr := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize, variant),
		}

		ticket := [32]byte{}
		for i := range ticket {
			ticket[i] = 1
		}

		pieces, err := ParsePieceInfos(cctx, 3)
		if err != nil {
			return err
		}

		start := time.Now()

		p1o, err := sealer.SealPreCommit1(ctx, sr, ticket[:], pieces)
		if err != nil {
			return xerrors.Errorf("precommit1: %w", err)
		}

		took := time.Since(start)

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
		&cli.BoolFlag{
			Name:  "synthetic",
			Usage: "generate synthetic PoRep proofs",
		},
		&cli.BoolFlag{
			Name:  "non-interactive",
			Usage: "generate NI-PoRep proofs",
		},
		&cli.StringFlag{
			Name:  "external-pc2",
			Usage: "command for computing PC2 externally",
		},
	},
	Description: `Compute PreCommit2 inputs and seal a sector.

--external-pc2 can be used to compute the PreCommit2 inputs externally.
The flag behaves similarly to the related lotus-worker flag, using it in
lotus-bench may be useful for testing if the external PreCommit2 command is
invoked correctly.

The command will be called with a number of environment variables set:
* EXTSEAL_PC2_SECTOR_NUM: the sector number
* EXTSEAL_PC2_SECTOR_MINER: the miner id
* EXTSEAL_PC2_PROOF_TYPE: the proof type
* EXTSEAL_PC2_SECTOR_SIZE: the sector size in bytes
* EXTSEAL_PC2_CACHE: the path to the cache directory
* EXTSEAL_PC2_SEALED: the path to the sealed sector file (initialized with unsealed data by the caller)
* EXTSEAL_PC2_PC1OUT: output from rust-fil-proofs precommit1 phase (base64 encoded json)

The command is expected to:
* Create cache sc-02-data-tree-r* files
* Create cache sc-02-data-tree-c* files
* Create cache p_aux / t_aux files
* Transform the sealed file in place

Example invocation of lotus-bench as external executor:
'./lotus-bench simple precommit2 --sector-size $EXTSEAL_PC2_SECTOR_SIZE $EXTSEAL_PC2_SEALED $EXTSEAL_PC2_CACHE $EXTSEAL_PC2_PC1OUT'
`,
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

		var opts []ffiwrapper.FFIWrapperOpt

		if cctx.IsSet("external-pc2") {
			extSeal := ffiwrapper.ExternalSealer{
				PreCommit2: ffiwrapper.MakeExternPrecommit2(cctx.String("external-pc2")),
			}

			opts = append(opts, ffiwrapper.WithExternalSealCalls(extSeal))
		}

		sealer, err := ffiwrapper.New(pp, opts...)
		if err != nil {
			return err
		}

		p1o, err := base64.StdEncoding.DecodeString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		variant, err := variantFromArgs(cctx)
		if err != nil {
			return err
		}

		sr := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize, variant),
		}

		start := time.Now()

		p2o, err := sealer.SealPreCommit2(ctx, sr, p1o)
		if err != nil {
			return xerrors.Errorf("precommit2: %w", err)
		}

		took := time.Since(start)

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
		&cli.BoolFlag{
			Name:  "synthetic",
			Usage: "generate synthetic PoRep proofs",
		},
		&cli.BoolFlag{
			Name:  "non-interactive",
			Usage: "generate NI-PoRep proofs",
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

		variant, err := variantFromArgs(cctx)
		if err != nil {
			return err
		}

		sr := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize, variant),
		}

		start := time.Now()

		ticket := [32]byte{}
		seed := [32]byte{}
		for i := range ticket {
			ticket[i] = 1
			seed[i] = 1
		}

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
		}, storiface.SectorCids{
			Unsealed: commd,
			Sealed:   commr,
		})
		if err != nil {
			return xerrors.Errorf("commit1: %w", err)
		}

		took := time.Since(start)

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

		if err := os.WriteFile(cctx.Args().Get(4), b, 0664); err != nil {
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
		&cli.BoolFlag{
			Name:  "synthetic",
			Usage: "generate synthetic PoRep proofs",
		},
		&cli.BoolFlag{
			Name:  "non-interactive",
			Usage: "generate NI-PoRep proofs",
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

		inb, err := os.ReadFile(c.Args().First())
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

		variant, err := variantFromArgs(c)
		if err != nil {
			return err
		}

		ref := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(c2in.SectorNum),
			},
			ProofType: spt(abi.SectorSize(c2in.SectorSize), variant),
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

		wpt, err := spt(sectorSize, miner.SealProofVariant_Standard).RegisteredWindowPoStProof()
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
				SealProof:    spt(sectorSize, miner.SealProofVariant_Standard),
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
		&cli.BoolFlag{
			Name:  "show-inputs",
			Usage: "output inputs for winning post generation",
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

		wpt, err := spt(sectorSize, miner.SealProofVariant_Standard).RegisteredWinningPoStProof()
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
				SealProof:    spt(sectorSize, miner.SealProofVariant_Standard),
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

		if cctx.Bool("show-inputs") {
			fmt.Println("GenerateWinningPoStWithVanilla info:")

			fmt.Printf(" wpt: %d\n", wpt)
			fmt.Printf(" mid: %d\n", mid)
			fmt.Printf(" rand: %x\n", rand)
			fmt.Printf(" vp: %x\n", vp)
			fmt.Printf(" proof: %x\n", proof)
		}

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
		sr := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize, miner.SealProofVariant_Standard),
		}

		start := time.Now()

		ruo, err := sealer.ReplicaUpdate(ctx, sr, pieces)
		if err != nil {
			return xerrors.Errorf("replica update: %w", err)
		}

		took := time.Since(start)

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

		sr := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize, miner.SealProofVariant_Standard),
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

		took := time.Since(start)

		fmt.Printf("ProveReplicaUpdate1 %s (%s)\n", took, bps(sectorSize, 1, took))

		vpjb, err := json.Marshal(&rvp)
		if err != nil {
			return xerrors.Errorf("json marshal vanilla proofs: %w", err)
		}

		if err := os.WriteFile(cctx.Args().Get(7), vpjb, 0666); err != nil {
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

		sr := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: 1,
			},
			ProofType: spt(sectorSize, miner.SealProofVariant_Standard),
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

		vpb, err := os.ReadFile(cctx.Args().Get(3))
		if err != nil {
			return xerrors.Errorf("reading valilla proof file: %w", err)
		}

		var vp storiface.ReplicaVanillaProofs
		if err := json.Unmarshal(vpb, &vp); err != nil {
			return xerrors.Errorf("unmarshalling vanilla proofs: %w", err)
		}

		p, err := sealer.ProveReplicaUpdate2(ctx, sr, oldcommr, commr, commd, vp)
		if err != nil {
			return xerrors.Errorf("prove replica update2: %w", err)
		}

		took := time.Since(start)

		fmt.Printf("ProveReplicaUpdate2 %s (%s)\n", took, bps(sectorSize, 1, took))
		fmt.Println("p:", base64.StdEncoding.EncodeToString(p))

		return nil
	},
}

func ParsePieceInfos(cctx *cli.Context, firstArg int) ([]abi.PieceInfo, error) {
	args := cctx.NArg() - firstArg
	if args%2 != 0 {
		return nil, xerrors.Errorf("piece info argunemts need to be supplied in pairs")
	}
	if args < 2 {
		return nil, xerrors.Errorf("need at least one piece info argument")
	}

	out := make([]abi.PieceInfo, args/2)

	for i := 0; i < args/2; i++ {
		c, err := cid.Parse(cctx.Args().Get(firstArg + (i * 2)))
		if err != nil {
			return nil, xerrors.Errorf("parse piece cid: %w", err)
		}

		psize, err := strconv.ParseUint(cctx.Args().Get(firstArg+(i*2)+1), 10, 64)
		if err != nil {
			return nil, xerrors.Errorf("parse piece size: %w", err)
		}

		out[i] = abi.PieceInfo{
			Size:     abi.PaddedPieceSize(psize),
			PieceCID: c,
		}
	}

	return out, nil
}

func variantFromArgs(cctx *cli.Context) (miner.SealProofVariant, error) {
	variant := miner.SealProofVariant_Standard
	if cctx.Bool("synthetic") {
		if cctx.Bool("non-interactive") {
			return variant, xerrors.Errorf("can't use both synthetic and non-interactive")
		}
		variant = miner.SealProofVariant_Synthetic
	} else if cctx.Bool("non-interactive") {
		variant = miner.SealProofVariant_NonInteractive
	}
	return variant, nil
}
