package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

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
		bitFieldIntersectCmd,
		bitFieldEncodeCmd,
		bitFieldSubCmd,
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
		rle, err := decode(cctx, 0)
		if err != nil {
			return err
		}

		vals, err := rle.All(100000000000)
		if err != nil {
			return xerrors.Errorf("getting all items: %w", err)
		}
		fmt.Println(vals)

		return nil
	},
}

var bitFieldIntersectCmd = &cli.Command{
	Name:        "intersect",
	Description: "intersect 2 bitfields and print the resulting bitfield as base64",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "enc",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
	},
	Action: func(cctx *cli.Context) error {
		b, err := decode(cctx, 1)
		if err != nil {
			return err
		}

		a, err := decode(cctx, 0)
		if err != nil {
			return err
		}

		o, err := bitfield.IntersectBitField(a, b)
		if err != nil {
			return xerrors.Errorf("intersect: %w", err)
		}

		s, err := o.RunIterator()
		if err != nil {
			return err
		}

		bytes, err := rlepluslazy.EncodeRuns(s, []byte{})
		if err != nil {
			return err
		}

		fmt.Println(base64.StdEncoding.EncodeToString(bytes))

		return nil
	},
}

var bitFieldSubCmd = &cli.Command{
	Name:        "sub",
	Description: "subtract 2 bitfields and print the resulting bitfield as base64",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "enc",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
	},
	Action: func(cctx *cli.Context) error {
		b, err := decode(cctx, 1)
		if err != nil {
			return err
		}

		a, err := decode(cctx, 0)
		if err != nil {
			return err
		}

		o, err := bitfield.SubtractBitField(a, b)
		if err != nil {
			return xerrors.Errorf("intersect: %w", err)
		}

		s, err := o.RunIterator()
		if err != nil {
			return err
		}

		bytes, err := rlepluslazy.EncodeRuns(s, []byte{})
		if err != nil {
			return err
		}

		fmt.Println(base64.StdEncoding.EncodeToString(bytes))

		return nil
	},
}

var bitFieldEncodeCmd = &cli.Command{
	Name:        "encode",
	Description: "encode a series of decimal numbers into a bitfield",
	ArgsUsage:   "[infile]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "enc",
			Value: "base64",
			Usage: "specify input encoding to parse",
		},
	},
	Action: func(cctx *cli.Context) error {
		f, err := os.Open(cctx.Args().First())
		if err != nil {
			return err
		}
		defer f.Close() // nolint

		out := bitfield.New()
		for {
			var i uint64
			_, err := fmt.Fscan(f, &i)
			if err == io.EOF {
				break
			}
			out.Set(i)
		}

		s, err := out.RunIterator()
		if err != nil {
			return err
		}

		bytes, err := rlepluslazy.EncodeRuns(s, []byte{})
		if err != nil {
			return err
		}

		fmt.Println(base64.StdEncoding.EncodeToString(bytes))

		return nil
	},
}

func decode(cctx *cli.Context, a int) (bitfield.BitField, error) {
	var val string
	if cctx.Args().Present() {
		if a >= cctx.NArg() {
			return bitfield.BitField{}, xerrors.Errorf("need more than %d args", a)
		}
		val = cctx.Args().Get(a)
	} else {
		if a > 0 {
			return bitfield.BitField{}, xerrors.Errorf("need more than %d args", a)
		}
		b, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return bitfield.BitField{}, err
		}
		val = string(b)
	}

	var dec []byte
	switch cctx.String("enc") {
	case "base64":
		d, err := base64.StdEncoding.DecodeString(val)
		if err != nil {
			return bitfield.BitField{}, fmt.Errorf("decoding base64 value: %w", err)
		}
		dec = d
	case "hex":
		d, err := hex.DecodeString(val)
		if err != nil {
			return bitfield.BitField{}, fmt.Errorf("decoding hex value: %w", err)
		}
		dec = d
	default:
		return bitfield.BitField{}, fmt.Errorf("unrecognized encoding: %s", cctx.String("enc"))
	}

	return bitfield.NewFromBytes(dec)
}
