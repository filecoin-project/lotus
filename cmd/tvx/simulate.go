package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/conformance"
)

var simulateFlags struct {
	msg       string
	epoch     int64
	out       string
	statediff bool
}

var simulateCmd = &cli.Command{
	Name: "simulate",
	Description: "simulate a raw message on top of the supplied epoch (or HEAD), " +
		"reporting the result on stderr and writing a test vector on stdout " +
		"or into the specified file",
	Action: runSimulateCmd,
	Before: initialize,
	After:  destroy,
	Flags: []cli.Flag{
		&repoFlag,
		&cli.StringFlag{
			Name:        "msg",
			Usage:       "base64 cbor-encoded message",
			Destination: &simulateFlags.msg,
			Required:    true,
		},
		&cli.Int64Flag{
			Name:        "at-epoch",
			Usage:       "epoch at which to run this message (or HEAD if not provided)",
			Destination: &simulateFlags.epoch,
		},
		&cli.StringFlag{
			Name:        "out",
			Usage:       "file to write the test vector to; if nil, the vector will be written to stdout",
			TakesFile:   true,
			Destination: &simulateFlags.out,
		},
		&cli.BoolFlag{
			Name:        "statediff",
			Usage:       "display a statediff of the precondition and postcondition states",
			Destination: &simulateFlags.statediff,
		},
	},
}

func runSimulateCmd(_ *cli.Context) error {
	ctx := context.Background()
	r := new(conformance.LogReporter)

	msgb, err := base64.StdEncoding.DecodeString(simulateFlags.msg)
	if err != nil {
		return fmt.Errorf("failed to base64-decode message: %w", err)
	}

	msg, err := types.DecodeMessage(msgb)
	if err != nil {
		return fmt.Errorf("failed to deserialize message: %w", err)
	}

	log.Printf("message to simulate has CID: %s", msg.Cid())

	msgjson, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize message to json for printing: %w", err)
	}

	log.Printf("message to simulate: %s", string(msgjson))

	// Resolve the tipset, root, epoch.
	var ts *types.TipSet
	if epochIn := simulateFlags.epoch; epochIn == 0 {
		ts, err = FullAPI.ChainHead(ctx)
	} else {
		ts, err = FullAPI.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(epochIn), types.EmptyTSK)
	}

	if err != nil {
		return fmt.Errorf("failed to get tipset: %w", err)
	}

	var (
		preroot    = ts.ParentState()
		epoch      = ts.Height()
		baseFee    = ts.Blocks()[0].ParentBaseFee
		circSupply api.CirculatingSupply
	)

	// Get circulating supply.
	circSupply, err = FullAPI.StateVMCirculatingSupplyInternal(ctx, ts.Key())
	if err != nil {
		return fmt.Errorf("failed to get circulating supply for tipset %s: %w", ts.Key(), err)
	}

	// Create the driver.
	stores := NewProxyingStores(ctx, FullAPI)
	driver := conformance.NewDriver(ctx, schema.Selector{}, conformance.DriverOpts{
		DisableVMFlush: true,
	})
	rand := conformance.NewRecordingRand(r, FullAPI)

	tbs, ok := stores.Blockstore.(TracingBlockstore)
	if !ok {
		return fmt.Errorf("no tracing blockstore available")
	}
	tbs.StartTracing()
	applyret, postroot, err := driver.ExecuteMessage(stores.Blockstore, conformance.ExecuteMessageParams{
		Preroot:    preroot,
		Epoch:      epoch,
		Message:    msg,
		CircSupply: circSupply.FilCirculating,
		BaseFee:    baseFee,
		Rand:       rand,
		// TODO NetworkVersion
	})
	if err != nil {
		return fmt.Errorf("failed to apply message: %w", err)
	}

	accessed := tbs.FinishTracing()

	var (
		out = new(bytes.Buffer)
		gw  = gzip.NewWriter(out)
		g   = NewSurgeon(ctx, FullAPI, stores)
	)
	if err := g.WriteCARIncluding(gw, accessed, preroot, postroot); err != nil {
		return err
	}
	if err = gw.Flush(); err != nil {
		return err
	}
	if err = gw.Close(); err != nil {
		return err
	}

	version, err := FullAPI.Version(ctx)
	if err != nil {
		log.Printf("failed to get node version: %s; falling back to unknown", err)
		version = api.APIVersion{}
	}

	nv, err := FullAPI.StateNetworkVersion(ctx, ts.Key())
	if err != nil {
		return err
	}

	codename := GetProtocolCodename(epoch)

	// Write out the test vector.
	vector := schema.TestVector{
		Class: schema.ClassMessage,
		Meta: &schema.Metadata{
			ID: fmt.Sprintf("simulated-%s", msg.Cid()),
			Gen: []schema.GenerationData{
				{Source: "github.com/filecoin-project/lotus", Version: version.String()}},
		},
		Selector: schema.Selector{
			schema.SelectorMinProtocolVersion: codename,
		},
		Randomness: rand.Recorded(),
		CAR:        out.Bytes(),
		Pre: &schema.Preconditions{
			Variants: []schema.Variant{
				{ID: codename, Epoch: int64(epoch), NetworkVersion: uint(nv)},
			},
			CircSupply: circSupply.FilCirculating.Int,
			BaseFee:    baseFee.Int,
			StateTree: &schema.StateTree{
				RootCID: preroot,
			},
		},
		ApplyMessages: []schema.Message{{Bytes: msgb}},
		Post: &schema.Postconditions{
			StateTree: &schema.StateTree{
				RootCID: postroot,
			},
			Receipts: []*schema.Receipt{
				{
					ExitCode:    int64(applyret.ExitCode),
					ReturnValue: applyret.Return,
					GasUsed:     applyret.GasUsed,
				},
			},
		},
	}

	if err := writeVector(&vector, simulateFlags.out); err != nil {
		return fmt.Errorf("failed to write vector: %w", err)
	}

	log.Printf(color.GreenString("wrote vector at: %s"), simulateFlags.out)

	if !simulateFlags.statediff {
		return nil
	}

	if simulateFlags.out == "" {
		log.Print("omitting statediff in non-file mode")
		return nil
	}

	// check if statediff is installed; if not, skip.
	if err := exec.Command("statediff", "--help").Run(); err != nil {
		log.Printf("could not perform statediff on generated vector; command not found (%s)", err)
		log.Printf("install statediff with:")
		log.Printf("$ GOMODULE111=off go get github.com/filecoin-project/statediff/cmd/statediff")
		return err
	}

	stdiff, err := exec.Command("statediff", "vector", "--file", simulateFlags.out).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to statediff: %w", err)
	}

	log.Print(string(stdiff))
	return nil
}
