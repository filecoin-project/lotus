package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"sort"
	"strconv"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/parmap"
)

var sectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "Tools for interacting with sectors",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		terminateSectorCmd,
		visAllocatedSectorsCmd,
	},
}

var terminateSectorCmd = &cli.Command{
	Name:      "terminate",
	Usage:     "Forcefully terminate a sector (WARNING: This means losing power and pay a one-time termination penalty(including collateral) for the terminated sector)",
	ArgsUsage: "[sectorNum1 sectorNum2 ...]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 1 {
			return fmt.Errorf("at least one sector must be specified")
		}

		if !cctx.Bool("really-do-it") {
			return fmt.Errorf("this is a command for advanced users, only use it if you are sure of what you are doing")
		}

		nodeApi, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := api.ActorAddress(ctx)
		if err != nil {
			return err
		}

		mi, err := nodeApi.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		terminationDeclarationParams := []miner2.TerminationDeclaration{}

		for _, sn := range cctx.Args().Slice() {
			sectorNum, err := strconv.ParseUint(sn, 10, 64)
			if err != nil {
				return fmt.Errorf("could not parse sector number: %w", err)
			}

			sectorbit := bitfield.New()
			sectorbit.Set(sectorNum)

			loca, err := nodeApi.StateSectorPartition(ctx, maddr, abi.SectorNumber(sectorNum), types.EmptyTSK)
			if err != nil {
				return fmt.Errorf("get state sector partition %s", err)
			}

			para := miner2.TerminationDeclaration{
				Deadline:  loca.Deadline,
				Partition: loca.Partition,
				Sectors:   sectorbit,
			}

			terminationDeclarationParams = append(terminationDeclarationParams, para)
		}

		terminateSectorParams := &miner2.TerminateSectorsParams{
			Terminations: terminationDeclarationParams,
		}

		sp, err := actors.SerializeParams(terminateSectorParams)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := nodeApi.MpoolPushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: miner.Methods.TerminateSectors,

			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push message: %w", err)
		}

		fmt.Println("sent termination message:", smsg.Cid())

		wait, err := nodeApi.StateWaitMsg(ctx, smsg.Cid(), uint64(cctx.Int("confidence")))
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("terminate sectors message returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var visAllocatedSectorsCmd = &cli.Command{
	Name:  "vis-allocated",
	Usage: "Produces a html with visualisation of allocated sectors",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		var miners []address.Address
		if cctx.NArg() == 0 {
			miners, err = api.StateListMiners(ctx, types.EmptyTSK)
			if err != nil {
				return err
			}
			powCache := make(map[address.Address]types.BigInt)
			var lk sync.Mutex
			parmap.Par(32, miners, func(a address.Address) {
				pow, err := api.StateMinerPower(ctx, a, types.EmptyTSK)
				if err != nil {
				}

				lk.Lock()
				if err == nil {
					powCache[a] = pow.MinerPower.QualityAdjPower
				} else {
					powCache[a] = types.NewInt(0)
				}
				lk.Unlock()
			})
			sort.Slice(miners, func(i, j int) bool {
				return powCache[miners[i]].GreaterThan(powCache[miners[j]])
			})
			n := sort.Search(len(miners), func(i int) bool {
				pow := powCache[miners[i]]
				log.Infof("pow @%d = %s", i, pow)
				return pow.IsZero()
			})
			miners = miners[:n]
		} else {
			for _, mS := range cctx.Args().Slice() {
				mA, err := address.NewFromString(mS)
				if err != nil {
					return xerrors.Errorf("parsing address '%s': %w", mS, err)
				}
				miners = append(miners, mA)
			}
		}

		pngs := make([][]byte, len(miners))
		for i := 0; i < len(miners); i++ {
			func() {
				state, err := api.StateReadState(ctx, miners[i], types.EmptyTSK)
				if err != nil {
					log.Errorf("getting state: %+v", err)
					return
				}
				allocSString := state.State.(map[string]interface{})["AllocatedSectors"].(map[string]interface{})["/"].(string)

				allocCid, err := cid.Decode(allocSString)
				if err != nil {
					log.Errorf("decoding cid: %+v", err)
					return
				}
				rle, err := api.ChainReadObj(ctx, allocCid)
				if err != nil {
					log.Errorf("reading AllocatedSectors: %+v", err)
					return
				}
				png, err := rleToPng(rle)
				if err != nil {
					log.Errorf("converting to png: %+v", err)
					return
				}
				pngs[i] = png
				encoded := base64.StdEncoding.EncodeToString(pngs[i])
				fmt.Printf(`%s:</br><img src="data:image/png;base64,%s"></br>`+"\n", miners[i], encoded)
				_ = os.Stdout.Sync()
			}()
		}

		return nil
	},
}

func rleToPng(rleBytes []byte) ([]byte, error) {
	var bf bitfield.BitField
	err := bf.UnmarshalCBOR(bytes.NewReader(rleBytes))
	if err != nil {
		return nil, xerrors.Errorf("decoding bitfield: %w", err)
	}
	{
		last, err := bf.Last()
		if err != nil {
			return nil, xerrors.Errorf("getting last: %w", err)
		}
		if last == 0 {
			return nil, nil
		}
	}
	ri, err := bf.RunIterator()
	if err != nil {
		return nil, xerrors.Errorf("creating interator: %w", err)
	}

	const width = 1024
	const skipTh = 64
	const skipSize = 32

	var size uint64
	for ri.HasNext() {
		run, err := ri.NextRun()
		if err != nil {
			return nil, xerrors.Errorf("getting next run: %w", err)
		}
		if run.Len > skipTh*width {
			size += run.Len%(2*width) + skipSize*width
		} else {
			size += run.Len
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, width, int((size+width-1)/width)))
	for i := range img.Pix {
		img.Pix[i] = 255
	}

	ri, err = bf.RunIterator()
	if err != nil {
		return nil, xerrors.Errorf("creating interator: %w", err)
	}

	const shade = 15
	idx := uint64(0)
	realIdx := uint64(0)
	for ri.HasNext() {
		run, err := ri.NextRun()
		if err != nil {
			return nil, xerrors.Errorf("getting next run: %w", err)
		}
		var cut = false
		var oldLen uint64
		if run.Len > skipTh*width {
			oldLen = run.Len
			run.Len = run.Len%(2*width) + skipSize*width
			cut = true
		}
		for i := uint64(0); i < run.Len; i++ {
			col := color.Gray{0}
			stripe := (realIdx+i)/width%256 >= 128
			if cut && i > skipSize*width/2 {
				stripe = (realIdx+i+(skipSize/2*width))/width%256 >= 128
			}
			if !run.Val {
				col.Y = 255
				if stripe {
					col.Y -= shade
				}
			} else if stripe {
				col.Y += shade
			}
			img.Set(int((idx+i)%width), int((idx+i)/width), col)
		}
		if cut {
			i := (idx + run.Len/2 + width) &^ (width - 1)
			iend := i + width
			col := color.RGBA{255, 0, 0, 255}
			for ; i < iend; i++ {
				img.Set(int(i)%width, int(i)/width, col)
			}
			realIdx += oldLen
			idx += run.Len
		} else {
			realIdx += run.Len
			idx += run.Len
		}
	}
	buf := &bytes.Buffer{}
	err = png.Encode(buf, img)
	if err != nil {
		return nil, xerrors.Errorf("encoding png: %w", err)
	}

	return buf.Bytes(), nil
}
