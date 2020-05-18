package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	cid "github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	types "github.com/filecoin-project/lotus/chain/types"
)

var chainCmd = &cli.Command{
	Name:  "chain",
	Usage: "Interact with filecoin blockchain",
	Subcommands: []*cli.Command{
		chainHeadCmd,
		chainGetBlock,
		chainReadObjCmd,
		chainStatObjCmd,
		chainGetMsgCmd,
		chainSetHeadCmd,
		chainListCmd,
		chainGetCmd,
		chainBisectCmd,
		chainExportCmd,
		slashConsensusFault,
	},
}

var chainHeadCmd = &cli.Command{
	Name:  "head",
	Usage: "Print chain head",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		for _, c := range head.Cids() {
			fmt.Println(c)
		}
		return nil
	},
}

var chainGetBlock = &cli.Command{
	Name:      "getblock",
	Usage:     "Get a block and print its details",
	ArgsUsage: "[blockCid]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "raw",
			Usage: "print just the raw block header",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass cid of block to print")
		}

		bcid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}

		blk, err := api.ChainGetBlock(ctx, bcid)
		if err != nil {
			return xerrors.Errorf("get block failed: %w", err)
		}

		if cctx.Bool("raw") {
			out, err := json.MarshalIndent(blk, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(out))
			return nil
		}

		msgs, err := api.ChainGetBlockMessages(ctx, bcid)
		if err != nil {
			return xerrors.Errorf("failed to get messages: %w", err)
		}

		pmsgs, err := api.ChainGetParentMessages(ctx, bcid)
		if err != nil {
			return xerrors.Errorf("failed to get parent messages: %w", err)
		}

		recpts, err := api.ChainGetParentReceipts(ctx, bcid)
		if err != nil {
			log.Warn(err)
			//return xerrors.Errorf("failed to get receipts: %w", err)
		}

		cblock := struct {
			types.BlockHeader
			BlsMessages    []*types.Message
			SecpkMessages  []*types.SignedMessage
			ParentReceipts []*types.MessageReceipt
			ParentMessages []cid.Cid
		}{}

		cblock.BlockHeader = *blk
		cblock.BlsMessages = msgs.BlsMessages
		cblock.SecpkMessages = msgs.SecpkMessages
		cblock.ParentReceipts = recpts
		cblock.ParentMessages = apiMsgCids(pmsgs)

		out, err := json.MarshalIndent(cblock, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(out))
		return nil

	},
}

func apiMsgCids(in []api.Message) []cid.Cid {
	out := make([]cid.Cid, len(in))
	for k, v := range in {
		out[k] = v.Cid
	}
	return out
}

var chainReadObjCmd = &cli.Command{
	Name:      "read-obj",
	Usage:     "Read the raw bytes of an object",
	ArgsUsage: "[objectCid]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		c, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to parse cid input: %s", err)
		}

		obj, err := api.ChainReadObj(ctx, c)
		if err != nil {
			return err
		}

		fmt.Printf("%x\n", obj)
		return nil
	},
}

var chainStatObjCmd = &cli.Command{
	Name:      "stat-obj",
	Usage:     "Collect size and ipld link counts for objs",
	ArgsUsage: "[cid]",
	Description: `Collect object size and ipld link count for an object.

   When a base is provided it will be walked first, and all links visisted
   will be ignored when the passed in object is walked.
`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "base",
			Usage: "ignore links found in this obj",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		obj, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to parse cid input: %s", err)
		}

		base := cid.Undef
		if cctx.IsSet("base") {
			base, err = cid.Decode(cctx.String("base"))
		}

		stats, err := api.ChainStatObj(ctx, obj, base)
		if err != nil {
			return err
		}

		fmt.Printf("Links: %d\n", stats.Links)
		fmt.Printf("Size: %s (%d)\n", units.BytesSize(float64(stats.Size)), stats.Size)
		return nil
	},
}

var chainGetMsgCmd = &cli.Command{
	Name:      "getmessage",
	Usage:     "Get and print a message by its cid",
	ArgsUsage: "[messageCid]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass a cid of a message to get")
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		c, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("failed to parse cid input: %w", err)
		}

		mb, err := api.ChainReadObj(ctx, c)
		if err != nil {
			return xerrors.Errorf("failed to read object: %w", err)
		}

		var i interface{}
		m, err := types.DecodeMessage(mb)
		if err != nil {
			sm, err := types.DecodeSignedMessage(mb)
			if err != nil {
				return xerrors.Errorf("failed to decode object as a message: %w", err)
			}
			i = sm
		} else {
			i = m
		}

		enc, err := json.MarshalIndent(i, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(enc))
		return nil
	},
}

var chainSetHeadCmd = &cli.Command{
	Name:      "sethead",
	Usage:     "manually set the local nodes head tipset (Caution: normally only used for recovery)",
	ArgsUsage: "[tipsetkey]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "genesis",
			Usage: "reset head to genesis",
		},
		&cli.Uint64Flag{
			Name:  "epoch",
			Usage: "reset head to given epoch",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var ts *types.TipSet

		if cctx.Bool("genesis") {
			ts, err = api.ChainGetGenesis(ctx)
		}
		if ts == nil && cctx.IsSet("epoch") {
			ts, err = api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(cctx.Uint64("epoch")), types.EmptyTSK)
		}
		if ts == nil {
			ts, err = parseTipSet(api, ctx, cctx.Args().Slice())
		}
		if err != nil {
			return err
		}

		if ts == nil {
			return fmt.Errorf("must pass cids for tipset to set as head")
		}

		if err := api.ChainSetHead(ctx, ts.Key()); err != nil {
			return err
		}

		return nil
	},
}

func parseTipSet(api api.FullNode, ctx context.Context, vals []string) (*types.TipSet, error) {
	var headers []*types.BlockHeader
	for _, c := range vals {
		blkc, err := cid.Decode(c)
		if err != nil {
			return nil, err
		}

		bh, err := api.ChainGetBlock(ctx, blkc)
		if err != nil {
			return nil, err
		}

		headers = append(headers, bh)
	}

	return types.NewTipSet(headers)
}

var chainListCmd = &cli.Command{
	Name:  "list",
	Usage: "View a segment of the chain",
	Flags: []cli.Flag{
		&cli.Uint64Flag{Name: "height"},
		&cli.IntFlag{Name: "count", Value: 30},
		&cli.StringFlag{
			Name:  "format",
			Usage: "specify the format to print out tipsets",
			Value: "<height>: (<time>) <blocks>",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var head *types.TipSet

		if cctx.IsSet("height") {
			head, err = api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(cctx.Uint64("height")), types.EmptyTSK)
		} else {
			head, err = api.ChainHead(ctx)
		}
		if err != nil {
			return err
		}

		count := cctx.Int("count")
		if count < 1 {
			return nil
		}

		tss := make([]*types.TipSet, 0, count)
		tss = append(tss, head)

		for i := 1; i < count; i++ {
			if head.Height() == 0 {
				break
			}

			head, err = api.ChainGetTipSet(ctx, head.Parents())
			if err != nil {
				return err
			}

			tss = append(tss, head)
		}

		for i := len(tss) - 1; i >= 0; i-- {
			printTipSet(cctx.String("format"), tss[i])
		}
		return nil
	},
}

var chainGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "Get chain DAG node by path",
	ArgsUsage: "[path]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "as-type",
			Usage: "specify type to interpret output as",
		},
		&cli.BoolFlag{
			Name:  "verbose",
			Value: false,
		},
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset for /pstate (pass comma separated array of cids)",
		},
	},
	Description: `Get ipld node under a specified path:

   lotus chain get /ipfs/[cid]/some/path

   Path prefixes:
   - /ipfs/[cid], /ipld/[cid] - traverse IPLD path
   - /pstate - traverse from head.ParentStateRoot

   Note:
   You can use special path elements to traverse through some data structures:
   - /ipfs/[cid]/@H:elem - get 'elem' from hamt
   - /ipfs/[cid]/@Hi:123 - get varint elem 123 from hamt
   - /ipfs/[cid]/@Hu:123 - get uvarint elem 123 from hamt
   - /ipfs/[cid]/@Ha:t01 - get element under Addr(t01).Bytes
   - /ipfs/[cid]/@A:10 - get 10th amt element

   List of --as-type types:
   - raw
   - block
   - message
   - smessage, signedmessage
   - actor
   - amt
   - hamt-epoch
   - hamt-address
   - cronevent
   - account-state
`,
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		p := path.Clean(cctx.Args().First())
		if strings.HasPrefix(p, "/pstate") {
			p = p[len("/pstate"):]

			ts, err := LoadTipSet(ctx, cctx, api)
			if err != nil {
				return err
			}

			if ts == nil {
				ts, err = api.ChainHead(ctx)
				if err != nil {
					return err
				}
			}
			p = "/ipfs/" + ts.ParentState().String() + p
			if cctx.Bool("verbose") {
				fmt.Println(p)
			}
		}

		obj, err := api.ChainGetNode(ctx, p)
		if err != nil {
			return err
		}

		t := strings.ToLower(cctx.String("as-type"))
		if t == "" {
			b, err := json.MarshalIndent(obj.Obj, "", "\t")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}

		var cbu cbg.CBORUnmarshaler
		switch t {
		case "raw":
			cbu = nil
		case "block":
			cbu = new(types.BlockHeader)
		case "message":
			cbu = new(types.Message)
		case "smessage", "signedmessage":
			cbu = new(types.SignedMessage)
		case "actor":
			cbu = new(types.Actor)
		case "amt":
			return handleAmt(ctx, api, obj.Cid)
		case "hamt-epoch":
			return handleHamtEpoch(ctx, api, obj.Cid)
		case "hamt-address":
			return handleHamtAddress(ctx, api, obj.Cid)
		case "cronevent":
			cbu = new(power.CronEvent)
		case "account-state":
			cbu = new(account.State)
		case "miner-state":
			cbu = new(miner.State)
		case "power-state":
			cbu = new(power.State)
		case "market-state":
			cbu = new(market.State)
		default:
			return fmt.Errorf("unknown type: %q", t)
		}

		raw, err := api.ChainReadObj(ctx, obj.Cid)
		if err != nil {
			return err
		}

		if cbu == nil {
			fmt.Printf("%x", raw)
			return nil
		}

		if err := cbu.UnmarshalCBOR(bytes.NewReader(raw)); err != nil {
			return fmt.Errorf("failed to unmarshal as %q", t)
		}

		b, err := json.MarshalIndent(cbu, "", "\t")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	},
}

type apiIpldStore struct {
	ctx context.Context
	api api.FullNode
}

func (ht *apiIpldStore) Context() context.Context {
	return ht.ctx
}

func (ht *apiIpldStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	raw, err := ht.api.ChainReadObj(ctx, c)
	if err != nil {
		return err
	}

	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(raw)); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("Object does not implement CBORUnmarshaler")
}

func (ht *apiIpldStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	panic("No mutations allowed")
}

func handleAmt(ctx context.Context, api api.FullNode, r cid.Cid) error {
	s := &apiIpldStore{ctx, api}
	mp, err := adt.AsArray(s, r)
	if err != nil {
		return err
	}

	return mp.ForEach(nil, func(key int64) error {
		fmt.Printf("%d\n", key)
		return nil
	})
}

func handleHamtEpoch(ctx context.Context, api api.FullNode, r cid.Cid) error {
	s := &apiIpldStore{ctx, api}
	mp, err := adt.AsMap(s, r)
	if err != nil {
		return err
	}

	return mp.ForEach(nil, func(key string) error {
		ik, err := adt.ParseIntKey(key)
		if err != nil {
			return err
		}

		fmt.Printf("%d\n", ik)
		return nil
	})
}

func handleHamtAddress(ctx context.Context, api api.FullNode, r cid.Cid) error {
	s := &apiIpldStore{ctx, api}
	mp, err := adt.AsMap(s, r)
	if err != nil {
		return err
	}

	return mp.ForEach(nil, func(key string) error {
		addr, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", addr)
		return nil
	})
}

func printTipSet(format string, ts *types.TipSet) {
	format = strings.ReplaceAll(format, "<height>", fmt.Sprint(ts.Height()))
	format = strings.ReplaceAll(format, "<time>", time.Unix(int64(ts.MinTimestamp()), 0).Format(time.Stamp))
	blks := "[ "
	for _, b := range ts.Blocks() {
		blks += fmt.Sprintf("%s: %s,", b.Cid(), b.Miner)
	}
	blks += " ]"

	sCids := make([]string, 0, len(blks))

	for _, c := range ts.Cids() {
		sCids = append(sCids, c.String())
	}

	format = strings.ReplaceAll(format, "<tipset>", strings.Join(sCids, ","))
	format = strings.ReplaceAll(format, "<blocks>", blks)
	format = strings.ReplaceAll(format, "<weight>", fmt.Sprint(ts.Blocks()[0].ParentWeight))

	fmt.Println(format)
}

var chainBisectCmd = &cli.Command{
	Name:      "bisect",
	Usage:     "bisect chain for an event",
	ArgsUsage: "[minHeight maxHeight path shellCommand <shellCommandArgs (if any)>]",
	Description: `Bisect the chain state tree:

   lotus chain bisect [min height] [max height] '1/2/3/state/path' 'shell command' 'args'

   Returns the first tipset in which condition is true
                  v
   [start] FFFFFFFTTT [end]

   Example: find height at which deal ID 100 000 appeared
    - lotus chain bisect 1 32000 '@Ha:t03/1' jq -e '.[2] > 100000'

   For special path elements see 'chain get' help
`,
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.Args().Len() < 4 {
			return xerrors.New("need at least 4 args")
		}

		start, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return err
		}

		end, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		subPath := cctx.Args().Get(2)

		highest, err := api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(end), types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting end tipset: %w", err)
		}

		prev := highest.Height()

		for {
			mid := (start + end) / 2
			if end-start == 1 {
				mid = end
				start = end
			}

			midTs, err := api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(mid), highest.Key())
			if err != nil {
				return err
			}

			path := "/ipld/" + midTs.ParentState().String() + "/" + subPath
			fmt.Printf("* Testing %d (%d - %d) (%s): ", mid, start, end, path)

			nd, err := api.ChainGetNode(ctx, path)
			if err != nil {
				return err
			}

			b, err := json.MarshalIndent(nd.Obj, "", "\t")
			if err != nil {
				return err
			}

			cmd := exec.CommandContext(ctx, cctx.Args().Get(3), cctx.Args().Slice()[4:]...)
			cmd.Stdin = bytes.NewReader(b)

			var out bytes.Buffer
			var serr bytes.Buffer

			cmd.Stdout = &out
			cmd.Stderr = &serr

			switch cmd.Run().(type) {
			case nil:
				// it's lower
				if strings.TrimSpace(out.String()) != "false" {
					end = mid
					highest = midTs
					fmt.Println("true")
				} else {
					start = mid
					fmt.Printf("false (cli)\n")
				}
			case *exec.ExitError:
				if len(serr.String()) > 0 {
					fmt.Println("error")

					fmt.Printf("> Command: %s\n---->\n", strings.Join(cctx.Args().Slice()[3:], " "))
					fmt.Println(string(b))
					fmt.Println("<----")
					return xerrors.Errorf("error running bisect check: %s", serr.String())
				}

				start = mid
				fmt.Println("false")
			default:
				return err
			}

			if start == end {
				if strings.TrimSpace(out.String()) == "true" {
					fmt.Println(midTs.Height())
				} else {
					fmt.Println(prev)
				}
				return nil
			}

			prev = abi.ChainEpoch(mid)
		}
	},
}

var chainExportCmd = &cli.Command{
	Name:      "export",
	Usage:     "export chain to a car file",
	ArgsUsage: "[outputPath]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "tipset",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify filename to export chain to")
		}

		fi, err := os.Create(cctx.Args().First())
		if err != nil {
			return err
		}
		defer fi.Close()

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		stream, err := api.ChainExport(ctx, ts.Key())
		if err != nil {
			return err
		}

		for b := range stream {
			_, err := fi.Write(b)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

var slashConsensusFault = &cli.Command{
	Name:      "slash-consensus",
	Usage:     "Report consensus fault",
	ArgsUsage: "[blockCid1 blockCid2]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "miner",
			Usage: "Miner address",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		c1, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("parsing cid 1: %w", err)
		}

		b1, err := api.ChainGetBlock(ctx, c1)
		if err != nil {
			return xerrors.Errorf("getting block 1: %w", err)
		}

		c2, err := cid.Parse(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing cid 2: %w", err)
		}

		b2, err := api.ChainGetBlock(ctx, c2)
		if err != nil {
			return xerrors.Errorf("getting block 2: %w", err)
		}

		def, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return err
		}

		bh1, err := cborutil.Dump(b1)
		if err != nil {
			return err
		}

		bh2, err := cborutil.Dump(b2)
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&miner.ReportConsensusFaultParams{
			BlockHeader1: bh1,
			BlockHeader2: bh2,
		})

		if cctx.String("miner") == "" {
			return xerrors.Errorf("--miner flag is required")
		}

		maddr, err := address.NewFromString(cctx.String("miner"))
		if err != nil {
			return err
		}

		msg := &types.Message{
			To:       maddr,
			From:     def,
			Value:    types.NewInt(0),
			GasPrice: types.NewInt(1),
			GasLimit: 10000000,
			Method:   builtin.MethodsMiner.ReportConsensusFault,
			Params:   params,
		}

		smsg, err := api.MpoolPushMessage(ctx, msg)
		if err != nil {
			return err
		}

		fmt.Println(smsg.Cid())

		return nil
	},
}
