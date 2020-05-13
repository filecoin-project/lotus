package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"

	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-bitfield"
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
)

var bitFieldCmd = &cli.Command{
	Name:        "bitfield",
	Description: "analyze bitfields",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "enc",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
	},
	Subcommands: []*cli.Command{
		bitFieldRunsCmd,
		bitFieldStatCmd,
		bitFieldDecodeCmd,
	},
}

var bitFieldRunsCmd = &cli.Command{
	Name:        "runs",
	Description: "print bit runs in a bitfield",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "enc",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
	},
	Action: func(cctx *cli.Context) error {
		var val string
		if cctx.Args().Present() {
			val = cctx.Args().Get(0)
		} else {
			b, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			val = string(b)
		}

		var dec []byte
		switch cctx.String("enc") {
		case "base64":
			d, err := base64.StdEncoding.DecodeString(val)
			if err != nil {
				return fmt.Errorf("decoding base64 value: %w", err)
			}
			dec = d
		case "hex":
			d, err := hex.DecodeString(val)
			if err != nil {
				return fmt.Errorf("decoding hex value: %w", err)
			}
			dec = d
		default:
			return fmt.Errorf("unrecognized encoding: %s", cctx.String("enc"))
		}

		rle, err := rlepluslazy.FromBuf(dec)
		if err != nil {
			return xerrors.Errorf("opening rle: %w", err)
		}

		rit, err := rle.RunIterator()
		if err != nil {
			return xerrors.Errorf("getting run iterator: %w", err)
		}
		var idx uint64
		for rit.HasNext() {
			r, err := rit.NextRun()
			if err != nil {
				return xerrors.Errorf("next run: %w", err)
			}
			if !r.Valid() {
				fmt.Print("!INVALID ")
			}
			s := "TRUE "
			if !r.Val {
				s = "FALSE"
			}

			fmt.Printf("@%d %s * %d\n", idx, s, r.Len)

			idx += r.Len
		}

		return nil
	},
}

var bitFieldStatCmd = &cli.Command{
	Name:        "stat",
	Description: "print bitfield stats",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "enc",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
	},
	Action: func(cctx *cli.Context) error {
		var val string
		if cctx.Args().Present() {
			val = cctx.Args().Get(0)
		} else {
			b, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			val = string(b)
		}

		var dec []byte
		switch cctx.String("enc") {
		case "base64":
			d, err := base64.StdEncoding.DecodeString(val)
			if err != nil {
				return fmt.Errorf("decoding base64 value: %w", err)
			}
			dec = d
		case "hex":
			d, err := hex.DecodeString(val)
			if err != nil {
				return fmt.Errorf("decoding hex value: %w", err)
			}
			dec = d
		default:
			return fmt.Errorf("unrecognized encoding: %s", cctx.String("enc"))
		}

		rle, err := rlepluslazy.FromBuf(dec)
		if err != nil {
			return xerrors.Errorf("opening rle: %w", err)
		}

		rit, err := rle.RunIterator()
		if err != nil {
			return xerrors.Errorf("getting run iterator: %w", err)
		}

		fmt.Printf("Raw length: %d bits (%d bytes)\n", len(dec)*8, len(dec))

		var ones, zeros, oneRuns, zeroRuns, invalid uint64

		for rit.HasNext() {
			r, err := rit.NextRun()
			if err != nil {
				return xerrors.Errorf("next run: %w", err)
			}
			if !r.Valid() {
				invalid++
			}
			if r.Val {
				ones += r.Len
				oneRuns++
			} else {
				zeros += r.Len
				zeroRuns++
			}
		}

		if _, err := rle.Count(); err != nil { // check overflows
			fmt.Println("Error: ", err)
		}

		fmt.Printf("Decoded length: %d bits\n", ones+zeros)
		fmt.Printf("\tOnes:  %d\n", ones)
		fmt.Printf("\tZeros: %d\n", zeros)
		fmt.Printf("Runs: %d\n", oneRuns+zeroRuns)
		fmt.Printf("\tOne Runs:  %d\n", oneRuns)
		fmt.Printf("\tZero Runs: %d\n", zeroRuns)
		fmt.Printf("Invalid runs: %d\n", invalid)
		return nil
	},
}

var bitFieldDecodeCmd = &cli.Command{
	Name:        "decode",
	Description: "decode bitfield and print all numbers in it",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "enc",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
	},
	Action: func(cctx *cli.Context) error {
		var val string
		if cctx.Args().Present() {
			val = cctx.Args().Get(0)
		} else {
			b, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			val = string(b)
		}

		var dec []byte
		switch cctx.String("enc") {
		case "base64":
			d, err := base64.StdEncoding.DecodeString(val)
			if err != nil {
				return fmt.Errorf("decoding base64 value: %w", err)
			}
			dec = d
		case "hex":
			d, err := hex.DecodeString(val)
			if err != nil {
				return fmt.Errorf("decoding hex value: %w", err)
			}
			dec = d
		default:
			return fmt.Errorf("unrecognized encoding: %s", cctx.String("enc"))
		}

		rle, err := bitfield.NewFromBytes(dec)
		if err != nil {
			return xerrors.Errorf("failed to parse bitfield: %w", err)
		}

		vals, err := rle.All(100000000000)
		if err != nil {
			return xerrors.Errorf("getting all items: %w", err)
		}
		fmt.Println(vals)

		return nil
	},
}
