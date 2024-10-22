package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var (
	F3Cmd = &cli.Command{
		Name:  "f3",
		Usage: "Manages Filecoin Fast Finality (F3) interactions",
		Subcommands: []*cli.Command{
			{
				Name:    "list-miners",
				Aliases: []string{"lp"},
				Usage:   "Lists the miners that currently participate in F3 via this node.",
				Action: func(cctx *cli.Context) error {
					api, closer, err := GetFullNodeAPIV1(cctx)
					if err != nil {
						return err
					}
					defer closer()

					miners, err := api.F3ListParticipants(cctx.Context)
					if err != nil {
						return fmt.Errorf("listing participants: %w", err)
					}
					if len(miners) == 0 {
						_, err = fmt.Fprintln(cctx.App.Writer, "No miners.")
						return err
					}
					const (
						idColumn   = "ID"
						fromColumn = "From"
						ToColumn   = "To"
					)
					tw := tablewriter.New(
						tablewriter.Col(idColumn),
						tablewriter.Col(fromColumn),
						tablewriter.Col(ToColumn),
					)
					for _, participant := range miners {
						tw.Write(map[string]interface{}{
							idColumn:   participant.MinerID,
							fromColumn: participant.FromInstance,
							ToColumn:   participant.FromInstance + participant.ValidityTerm,
						})
					}
					return tw.Flush(cctx.App.Writer)
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
						return fmt.Errorf("getting manifest: %w", err)
					}
					switch output := cctx.String(f3FlagOutput.Name); strings.ToLower(output) {
					case "text":
						return prettyPrintManifest(cctx.App.Writer, manifest)
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
						return fmt.Errorf("getting running state: %w", err)
					}
					_, _ = fmt.Fprintf(cctx.App.Writer, "Running: %t\n", running)
					if !running {
						return nil
					}

					progress, err := api.F3GetProgress(cctx.Context)
					if err != nil {
						return fmt.Errorf("getting progress: %w", err)
					}

					_, _ = fmt.Fprintln(cctx.App.Writer, "Progress:")
					_, _ = fmt.Fprintf(cctx.App.Writer, "  Instance: %d\n", progress.ID)
					_, _ = fmt.Fprintf(cctx.App.Writer, "  Round:    %d\n", progress.Round)
					_, _ = fmt.Fprintf(cctx.App.Writer, "  Phase:    %s\n", progress.Phase)

					manifest, err := api.F3GetManifest(cctx.Context)
					if err != nil {
						return fmt.Errorf("getting manifest: %w", err)
					}
					return prettyPrintManifest(cctx.App.Writer, manifest)
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

func prettyPrintManifest(out io.Writer, manifest *manifest.Manifest) error {
	if manifest == nil {
		_, err := fmt.Fprintln(out, "Manifest: None")
		return err
	}

	const manifestTemplate = `Manifest: 
  Protocol Version:     {{.ProtocolVersion}}
  Paused:               {{.Pause}}
  Initial Instance:     {{.InitialInstance}}
  Initial Power Table:  {{if .InitialPowerTable.Defined}}{{.InitialPowerTable}}{{else}}unknown{{end}}
  Bootstrap Epoch:      {{.BootstrapEpoch}}
  Network Name:         {{.NetworkName}}
  Ignore EC Power:      {{.IgnoreECPower}}
  Committee Lookback:   {{.CommitteeLookback}}
  Catch Up Alignment:   {{.CatchUpAlignment}}

  GPBFT Delta:                       {{.Gpbft.Delta}}
  GPBFT Delta BackOff Exponent:      {{.Gpbft.DeltaBackOffExponent}}
  GPBFT Max Lookahead Rounds:        {{.Gpbft.MaxLookaheadRounds}}
  GPBFT Rebroadcast Backoff Base:    {{.Gpbft.RebroadcastBackoffBase}}
  GPBFT Rebroadcast Backoff Spread:  {{.Gpbft.RebroadcastBackoffSpread}}
  GPBFT Rebroadcast Backoff Max:     {{.Gpbft.RebroadcastBackoffMax}}

  EC Period:            {{.EC.Period}}
  EC Finality:          {{.EC.Finality}}
  EC Delay Multiplier:  {{.EC.DelayMultiplier}}
  EC Head Lookback:     {{.EC.HeadLookback}}
  EC Finalize:          {{.EC.Finalize}}

  Certificate Exchange Client Timeout:    {{.CertificateExchange.ClientRequestTimeout}}
  Certificate Exchange Server Timeout:    {{.CertificateExchange.ServerRequestTimeout}}
  Certificate Exchange Min Poll Interval: {{.CertificateExchange.MinimumPollInterval}}
  Certificate Exchange Max Poll Interval: {{.CertificateExchange.MaximumPollInterval}}
`
	t, err := template.New("manifest").Parse(manifestTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse manifest template: %w", err)
	}
	return t.ExecuteTemplate(out, "manifest", manifest)
}
