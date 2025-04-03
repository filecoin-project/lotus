package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"

	"github.com/fatih/color"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	carbstore "github.com/ipld/go-car/v2/blockstore"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

const MultisigMagicNumber = 0x6d736967

var msgCmd = &cli.Command{
	Name:      "message",
	Aliases:   []string{"msg"},
	Usage:     "Inspect a message, either on chain (by CID) or attempt to decode raw bytes (hex, base64, json)",
	ArgsUsage: "Message in any form",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-message",
			Usage: "Print the message details",
			Value: true,
		},
		&cli.StringFlag{
			Name:  "message-format",
			Usage: "Format of the message (hex, base64, json)",
			Value: "json",
		},
		&cli.BoolFlag{
			Name:  "exec-trace",
			Usage: "Print the execution trace",
		},
		&cli.BoolFlag{
			Name:  "show-receipt",
			Usage: "Print the message receipt",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "gas-stats",
			Usage: "Print a summary of gas charges",
		},
		&cli.BoolFlag{
			Name:  "block-analysis",
			Usage: "Perform and print an analysis of the blocks written during message execution",
		},
		&cli.BoolFlag{
			Name:  "dump-car",
			Usage: "Dump the blocks (intermediate and final) to a CAR temp file",
		},
	},
	Action: func(cctx *cli.Context) error {
		color.Output = cctx.App.Writer

		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		msg, err := messageFromString(ctx, api, cctx.Args().First())
		if err != nil {
			return err
		}

		needTrace := cctx.Bool("exec-trace") || cctx.Bool("gas-stats") || cctx.Bool("block-analysis") || cctx.Bool("dump-car")

		// Search for the message on-chain
		lookup, err := api.StateSearchMsg(ctx, types.EmptyTSK, msg.Cid(), lapi.LookbackNoLimit, true)
		if err != nil {
			return err
		} else if lookup == nil {
			color.Red("Message not found on-chain")
		} else if needTrace {
			// Re-execute the message to get the trace
			executionTs, err := api.ChainGetTipSet(ctx, lookup.TipSet)
			if err != nil {
				return xerrors.Errorf("getting tipset: %w", err)
			}
			res, err := api.StateCall(ctx, lapi.NewStateCallParams(msg.VMMessage(), executionTs.Parents()).WithIncludeBlocks().ToRaw())
			if err != nil {
				return xerrors.Errorf("executing message: %w", err)
			}

			if cctx.Bool("exec-trace") {
				if err := printTrace(cctx.App.Writer, res.ExecutionTrace); err != nil {
					return xerrors.Errorf("printing execution trace: %w", err)
				}
				_, _ = fmt.Fprintln(cctx.App.Writer)
			}

			if res.Blocks == nil && (cctx.Bool("dump-car") || cctx.Bool("block-analysis")) {
				// could be an incompatible endpoint that doesn't serve blocks
				color.Red("no blocks found in execution trace")
			} else {
				if cctx.Bool("dump-car") {
					if err := saveBlocks(ctx, res); err != nil {
						return xerrors.Errorf("writing blocks to car: %w", err)
					}
					_, _ = fmt.Fprintln(cctx.App.Writer)
				}

				if cctx.Bool("block-analysis") {
					if err := printBlockAnalysis(ctx, cctx.App.Writer, res); err != nil {
						return err
					}
					_, _ = fmt.Fprintln(cctx.App.Writer)
				}
			}

			if cctx.Bool("gas-stats") {
				if err := printGasStats(cctx.App.Writer, res.ExecutionTrace); err != nil {
					return xerrors.Errorf("printing gas stats: %w", err)
				}
				_, _ = fmt.Fprintln(cctx.App.Writer)
			}

			if cctx.Bool("show-receipt") {
				printReceipt(cctx.App.Writer, res)
				_, _ = fmt.Fprintln(cctx.App.Writer)
			}
		}

		if cctx.Bool("show-message") {
			if err := printMessage(ctx, cctx.App.Writer, api, msg, cctx.String("message-format")); err != nil {
				return xerrors.Errorf("printing message: %w", err)
			}
		}

		return nil
	},
}

func printMessage(ctx context.Context, out io.Writer, api v1api.FullNode, msg types.ChainMsg, format string) error {
	switch msg := msg.(type) {
	case *types.SignedMessage:
		if err := printSignedMessage(ctx, out, api, msg, format); err != nil {
			return err
		}
	case *types.Message:
		if err := printUnsignedMessage(ctx, out, api, msg, format); err != nil {
			return err
		}
	default:
		return xerrors.Errorf("this error message can't be printed")
	}
	return nil
}

func printSignedMessage(ctx context.Context, out io.Writer, api v1api.FullNode, smsg *types.SignedMessage, format string) error {
	_, _ = color.New(color.Bold, color.FgGreen).Println("Signed message:")
	_, _ = fmt.Fprintf(out, "CID: %s\n", smsg.Cid())

	b, err := smsg.Serialize()
	if err != nil {
		return err
	}
	switch format {
	case "hex":
		_, _ = fmt.Fprintf(out, "%x\n", b)
	case "base64":
		_, _ = fmt.Fprintln(out, base64.StdEncoding.EncodeToString(b))
	case "json":
		if jm, err := json.MarshalIndent(smsg, "", "  "); err != nil {
			return xerrors.Errorf("marshaling as json: %w", err)
		} else {
			_, _ = fmt.Println(string(jm))
		}
	}

	_, _ = color.New(color.Bold, color.FgGreen).Println("\nSigned Message Details:")
	_, _ = fmt.Fprintf(out, "Signature(hex): %x\n", smsg.Signature.Data)
	_, _ = fmt.Fprintf(out, "Signature(b64): %s\n", base64.StdEncoding.EncodeToString(smsg.Signature.Data))

	sigtype, err := smsg.Signature.Type.Name()
	if err != nil {
		sigtype = err.Error()
	}
	_, _ = fmt.Fprintf(out, "Signature type: %d (%s)\n", smsg.Signature.Type, sigtype)
	_, _ = fmt.Fprintln(out)

	return printUnsignedMessage(ctx, out, api, &smsg.Message, format)
}

func printUnsignedMessage(ctx context.Context, out io.Writer, api v1api.FullNode, msg *types.Message, format string) error {
	if msg.Version != MultisigMagicNumber {
		_, _ = color.New(color.Bold, color.FgGreen).Println("Unsigned message:")
		_, _ = fmt.Fprintf(out, "CID: %s\n", msg.Cid())

		b, err := msg.Serialize()
		if err != nil {
			return err
		}

		switch format {
		case "hex":
			_, _ = fmt.Fprintf(out, "%x\n", b)
		case "base64":
			_, _ = fmt.Fprintln(out, base64.StdEncoding.EncodeToString(b))
		case "json":
			if jm, err := json.MarshalIndent(msg, "", "  "); err != nil {
				return xerrors.Errorf("marshaling as json: %w", err)
			} else {
				_, _ = fmt.Println(string(jm))
			}
		}
	} else {
		_, _ = color.New(color.Bold, color.FgGreen).Println("Msig Propose:")
		pp := &multisig.ProposeParams{
			To:     msg.To,
			Value:  msg.Value,
			Method: msg.Method,
			Params: msg.Params,
		}
		var b bytes.Buffer
		if err := pp.MarshalCBOR(&b); err != nil {
			return err
		}

		switch format {
		case "hex":
			_, _ = fmt.Fprintf(out, "%x\n", b.Bytes())
		case "base64":
			_, _ = fmt.Fprintln(out, base64.StdEncoding.EncodeToString(b.Bytes()))
		case "json":
			if jm, err := json.MarshalIndent(pp, "", "  "); err != nil {
				return xerrors.Errorf("marshaling as json: %w", err)
			} else {
				_, _ = fmt.Println(string(jm))
			}
		}
	}

	_, _ = color.New(color.Bold, color.FgGreen).Println("\nMessage Details:")
	_, _ = fmt.Fprintln(out, "Value:         ", types.FIL(msg.Value))
	_, _ = fmt.Fprintln(out, "Max Fees:      ", types.FIL(msg.RequiredFunds()))
	_, _ = fmt.Fprintln(out, "Max Total Cost:", types.FIL(big.Add(msg.RequiredFunds(), msg.Value)))

	toact, err := api.StateGetActor(ctx, msg.To, types.EmptyTSK)
	if err != nil {
		color.Red("Failed to get actor for further details: %s", err)
		return nil
	}

	_, _ = fmt.Fprintf(out, "Method:         %s (%d)\n",
		color.New(color.Bold).Sprint(consensus.NewActorRegistry().Methods[toact.Code][msg.Method].Name),
		msg.Method,
	) // todo use remote

	if p, err := lcli.JsonParams(toact.Code, msg.Method, msg.Params); err != nil {
		return err
	} else {
		_, _ = fmt.Fprintf(out, "Params:\n%s\n", p)
	}

	if msg, err := messageFromBytes(msg.Params); err == nil {
		_, _ = color.New(color.Bold).Println("Params message:")
		if err := printMessage(ctx, out, api, msg.VMMessage(), format); err != nil {
			return err
		}
	}

	return nil
}

func messageFromString(ctx context.Context, api v1api.FullNode, smsg string) (types.ChainMsg, error) {
	// a CID is least likely to just decode
	if c, err := cid.Parse(smsg); err == nil {
		return messageFromCID(ctx, api, c)
	}

	// try baseX serializations next
	{
		// hex first, some hay strings may be decodable as b64
		if b, err := hex.DecodeString(smsg); err == nil {
			return messageFromBytes(b)
		}

		// b64 next
		if b, err := base64.StdEncoding.DecodeString(smsg); err == nil {
			return messageFromBytes(b)
		}

		// b64u??
		if b, err := base64.URLEncoding.DecodeString(smsg); err == nil {
			return messageFromBytes(b)
		}
	}

	// maybe it's json?
	if _, err := messageFromJson([]byte(smsg)); err == nil {
		return nil, err
	}

	// declare defeat
	return nil, xerrors.Errorf("couldn't decode the message")
}

func messageFromJson(msgb []byte) (types.ChainMsg, error) {
	// Unsigned
	{
		var msg types.Message
		if err := json.Unmarshal(msgb, &msg); err == nil {
			if msg.To != address.Undef {
				return &msg, nil
			}
		}
	}

	// Signed
	{
		var msg types.SignedMessage
		if err := json.Unmarshal(msgb, &msg); err == nil {
			if msg.Message.To != address.Undef {
				return &msg, nil
			}
		}
	}

	return nil, xerrors.New("probably not a json-serialized message")
}

func messageFromBytes(msgb []byte) (types.ChainMsg, error) {
	// Signed
	{
		var msg types.SignedMessage
		if err := msg.UnmarshalCBOR(bytes.NewReader(msgb)); err == nil {
			return &msg, nil
		}
	}

	// Unsigned
	{
		var msg types.Message
		if err := msg.UnmarshalCBOR(bytes.NewReader(msgb)); err == nil {
			return &msg, nil
		}
	}

	// Multisig propose?
	{
		var pp multisig.ProposeParams
		if err := pp.UnmarshalCBOR(bytes.NewReader(msgb)); err == nil {
			i, err := address.NewIDAddress(0)
			if err != nil {
				return nil, err
			}

			return &types.Message{
				// Hack(-ish)
				Version: MultisigMagicNumber,
				From:    i,

				To:    pp.To,
				Value: pp.Value,

				Method: pp.Method,
				Params: pp.Params,

				GasFeeCap:  big.Zero(),
				GasPremium: big.Zero(),
			}, nil
		}
	}

	// Encoded json???
	{
		if msg, err := messageFromJson(msgb); err == nil {
			return msg, nil
		}
	}

	return nil, xerrors.New("couldn't decode the message")
}

func messageFromCID(ctx context.Context, api v1api.FullNode, c cid.Cid) (types.ChainMsg, error) {
	msgb, err := api.ChainReadObj(ctx, c)
	if err != nil {
		return nil, err
	}

	return messageFromBytes(msgb)
}

type gasTally struct {
	storageGas int64
	computeGas int64
	count      int
}

func printTrace(out io.Writer, trace types.ExecutionTrace) error {
	// Print the execution trace
	_, _ = color.New(color.Bold, color.FgGreen).Println("Execution trace:")
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(trace); err != nil {
		return xerrors.Errorf("encoding execution trace: %w", err)
	}
	return nil
}

func printReceipt(out io.Writer, res *api.InvocResult) {
	_, _ = color.New(color.Bold, color.FgGreen).Println("Receipt:")
	_, _ = fmt.Fprintf(out, "Exit code: %d\n", res.MsgRct.ExitCode)
	_, _ = fmt.Fprintf(out, "Return: %x\n", res.MsgRct.Return)
	_, _ = fmt.Fprintf(out, "Gas used: %d\n", res.MsgRct.GasUsed)
}

func accumGasTallies(charges map[string]*gasTally, totals *gasTally, trace types.ExecutionTrace, recurse bool) {
	for _, charge := range trace.GasCharges {
		name := charge.Name
		if _, ok := charges[name]; !ok {
			charges[name] = &gasTally{}
		}
		charges[name].computeGas += charge.ComputeGas
		charges[name].storageGas += charge.StorageGas
		charges[name].count++
		totals.computeGas += charge.ComputeGas
		totals.storageGas += charge.StorageGas
		totals.count++
	}
	if recurse {
		for _, subtrace := range trace.Subcalls {
			accumGasTallies(charges, totals, subtrace, recurse)
		}
	}
}

func statsTable(out io.Writer, trace types.ExecutionTrace, recurse bool) error {
	tw := tablewriter.New(
		tablewriter.Col("Type"),
		tablewriter.Col("Count", tablewriter.RightAlign()),
		tablewriter.Col("Storage Gas", tablewriter.RightAlign()),
		tablewriter.Col("S%", tablewriter.RightAlign()),
		tablewriter.Col("Compute Gas", tablewriter.RightAlign()),
		tablewriter.Col("C%", tablewriter.RightAlign()),
		tablewriter.Col("Total Gas", tablewriter.RightAlign()),
		tablewriter.Col("T%", tablewriter.RightAlign()),
	)

	totals := &gasTally{}
	charges := make(map[string]*gasTally)
	accumGasTallies(charges, totals, trace, recurse)

	// Sort by name
	names := make([]string, 0, len(charges))
	for name := range charges {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		charge := charges[name]
		tw.Write(map[string]interface{}{
			"Type":        name,
			"Count":       charge.count,
			"Storage Gas": charge.storageGas,
			"S%":          fmt.Sprintf("%.2f", safeDivide(charge.storageGas, totals.storageGas)*100),
			"Compute Gas": charge.computeGas,
			"C%":          fmt.Sprintf("%.2f", safeDivide(charge.computeGas, totals.computeGas)*100),
			"Total Gas":   charge.storageGas + charge.computeGas,
			"T%":          fmt.Sprintf("%.2f", safeDivide(charge.storageGas+charge.computeGas, totals.storageGas+totals.computeGas)*100),
		})
	}
	tw.Write(map[string]interface{}{
		"Type":        "Total",
		"Count":       totals.count,
		"Storage Gas": totals.storageGas,
		"S%":          "100.00",
		"Compute Gas": totals.computeGas,
		"C%":          "100.00",
		"Total Gas":   totals.storageGas + totals.computeGas,
		"T%":          "100.00",
	})
	return tw.Flush(out, tablewriter.WithBorders())
}

// Takes an execution trace and returns a new trace that groups all the gas charges by the message
// they were charged in, with the gas charges named per message; the output is partial and only
// suitable for calling statsTable() with.
func gasTracesPerCall(inTrace types.ExecutionTrace) types.ExecutionTrace {
	outTrace := types.ExecutionTrace{
		GasCharges: []*types.GasTrace{},
	}
	count := 1
	var accum func(name string, trace types.ExecutionTrace)
	accum = func(name string, trace types.ExecutionTrace) {
		totals := &gasTally{}
		charges := make(map[string]*gasTally)
		accumGasTallies(charges, totals, trace, false)
		outTrace.GasCharges = append(outTrace.GasCharges, &types.GasTrace{
			Name:       fmt.Sprintf("#%d %s", count, name),
			ComputeGas: totals.computeGas,
			StorageGas: totals.storageGas,
		})
		count++
		for _, subtrace := range trace.Subcalls {
			accum(name+"➜"+subtrace.Msg.To.String(), subtrace)
		}
	}
	accum(inTrace.Msg.To.String(), inTrace)
	return outTrace
}

func saveBlocks(ctx context.Context, res *api.InvocResult) (err error) {
	cachedBlocksFile := path.Join(os.TempDir(), res.MsgCid.String()+".car")
	bs, err := carbstore.OpenReadWrite(cachedBlocksFile, nil, carbstore.WriteAsCarV1(true))
	if err != nil {
		return xerrors.Errorf("opening blocks file: %w", err)
	}

	defer func() {
		if cerr := bs.Close(); cerr != nil {
			err = xerrors.Errorf("closing blocks file: %w", cerr)
		}
	}()

	for _, b := range res.Blocks {
		bc, err := blocks.NewBlockWithCid(b.Data, b.Cid)
		if err != nil {
			return xerrors.Errorf("creating block: %w", err)
		}
		if err := bs.Put(ctx, bc); err != nil {
			return xerrors.Errorf("writing block: %w", err)
		}
	}
	_, _ = color.New(color.Bold, color.FgGreen).Printf("Blocks written (final and intermediate) dumped to CAR: %s\n", cachedBlocksFile)
	return nil
}

func printBlockAnalysis(ctx context.Context, out io.Writer, res *api.InvocResult) (err error) {
	_, _ = color.New(color.Bold, color.FgGreen).Println("Analysis of blocks written (final and intermediate):")

	blockMap := make(map[cid.Cid][]byte, len(res.Blocks))
	for _, b := range res.Blocks {
		blockMap[b.Cid] = b.Data
	}

	type blkStat struct {
		cid          cid.Cid
		matchingType string
		size         int64
		estimatedGas int64
	}

	var explainBlocks func(descPfx string, trace types.ExecutionTrace) error
	explainBlocks = func(descPfx string, trace types.ExecutionTrace) error {
		if trace.IpldOps == nil {
			return nil
		}

		typ := "Message"
		if descPfx != "" {
			typ = "Subcall"
		}

		blkStats := make([]blkStat, 0, len(res.Blocks))
		var totalBytes, totalGas int64

		for _, ipldOp := range trace.IpldOps {
			if ipldOp.Op == types.IpldOpPut {
				if _, ok := blockMap[ipldOp.Cid]; !ok {
					return xerrors.Errorf("block cid not found in execution trace: %s", ipldOp.Cid)
				}
				blk, err := blocks.NewBlockWithCid(blockMap[ipldOp.Cid], ipldOp.Cid)
				if err != nil {
					return xerrors.Errorf("creating block: %w", err)
				}
				m, err := matchKnownBlockType(ctx, blk)
				if err != nil {
					return xerrors.Errorf("matching block type: %w", err)
				}
				size := int64(len(blk.RawData()))
				gas := 172000 + 334000 + 3340*size
				blkStats = append(blkStats, blkStat{cid: ipldOp.Cid, matchingType: m, size: size, estimatedGas: gas})
				totalBytes += size
				totalGas += gas
			}
		}

		if len(blkStats) == 0 {
			return nil
		}

		_, _ = color.New(color.Bold).Printf("%s (%s%s) block writes:\n", typ, descPfx, trace.Msg.To)
		tw := tablewriter.New(
			tablewriter.Col("CID"),
			tablewriter.Col("Matching Type"),
			tablewriter.Col("Size", tablewriter.RightAlign()),
			tablewriter.Col("S%", tablewriter.RightAlign()),
			tablewriter.Col("Estimated Gas", tablewriter.RightAlign()),
			tablewriter.Col("G%", tablewriter.RightAlign()),
		)
		for _, bs := range blkStats {
			tw.Write(map[string]interface{}{
				"CID":           bs.cid,
				"Matching Type": bs.matchingType,
				"Size":          bs.size,
				"S%":            fmt.Sprintf("%.2f", safeDivide(bs.size, totalBytes)*100),
				"Estimated Gas": bs.estimatedGas,
				"G%":            fmt.Sprintf("%.2f", safeDivide(bs.estimatedGas, totalGas)*100),
			})
		}
		tw.Write(map[string]interface{}{
			"CID":           "Total",
			"Matching Type": "",
			"Size":          totalBytes,
			"S%":            "100.00",
			"Estimated Gas": totalGas,
			"G%":            "100.00",
		})
		if err := tw.Flush(out, tablewriter.WithBorders()); err != nil {
			return xerrors.Errorf("flushing table: %w", err)
		}

		for _, subtrace := range trace.Subcalls {
			if err := explainBlocks(descPfx+trace.Msg.To.String()+"➜", subtrace); err != nil {
				return err
			}
		}
		return nil
	}
	if err := explainBlocks("", res.ExecutionTrace); err != nil {
		return xerrors.Errorf("explaining blocks: %w", err)
	}
	return
}

func printGasStats(out io.Writer, trace types.ExecutionTrace) error {
	_, _ = color.New(color.Bold, color.FgGreen).Println("Gas charges:")

	var printTrace func(descPfx string, trace types.ExecutionTrace) error
	printTrace = func(descPfx string, trace types.ExecutionTrace) error {
		typ := "Message"
		if descPfx != "" {
			typ = "Subcall"
		}
		_, _ = color.New(color.Bold).Printf("%s (%s%s) gas charges:\n", typ, descPfx, trace.Msg.To)
		if err := statsTable(out, trace, false); err != nil {
			return err
		}
		for _, subtrace := range trace.Subcalls {
			if err := printTrace(descPfx+trace.Msg.To.String()+"➜", subtrace); err != nil {
				return err
			}
		}
		return nil
	}
	if err := printTrace("", trace); err != nil {
		return err
	}
	if len(trace.Subcalls) > 0 {
		_, _ = color.New(color.Bold).Println("Total gas charges:")
		if err := statsTable(out, trace, true); err != nil {
			return err
		}
		perCallTrace := gasTracesPerCall(trace)
		_, _ = color.New(color.Bold).Println("Gas charges per call:")
		if err := statsTable(out, perCallTrace, false); err != nil {
			return err
		}
	}
	return nil
}

func safeDivide(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
