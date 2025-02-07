package cli

import (
    "bytes"
    "context"
    "encoding/base64"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path"
    "sort"
    "strconv"
    "strings"
    "time"

    "github.com/ipfs/go-cid"
    "github.com/urfave/cli/v2"
    cbg "github.com/whyrusleeping/cbor-gen"
    "golang.org/x/xerrors"

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

    "github.com/filecoin-project/lotus/api"
    lapi "github.com/filecoin-project/lotus/api"
    "github.com/filecoin-project/lotus/api/v0api"
    "github.com/filecoin-project/lotus/build/buildconstants"
    "github.com/filecoin-project/lotus/chain/actors"
    "github.com/filecoin-project/lotus/chain/actors/policy"
    "github.com/filecoin-project/lotus/chain/consensus"
    "github.com/filecoin-project/lotus/chain/types"
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
        ChainExportRangeCmd,
        SlashConsensusFault,
        ChainGasPriceCmd,
        ChainInspectUsage,
        ChainDecodeCmd,
        ChainEncodeCmd,
        ChainDisputeSetCmd,
        ChainPruneCmd,
    },
}

var ChainHeadCmd = &cli.Command{
    Name:  "head",
    Usage: "Print chain head",
    Flags: []cli.Flag{
        &cli.BoolFlag{
            Name:  "height",
            Usage: "print just the epoch number of the chain head",
        },
    },
    Action: func(cctx *cli.Context) error {
        afmt := NewAppFmt(cctx.App)

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

        if cctx.Bool("height") {
            afmt.Println(head.Height())
        } else {
            for _, c := range head.Cids() {
                afmt.Println(c)
            }
        }
        return nil
    },
}

var ChainGetBlock = &cli.Command{
    Name:      "get-block",
    Aliases:   []string{"getblock"},
    Usage:     "Get a block and print its details",
    ArgsUsage: "[blockCid]",
    Flags: []cli.Flag{
        &cli.BoolFlag{
            Name:  "raw",
            Usage: "print just the raw block header",
        },
    },
    Action: func(cctx *cli.Context) error {
        afmt := NewAppFmt(cctx.App)

        api, closer, err := GetFullNodeAPI(cctx)
        if err != nil {
            return err
        }
        defer closer()
        ctx := ReqContext(cctx)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
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

            afmt.Println(string(out))
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
            // return xerrors.Errorf("failed to get receipts: %w", err)
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

        afmt.Println(string(out))
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
        afmt := NewAppFmt(cctx.App)

        api, closer, err := GetFullNodeAPI(cctx)
        if err != nil {
            return err
        }
        defer closer()
        ctx := ReqContext(cctx)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
        }

        c, err := cid.Decode(cctx.Args().First())
        if err != nil {
            return fmt.Errorf("failed to parse cid input: %s", err)
        }

        obj, err := api.ChainReadObj(ctx, c)
        if err != nil {
            return err
        }

        afmt.Printf("%x\n", obj)
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
        afmt := NewAppFmt(cctx.App)

        api, closer, err := GetFullNodeAPI(cctx)
        if err != nil {
            return err
        }
        defer closer()
        ctx := ReqContext(cctx)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
        }

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

        afmt.Printf("Obj %s deleted\n", c.String())
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
        afmt := NewAppFmt(cctx.App)
        api, closer, err := GetFullNodeAPI(cctx)
        if err != nil {
            return err
        }
        defer closer()
        ctx := ReqContext(cctx)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
        }

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

        afmt.Printf("Links: %d\n", stats.Links)
        afmt.Printf("Size: %s (%d)\n", types.SizeStr(types.NewInt(stats.Size)), stats.Size)
        return nil
    },
}

var ChainGetMsgCmd = &cli.Command{
    Name:      "getmessage",
    Aliases:   []string{"get-message", "get-msg"},
    Usage:     "Get and print a message by its cid",
    ArgsUsage: "[messageCid]",
    Action: func(cctx *cli.Context) error {
        afmt := NewAppFmt(cctx.App)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
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

        afmt.Println(string(enc))
        return nil
    },
}

var ChainSetHeadCmd = &cli.Command{
    Name:      "sethead",
    Aliases:   []string{"set-head"},
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

        if !cctx.Bool("genesis") && !cctx.IsSet("epoch") && cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
        }

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
        afmt := NewAppFmt(cctx.App)
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

            mm := consensus.NewActorRegistry().Methods[code][m.Message.Method] // TODO: use remote map

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

        data := map[string]interface{}{
            "TotalGasLimit": sum,
            "BySender":      senderVals,
            "ByReceiver":    destVals,
            "ByMethod":      methodVals,
        }

        return util.FormatOutput(cctx, data)
    },
}

var ChainListCmd = &cli.Command{
    Name:    "list",
    Aliases: []string{"love"},
    Usage:   "View a segment of the chain",
    Flags: []cli.Flag{
        &cli.Uint64Flag{Name: "epoch", Aliases: []string{"height"}, DefaultText: "current head"},
        &cli.IntFlag{Name: "count", Value: 30},
        &cli.StringFlag{
            Name:  "format",
            Usage: "specify the format to print out tipsets using placeholders: <epoch>, <time>, <blocks>, <weight>, <tipset>, <json_tipset>\n",
            Value: "<epoch>: (<time>) <blocks>",
        },
        &cli.BoolFlag{
            Name:  "gas-stats",
            Usage: "view gas statistics for the chain",
        },
    },
    Action: func(cctx *cli.Context) error {
        afmt := NewAppFmt(cctx.App)
        api, closer, err := GetFullNodeAPI(cctx)
        if err != nil {
            return err
        }
        defer closer()
        ctx := ReqContext(cctx)

        var head *types.TipSet

        if cctx.IsSet("height") {
            head, err = api.ChainGetTipSetByHeight(ctxpackage cli

import (
    "bytes"
    "context"
    "encoding/base64"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path"
    "sort"
    "strconv"
    "strings"
    "time"

    "github.com/ipfs/go-cid"
    "github.com/urfave/cli/v2"
    cbg "github.com/whyrusleeping/cbor-gen"
    "golang.org/x/xerrors"

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

    "github.com/filecoin-project/lotus/api"
    lapi "github.com/filecoin-project/lotus/api"
    "github.com/filecoin-project/lotus/api/v0api"
    "github.com/filecoin-project/lotus/build/buildconstants"
    "github.com/filecoin-project/lotus/chain/actors"
    "github.com/filecoin-project/lotus/chain/actors/policy"
    "github.com/filecoin-project/lotus/chain/consensus"
    "github.com/filecoin-project/lotus/chain/types"
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
        ChainExportRangeCmd,
        SlashConsensusFault,
        ChainGasPriceCmd,
        ChainInspectUsage,
        ChainDecodeCmd,
        ChainEncodeCmd,
        ChainDisputeSetCmd,
        ChainPruneCmd,
    },
}

var ChainHeadCmd = &cli.Command{
    Name:  "head",
    Usage: "Print chain head",
    Flags: []cli.Flag{
        &cli.BoolFlag{
            Name:  "height",
            Usage: "print just the epoch number of the chain head",
        },
    },
    Action: func(cctx *cli.Context) error {
        afmt := NewAppFmt(cctx.App)

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

        if cctx.Bool("height") {
            afmt.Println(head.Height())
        } else {
            for _, c := range head.Cids() {
                afmt.Println(c)
            }
        }
        return nil
    },
}

var ChainGetBlock = &cli.Command{
    Name:      "get-block",
    Aliases:   []string{"getblock"},
    Usage:     "Get a block and print its details",
    ArgsUsage: "[blockCid]",
    Flags: []cli.Flag{
        &cli.BoolFlag{
            Name:  "raw",
            Usage: "print just the raw block header",
        },
    },
    Action: func(cctx *cli.Context) error {
        afmt := NewAppFmt(cctx.App)

        api, closer, err := GetFullNodeAPI(cctx)
        if err != nil {
            return err
        }
        defer closer()
        ctx := ReqContext(cctx)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
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

            afmt.Println(string(out))
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
            // return xerrors.Errorf("failed to get receipts: %w", err)
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

        afmt.Println(string(out))
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
        afmt := NewAppFmt(cctx.App)

        api, closer, err := GetFullNodeAPI(cctx)
        if err != nil {
            return err
        }
        defer closer()
        ctx := ReqContext(cctx)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
        }

        c, err := cid.Decode(cctx.Args().First())
        if err != nil {
            return fmt.Errorf("failed to parse cid input: %s", err)
        }

        obj, err := api.ChainReadObj(ctx, c)
        if err != nil {
            return err
        }

        afmt.Printf("%x\n", obj)
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
        afmt := NewAppFmt(cctx.App)

        api, closer, err := GetFullNodeAPI(cctx)
        if err != nil {
            return err
        }
        defer closer()
        ctx := ReqContext(cctx)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
        }

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

        afmt.Printf("Obj %s deleted\n", c.String())
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
        afmt := NewAppFmt(cctx.App)
        api, closer, err := GetFullNodeAPI(cctx)
        if err != nil {
            return err
        }
        defer closer()
        ctx := ReqContext(cctx)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
        }

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

        afmt.Printf("Links: %d\n", stats.Links)
        afmt.Printf("Size: %s (%d)\n", types.SizeStr(types.NewInt(stats.Size)), stats.Size)
        return nil
    },
}

var ChainGetMsgCmd = &cli.Command{
    Name:      "getmessage",
    Aliases:   []string{"get-message", "get-msg"},
    Usage:     "Get and print a message by its cid",
    ArgsUsage: "[messageCid]",
    Action: func(cctx *cli.Context) error {
        afmt := NewAppFmt(cctx.App)

        if cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
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

        afmt.Println(string(enc))
        return nil
    },
}

var ChainSetHeadCmd = &cli.Command{
    Name:      "sethead",
    Aliases:   []string{"set-head"},
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

        if !cctx.Bool("genesis") && !cctx.IsSet("epoch") && cctx.NArg() != 1 {
            return IncorrectNumArgs(cctx)
        }

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
        afmt := NewAppFmt(cctx.App)
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

            mm := consensus.NewActorRegistry().Methods[code][m.Message.Method] // TODO: use remote map

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

        data := map[string]interface{}{
            "TotalGasLimit": sum,
            "BySender":      senderVals,
            "ByReceiver":    destVals,
            "ByMethod":      methodVals,
        }

        return util.FormatOutput(cctx, data)
    },
}

var ChainListCmd = &cli.Command{
    Name:    "list",
    Aliases: []string{"love"},
    Usage:   "View a segment of the chain",
    Flags: []cli.Flag{
        &cli.Uint64Flag{Name: "epoch", Aliases: []string{"height"}, DefaultText: "current head"},
        &cli.IntFlag{Name: "count", Value: 30},
        &cli.StringFlag{
            Name:  "format",
            Usage: "specify the format to print out tipsets using placeholders: <epoch>, <time>, <blocks>, <weight>, <tipset>, <json_tipset>\n",
            Value: "<epoch>: (<time>) <blocks>",
        },
        &cli.BoolFlag{
            Name:  "gas-stats",
            Usage: "view gas statistics for the chain",
        },
    },
    Action: func(cctx *cli.Context) error {
        afmt := NewAppFmt(cctx.App)
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
					afmt.Printf("%d: %d blocks (baseFee: %s -> maxFee: %s)\n", ts.Height(), len(ts.Blocks()), ts.Blocks()[0].ParentBaseFee, types.FIL(types.BigMul(pbf, types.NewInt(uint64(buildconstants.BlockGasLimit)))))
	
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
	
						afmt.Printf("\t%s: \t%d msgs, gasLimit: %d / %d (%0.2f%%), avgPremium: %s\n", b.Miner, len(msgs.BlsMessages)+len(msgs.SecpkMessages), limitSum, buildconstants.BlockGasLimit, 100*float64(limitSum)/float64(buildconstants.BlockGasLimit), avgpremium)
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
						gasCapacity := 100 * float64(limitSum) / float64(buildconstants.BlockGasLimit)
	
						afmt.Printf("\ttipset: \t%d msgs, %d (%0.2f%%) / %d (%0.2f%%)\n", len(msgs), gasUsed, gasEfficiency, limitSum, gasCapacity)
					}
					afmt.Println()
				}
			} else {
				for i := len(tss) - 1; i >= 0; i-- {
					printTipSet(cctx.String("format"), tss[i], afmt)
				}
			}
			return nil
		},
	}
