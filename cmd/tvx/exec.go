package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/conformance"

	"github.com/filecoin-project/test-vectors/schema"
)

var execFlags struct {
	file string
}

var execCmd = &cli.Command{
	Name:        "exec",
	Description: "execute one or many test vectors against Lotus; supplied as a single JSON file, or a ndjson stdin stream",
	Action:      runExecLotus,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "file",
			Usage:       "input file; if not supplied, the vector will be read from stdin",
			TakesFile:   true,
			Destination: &execFlags.file,
		},
	},
}

func runExecLotus(_ *cli.Context) error {
	if file := execFlags.file; file != "" {
		// we have a single test vector supplied as a file.
		file, err := os.Open(file)
		if err != nil {
			return fmt.Errorf("failed to open test vector: %w", err)
		}

		var (
			dec = json.NewDecoder(file)
			tv  schema.TestVector
		)

		if err = dec.Decode(&tv); err != nil {
			return fmt.Errorf("failed to decode test vector: %w", err)
		}

		return executeTestVector(tv)
	}

	for dec := json.NewDecoder(os.Stdin); ; {
		var tv schema.TestVector
		switch err := dec.Decode(&tv); err {
		case nil:
			if err = executeTestVector(tv); err != nil {
				return err
			}
		case io.EOF:
			// we're done.
			return nil
		default:
			// something bad happened.
			return err
		}
	}
}

func executeTestVector(tv schema.TestVector) error {
	log.Println("executing test vector:", tv.Meta.ID)
	r := new(conformance.LogReporter)
	switch class := tv.Class; class {
	case "message":
		conformance.ExecuteMessageVector(r, &tv)
	case "tipset":
		conformance.ExecuteTipsetVector(r, &tv)
	default:
		return fmt.Errorf("test vector class %s not supported", class)
	}

	if r.Failed() {
		log.Println(color.HiRedString("❌ test vector failed"))
	} else {
		log.Println(color.GreenString("✅ test vector succeeded"))
	}

	return nil
}
