package cli

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/template"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var F3Cmd = &cli.Command{
	Name:  "f3",
	Usage: "Manages Filecoin Fast Finality (F3) interactions",
	Subcommands: []*cli.Command{
		f3SbCmdListMiners,
		f3SubCmdPowerTable,
		f3SubCmdCerts,
		f3SubCmdManifest,
		f3SubCmdStatus,
	},
}
var f3SbCmdListMiners = &cli.Command{
	Name:    "list-miners",
	Aliases: []string{"lm"},
	Usage:   `Lists the miners that currently participate in F3 via this node.`,
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
}
var f3SubCmdPowerTable = &cli.Command{
	Name:    "powertable",
	Aliases: []string{"pt"},
	Usage:   "Manages interactions with F3 power tables.",
	Subcommands: []*cli.Command{
		{
			Name:      "get",
			Aliases:   []string{"g"},
			Usage:     `Get F3 power table at a specific instance ID or latest instance if none is specified.`,
			ArgsUsage: "[instance]",
			Flags: []cli.Flag{
				f3FlagPowerTableFromEC,
				&cli.BoolFlag{
					Name:  "by-tipset",
					Usage: "Gets power table by translating instance into tipset.",
				},
			},
			Before: func(cctx *cli.Context) error {
				if cctx.Args().Len() > 1 {
					return fmt.Errorf("too many arguments")
				}
				return nil
			},
			Action: func(cctx *cli.Context) error {
				api, closer, err := GetFullNodeAPIV1(cctx)
				if err != nil {
					return err
				}
				defer closer()

				progress, err := api.F3GetProgress(cctx.Context)
				if err != nil {
					return fmt.Errorf("getting progress: %w", err)
				}

				var instance uint64
				if cctx.Args().Present() {
					instance, err = strconv.ParseUint(cctx.Args().First(), 10, 64)
					if err != nil {
						return fmt.Errorf("parsing instance: %w", err)
					}
					if instance > progress.ID {
						// TODO: Technically we can return power table for instances ahead as long as
						//       instance is within lookback. Implement it.
						return fmt.Errorf("instance is ahead the current instance in progress: %d > %d", instance, progress.ID)
					}
				} else {
					instance = progress.ID
				}

				var result = struct {
					Instance   uint64
					FromEC     bool
					ByTipset   bool
					PowerTable struct {
						CID         string
						Entries     gpbft.PowerEntries
						Total       gpbft.StoragePower
						ScaledTotal int64
					}
				}{
					Instance: instance,
					FromEC:   cctx.Bool(f3FlagPowerTableFromEC.Name),
					ByTipset: cctx.Bool("by-tipset"),
				}

				var expectedPowerTableCID cid.Cid
				if !result.ByTipset {
					result.PowerTable.Entries, err = api.F3GetPowerTableByInstance(cctx.Context, instance)
					if err != nil {
						return fmt.Errorf("getting f3 power table at instance %d: %w", instance, err)
					}
				} else {
					var ltsk types.TipSetKey
					ltsk, expectedPowerTableCID, err = f3GetPowerTableTSKByInstance(cctx.Context, api, instance)
					if err != nil {
						return fmt.Errorf("getting power table tsk for instance %d: %w", instance, err)
					}

					if result.FromEC {
						result.PowerTable.Entries, err = api.F3GetECPowerTable(cctx.Context, ltsk)
					} else {
						result.PowerTable.Entries, err = api.F3GetF3PowerTable(cctx.Context, ltsk)
					}
					if err != nil {
						return fmt.Errorf("getting f3 power table at instance %d: %w", instance, err)
					}
				}

				pt := gpbft.NewPowerTable()
				if err := pt.Add(result.PowerTable.Entries...); err != nil {
					// Sanity check the entries returned by the API.
					return fmt.Errorf("retrieved power table is not valid for instance %d: %w", instance, err)
				}
				result.PowerTable.Total = pt.Total
				result.PowerTable.ScaledTotal = pt.ScaledTotal

				actualPowerTableCID, err := certs.MakePowerTableCID(result.PowerTable.Entries)
				if err != nil {
					return fmt.Errorf("gettingh power table CID at instance %d: %w", instance, err)
				}
				if !cid.Undef.Equals(expectedPowerTableCID) && !expectedPowerTableCID.Equals(actualPowerTableCID) {
					return fmt.Errorf("expected power table CID %s at instance %d, got: %s", expectedPowerTableCID, instance, actualPowerTableCID)
				}
				result.PowerTable.CID = actualPowerTableCID.String()

				output, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return fmt.Errorf("marshalling f3 power table at instance %d: %w", instance, err)
				}
				_, _ = fmt.Fprint(cctx.App.Writer, string(output))
				return nil
			},
		},
		{
			Name:      "get-proportion",
			Aliases:   []string{"gp"},
			Usage:     `Gets the total proportion of power for a list of actors at a given instance.`,
			ArgsUsage: "<actor-id> [actor-id] ...",
			Flags: []cli.Flag{
				f3FlagPowerTableFromEC,
				f3FlagInstanceID,
			},
			Before: func(cctx *cli.Context) error {
				if cctx.Args().Len() < 1 {
					return fmt.Errorf("at least one actor ID must be specified")
				}
				return nil
			},
			Action: func(cctx *cli.Context) error {
				api, closer, err := GetFullNodeAPIV1(cctx)
				if err != nil {
					return err
				}
				defer closer()

				progress, err := api.F3GetProgress(cctx.Context)
				if err != nil {
					return fmt.Errorf("getting progress: %w", err)
				}

				var instance uint64
				if cctx.IsSet(f3FlagInstanceID.Name) {
					instance = cctx.Uint64(f3FlagInstanceID.Name)
					if instance > progress.ID {
						// TODO: Technically we can return power table for instances ahead as long as
						//       instance is within lookback. Implement it.
						return fmt.Errorf("instance is ahead the current instance in progress: %d > %d", instance, progress.ID)
					}
				} else {
					instance = progress.ID
				}

				ltsk, expectedPowerTableCID, err := f3GetPowerTableTSKByInstance(cctx.Context, api, instance)
				if err != nil {
					return fmt.Errorf("getting power table tsk for instance %d: %w", instance, err)
				}

				var result = struct {
					Instance   uint64
					FromEC     bool
					PowerTable struct {
						CID         string
						ScaledTotal int64
					}
					ScaledSum  int64
					Proportion float64
					NotFound   []gpbft.ActorID
				}{
					Instance: instance,
					FromEC:   cctx.Bool(f3FlagPowerTableFromEC.Name),
				}

				var powerEntries gpbft.PowerEntries
				if result.FromEC {
					powerEntries, err = api.F3GetECPowerTable(cctx.Context, ltsk)
				} else {
					powerEntries, err = api.F3GetF3PowerTable(cctx.Context, ltsk)
				}
				if err != nil {
					return fmt.Errorf("getting f3 power table at instance %d: %w", instance, err)
				}

				actualPowerTableCID, err := certs.MakePowerTableCID(powerEntries)
				if err != nil {
					return fmt.Errorf("gettingh power table CID at instance %d: %w", instance, err)
				}
				if !cid.Undef.Equals(expectedPowerTableCID) && !expectedPowerTableCID.Equals(actualPowerTableCID) {
					return fmt.Errorf("expected power table CID %s at instance %d, got: %s", expectedPowerTableCID, instance, actualPowerTableCID)
				}
				result.PowerTable.CID = actualPowerTableCID.String()

				pt := gpbft.NewPowerTable()
				if err := pt.Add(powerEntries...); err != nil {
					return fmt.Errorf("constructing power table from entries: %w", err)
				}
				result.PowerTable.ScaledTotal = pt.ScaledTotal

				inputActorIDs := cctx.Args().Slice()
				seenIDs := map[gpbft.ActorID]struct{}{}
				for _, stringID := range inputActorIDs {
					var actorID gpbft.ActorID
					switch addr, err := address.NewFromString(stringID); {
					case err == nil:
						idAddr, err := address.IDFromAddress(addr)
						if err != nil {
							return fmt.Errorf("parsing ID from address %q: %w", stringID, err)
						}
						actorID = gpbft.ActorID(idAddr)
					case errors.Is(err, address.ErrUnknownNetwork),
						errors.Is(err, address.ErrUnknownProtocol):
						// Try parsing as uint64 straight up.
						id, err := strconv.ParseUint(stringID, 10, 64)
						if err != nil {
							return fmt.Errorf("parsing as uint64 %q: %w", stringID, err)
						}
						actorID = gpbft.ActorID(id)
					default:
						return fmt.Errorf("parsing address %q: %w", stringID, err)
					}
					// Prune duplicate IDs.
					if _, ok := seenIDs[actorID]; ok {
						continue
					}
					seenIDs[actorID] = struct{}{}
					scaled, key := pt.Get(actorID)
					if key == nil {
						result.NotFound = append(result.NotFound, actorID)
						continue
					}
					result.ScaledSum += scaled
				}
				result.Proportion = float64(result.ScaledSum) / float64(result.PowerTable.ScaledTotal)
				output, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return fmt.Errorf("marshalling f3 power table at instance %d: %w", instance, err)
				}
				_, _ = fmt.Fprint(cctx.App.Writer, string(output))
				return nil
			},
		},
	},
}
var f3SubCmdCerts = &cli.Command{
	Name:    "certs",
	Aliases: []string{"c"},
	Usage:   "Manages interactions with F3 finality certificates.",
	Subcommands: []*cli.Command{
		{
			Name: "get",
			Usage: "Gets an F3 finality certificate to a given instance ID, " +
				"or the latest certificate if no instance is specified.",
			ArgsUsage: "[instance]",
			Flags: []cli.Flag{
				f3FlagOutput,
			},
			Before: func(cctx *cli.Context) error {
				if count := cctx.NArg(); count > 1 {
					return fmt.Errorf("too many arguments: expected at most 1 but got %d", count)
				}
				return nil
			},
			Action: func(cctx *cli.Context) error {
				api, closer, err := GetFullNodeAPIV1(cctx)
				if err != nil {
					return err
				}
				defer closer()

				// Get the certificate, either for the given instance or the latest if no
				// instance is specified.
				var cert *certs.FinalityCertificate
				if cctx.Args().Present() {
					var instance uint64
					instance, err = strconv.ParseUint(cctx.Args().First(), 10, 64)
					if err != nil {
						return fmt.Errorf("parsing instance: %w", err)
					}
					cert, err = api.F3GetCertificate(cctx.Context, instance)
				} else {
					cert, err = api.F3GetLatestCertificate(cctx.Context)
				}
				if err != nil {
					return fmt.Errorf("getting finality certificate: %w", err)
				}
				if cert == nil {
					_, _ = fmt.Fprintln(cctx.App.ErrWriter, "No certificate.")
					return nil
				}

				return outputFinalityCertificate(cctx, cert)
			},
		},
		{
			Name: "list",
			Usage: `Lists a range of F3 finality certificates.

By default the certificates are listed in newest to oldest order,
i.e. descending instance IDs. The order may be reversed using the
'--reverse' flag.

A range may optionally be specified as the first argument to indicate
inclusive range of 'from' and 'to' instances in following notation:
'<from>..<to>'. Either <from> or <to> may be omitted, but not both.
An omitted <from> value is always interpreted as 0, and an omitted
<to> value indicates the latest instance. If both are specified, <from>
must never exceed <to>.

If no range is specified, the latest 10 certificates are listed, i.e.
the range of '0..' with limit of 10. Otherwise, all certificates in
the specified range are listed unless limit is explicitly specified.

Examples:
  * All certificates from newest to oldest:
      $ lotus f3 certs list 0..

  * Three newest certificates:
      $ lotus f3 certs list --limit 3 0..

  * Three oldest certificates:
      $ lotus f3 certs list --limit 3 --reverse 0..

  * Up to three certificates starting from instance 1413 to the oldest:
      $ lotus f3 certs list --limit 3 ..1413

  * Up to 3 certificates starting from instance 1413 to the newest:
      $ lotus f3 certs list --limit 3 --reverse 1413..

  * All certificates from instance 3 to 1413 in order of newest to oldest:
      $ lotus f3 certs list 3..1413
`,
			ArgsUsage: "[range]",
			Flags: []cli.Flag{
				f3FlagOutput,
				f3FlagInstanceLimit,
				f3FlagReverseOrder,
			},
			Before: func(cctx *cli.Context) error {
				if count := cctx.NArg(); count > 1 {
					return fmt.Errorf("too many arguments: expected at most 1 but got %d", count)
				}
				return nil
			},
			Action: func(cctx *cli.Context) error {
				api, closer, err := GetFullNodeAPIV1(cctx)
				if err != nil {
					return err
				}
				defer closer()

				limit := cctx.Int(f3FlagInstanceLimit.Name)
				reverse := cctx.Bool(f3FlagReverseOrder.Name)
				fromTo := cctx.Args().First()
				if fromTo == "" {
					fromTo = "0.."
					if !cctx.IsSet(f3FlagInstanceLimit.Name) {
						// Default to limit of 10 if no explicit range and limit is given.
						limit = 10
					}
				}
				r, err := newRanger(fromTo, limit, reverse, func() (uint64, error) {
					latest, err := api.F3GetLatestCertificate(cctx.Context)
					if err != nil {
						return 0, fmt.Errorf("getting latest finality certificate: %w", err)
					}
					if latest == nil {
						return 0, errors.New("no latest finality certificate")
					}
					return latest.GPBFTInstance, nil
				})
				if err != nil {
					return err
				}

				var cert *certs.FinalityCertificate
				for cctx.Context.Err() == nil {
					next, proceed := r.next()
					if !proceed {
						return nil
					}
					cert, err = api.F3GetCertificate(cctx.Context, next)
					if err != nil {
						return fmt.Errorf("getting finality certificate for instance %d: %w", next, err)
					}
					if cert == nil {
						// This is unexpected, because the range of iteration was determined earlier and
						// certstore should to have all the certs. Error out.
						return fmt.Errorf("nil finality certificate for instance %d", next)
					}
					if err := outputFinalityCertificate(cctx, cert); err != nil {
						return err
					}
					_, _ = fmt.Fprintln(cctx.App.Writer)
				}
				return nil
			},
		},
	},
}
var f3SubCmdManifest = &cli.Command{
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
}
var f3SubCmdStatus = &cli.Command{
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
}

// TODO: we should standardise format as a top level flag. For now, here is an f3
//
//	specific one.
//	See: https://github.com/filecoin-project/lotus/issues/12616
var f3FlagOutput = &cli.StringFlag{
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
var f3FlagInstanceLimit = &cli.IntFlag{
	Name:        "limit",
	Usage:       "The maximum number of instances. A value less than 0 indicates no limit.",
	DefaultText: "10 when no range is specified. Otherwise, unlimited.",
	Value:       -1,
}
var f3FlagReverseOrder = &cli.BoolFlag{
	Name:  "reverse",
	Usage: "Reverses the default order of output. ",
}
var f3FlagPowerTableFromEC = &cli.BoolFlag{
	Name:  "ec",
	Usage: "Whether to get the power table from EC.",
}
var f3FlagInstanceID = &cli.Uint64Flag{
	Name:        "instance",
	Aliases:     []string{"i"},
	Usage:       "The F3 instance ID.",
	DefaultText: "Latest Instance",
}

//go:embed templates/f3_*.go.tmpl
var f3TemplatesFS embed.FS
var f3Templates = template.Must(
	template.New("").
		Funcs(template.FuncMap{
			"ptDiffToString":            f3PowerTableDiffsToString,
			"tipSetKeyToLotusTipSetKey": types.TipSetKeyFromBytes,
			"add":                       func(a, b int) int { return a + b },
			"sub":                       func(a, b int) int { return a - b },
		}).
		ParseFS(f3TemplatesFS, "templates/f3_*.go.tmpl"),
)

func f3GetPowerTableTSKByInstance(ctx context.Context, api v1api.FullNode, instance uint64) (types.TipSetKey, cid.Cid, error) {
	mfst, err := api.F3GetManifest(ctx)
	if err != nil {
		return types.EmptyTSK, cid.Undef, fmt.Errorf("getting manifest: %w", err)
	}

	if instance < mfst.InitialInstance+mfst.CommitteeLookback {
		ts, err := api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(mfst.BootstrapEpoch-mfst.EC.Finality), types.EmptyTSK)
		if err != nil {
			return types.EmptyTSK, cid.Undef, fmt.Errorf("getting bootstrap epoch tipset: %w", err)
		}
		return ts.Key(), mfst.InitialPowerTable, nil
	}

	previous, err := api.F3GetCertificate(ctx, instance-1)
	if err != nil {
		return types.EmptyTSK, cid.Undef, fmt.Errorf("getting certificate for previous instance: %w", err)
	}
	lookback, err := api.F3GetCertificate(ctx, instance-mfst.CommitteeLookback)
	if err != nil {
		return types.EmptyTSK, cid.Undef, fmt.Errorf("getting certificate for lookback instance: %w", err)
	}
	ltsk, err := types.TipSetKeyFromBytes(lookback.ECChain.Head().Key)
	if err != nil {
		return types.EmptyTSK, cid.Undef, fmt.Errorf("getting lotus tipset key from head of lookback certificate: %w", err)
	}
	return ltsk, previous.SupplementalData.PowerTable, nil
}

func outputFinalityCertificate(cctx *cli.Context, cert *certs.FinalityCertificate) error {

	switch output := cctx.String(f3FlagOutput.Name); strings.ToLower(output) {
	case "text":
		return prettyPrintFinalityCertificate(cctx.App.Writer, cert)
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

	return f3Templates.ExecuteTemplate(out, "f3_manifest.go.tmpl", manifest)
}

func prettyPrintFinalityCertificate(out io.Writer, cert *certs.FinalityCertificate) error {
	if cert == nil {
		_, _ = fmt.Fprintln(out, "Certificate: None")
		return nil
	}
	return f3Templates.ExecuteTemplate(out, "f3_finality_cert.go.tmpl", cert)
}

func f3PowerTableDiffsToString(diff certs.PowerTableDiff) (string, error) {
	if len(diff) == 0 {
		return "None", nil
	}
	totalDiff := gpbft.NewStoragePower(0).Int
	for _, delta := range diff {
		if !delta.IsZero() {
			totalDiff = totalDiff.Add(totalDiff, delta.PowerDelta.Int)
		}
	}
	if totalDiff.Cmp(gpbft.NewStoragePower(0).Int) == 0 {
		return "None", nil
	}
	return fmt.Sprintf("Total of %s storage power across %d miner(s).", totalDiff, len(diff)), nil
}

type ranger struct {
	from, to uint64
	limit    int
	ascend   bool
}

func newRanger(fromTo string, limit int, ascend bool, latest func() (uint64, error)) (*ranger, error) {
	parts := strings.Split(strings.TrimSpace(fromTo), "..")
	if len(parts) != 2 || (parts[0] == "" && parts[1] == "") {
		return nil, fmt.Errorf("invalid range format: expected '<from>..<to>', got: %s", fromTo)
	}

	r := ranger{
		limit:  limit,
		ascend: ascend,
	}
	var err error
	if parts[0] != "" {
		r.from, err = strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("determining 'from' instance: %v", err)
		}
	}
	if parts[1] != "" {
		r.to, err = strconv.ParseUint(parts[1], 10, 64)
	} else {
		r.to, err = latest()
	}
	if err != nil {
		return nil, fmt.Errorf("determining 'to' instance: %v", err)
	}
	if r.from > r.to {
		return nil, fmt.Errorf("invalid range: 'from' cannot exceed 'to':  %d > %d", r.from, r.to)
	}
	if !ascend {
		r.from, r.to = r.to, r.from
	}
	return &r, nil
}

func (r *ranger) next() (uint64, bool) {
	if r.limit == 0 {
		return 0, false
	}

	next := r.from
	if r.from == r.to {
		r.limit = 0
		return next, true
	}
	if r.ascend {
		r.from++
	} else if r.from > 0 {
		r.from--
	}
	r.limit--
	return next, true
}
