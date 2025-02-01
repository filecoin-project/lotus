package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/node/repo"
)

var f3Cmd = &cli.Command{
	Name:        "f3",
	Description: "f3 related commands",
	Subcommands: []*cli.Command{
		f3ClearStateCmd,
		f3GenExplicitPower,
		f3CheckActivation,
	},
}

func loadF3IDList(path string) ([]gpbft.ActorID, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close() //nolint:errcheck

	var ids []gpbft.ActorID
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		id, err := strconv.ParseUint(line, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ID: %w", err)
		}

		ids = append(ids, gpbft.ActorID(id))
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return ids, nil
}

var f3CheckActivation = &cli.Command{
	Name: "check-activation",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "contract",
		},
	},
	Action: func(cctx *cli.Context) error {
		address, err := ethtypes.ParseEthAddress(cctx.String("contract"))
		if err != nil {
			return fmt.Errorf("trying to parse contract address: %s: %w", cctx.String("contract"), err)
		}

		ethCall := ethtypes.EthCall{
			To:   &address,
			Data: must.One(ethtypes.DecodeHexString("0x2587660d")), // method ID of activationInformation()
		}
		fMessage, err := ethCall.ToFilecoinMessage()
		if err != nil {
			return fmt.Errorf("converting to filecoin message: %w", err)
		}

		ctx := cliutil.ReqContext(cctx)
		api, closer, err := cliutil.GetFullNodeAPIV1(cctx)
		if err != nil {
			return fmt.Errorf("getting api: %w", err)
		}
		defer closer()

		msgRes, err := api.StateCall(ctx, fMessage, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("state call error: %w", err)
		}
		if msgRes.MsgRct.ExitCode != 0 {
			return fmt.Errorf("message returned exit code: %v", msgRes.MsgRct.ExitCode)
		}

		var ethReturn abi.CborBytes
		err = ethReturn.UnmarshalCBOR(bytes.NewReader(msgRes.MsgRct.Return))
		if err != nil {
			return fmt.Errorf("could not decode return value: %w", err)
		}
		slot, retBytes := []byte{}, []byte(ethReturn)
		// 3*32 because there should be 3 slots minimum
		if len(retBytes) < 3*32 {
			return fmt.Errorf("no activation infromation")
		}

		// split off first slot
		slot, retBytes = retBytes[:32], retBytes[32:]
		// it is uint64 so we want the last 8 bytes
		slot = slot[24:32]
		activationEpoch := binary.BigEndian.Uint64(slot)
		_ = activationEpoch

		slot, retBytes = retBytes[:32], retBytes[32:]
		for i := 0; i < 31; i++ {
			if slot[i] != 0 {
				return fmt.Errorf("wrong value for offest (padding): slot[%d] = 0x%x != 0x00", i, slot[i])
			}
		}
		if slot[31] != 0x40 {
			return fmt.Errorf("wrong value for offest : slot[31] = 0x%x != 0x40", slot[31])
		}
		slot, retBytes = retBytes[:32], retBytes[32:]
		slot = slot[24:32]
		pLen := binary.BigEndian.Uint64(slot)
		if pLen > 4<<10 {
			return fmt.Errorf("too long declared payload: %d > %d", pLen, 4<<10)
		}
		payloadLength := int(pLen)

		if payloadLength > len(retBytes) {
			return fmt.Errorf("not enough remaining bytes: %d > %d", payloadLength, retBytes)
		}

		if activationEpoch == math.MaxUint64 || payloadLength == 0 {
			fmt.Printf("no active activation")
		} else {
			compressedManifest := retBytes[:payloadLength]
			reader := io.LimitReader(flate.NewReader(bytes.NewReader(compressedManifest)), 1<<20)
			var m manifest.Manifest
			err = json.NewDecoder(reader).Decode(&m)
			if err != nil {
				return fmt.Errorf("got error while decoding manifest: %w", err)
			}

			if m.BootstrapEpoch < 0 || uint64(m.BootstrapEpoch) != activationEpoch {
				return fmt.Errorf("bootstrap epoch does not match: %d != %d", m.BootstrapEpoch, activationEpoch)
			}
			fmt.Printf("%+v\n", m)
		}
		return nil
	},
}

var f3GenExplicitPower = &cli.Command{
	Name:        "gen-explicit-power",
	Description: "generates an explicit power table",

	Flags: []cli.Flag{
		&cli.PathFlag{
			Name:  "good-list",
			Usage: "new line delimited file with known good IDs to be included",
		},
		&cli.PathFlag{
			Name:  "bad-list",
			Usage: "new line delimited file with known bad IDs to be excluded",
		},
		&cli.IntFlag{
			Name:  "n",
			Usage: "generate n entries, exclusive with ratio",
		},
		&cli.Float64Flag{
			Name:  "ratio",
			Usage: "generate given ratio of full power table, exclusive with N",
		},
		&cli.Int64Flag{
			Name:  "seed",
			Usage: "seed for randomization, -1 will use current nano time",
			Value: -1,
		},
		&cli.Uint64Flag{
			Name:  "iteration",
			Usage: "the iteration of randomization, random entries will be exclusive across iterations",
			Value: 0,
		},
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to call method on (pass comma separated array of cids) or @epoch",
		},
	},

	Action: func(cctx *cli.Context) error {
		ctx := cliutil.ReqContext(cctx)
		api, closer, err := cliutil.GetFullNodeAPIV1(cctx)
		if err != nil {
			return fmt.Errorf("getting api: %w", err)
		}
		defer closer()

		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return fmt.Errorf("getting chain head: %w", err)
		}
		if cctx.IsSet("n") && cctx.IsSet("ratio") {
			return fmt.Errorf("n and ratio options are exclusive")
		}

		allPowerEntries, err := api.F3GetECPowerTable(ctx, ts.Key())
		if err != nil {
			return fmt.Errorf("getting power entries: %w", err)
		}

		powerMap := map[gpbft.ActorID]gpbft.PowerEntry{}
		for _, pe := range allPowerEntries {
			powerMap[pe.ID] = pe
		}
		var goodList []gpbft.ActorID
		if goodPath := cctx.Path("good-list"); goodPath != "" {
			goodList, err = loadF3IDList(goodPath)
			if err != nil {
				return fmt.Errorf("loading good list: %w", err)
			}
		}

		var badList []gpbft.ActorID
		if badPath := cctx.Path("bad-list"); badPath != "" {
			badList, err = loadF3IDList(badPath)
			if err != nil {
				return fmt.Errorf("loading bad list: %w", err)
			}
		}
		total := len(powerMap)
		for _, id := range badList {
			delete(powerMap, id)
		}

		var result gpbft.PowerEntries
		add := func(id gpbft.ActorID) {
			result = append(result, powerMap[id])
			delete(powerMap, id)
		}

		for _, id := range goodList {
			if _, ok := powerMap[id]; ok {
				add(id)
			}
		}

		seed := cctx.Int64("seed")
		if seed == -1 {
			seed = time.Now().UnixNano()
		}
		rng := rand.New(rand.NewSource(seed))

		endSize := cctx.Int("n")
		if cctx.IsSet("ratio") {
			endSize = int(float64(total) * cctx.Float64("ratio"))
		}
		if toAdd := endSize - len(result); toAdd > 0 {
			var powerList gpbft.PowerEntries
			for _, pe := range powerMap {
				powerList = append(powerList, pe)
			}
			sort.Sort(powerList)
			rng.Shuffle(len(powerList), powerList.Swap)

			iteration := cctx.Int("iteration")
			startIdx := min(toAdd*iteration, len(powerList))
			endIdx := min(toAdd*(iteration+1), len(powerList))
			result = append(result, powerList[startIdx:endIdx]...)
		}

		if len(result) > endSize {
			result = result[:endSize]
		}
		sort.Sort(result)
		res, err := json.MarshalIndent(result, "  ", "  ")
		if err != nil {
			return fmt.Errorf("marshalling to json: %w", err)
		}
		_, err = cctx.App.Writer.Write(res)
		if err != nil {
			return fmt.Errorf("writing result: %w", err)
		}

		return nil
	},
}

var f3ClearStateCmd = &cli.Command{
	Name:        "clear-state",
	Description: "remove all f3 state",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "really-do-it",
		},
	},
	Action: func(cctx *cli.Context) error {
		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		ds, err := lr.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		dryRun := !cctx.Bool("really-do-it")

		q, err := ds.Query(context.Background(), dsq.Query{
			Prefix:   "/f3",
			KeysOnly: true,
		})
		if err != nil {
			return xerrors.Errorf("datastore query: %w", err)
		}
		defer q.Close() //nolint:errcheck

		batch, err := ds.Batch(cctx.Context)
		if err != nil {
			return xerrors.Errorf("failed to create a datastore batch: %w", err)
		}

		for r, ok := q.NextSync(); ok; r, ok = q.NextSync() {
			if r.Error != nil {
				return xerrors.Errorf("failed to read datastore: %w", err)
			}
			_, _ = fmt.Fprintf(cctx.App.Writer, "deleting: %q\n", r.Key)
			if !dryRun {
				if err := batch.Delete(cctx.Context, datastore.NewKey(r.Key)); err != nil {
					return xerrors.Errorf("failed to delete %q: %w", r.Key, err)
				}
			}
		}
		if !dryRun {
			if err := batch.Commit(cctx.Context); err != nil {
				return xerrors.Errorf("failed to flush the batch: %w", err)
			}
		}

		if err := ds.Close(); err != nil {
			return xerrors.Errorf("error when closing datastore: %w", err)
		}

		if dryRun {
			_, _ = fmt.Fprintln(cctx.App.Writer, "NOTE: dry run complete, re-run with --really-do-it to actually delete F3 state")
		}

		return nil
	},
}
