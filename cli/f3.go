package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var (
	F3Cmd = &cli.Command{
		Name:  "f3",
		Usage: "Manages Filecoin Fast Finality (F3) interactions",
		Subcommands: []*cli.Command{
			{
				Name:    "list-miners",
				Aliases: []string{"lm"},
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
						miner = "Miner"
						from  = "From"
						to    = "To"
					)
					tw := tablewriter.New(
						tablewriter.Col(miner),
						tablewriter.Col(from),
						tablewriter.Col(to),
					)
					for _, participant := range miners {
						addr, err := address.NewIDAddress(participant.MinerID)
						if err != nil {
							return fmt.Errorf("converting miner ID to address: %w", err)
						}

						tw.Write(map[string]interface{}{
							miner: addr,
							from:  participant.FromInstance,
							to:    participant.FromInstance + participant.ValidityTerm,
						})
					}
					return tw.Flush(cctx.App.Writer)
				},
			},
			{
				Name:    "certs",
				Aliases: []string{"fc", "c", "cert"},
				Usage:   "Manages interactions with F3 finality certificates.",
				Subcommands: []*cli.Command{
					{
						Name:  "get",
						Usage: "Gets an F3 finality certificate.",
						Flags: []cli.Flag{
							f3FlagOutput,
							f3FlagInstanceID,
						},
						After: func(cctx *cli.Context) error {
							api, closer, err := GetFullNodeAPIV1(cctx)
							if err != nil {
								return err
							}
							defer closer()

							// Get the certificate, either for the given instance or the latest if no
							// instance is specified.
							var cert *certs.FinalityCertificate
							if cctx.IsSet(f3FlagInstanceID.Name) {
								cert, err = api.F3GetCertificate(cctx.Context, cctx.Uint64(f3FlagInstanceID.Name))
							} else {
								cert, err = api.F3GetLatestCertificate(cctx.Context)
							}
							if err != nil {
								return fmt.Errorf("getting finality certificate: %w", err)
							}
							if cert == nil {
								_, _ = fmt.Fprintln(cctx.App.Writer, "No certificate.")
								return nil
							}

							return outputFinalityCertificate(cctx, api, cert)
						},
					},
					{
						Name:  "list",
						Usage: "Lists a set of F3 finality certificates.",
						Flags: []cli.Flag{
							f3FlagOutput,
							f3FlagInstanceFrom,
							f3FlagInstanceLimit,
						},
						After: func(cctx *cli.Context) error {
							api, closer, err := GetFullNodeAPIV1(cctx)
							if err != nil {
								return err
							}
							defer closer()

							var count int
							limit := cctx.Int(f3FlagInstanceLimit.Name)
							var cert *certs.FinalityCertificate
							for cctx.Context.Err() == nil && count < limit {
								if cert == nil {
									if cctx.IsSet(f3FlagInstanceFrom.Name) {
										cert, err = api.F3GetCertificate(cctx.Context, cctx.Uint64(f3FlagInstanceFrom.Name))
									} else {
										cert, err = api.F3GetLatestCertificate(cctx.Context)
									}
									if err != nil {
										return fmt.Errorf("getting finality certificate: %w", err)
									}
								} else if cert.GPBFTInstance > 0 {
									cert, err = api.F3GetCertificate(cctx.Context, cert.GPBFTInstance-1)
									if err != nil {
										return fmt.Errorf("getting finality certificate for instance %d: %w", cert.GPBFTInstance-1, err)
									}
								} else {
									return nil
								}

								if cert == nil {
									_, _ = fmt.Fprintln(cctx.App.Writer, "No certificate.")
									return nil
								}
								if err := outputFinalityCertificate(cctx, api, cert); err != nil {
									return err
								}
								count++
							}
							return nil
						},
					},
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
	f3FlagInstanceID = &cli.Uint64Flag{
		Name:        "instance",
		Aliases:     []string{"i", "id"},
		Usage:       "The instance ID for which to get the finality certificate.",
		DefaultText: "Latest instance.",
	}
	f3FlagInstanceFrom = &cli.Uint64Flag{
		Name:        "fromInstance",
		Usage:       "The start instance ID.",
		DefaultText: "Latest instance.",
	}
	f3FlagInstanceLimit = &cli.IntFlag{
		Name:  "limit",
		Usage: "The maximum number of instances. A value less than 0 indicates no limit.",
		Value: 10,
	}
)

func outputFinalityCertificate(cctx *cli.Context, api v1api.FullNode, cert *certs.FinalityCertificate) error {
	// Get the power table used by the GPBFT instance that resulted in the cert, i.e.
	// the power table corresponding the base of the finalised chin.
	ltsk, err := types.TipSetKeyFromBytes(cert.ECChain.Base().Key)
	if err != nil {
		return fmt.Errorf("decoding latest certificate tipset key: %w", err)
	}
	pt, err := api.F3GetF3PowerTable(cctx.Context, ltsk)
	if err != nil {
		return fmt.Errorf("getting F3 power table: %w", err)
	}
	switch output := cctx.String(f3FlagOutput.Name); strings.ToLower(output) {
	case "text":
		return prettyPrintFinalityCertificate(cctx.App.Writer, cert, pt)
	case "json":
		encoder := json.NewEncoder(cctx.App.Writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(cert)
	default:
		return fmt.Errorf("unknown output format: %s", output)
	}
}

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

func prettyPrintFinalityCertificate(out io.Writer, cert *certs.FinalityCertificate, entries gpbft.PowerEntries) error {
	if cert == nil {
		_, _ = fmt.Fprintln(out, "Certificate: None")
		return nil
	}

	const certificateTemplate = `---
Instance: {{.GPBFTInstance}}
Finalized Chain:
  {{ .ECChain }}

Supplemental Data:
  Commitments:          {{printf "%X" .SupplementalData.Commitments}}
  Power Table CID:      {{.SupplementalData.PowerTable}}

Power Table Delta:
{{ptDiffToString .PowerTableDelta}}
Signers:
{{ signersToString .Signers }}
Signature (base64 encoded):
  {{ base64 .Signature }}
`

	t, err := template.New("certificate").
		Funcs(template.FuncMap{
			"ptDiffToString": func(diff certs.PowerTableDiff) (string, error) {
				var buf bytes.Buffer
				if err := prettyPrintPowerTableDiffs(&buf, diff); err != nil {
					return "", err
				}
				return buf.String(), nil
			},
			"signersToString": func(signers bitfield.BitField) (string, error) {
				var buf bytes.Buffer
				if err := prettyPrintSigners(&buf, signers, entries); err != nil {
					return "", err
				}
				return buf.String(), nil
			},
			"base64": base64.StdEncoding.EncodeToString,
		}).Parse(certificateTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse certificate template: %w", err)
	}
	return t.Execute(out, cert)
}

func prettyPrintPowerTableDiffs(out io.Writer, diff certs.PowerTableDiff) error {
	if len(diff) == 0 {
		_, _ = fmt.Fprintln(out, "[]")
		return nil
	}
	const (
		miner       = "Miner"
		deltaColumn = "Power Delta"
	)
	tw := tablewriter.New(
		tablewriter.Col(miner),
		tablewriter.Col(deltaColumn),
	)
	for _, delta := range diff {
		addr, err := address.NewIDAddress(uint64(delta.ParticipantID))
		if err != nil {
			return fmt.Errorf("getting ID address: %w", err)
		}
		tw.Write(map[string]interface{}{
			miner:       addr,
			deltaColumn: delta.PowerDelta,
		})
	}
	return tw.Flush(out)
}

func prettyPrintSigners(out io.Writer, signers bitfield.BitField, entries gpbft.PowerEntries) error {
	pt := gpbft.NewPowerTable()
	if err := pt.Add(entries...); err != nil {
		return fmt.Errorf("populating power table from entries: %w", err)
	}

	const (
		miner        = "Miner"
		powerScaled  = "Scaled Power"
		powerPercent = "Power %"
	)
	tw := tablewriter.New(
		tablewriter.Col(miner),
		tablewriter.Col(powerScaled),
		tablewriter.Col(powerPercent),
	)
	if err := signers.ForEach(func(index uint64) error {
		entry := pt.Entries[index]
		addr, err := address.NewIDAddress(uint64(entry.ID))
		if err != nil {
			return fmt.Errorf("getting ID address: %w", err)
		}
		signerPowerPercent := (float64(pt.ScaledPower[index]) / float64(pt.ScaledTotal)) * 100
		tw.Write(map[string]interface{}{
			miner:        addr,
			powerScaled:  pt.ScaledPower[index],
			powerPercent: fmt.Sprintf("%.2f%%", signerPowerPercent),
		})
		return nil
	}); err != nil {
		return err
	}
	return tw.Flush(out)
}
