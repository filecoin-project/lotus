package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	cid "github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	types "github.com/filecoin-project/lotus/chain/types"
)

var ChainCmd = &cli.Command{
	Name:  "chain",
	Usage: "Interact with filecoin blockchain",
	Subcommands: []*cli.Command{
		ChainHeadCmd,
		ChainGetBlock,
		ChainReadObjCmd,
		ChainDeleteObjCmd,
		ChainStatObjCmd,
		ChainGetMsgCmd,
		ChainSetHeadCmd,
		ChainListCmd,
		ChainGetCmd,
		ChainBisectCmd,
		ChainExportCmd,
		SlashConsensusFault,
		ChainGasPriceCmd,
		ChainInspectUsage,
		ChainDecodeCmd,
		ChainEncodeCmd,
		ChainDisputeSetCmd,
	},
}

var ChainHeadCmd = &cli.Command{
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

var ChainGetBlock = &cli.Command{
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

func apiMsgCids(in []lapi.Message) []cid.Cid {
	out := make([]cid.Cid, len(in))
	for k, v := range in {
		out[k] = v.Cid
	}
	return out
}

var ChainReadObjCmd = &cli.Command{
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

var ChainDeleteObjCmd = &cli.Command{
	Name:        "delete-obj",
	Usage:       "Delete an object from the chain blockstore",
	Description: "WARNING: Removing wrong objects from the chain blockstore may lead to sync issues",
	ArgsUsage:   "[objectCid]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "really-do-it",
		},
	},
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

		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("pass the --really-do-it flag to proceed")
		}

		err = api.ChainDeleteObj(ctx, c)
		if err != nil {
			return err
		}

		fmt.Printf("Obj %s deleted\n", c.String())
		return nil
	},
}

var ChainStatObjCmd = &cli.Command{
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
			if err != nil {
				return err
			}
		}

		stats, err := api.ChainStatObj(ctx, obj, base)
		if err != nil {
			return err
		}

		fmt.Printf("Links: %d\n", stats.Links)
		fmt.Printf("Size: %s (%d)\n", types.SizeStr(types.NewInt(stats.Size)), stats.Size)
		return nil
	},
}

var ChainGetMsgCmd = &cli.Command{
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

var ChainSetHeadCmd = &cli.Command{
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
			ts, err = parseTipSet(ctx, api, cctx.Args().Slice())
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

var ChainInspectUsage = &cli.Command{
	Name:  "inspect-usage",
	Usage: "Inspect block space usage of a given tipset",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to view block space usage of",
			Value: "@head",
		},
		&cli.IntFlag{
			Name:  "length",
			Usage: "length of chain to inspect block space usage for",
			Value: 1,
		},
		&cli.IntFlag{
			Name:  "num-results",
			Usage: "number of results to print per category",
			Value: 10,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		cur := ts
		var msgs []lapi.Message
		for i := 0; i < cctx.Int("length"); i++ {
			pmsgs, err := api.ChainGetParentMessages(ctx, cur.Blocks()[0].Cid())
			if err != nil {
				return err
			}

			msgs = append(msgs, pmsgs...)

			next, err := api.ChainGetTipSet(ctx, cur.Parents())
			if err != nil {
				return err
			}

			cur = next
		}

		codeCache := make(map[address.Address]cid.Cid)

		lookupActorCode := func(a address.Address) (cid.Cid, error) {
			c, ok := codeCache[a]
			if ok {
				return c, nil
			}

			act, err := api.StateGetActor(ctx, a, ts.Key())
			if err != nil {
				return cid.Undef, err
			}

			codeCache[a] = act.Code
			return act.Code, nil
		}

		bySender := make(map[string]int64)
		byDest := make(map[string]int64)
		byMethod := make(map[string]int64)
		bySenderC := make(map[string]int64)
		byDestC := make(map[string]int64)
		byMethodC := make(map[string]int64)

		var sum int64
		for _, m := range msgs {
			bySender[m.Message.From.String()] += m.Message.GasLimit
			bySenderC[m.Message.From.String()]++
			byDest[m.Message.To.String()] += m.Message.GasLimit
			byDestC[m.Message.To.String()]++
			sum += m.Message.GasLimit

			code, err := lookupActorCode(m.Message.To)
			if err != nil {
				if strings.Contains(err.Error(), types.ErrActorNotFound.Error()) {
					continue
				}
				return err
			}

			mm := filcns.NewActorRegistry().Methods[code][m.Message.Method] // TODO: use remote map

			byMethod[mm.Name] += m.Message.GasLimit
			byMethodC[mm.Name]++
		}

		type keyGasPair struct {
			Key string
			Gas int64
		}

		mapToSortedKvs := func(m map[string]int64) []keyGasPair {
			var vals []keyGasPair
			for k, v := range m {
				vals = append(vals, keyGasPair{
					Key: k,
					Gas: v,
				})
			}
			sort.Slice(vals, func(i, j int) bool {
				return vals[i].Gas > vals[j].Gas
			})
			return vals
		}

		senderVals := mapToSortedKvs(bySender)
		destVals := mapToSortedKvs(byDest)
		methodVals := mapToSortedKvs(byMethod)

		numRes := cctx.Int("num-results")

		fmt.Printf("Total Gas Limit: %d\n", sum)
		fmt.Printf("By Sender:\n")
		for i := 0; i < numRes && i < len(senderVals); i++ {
			sv := senderVals[i]
			fmt.Printf("%s\t%0.2f%%\t(total: %d, count: %d)\n", sv.Key, (100*float64(sv.Gas))/float64(sum), sv.Gas, bySenderC[sv.Key])
		}
		fmt.Println()
		fmt.Printf("By Receiver:\n")
		for i := 0; i < numRes && i < len(destVals); i++ {
			sv := destVals[i]
			fmt.Printf("%s\t%0.2f%%\t(total: %d, count: %d)\n", sv.Key, (100*float64(sv.Gas))/float64(sum), sv.Gas, byDestC[sv.Key])
		}
		fmt.Println()
		fmt.Printf("By Method:\n")
		for i := 0; i < numRes && i < len(methodVals); i++ {
			sv := methodVals[i]
			fmt.Printf("%s\t%0.2f%%\t(total: %d, count: %d)\n", sv.Key, (100*float64(sv.Gas))/float64(sum), sv.Gas, byMethodC[sv.Key])
		}

		return nil
	},
}

var ChainListCmd = &cli.Command{
	Name:    "list",
	Aliases: []string{"love"},
	Usage:   "View a segment of the chain",
	Flags: []cli.Flag{
		&cli.Uint64Flag{Name: "height", DefaultText: "current head"},
		&cli.IntFlag{Name: "count", Value: 30},
		&cli.StringFlag{
			Name:  "format",
			Usage: "specify the format to print out tipsets",
			Value: "<height>: (<time>) <blocks>",
		},
		&cli.BoolFlag{
			Name:  "gas-stats",
			Usage: "view gas statistics for the chain",
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

		if cctx.Bool("gas-stats") {
			otss := make([]*types.TipSet, 0, len(tss))
			for i := len(tss) - 1; i >= 0; i-- {
				otss = append(otss, tss[i])
			}
			tss = otss
			for i, ts := range tss {
				pbf := ts.Blocks()[0].ParentBaseFee
				fmt.Printf("%d: %d blocks (baseFee: %s -> maxFee: %s)\n", ts.Height(), len(ts.Blocks()), ts.Blocks()[0].ParentBaseFee, types.FIL(types.BigMul(pbf, types.NewInt(uint64(build.BlockGasLimit)))))

				for _, b := range ts.Blocks() {
					msgs, err := api.ChainGetBlockMessages(ctx, b.Cid())
					if err != nil {
						return err
					}
					var limitSum int64
					psum := big.NewInt(0)
					for _, m := range msgs.BlsMessages {
						limitSum += m.GasLimit
						psum = big.Add(psum, m.GasPremium)
					}

					for _, m := range msgs.SecpkMessages {
						limitSum += m.Message.GasLimit
						psum = big.Add(psum, m.Message.GasPremium)
					}

					lenmsgs := len(msgs.BlsMessages) + len(msgs.SecpkMessages)

					avgpremium := big.Zero()
					if lenmsgs > 0 {
						avgpremium = big.Div(psum, big.NewInt(int64(lenmsgs)))
					}

					fmt.Printf("\t%s: \t%d msgs, gasLimit: %d / %d (%0.2f%%), avgPremium: %s\n", b.Miner, len(msgs.BlsMessages)+len(msgs.SecpkMessages), limitSum, build.BlockGasLimit, 100*float64(limitSum)/float64(build.BlockGasLimit), avgpremium)
				}
				if i < len(tss)-1 {
					msgs, err := api.ChainGetParentMessages(ctx, tss[i+1].Blocks()[0].Cid())
					if err != nil {
						return err
					}
					var limitSum int64
					for _, m := range msgs {
						limitSum += m.Message.GasLimit
					}

					recpts, err := api.ChainGetParentReceipts(ctx, tss[i+1].Blocks()[0].Cid())
					if err != nil {
						return err
					}

					var gasUsed int64
					for _, r := range recpts {
						gasUsed += r.GasUsed
					}

					gasEfficiency := 100 * float64(gasUsed) / float64(limitSum)
					gasCapacity := 100 * float64(limitSum) / float64(build.BlockGasLimit)

					fmt.Printf("\ttipset: \t%d msgs, %d (%0.2f%%) / %d (%0.2f%%)\n", len(msgs), gasUsed, gasEfficiency, limitSum, gasCapacity)
				}
				fmt.Println()
			}
		} else {
			for i := len(tss) - 1; i >= 0; i-- {
				printTipSet(cctx.String("format"), tss[i])
			}
		}
		return nil
	},
}

var ChainGetCmd = &cli.Command{
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
   - /ipfs/[cid]/@A:10   - get 10th amt element
   - .../@Ha:t01/@state  - get pretty map-based actor state

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
	api v0api.FullNode
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

func handleAmt(ctx context.Context, api v0api.FullNode, r cid.Cid) error {
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

func handleHamtEpoch(ctx context.Context, api v0api.FullNode, r cid.Cid) error {
	s := &apiIpldStore{ctx, api}
	mp, err := adt.AsMap(s, r)
	if err != nil {
		return err
	}

	return mp.ForEach(nil, func(key string) error {
		ik, err := abi.ParseIntKey(key)
		if err != nil {
			return err
		}

		fmt.Printf("%d\n", ik)
		return nil
	})
}

func handleHamtAddress(ctx context.Context, api v0api.FullNode, r cid.Cid) error {
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

var ChainBisectCmd = &cli.Command{
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

var ChainExportCmd = &cli.Command{
	Name:      "export",
	Usage:     "export chain to a car file",
	ArgsUsage: "[outputPath]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to start the export from",
			Value: "@head",
		},
		&cli.Int64Flag{
			Name:  "recent-stateroots",
			Usage: "specify the number of recent state roots to include in the export",
		},
		&cli.BoolFlag{
			Name: "skip-old-msgs",
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

		rsrs := abi.ChainEpoch(cctx.Int64("recent-stateroots"))
		if cctx.IsSet("recent-stateroots") && rsrs < build.Finality {
			return fmt.Errorf("\"recent-stateroots\" has to be greater than %d", build.Finality)
		}

		fi, err := os.Create(cctx.Args().First())
		if err != nil {
			return err
		}
		defer func() {
			err := fi.Close()
			if err != nil {
				fmt.Printf("error closing output file: %+v", err)
			}
		}()

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		skipold := cctx.Bool("skip-old-msgs")

		if rsrs == 0 && skipold {
			return fmt.Errorf("must pass recent stateroots along with skip-old-msgs")
		}

		stream, err := api.ChainExport(ctx, rsrs, skipold, ts.Key())
		if err != nil {
			return err
		}

		var last bool
		for b := range stream {
			last = len(b) == 0

			_, err := fi.Write(b)
			if err != nil {
				return err
			}
		}

		if !last {
			return xerrors.Errorf("incomplete export (remote connection lost?)")
		}

		return nil
	},
}

var SlashConsensusFault = &cli.Command{
	Name:      "slash-consensus",
	Usage:     "Report consensus fault",
	ArgsUsage: "[blockCid1 blockCid2]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to report consensus from",
		},
		&cli.StringFlag{
			Name:  "extra",
			Usage: "Extra block cid",
		},
	},
	Action: func(cctx *cli.Context) error {
		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		a := srv.FullNodeAPI()
		ctx := ReqContext(cctx)

		c1, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("parsing cid 1: %w", err)
		}

		b1, err := a.ChainGetBlock(ctx, c1)
		if err != nil {
			return xerrors.Errorf("getting block 1: %w", err)
		}

		c2, err := cid.Parse(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing cid 2: %w", err)
		}

		b2, err := a.ChainGetBlock(ctx, c2)
		if err != nil {
			return xerrors.Errorf("getting block 2: %w", err)
		}

		if b1.Miner != b2.Miner {
			return xerrors.Errorf("block1.miner:%s block2.miner:%s", b1.Miner, b2.Miner)
		}

		var fromAddr address.Address
		if from := cctx.String("from"); from == "" {
			defaddr, err := a.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}

			fromAddr = defaddr
		} else {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			fromAddr = addr
		}

		bh1, err := cborutil.Dump(b1)
		if err != nil {
			return err
		}

		bh2, err := cborutil.Dump(b2)
		if err != nil {
			return err
		}

		params := miner.ReportConsensusFaultParams{
			BlockHeader1: bh1,
			BlockHeader2: bh2,
		}

		if cctx.String("extra") != "" {
			cExtra, err := cid.Parse(cctx.String("extra"))
			if err != nil {
				return xerrors.Errorf("parsing cid extra: %w", err)
			}

			bExtra, err := a.ChainGetBlock(ctx, cExtra)
			if err != nil {
				return xerrors.Errorf("getting block extra: %w", err)
			}

			be, err := cborutil.Dump(bExtra)
			if err != nil {
				return err
			}

			params.BlockHeaderExtra = be
		}

		enc, err := actors.SerializeParams(&params)
		if err != nil {
			return err
		}

		proto := &api.MessagePrototype{
			Message: types.Message{
				To:     b2.Miner,
				From:   fromAddr,
				Value:  types.NewInt(0),
				Method: builtin.MethodsMiner.ReportConsensusFault,
				Params: enc,
			},
		}

		smsg, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		fmt.Println(smsg.Cid())

		return nil
	},
}

var ChainGasPriceCmd = &cli.Command{
	Name:  "gas-price",
	Usage: "Estimate gas prices",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		nb := []int{1, 2, 3, 5, 10, 20, 50, 100, 300}
		for _, nblocks := range nb {
			addr := builtin.SystemActorAddr // TODO: make real when used in GasEstimateGasPremium

			est, err := api.GasEstimateGasPremium(ctx, uint64(nblocks), addr, 10000, types.EmptyTSK)
			if err != nil {
				return err
			}

			fmt.Printf("%d blocks: %s (%s)\n", nblocks, est, types.FIL(est))
		}

		return nil
	},
}

var ChainDecodeCmd = &cli.Command{
	Name:  "decode",
	Usage: "decode various types",
	Subcommands: []*cli.Command{
		chainDecodeParamsCmd,
	},
}

var chainDecodeParamsCmd = &cli.Command{
	Name:      "params",
	Usage:     "Decode message params",
	ArgsUsage: "[toAddr method params]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "tipset",
		},
		&cli.StringFlag{
			Name:  "encoding",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if cctx.Args().Len() != 3 {
			return ShowHelp(cctx, fmt.Errorf("incorrect number of arguments"))
		}

		to, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("parsing toAddr: %w", err)
		}

		method, err := strconv.ParseInt(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing method id: %w", err)
		}

		var params []byte
		switch cctx.String("encoding") {
		case "base64":
			params, err = base64.StdEncoding.DecodeString(cctx.Args().Get(2))
			if err != nil {
				return xerrors.Errorf("decoding base64 value: %w", err)
			}
		case "hex":
			params, err = hex.DecodeString(cctx.Args().Get(2))
			if err != nil {
				return xerrors.Errorf("decoding hex value: %w", err)
			}
		default:
			return xerrors.Errorf("unrecognized encoding: %s", cctx.String("encoding"))
		}
		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, to, ts.Key())
		if err != nil {
			return xerrors.Errorf("getting actor: %w", err)
		}

		pstr, err := JsonParams(act.Code, abi.MethodNum(method), params)
		if err != nil {
			return err
		}

		fmt.Println(pstr)

		return nil
	},
}

var ChainEncodeCmd = &cli.Command{
	Name:  "encode",
	Usage: "encode various types",
	Subcommands: []*cli.Command{
		chainEncodeParamsCmd,
	},
}

var chainEncodeParamsCmd = &cli.Command{
	Name:      "params",
	Usage:     "Encodes the given JSON params",
	ArgsUsage: "[dest method params]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "tipset",
		},
		&cli.StringFlag{
			Name:  "encoding",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
		&cli.BoolFlag{
			Name:  "to-code",
			Usage: "interpret dest as code CID instead of as address",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return ShowHelp(cctx, fmt.Errorf("incorrect number of arguments"))
		}

		method, err := strconv.ParseInt(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing method id: %w", err)
		}

		ctx := ReqContext(cctx)

		var p []byte
		if !cctx.Bool("to-code") {
			svc, err := GetFullNodeServices(cctx)
			if err != nil {
				return err
			}
			defer svc.Close() // nolint

			to, err := address.NewFromString(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("parsing to addr: %w", err)
			}

			p, err = svc.DecodeTypedParamsFromJSON(ctx, to, abi.MethodNum(method), cctx.Args().Get(2))
			if err != nil {
				return xerrors.Errorf("decoding json params: %w", err)
			}
		} else {
			api, done, err := GetFullNodeAPIV1(cctx)
			if err != nil {
				return err
			}
			defer done()

			to, err := cid.Parse(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("parsing to addr: %w", err)
			}

			p, err = api.StateEncodeParams(ctx, to, abi.MethodNum(method), json.RawMessage(cctx.Args().Get(2)))
			if err != nil {
				return xerrors.Errorf("decoding json params: %w", err)
			}
		}

		switch cctx.String("encoding") {
		case "base64", "b64":
			fmt.Println(base64.StdEncoding.EncodeToString(p))
		case "hex":
			fmt.Println(hex.EncodeToString(p))
		default:
			return xerrors.Errorf("unknown encoding")
		}

		return nil
	},
}
