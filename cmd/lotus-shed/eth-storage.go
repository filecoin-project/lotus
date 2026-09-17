package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	carbs "github.com/ipld/go-car/v2/blockstore"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-shed/evmstorage"
)

var ethStorageDumpCmd = &cli.Command{
	Name:      "storage-dump",
	Usage:     "Enumerate every storage slot of an EVM contract",
	ArgsUsage: "<contractAddress (0x, f410 or ID form)>",
	Description: `Walks the contract's storage KAMT directly (via ChainReadObj) and emits
one NDJSON record per slot, in ascending slot order:

   {"slot":"0x<64 hex>","value":"0x<64 hex>"}

Output order is deterministic, so dumps of the same contract at two
tipsets diff line-by-line. Structural statistics of the KAMT are
reported separately on completion (see --stats-out).

eth_getStorageAt reads one slot at a time through the FVM; this command
is the enumeration path that API cannot provide.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "tipset to read state at: @head, @<height>, or comma-separated block CIDs (default head)",
		},
		&cli.StringFlag{
			Name:  "out",
			Usage: "write slot records to this file instead of stdout",
		},
		&cli.StringFlag{
			Name:  "stats-out",
			Usage: "write the stats JSON document to this file instead of stderr",
		},
		&cli.StringFlag{
			Name:  "car",
			Usage: "also write every block backing the storage to this CARv1 file",
		},
		&cli.BoolFlag{
			Name:  "transient",
			Usage: "also dump the transient storage KAMT if present (records gain \"transient\":true)",
		},
		&cli.IntFlag{
			Name:  "verify",
			Usage: "cross-check this many sampled slots against eth_getStorageAt",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return fmt.Errorf("expected one contract address argument")
		}
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		bs := blockstore.NewAPIBlockstore(api)
		store := adt.WrapStore(ctx, cbor.NewCborStore(bs))
		contract, err := evmstorage.ResolveContract(ctx, api, store, cctx.Args().First(), ts)
		if err != nil {
			return err
		}

		out := io.Writer(os.Stdout)
		if path := cctx.String("out"); path != "" {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close() //nolint:errcheck
			out = f
		}
		w := bufio.NewWriter(out)

		var sink func(block.Block) error
		if path := cctx.String("car"); path != "" {
			roots := []cid.Cid{contract.StorageRoot}
			if cctx.Bool("transient") && contract.Transient != nil {
				roots = append(roots, contract.Transient.Root)
			}
			carStore, err := carbs.OpenReadWrite(path, roots, carbs.WriteAsCarV1(true))
			if err != nil {
				return fmt.Errorf("opening CAR output %s: %w", path, err)
			}
			defer func() {
				if err := carStore.Finalize(); err != nil {
					fmt.Fprintf(os.Stderr, "finalizing CAR: %s\n", err)
				}
			}()
			sink = func(blk block.Block) error { return carStore.Put(ctx, blk) }
		}

		sampler := newSlotSampler(cctx.Int("verify"))
		report := dumpReport{
			Contract: contract,
			TipSet:   ts.Key(),
			Height:   ts.Height(),
		}

		report.Storage, err = evmstorage.WalkStorage(ctx, bs, contract.StorageRoot,
			func(slot, value [32]byte) error {
				sampler.offer(slot, value)
				_, err := fmt.Fprintf(w, "{\"slot\":\"0x%x\",\"value\":\"0x%x\"}\n", slot, value)
				return err
			}, sink)
		if err != nil {
			return err
		}

		if cctx.Bool("transient") && contract.Transient != nil {
			report.Transient, err = evmstorage.WalkStorage(ctx, bs, contract.Transient.Root,
				func(slot, value [32]byte) error {
					_, err := fmt.Fprintf(w, "{\"slot\":\"0x%x\",\"value\":\"0x%x\",\"transient\":true}\n", slot, value)
					return err
				}, sink)
			if err != nil {
				return err
			}
		}
		if err := w.Flush(); err != nil {
			return err
		}

		if len(sampler.slots) > 0 {
			report.VerifiedSlots, err = verifySlots(ctx, api, contract.EthAddress, ts, sampler)
			if err != nil {
				return err
			}
		}

		statsOut := io.Writer(os.Stderr)
		if path := cctx.String("stats-out"); path != "" {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close() //nolint:errcheck
			statsOut = f
		}
		enc := json.NewEncoder(statsOut)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	},
}

type dumpReport struct {
	Contract      *evmstorage.Contract `json:"contract"`
	TipSet        types.TipSetKey      `json:"tipset"`
	Height        abi.ChainEpoch       `json:"height"`
	Storage       *evmstorage.Stats    `json:"storage"`
	Transient     *evmstorage.Stats    `json:"transient,omitempty"`
	VerifiedSlots int                  `json:"verifiedSlots,omitempty"`
}

// slotSampler reservoir-samples up to n slots from the emission stream.
type slotSampler struct {
	n     int
	seen  int
	slots [][2][32]byte
}

func newSlotSampler(n int) *slotSampler {
	return &slotSampler{n: n}
}

func (s *slotSampler) offer(slot, value [32]byte) {
	if s.n <= 0 {
		return
	}
	s.seen++
	if len(s.slots) < s.n {
		s.slots = append(s.slots, [2][32]byte{slot, value})
	} else if i := rand.Intn(s.seen); i < s.n {
		s.slots[i] = [2][32]byte{slot, value}
	}
}

// verifySlots replays sampled slots through eth_getStorageAt, proving the
// walk agrees with the FVM's own view of the state. The dump reads the
// parent state of ts, which the eth API addresses as the parent block, so
// query by the parent tipset's hash.
func verifySlots(ctx context.Context, api v1api.FullNode, ethAddr ethtypes.EthAddress, ts *types.TipSet, sampler *slotSampler) (int, error) {
	parentKeyCid, err := ts.Parents().Cid()
	if err != nil {
		return 0, err
	}
	parentHash, err := ethtypes.EthHashFromCid(parentKeyCid)
	if err != nil {
		return 0, err
	}
	blkParam := ethtypes.EthBlockNumberOrHash{BlockHash: &parentHash}
	for _, sv := range sampler.slots {
		got, err := api.EthGetStorageAt(ctx, ethAddr, sv[0][:], blkParam)
		if err != nil {
			return 0, fmt.Errorf("eth_getStorageAt(0x%x): %w", sv[0], err)
		}
		if !bytes.Equal(got, sv[1][:]) {
			return 0, fmt.Errorf("slot 0x%x: walked value 0x%x but eth_getStorageAt returned 0x%x", sv[0], sv[1], got)
		}
	}
	return len(sampler.slots), nil
}

var ethStorageDecodeCmd = &cli.Command{
	Name:      "storage-decode",
	Usage:     "Annotate a storage-dump using a solc/forge storage layout",
	ArgsUsage: "[dump.ndjson (default stdin)]",
	Description: `Labels the slots of an "eth storage-dump" output using the contract's
storage layout ("forge inspect <Contract> storageLayout --json"). Static
variables label directly; mapping entries live at irreversible
keccak-derived slots, so candidate keys are probed forward: integer ids
(0..--probe-range plus every id read from labelled slots), addresses
(from values and --keys), and strings (from labelled string slots and
--keys), repeating until nothing new labels. For a proxy, dump the proxy
address but pass the implementation's layout.

A coverage summary goes to stderr; re-run with more --keys or a larger
--probe-range to grow it.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "layout",
			Usage:    "storage layout JSON file (required)",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "keys",
			Usage: "file of extra mapping keys to probe, one per line: 0x-address, 0x-bytes32, decimal integer, or bare string",
		},
		&cli.IntFlag{
			Name:  "probe-range",
			Usage: "probe integer mapping keys 0..N",
			Value: 8192,
		},
		&cli.IntFlag{
			Name: "nested-probe-range",
			Usage: "probe integer keys 0..N at the inner levels of nested mappings; raise for contracts with " +
				"few outer ids but large dense inner id spaces",
			Value: evmstorage.DefaultNestedProbeRange,
		},
		&cli.StringSliceFlag{
			Name: "namespace",
			Usage: "ERC-7201 namespaced storage as <id>=<layout.json> with slots relative to the namespace base, " +
				"e.g. --namespace mycorp.storage.Thing=thing.layout.json; ids come from @custom:storage-location " +
				"annotations in the source. Common OpenZeppelin namespaces are built in",
		},
		&cli.StringFlag{
			Name:  "out",
			Usage: "write annotated records to this file instead of stdout",
		},
	},
	Action: func(cctx *cli.Context) error {
		layout, err := evmstorage.LoadLayout(cctx.String("layout"))
		if err != nil {
			return err
		}
		namespaces := make(map[string]*evmstorage.Layout)
		for _, spec := range cctx.StringSlice("namespace") {
			id, path, ok := strings.Cut(spec, "=")
			if !ok {
				return fmt.Errorf("--namespace wants <id>=<layout.json>, got %q", spec)
			}
			if namespaces[id], err = evmstorage.LoadLayout(path); err != nil {
				return err
			}
		}
		hints := evmstorage.Hints{}
		if path := cctx.String("keys"); path != "" {
			if hints, err = loadHints(path); err != nil {
				return err
			}
		}

		in := io.Reader(os.Stdin)
		if cctx.NArg() > 0 {
			f, err := os.Open(cctx.Args().First())
			if err != nil {
				return err
			}
			defer f.Close() //nolint:errcheck
			in = f
		}
		type record struct {
			Slot      string `json:"slot"`
			Value     string `json:"value"`
			Transient bool   `json:"transient,omitempty"`
		}
		var records []record
		slots := make(map[[32]byte][32]byte)
		dec := json.NewDecoder(bufio.NewReaderSize(in, 1<<20))
		for dec.More() {
			var r record
			if err := dec.Decode(&r); err != nil {
				return fmt.Errorf("reading dump: %w", err)
			}
			records = append(records, r)
			if r.Transient {
				continue
			}
			slot, err := parse32(r.Slot)
			if err != nil {
				return err
			}
			value, err := parse32(r.Value)
			if err != nil {
				return err
			}
			slots[slot] = value
		}

		decoder := evmstorage.NewDecoder(layout, slots)
		for id, nsLayout := range namespaces {
			decoder.ApplyNamespace(id, nsLayout)
		}
		ann := decoder.Decode(hints, cctx.Int("probe-range"), cctx.Int("nested-probe-range"))

		out := io.Writer(os.Stdout)
		if path := cctx.String("out"); path != "" {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close() //nolint:errcheck
			out = f
		}
		w := bufio.NewWriter(out)
		enc := json.NewEncoder(w)
		for _, r := range records {
			rec := map[string]interface{}{"slot": r.Slot, "value": r.Value}
			if r.Transient {
				rec["transient"] = true
			} else if slot, err := parse32(r.Slot); err == nil {
				if a := ann[slot]; a != nil {
					rec["label"] = a.Label
					if a.Type != "" {
						rec["type"] = a.Type
					}
					if a.Decoded != nil {
						rec["decoded"] = a.Decoded
					}
				}
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		if err := w.Flush(); err != nil {
			return err
		}

		cov := decoder.Coverage()
		covEnc := json.NewEncoder(os.Stderr)
		covEnc.SetIndent("", "  ")
		return covEnc.Encode(cov)
	},
}

func parse32(s string) ([32]byte, error) {
	var out [32]byte
	s = strings.TrimPrefix(s, "0x")
	if len(s) != 64 {
		return out, fmt.Errorf("expected 32-byte hex quantity, got %q", s)
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return out, err
	}
	copy(out[:], b)
	return out, nil
}

// loadHints reads mapping-key candidates, one per line: 0x-addresses,
// decimal integers, or anything else as a literal string key.
func loadHints(path string) (evmstorage.Hints, error) {
	var hints evmstorage.Hints
	f, err := os.Open(path)
	if err != nil {
		return hints, err
	}
	defer f.Close() //nolint:errcheck
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "0x") && len(line) == 42 {
			b, err := hex.DecodeString(line[2:])
			if err != nil {
				return hints, fmt.Errorf("bad address hint %q: %w", line, err)
			}
			hints.Addrs = append(hints.Addrs, [20]byte(b))
			continue
		}
		if strings.HasPrefix(line, "0x") && len(line) == 66 {
			b, err := hex.DecodeString(line[2:])
			if err != nil {
				return hints, fmt.Errorf("bad bytes32 hint %q: %w", line, err)
			}
			hints.B32s = append(hints.B32s, [32]byte(b))
			continue
		}
		if n, err := strconv.ParseUint(line, 10, 64); err == nil {
			hints.Ints = append(hints.Ints, n)
			continue
		}
		hints.Strings = append(hints.Strings, line)
	}
	return hints, scanner.Err()
}
