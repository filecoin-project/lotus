package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	miner6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/types"
)

var mathCmd = &cli.Command{
	Name:  "math",
	Usage: "utility commands around doing math on a list of numbers",
	Subcommands: []*cli.Command{
		mathSumCmd,
		mathPreCommitAggFeesCmd,
		mathProveCommitAggFeesCmd,
	},
}

func readLargeNumbers(i io.Reader) ([]types.BigInt, error) {
	list := []types.BigInt{}
	reader := bufio.NewReader(i)

	exit := false
	for {
		if exit {
			break
		}

		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			break
		}
		if err == io.EOF {
			exit = true
		}

		line = strings.Trim(line, "\n")

		if len(line) == 0 {
			continue
		}

		value, err := types.BigFromString(line)
		if err != nil {
			return []types.BigInt{}, fmt.Errorf("failed to parse line: %s", line)
		}

		list = append(list, value)
	}

	return list, nil
}

var mathSumCmd = &cli.Command{
	Name:  "sum",
	Usage: "Sum numbers",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "avg",
			Value: false,
			Usage: "Print the average instead of the sum",
		},
		&cli.StringFlag{
			Name:  "format",
			Value: "raw",
			Usage: "format the number in a more readable way [fil,bytes2,bytes10]",
		},
	},
	Action: func(cctx *cli.Context) error {
		list, err := readLargeNumbers(os.Stdin)
		if err != nil {
			return err
		}

		val := types.NewInt(0)
		for _, value := range list {
			val = types.BigAdd(val, value)
		}

		if cctx.Bool("avg") {
			val = types.BigDiv(val, types.NewInt(uint64(len(list))))
		}

		switch cctx.String("format") {
		case "byte2":
			fmt.Printf("%s\n", types.SizeStr(val))
		case "byte10":
			fmt.Printf("%s\n", types.DeciStr(val))
		case "fil":
			fmt.Printf("%s\n", types.FIL(val))
		case "raw":
			fmt.Printf("%s\n", val)
		default:
			return fmt.Errorf("unknown format")
		}

		return nil
	},
}

var mathProveCommitAggFeesCmd = &cli.Command{
	Name: "agg-fees-commit",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:     "size",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "base-fee",
			Usage:    "baseFee aFIL",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "base-fee",
			Usage:    "baseFee aFIL",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		as := cctx.Int("size")

		bf, err := types.BigFromString(cctx.String("base-fee"))
		if err != nil {
			return xerrors.Errorf("parsing basefee: %w", err)
		}

		fmt.Println(types.FIL(miner6.AggregateProveCommitNetworkFee(as, bf)))

		return nil
	},
}

var mathPreCommitAggFeesCmd = &cli.Command{
	Name: "agg-fees-precommit",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:     "size",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "base-fee",
			Usage:    "baseFee aFIL",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "base-fee",
			Usage:    "baseFee aFIL",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		as := cctx.Int("size")

		bf, err := types.BigFromString(cctx.String("base-fee"))
		if err != nil {
			return xerrors.Errorf("parsing basefee: %w", err)
		}

		fmt.Println(types.FIL(miner6.AggregatePreCommitNetworkFee(as, bf)))

		return nil
	},
}
