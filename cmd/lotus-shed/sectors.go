package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"sort"
	"strconv"
	"sync"

	"github.com/cheggaaa/pb/v3"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli"
	"github.com/filecoin-project/lotus/lib/parmap"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/fr32"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var sectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "Tools for interacting with sectors",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		spcli.TerminateSectorCmd(shedGetActor),
		terminateSectorPenaltyEstimationCmd,
		visAllocatedSectorsCmd,
		dumpRLESectorCmd,
		dumpSectorOnChainInfoCmd,
		sectorReadCmd,
		sectorDeleteCmd,
	},
}

func shedGetActor(cctx *cli.Context) (address.Address, error) {
	minerApi, acloser, err := lcli.GetStorageMinerAPI(cctx)
	if err != nil {
		return address.Address{}, err
	}
	defer acloser()

	return minerApi.ActorAddress(cctx.Context)
}

func findPenaltyInInternalExecutions(prefix string, trace []types.ExecutionTrace) {
	for _, im := range trace {
		if im.Msg.To.String() == "f099" /*Burn actor*/ {
			fmt.Printf("Estimated termination penalty: %s attoFIL\n", im.Msg.Value)
			return
		}
		findPenaltyInInternalExecutions(prefix+"\t", im.Subcalls)
	}
}

var terminateSectorPenaltyEstimationCmd = &cli.Command{
	Name:      "termination-estimate",
	Usage:     "Estimate the termination penalty",
	ArgsUsage: "[sectorNum1 sectorNum2 ...]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "actor",
			Usage: "specify the address of miner actor",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() < 1 {
			return lcli.ShowHelp(cctx, fmt.Errorf("at least one sector must be specified"))
		}

		var maddr address.Address
		if act := cctx.String("actor"); act != "" {
			var err error
			maddr, err = address.NewFromString(act)
			if err != nil {
				return fmt.Errorf("parsing address %s: %w", act, err)
			}
		}

		nodeApi, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		if maddr.Empty() {
			minerApi, acloser, err := lcli.GetStorageMinerAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			maddr, err = minerApi.ActorAddress(ctx)
			if err != nil {
				return err
			}
		}

		mi, err := nodeApi.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		ownerAddr, err := nodeApi.StateAccountKey(ctx, mi.Owner, types.EmptyTSK)
		if err != nil {
			ownerAddr = mi.Owner
		}

		var fromAddr address.Address
		if ownerAddr.Protocol() == address.Delegated {
			fromAddr = mi.Worker
		} else {
			fromAddr = mi.Owner
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

		msg := &types.Message{
			From:   fromAddr,
			To:     maddr,
			Method: builtin.MethodsMiner.TerminateSectors,

			Value:  big.Zero(),
			Params: sp,
		}

		//TODO: 4667 add an option to give a more precise estimation with pending termination penalty excluded

		invocResult, err := nodeApi.StateCall(ctx, msg, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("fail to state call: %w", err)
		}

		findPenaltyInInternalExecutions("\t", invocResult.ExecutionTrace.Subcalls)
		return nil
	},
}

func activeMiners(ctx context.Context, api v0api.FullNode) ([]address.Address, error) {
	miners, err := api.StateListMiners(ctx, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	powCache := make(map[address.Address]types.BigInt)
	var lk sync.Mutex
	parmap.Par(32, miners, func(a address.Address) {
		pow, err := api.StateMinerPower(ctx, a, types.EmptyTSK)

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
		return pow.IsZero()
	})
	return append(miners[0:0:0], miners[:n]...), nil
}

var dumpRLESectorCmd = &cli.Command{
	Name:  "dump-rles",
	Usage: "Dump AllocatedSectors RLEs from miners passed as arguments as run lengths in uint64 LE format.\nIf no arguments are passed, dumps all active miners in the state tree.",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		var miners []address.Address
		if cctx.NArg() == 0 {
			miners, err = activeMiners(ctx, api)
			if err != nil {
				return xerrors.Errorf("getting active miners: %w", err)
			}
		} else {
			for _, mS := range cctx.Args().Slice() {
				mA, err := address.NewFromString(mS)
				if err != nil {
					return xerrors.Errorf("parsing address '%s': %w", mS, err)
				}
				miners = append(miners, mA)
			}
		}
		wbuf := make([]byte, 8)
		buf := &bytes.Buffer{}

		for i := 0; i < len(miners); i++ {
			buf.Reset()
			err := func() error {
				state, err := api.StateReadState(ctx, miners[i], types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("getting state: %+v", err)
				}
				allocSString := state.State.(map[string]interface{})["AllocatedSectors"].(map[string]interface{})["/"].(string)

				allocCid, err := cid.Decode(allocSString)
				if err != nil {
					return xerrors.Errorf("decoding cid: %+v", err)
				}
				rle, err := api.ChainReadObj(ctx, allocCid)
				if err != nil {
					return xerrors.Errorf("reading AllocatedSectors: %+v", err)
				}

				var bf bitfield.BitField
				err = bf.UnmarshalCBOR(bytes.NewReader(rle))
				if err != nil {
					return xerrors.Errorf("decoding bitfield: %w", err)
				}
				ri, err := bf.RunIterator()
				if err != nil {
					return xerrors.Errorf("creating iterator: %w", err)
				}

				for ri.HasNext() {
					run, err := ri.NextRun()
					if err != nil {
						return xerrors.Errorf("getting run: %w", err)
					}
					binary.LittleEndian.PutUint64(wbuf, run.Len)
					buf.Write(wbuf)
				}
				_, err = io.Copy(os.Stdout, buf)
				if err != nil {
					return xerrors.Errorf("copy: %w", err)
				}

				return nil
			}()
			if err != nil {
				log.Errorf("miner %d: %s: %+v", i, miners[i], err)
			}
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
			miners, err = activeMiners(ctx, api)
			if err != nil {
				return xerrors.Errorf("getting active miners: %w", err)
			}
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
		return nil, xerrors.Errorf("creating iterator: %w", err)
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
		return nil, xerrors.Errorf("creating iterator: %w", err)
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

var sectorReadCmd = &cli.Command{
	Name:      "read",
	Usage:     "read data from a sector into stdout",
	ArgsUsage: "[sector num] [padded length] [padded offset]",
	Description: `Read data from a sector.

TIP: to get sectornum/len/offset for a piece you can use 'boostd pieces piece-info [cid]'

fr32 padding is removed from the output.`,
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 3 {
			return xerrors.Errorf("must pass sectornum/len/offset")
		}

		sectorNum, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing sector number: %w", err)
		}

		length, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing length: %w", err)
		}

		offset, err := strconv.ParseUint(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing offset: %w", err)
		}

		ctx := lcli.ReqContext(cctx)
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}

		maddr, err := api.ActorAddress(ctx)
		if err != nil {
			return xerrors.Errorf("getting miner actor address: %w", err)
		}

		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return xerrors.Errorf("getting miner id: %w", err)
		}

		sid := abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: abi.SectorNumber(sectorNum),
		}

		si, err := api.SectorsStatus(ctx, sid.Number, false)
		if err != nil {
			return xerrors.Errorf("getting sector status: %w", err)
		}

		sref := storiface.SectorRef{
			ID:        sid,
			ProofType: si.SealProof,
		}

		defer closer()

		// Setup remote sector store
		sminfo, err := lcli.GetAPIInfo(cctx, repo.StorageMiner)
		if err != nil {
			return xerrors.Errorf("could not get api info: %w", err)
		}

		localStore, err := paths.NewLocal(ctx, &emptyLocalStorage{}, api, []string{})
		if err != nil {
			return err
		}

		remote := paths.NewRemote(localStore, api, sminfo.AuthHeader(), 10,
			&paths.DefaultPartialFileHandler{})

		readStarter, err := remote.Reader(ctx, sref, abi.PaddedPieceSize(offset), abi.PaddedPieceSize(length))
		if err != nil {
			return xerrors.Errorf("getting reader: %w", err)
		}

		rd, err := readStarter(0, storiface.PaddedByteIndex(length))
		if err != nil {
			return xerrors.Errorf("starting reader: %w", err)
		}

		upr, err := fr32.NewUnpadReaderBuf(rd, abi.PaddedPieceSize(length), make([]byte, 1<<20))
		if err != nil {
			return xerrors.Errorf("creating unpadded reader: %w", err)
		}

		l := int64(abi.PaddedPieceSize(length).Unpadded())

		bar := pb.Full.Start64(l)
		br := bar.NewProxyReader(upr)

		_, err = io.CopyN(os.Stdout, br, l)

		bar.Finish()

		if err != nil {
			return xerrors.Errorf("reading data: %w", err)
		}
		return nil
	},
}

var sectorDeleteCmd = &cli.Command{
	Name:  "delete",
	Usage: "delete a sector file from sector storage",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "really-do-it",
		},
	},
	ArgsUsage: "[sector num] [file type]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return xerrors.Errorf("must pass sectornum/filetype")
		}

		sectorNum, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing sector number: %w", err)
		}

		var ft storiface.SectorFileType
		switch cctx.Args().Get(1) {
		case "cache":
			ft = storiface.FTCache
		case "sealed":
			ft = storiface.FTSealed
		case "unsealed":
			ft = storiface.FTUnsealed
		case "update-cache":
			ft = storiface.FTUpdateCache
		case "update":
			ft = storiface.FTUpdate
		default:
			return xerrors.Errorf("invalid file type")
		}

		ctx := lcli.ReqContext(cctx)
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		maddr, err := api.ActorAddress(ctx)
		if err != nil {
			return xerrors.Errorf("getting miner actor address: %w", err)
		}

		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return xerrors.Errorf("getting miner id: %w", err)
		}

		sid := abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: abi.SectorNumber(sectorNum),
		}

		// get remote store
		sminfo, err := lcli.GetAPIInfo(cctx, repo.StorageMiner)
		if err != nil {
			return xerrors.Errorf("could not get api info: %w", err)
		}

		localStore, err := paths.NewLocal(ctx, &emptyLocalStorage{}, api, []string{})
		if err != nil {
			return err
		}

		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("pass --really-do-it to actually perform the deletion")
		}

		remote := paths.NewRemote(localStore, api, sminfo.AuthHeader(), 10,
			&paths.DefaultPartialFileHandler{})

		err = remote.Remove(ctx, sid, ft, true, nil)
		if err != nil {
			return xerrors.Errorf("removing sector: %w", err)
		}

		return nil
	},
}

var dumpSectorOnChainInfoCmd = &cli.Command{
	Name:  "dump-sectors",
	Usage: "Dump SectorOnChainInfo in CSV format for all sectors for all miners that have claimed power",
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		h, err := loadChainStore(ctx, cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("loading chainstore: %w", err)
		}
		defer h.closer()

		ts, err := lcli.LoadTipSet(ctx, cctx, &ChainStoreTipSetResolver{Chain: h.cs})
		if err != nil {
			return xerrors.Errorf("loading tipset: %w", err)
		}

		powerActor, err := h.sm.LoadActor(ctx, power.Address, ts)
		if err != nil {
			return xerrors.Errorf("failed to load power actor: %w", err)
		}

		powerState, err := power.Load(h.cs.ActorStore(ctx), powerActor)
		if err != nil {
			return xerrors.Errorf("failed to load power actor state: %w", err)
		}

		_, _ = fmt.Fprintf(cctx.App.Writer,
			"Miner,SectorNumber,SealProof,Activation,Expiration,DealWeight,VerifiedDealWeight,"+
				"InitialPledge,ExpectedDayReward,ExpectedStoragePledge,PowerBaseEpoch,Flags\n")

		var count int
		err = powerState.ForEachClaim(func(maddr address.Address, claim power.Claim) error {
			act, err := h.sm.LoadActorTsk(ctx, maddr, ts.Key())
			if err != nil {
				return xerrors.Errorf("failed to load miner actor: %w", err)
			}

			mas, err := miner.Load(h.sm.ChainStore().ActorStore(ctx), act)
			if err != nil {
				return xerrors.Errorf("failed to load miner actor state: %w", err)
			}

			soci, err := mas.LoadSectors(nil)
			if err != nil {
				return xerrors.Errorf("load sectors: %w", err)
			}

			for _, sector := range soci {
				_, _ = fmt.Fprintf(
					cctx.App.Writer,
					"%s,%d,%d,%d,%d,%s,%s,%s,%s,%s,%d,%x\n",
					maddr,
					sector.SectorNumber,
					sector.SealProof,
					sector.Activation,
					sector.Expiration,
					sector.DealWeight,
					sector.VerifiedDealWeight,
					sector.InitialPledge,
					sector.ExpectedDayReward,
					sector.ExpectedStoragePledge,
					sector.PowerBaseEpoch,
					sector.Flags,
				)
			}

			count++
			if count%1000 == 0 {
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Processed %d miners.\n", count)
			}

			return nil
		}, false)

		if err != nil {
			return xerrors.Errorf("iterating over claims: %w", err)
		}

		_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Processed %d miners. Complete.\n", count)

		return nil
	},
}

type emptyLocalStorage struct {
}

func (e *emptyLocalStorage) GetStorage() (storiface.StorageConfig, error) {
	return storiface.StorageConfig{}, nil
}

func (e *emptyLocalStorage) SetStorage(f func(*storiface.StorageConfig)) error {
	panic("don't call")
}

func (e *emptyLocalStorage) Stat(path string) (fsutil.FsStat, error) {
	panic("don't call")
}

func (e *emptyLocalStorage) DiskUsage(path string) (int64, error) {
	panic("don't call")
}

var _ paths.LocalStorage = &emptyLocalStorage{}
