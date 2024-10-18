package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-f3/manifest"
)

var (
	F3Cmd = &cli.Command{
		Name:  "f3",
		Usage: "Manages Filecoin Fast Finality (F3) interactions",
		Subcommands: []*cli.Command{
			{
				Name:    "list-participants",
				Aliases: []string{"lp"},
				Usage:   "Lists the miners that currently participate in F3 via this node.",
				Action: func(cctx *cli.Context) error {
					api, closer, err := GetFullNodeAPIV1(cctx)
					if err != nil {
						return err
					}
					defer closer()

					participants, err := api.F3ListParticipants(cctx.Context)
					if err != nil {
						return cli.Exit(fmt.Errorf("listing participants: %w", err), 1)
					}
					if len(participants) == 0 {
						_, _ = fmt.Fprintln(cctx.App.Writer, "No participants.")
						return nil
					}
					for i, participant := range participants {
						_, _ = fmt.Fprintf(cctx.App.Writer, "Participant %d:\n", i+1)
						_, _ = fmt.Fprintf(cctx.App.Writer, "  Miner ID:         %d\n", participant.MinerID)
						_, _ = fmt.Fprintf(cctx.App.Writer, "  From Instance:    %d\n", participant.FromInstance)
						_, _ = fmt.Fprintf(cctx.App.Writer, "  Validity Term:    %d\n", participant.ValidityTerm)
						_, _ = fmt.Fprintf(cctx.App.Writer, "\n")
					}
					_, _ = fmt.Fprintf(cctx.App.Writer, "Total of %d participant(s).\n", len(participants))
					return nil
				},
			},
			{
				Name:  "manifest",
				Usage: "Gets the current manifest used by F3.",
				Flags: []cli.Flag{f3FlagOutput},
				Action: func(cctx *cli.Context) error {
					api, closer, err := GetFullNodeAPIV1(cctx)
					if err != nil {
						return err
					}
					defer closer()

					manifest, err := api.F3GetManifest(cctx.Context)
					if err != nil {
						return cli.Exit(fmt.Errorf("getting manifest: %w", err), 1)
					}
					switch output := cctx.String("output"); strings.ToLower(output) {
					case "text":
						prettyPrintManifest(cctx.App.Writer, manifest)
						return nil
					case "json":
						encoder := json.NewEncoder(cctx.App.Writer)
						encoder.SetIndent("", "  ")
						return encoder.Encode(manifest)
					default:
						return fmt.Errorf("unknown output format: %s", output)
					}
				},
			},
			{
				Name:  "status",
				Usage: "Checks the F3 status.",
				Action: func(cctx *cli.Context) error {
					api, closer, err := GetFullNodeAPIV1(cctx)
					if err != nil {
						return err
					}
					defer closer()

					running, err := api.F3IsRunning(cctx.Context)
					if err != nil {
						return cli.Exit(fmt.Errorf("getting running state: %w", err), 1)
					}
					_, _ = fmt.Fprintf(cctx.App.Writer, "Running: %t\n", running)
					if !running {
						return nil
					}

					progress, err := api.F3GetProgress(cctx.Context)
					if err != nil {
						return cli.Exit(fmt.Errorf("getting progress: %w", err), 1)
					}

					_, _ = fmt.Fprintln(cctx.App.Writer, "Progress:")
					_, _ = fmt.Fprintf(cctx.App.Writer, "  Instance: %d\n", progress.ID)
					_, _ = fmt.Fprintf(cctx.App.Writer, "  Round:    %d\n", progress.Round)
					_, _ = fmt.Fprintf(cctx.App.Writer, "  Phase:    %s\n", progress.Phase)

					manifest, err := api.F3GetManifest(cctx.Context)
					if err != nil {
						return cli.Exit(fmt.Errorf("getting manifest: %w", err), 1)
					}
					prettyPrintManifest(cctx.App.Writer, manifest)
					return nil
				},
			},
		},
	}

	// TODO: we should standardise format as a top level flag. For now, here is an f3
	//       specific one.
	//       See: https://github.com/filecoin-project/lotus/issues/12616
	f3FlagOutput = &cli.StringFlag{
		Name:  "output",
		Usage: "The output format. Supported formats: text, json",
		Value: "text",
		Action: func(cctx *cli.Context, output string) error {
			switch output {
			case "text", "json":
				return nil
			default:
				return fmt.Errorf("unknown output format: %s", output)
			}
		},
	}
)

func prettyPrintManifest(out io.Writer, manifest *manifest.Manifest) {
	_, _ = fmt.Fprintf(out, "Manifest: ")
	if manifest == nil {
		_, _ = fmt.Fprintln(out, "None")
		return
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "  Protocol Version:   %d\n", manifest.ProtocolVersion)
	_, _ = fmt.Fprintf(out, "  Paused:             %t\n", manifest.Pause)
	_, _ = fmt.Fprintf(out, "  Initial Instance:   %d\n", manifest.InitialInstance)
	_, _ = fmt.Fprintf(out, "  Bootstrap Epoch:    %d\n", manifest.BootstrapEpoch)
	_, _ = fmt.Fprintf(out, "  Network Name:       %s\n", manifest.NetworkName)
	_, _ = fmt.Fprintf(out, "  Ignore EC Power:    %t\n", manifest.IgnoreECPower)
	_, _ = fmt.Fprintf(out, "  Committee Lookback: %d\n", manifest.CommitteeLookback)
	_, _ = fmt.Fprintf(out, "  Catch Up Alignment: %s\n", manifest.CatchUpAlignment)

	_, _ = fmt.Fprintf(out, "  GPBFT Delta:                     %s\n", manifest.Gpbft.Delta)
	_, _ = fmt.Fprintf(out, "  GPBFT Delta BackOff Exponent:    %.2f\n", manifest.Gpbft.DeltaBackOffExponent)
	_, _ = fmt.Fprintf(out, "  GPBFT Max Lookahead Rounds:      %d\n", manifest.Gpbft.MaxLookaheadRounds)
	_, _ = fmt.Fprintf(out, "  GPBFT Rebroadcast Backoff Base:  %s\n", manifest.Gpbft.RebroadcastBackoffBase)
	_, _ = fmt.Fprintf(out, "  GPBFT Rebroadcast Backoff Spread:%.2f\n", manifest.Gpbft.RebroadcastBackoffSpread)
	_, _ = fmt.Fprintf(out, "  GPBFT Rebroadcast Backoff Max:   %s\n", manifest.Gpbft.RebroadcastBackoffMax)

	_, _ = fmt.Fprintf(out, "  EC Period:            %s\n", manifest.EC.Period)
	_, _ = fmt.Fprintf(out, "  EC Finality:          %d\n", manifest.EC.Finality)
	_, _ = fmt.Fprintf(out, "  EC Delay Multiplier:  %.2f\n", manifest.EC.DelayMultiplier)
	_, _ = fmt.Fprintf(out, "  EC Head Lookback:     %d\n", manifest.EC.HeadLookback)
	_, _ = fmt.Fprintf(out, "  EC Finalize:          %t\n", manifest.EC.Finalize)

	_, _ = fmt.Fprintf(out, "  Certificate Exchange Client Timeout:  %s\n", manifest.CertificateExchange.ClientRequestTimeout)
	_, _ = fmt.Fprintf(out, "  Certificate Exchange Server Timeout:  %s\n", manifest.CertificateExchange.ServerRequestTimeout)
	_, _ = fmt.Fprintf(out, "  Certificate Exchange Min Poll Interval: %s\n", manifest.CertificateExchange.MinimumPollInterval)
	_, _ = fmt.Fprintf(out, "  Certificate Exchange Max Poll Interval: %s\n", manifest.CertificateExchange.MaximumPollInterval)
}
