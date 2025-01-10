package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/fatih/color"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"

	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var msgCmd = &cli.Command{
	Name:      "message",
	Aliases:   []string{"msg"},
	Usage:     "Translate message between various formats",
	ArgsUsage: "Message in any form",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-message",
			Usage: "Print the message details",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "exec-trace",
			Usage: "Print the execution trace",
		},
		&cli.BoolFlag{
			Name:  "gas-stats",
			Usage: "Print a summary of gas charges",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		msg, err := messageFromString(cctx, cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		// Get the CID of the message
		mcid := msg.Cid()

		// Search for the message on-chain
		lookup, err := api.StateSearchMsg(ctx, mcid)
		if err != nil {
			return err
		}
		if lookup == nil {
			fmt.Println("Message not found on-chain. Continuing...")
		} else {
			// Replay the message to get the execution trace
			res, err := api.StateReplay(ctx, types.EmptyTSK, mcid)
			if err != nil {
				return xerrors.Errorf("replay call failed: %w", err)
			}

			if cctx.Bool("exec-trace") {
				// Print the execution trace
				color.Green("Execution trace:")
				trace, err := json.MarshalIndent(res.ExecutionTrace, "", "  ")
				if err != nil {
					return xerrors.Errorf("marshaling execution trace: %w", err)
				}
				fmt.Println(string(trace))
				fmt.Println()

				color.Green("Receipt:")
				fmt.Printf("Exit code: %d\n", res.MsgRct.ExitCode)
				fmt.Printf("Return: %x\n", res.MsgRct.Return)
				fmt.Printf("Gas Used: %d\n", res.MsgRct.GasUsed)
			}

			if cctx.Bool("gas-stats") {
				var printTrace func(descPfx string, trace types.ExecutionTrace) error
				printTrace = func(descPfx string, trace types.ExecutionTrace) error {
					typ := "Message"
					if descPfx != "" {
						typ = "Subcall"
					}
					_, _ = fmt.Fprintln(cctx.App.Writer, color.New(color.Bold).Sprint(fmt.Sprintf("%s (%s%s) gas charges:", typ, descPfx, trace.Msg.To)))
					if err := statsTable(cctx.App.Writer, trace, false); err != nil {
						return err
					}
					for _, subtrace := range trace.Subcalls {
						_, _ = fmt.Fprintln(cctx.App.Writer)
						if err := printTrace(descPfx+trace.Msg.To.String()+"➜", subtrace); err != nil {
							return err
						}
					}
					return nil
				}
				if err := printTrace("", res.ExecutionTrace); err != nil {
					return err
				}
				if len(res.ExecutionTrace.Subcalls) > 0 {
					_, _ = fmt.Fprintln(cctx.App.Writer)
					_, _ = fmt.Fprintln(cctx.App.Writer, color.New(color.Bold).Sprint("Total gas charges:"))
					if err := statsTable(cctx.App.Writer, res.ExecutionTrace, true); err != nil {
						return err
					}
					perCallTrace := gasTracesPerCall(res.ExecutionTrace)
					_, _ = fmt.Fprintln(cctx.App.Writer)
					_, _ = fmt.Fprintln(cctx.App.Writer, color.New(color.Bold).Sprint("Gas charges per call:"))
					if err := statsTable(cctx.App.Writer, perCallTrace, false); err != nil {
						return err
					}
				}
			}
		}

		if cctx.Bool("show-message") {
			switch msg := msg.(type) {
			case *types.SignedMessage:
				if err := printSignedMessage(cctx, msg); err != nil {
					return err
				}
			case *types.Message:
				if err := printMessage(cctx, msg); err != nil {
					return err
				}
			default:
				return xerrors.Errorf("this error message can't be printed")
			}
		}

		return nil
	},
}

func printSignedMessage(cctx *cli.Context, smsg *types.SignedMessage) error {
	color.Green("Signed:")
	color.Blue("CID: %s\n", smsg.Cid())

	b, err := smsg.Serialize()
	if err != nil {
		return err
	}
	color.Magenta("HEX: %x\n", b)
	color.Blue("B64: %s\n", base64.StdEncoding.EncodeToString(b))
	jm, err := json.MarshalIndent(smsg, "", "  ")
	if err != nil {
		return xerrors.Errorf("marshaling as json: %w", err)
	}

	color.Magenta("JSON: %s\n", string(jm))
	fmt.Println()
	fmt.Println("---")
	color.Green("Signed Message Details:")
	fmt.Printf("Signature(hex): %x\n", smsg.Signature.Data)
	fmt.Printf("Signature(b64): %s\n", base64.StdEncoding.EncodeToString(smsg.Signature.Data))

	sigtype, err := smsg.Signature.Type.Name()
	if err != nil {
		sigtype = err.Error()
	}
	fmt.Printf("Signature type: %d (%s)\n", smsg.Signature.Type, sigtype)

	fmt.Println("-------")
	return printMessage(cctx, &smsg.Message)
}

func printMessage(cctx *cli.Context, msg *types.Message) error {
	if msg.Version != 0x6d736967 {
		color.Green("Unsigned:")
		color.Yellow("CID: %s\n", msg.Cid())

		b, err := msg.Serialize()
		if err != nil {
			return err
		}
		color.Cyan("HEX: %x\n", b)
		color.Yellow("B64: %s\n", base64.StdEncoding.EncodeToString(b))

		jm, err := json.MarshalIndent(msg, "", "  ")
		if err != nil {
			return xerrors.Errorf("marshaling as json: %w", err)
		}

		color.Cyan("JSON: %s\n", string(jm))
		fmt.Println()
	} else {
		color.Green("Msig Propose:")
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

		color.Cyan("HEX: %x\n", b.Bytes())
		color.Yellow("B64: %s\n", base64.StdEncoding.EncodeToString(b.Bytes()))
		jm, err := json.MarshalIndent(pp, "", "  ")
		if err != nil {
			return xerrors.Errorf("marshaling as json: %w", err)
		}

		color.Cyan("JSON: %s\n", string(jm))
		fmt.Println()
	}

	fmt.Println("---")
	color.Green("Message Details:")
	fmt.Println("Value:", types.FIL(msg.Value))
	fmt.Println("Max Fees:", types.FIL(msg.RequiredFunds()))
	fmt.Println("Max Total Cost:", types.FIL(big.Add(msg.RequiredFunds(), msg.Value)))

	api, closer, err := lcli.GetFullNodeAPI(cctx)
	if err != nil {
		return err
	}

	defer closer()
	ctx := lcli.ReqContext(cctx)

	toact, err := api.StateGetActor(ctx, msg.To, types.EmptyTSK)
	if err != nil {
		return nil
	}

	fmt.Println("Method:", consensus.NewActorRegistry().Methods[toact.Code][msg.Method].Name) // todo use remote
	p, err := lcli.JsonParams(toact.Code, msg.Method, msg.Params)
	if err != nil {
		return err
	}

	fmt.Println("Params:", p)

	if msg, err := messageFromBytes(cctx, msg.Params); err == nil {
		fmt.Println("---")
		color.Red("Params message:")

		if err := printMessage(cctx, msg.VMMessage()); err != nil {
			return err
		}
	}

	return nil
}

func messageFromString(cctx *cli.Context, smsg string) (types.ChainMsg, error) {
	// a CID is least likely to just decode
	if c, err := cid.Parse(smsg); err == nil {
		return messageFromCID(cctx, c)
	}

	// try baseX serializations next
	{
		// hex first, some hay strings may be decodable as b64
		if b, err := hex.DecodeString(smsg); err == nil {
			return messageFromBytes(cctx, b)
		}

		// b64 next
		if b, err := base64.StdEncoding.DecodeString(smsg); err == nil {
			return messageFromBytes(cctx, b)
		}

		// b64u??
		if b, err := base64.URLEncoding.DecodeString(smsg); err == nil {
			return messageFromBytes(cctx, b)
		}
	}

	// maybe it's json?
	if _, err := messageFromJson(cctx, []byte(smsg)); err == nil {
		return nil, err
	}

	// declare defeat
	return nil, xerrors.Errorf("couldn't decode the message")
}

func messageFromJson(cctx *cli.Context, msgb []byte) (types.ChainMsg, error) {
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

func messageFromBytes(cctx *cli.Context, msgb []byte) (types.ChainMsg, error) {
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
				Version: 0x6d736967,
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
		if msg, err := messageFromJson(cctx, msgb); err == nil {
			return msg, nil
		}
	}

	return nil, xerrors.New("probably not a cbor-serialized message")
}

func messageFromCID(cctx *cli.Context, c cid.Cid) (types.ChainMsg, error) {
	api, closer, err := lcli.GetFullNodeAPI(cctx)
	if err != nil {
		return nil, err
	}

	defer closer()
	ctx := lcli.ReqContext(cctx)

	msgb, err := api.ChainReadObj(ctx, c)
	if err != nil {
		return nil, err
	}

	return messageFromBytes(cctx, msgb)
}

type gasTally struct {
	storageGas int64
	computeGas int64
	count      int
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
			"S%":          fmt.Sprintf("%.2f", float64(charge.storageGas)/float64(totals.storageGas)*100),
			"Compute Gas": charge.computeGas,
			"C%":          fmt.Sprintf("%.2f", float64(charge.computeGas)/float64(totals.computeGas)*100),
			"Total Gas":   charge.storageGas + charge.computeGas,
			"T%":          fmt.Sprintf("%.2f", float64(charge.storageGas+charge.computeGas)/float64(totals.storageGas+totals.computeGas)*100),
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
