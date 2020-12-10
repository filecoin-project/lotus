package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/chain/stmgr"
)

var extractManyFlags struct {
	in      string
	outdir  string
	batchId string
}

var extractManyCmd = &cli.Command{
	Name: "extract-many",
	Description: `generate many test vectors by repeatedly calling tvx extract, using a csv file as input.

   The CSV file must have a format just like the following:

   message_cid,receiver_code,method_num,exit_code,height,block_cid,seq
   bafy2bzacedvuvgpsnwq7i7kltfap6hnp7fdmzf6lr4w34zycjrthb3v7k6zi6,fil/1/account,0,0,67972,bafy2bzacebthpxzlk7zhlkz3jfzl4qw7mdoswcxlf3rkof3b4mbxfj3qzfk7w,1
   bafy2bzacedwicofymn4imgny2hhbmcm4o5bikwnv3qqgohyx73fbtopiqlro6,fil/1/account,0,0,67860,bafy2bzacebj7beoxyzll522o6o76mt7von4psn3tlvunokhv4zhpwmfpipgti,2
   ...

   The first row MUST be a header row. At the bare minimum, those seven fields
   must appear, in the order specified. Extra fields are accepted, but always
   after these compulsory seven.
`,
	Action: runExtractMany,
	Before: initialize,
	After:  destroy,
	Flags: []cli.Flag{
		&repoFlag,
		&cli.StringFlag{
			Name:        "batch-id",
			Usage:       "batch id; a four-digit left-zero-padded sequential number (e.g. 0041)",
			Required:    true,
			Destination: &extractManyFlags.batchId,
		},
		&cli.StringFlag{
			Name:        "in",
			Usage:       "path to input file (csv)",
			Destination: &extractManyFlags.in,
		},
		&cli.StringFlag{
			Name:        "outdir",
			Usage:       "output directory",
			Destination: &extractManyFlags.outdir,
		},
	},
}

func runExtractMany(c *cli.Context) error {
	// LOTUS_DISABLE_VM_BUF disables what's called "VM state tree buffering",
	// which stashes write operations in a BufferedBlockstore
	// (https://github.com/filecoin-project/lotus/blob/b7a4dbb07fd8332b4492313a617e3458f8003b2a/lib/bufbstore/buf_bstore.go#L21)
	// such that they're not written until the VM is actually flushed.
	//
	// For some reason, the standard behaviour was not working for me (raulk),
	// and disabling it (such that the state transformations are written immediately
	// to the blockstore) worked.
	_ = os.Setenv("LOTUS_DISABLE_VM_BUF", "iknowitsabadidea")

	var (
		in     = extractManyFlags.in
		outdir = extractManyFlags.outdir
	)

	if in == "" {
		return fmt.Errorf("input file not provided")
	}

	if outdir == "" {
		return fmt.Errorf("output dir not provided")
	}

	// Open the CSV file for reading.
	f, err := os.Open(in)
	if err != nil {
		return fmt.Errorf("could not open file %s: %w", in, err)
	}

	// Ensure the output directory exists.
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return fmt.Errorf("could not create output dir %s: %w", outdir, err)
	}

	// Create a CSV reader and validate the header row.
	reader := csv.NewReader(f)
	if header, err := reader.Read(); err != nil {
		return fmt.Errorf("failed to read header from csv: %w", err)
	} else if l := len(header); l < 7 {
		return fmt.Errorf("insufficient number of fields: %d", l)
	} else if f := header[0]; f != "message_cid" {
		return fmt.Errorf("csv sanity check failed: expected first field in header to be 'message_cid'; was: %s", f)
	} else {
		log.Println(color.GreenString("csv sanity check succeeded; header contains fields: %v", header))
	}

	codeCidBuilder := cid.V1Builder{Codec: cid.Raw, MhType: multihash.IDENTITY}

	var (
		generated []string
		merr      = new(multierror.Error)
		retry     []extractOpts // to retry with 'canonical' precursor selection mode
	)

	// Read each row and extract the requested message.
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("failed to read row: %w", err)
		}
		var (
			mcid         = row[0]
			actorcode    = row[1]
			methodnumstr = row[2]
			exitcodestr  = row[3]
			_            = row[4]
			block        = row[5]
			seq          = row[6]

			exit       int
			methodnum  int
			methodname string
		)

		// Parse the exit code.
		if exit, err = strconv.Atoi(exitcodestr); err != nil {
			return fmt.Errorf("invalid exitcode number: %d", exit)
		}
		// Parse the method number.
		if methodnum, err = strconv.Atoi(methodnumstr); err != nil {
			return fmt.Errorf("invalid method number: %s", methodnumstr)
		}

		codeCid, err := codeCidBuilder.Sum([]byte(actorcode))
		if err != nil {
			return fmt.Errorf("failed to compute actor code CID")
		}

		// Lookup the method in actor method table.
		if m, ok := stmgr.MethodsMap[codeCid]; !ok {
			return fmt.Errorf("unrecognized actor: %s", actorcode)
		} else if methodnum >= len(m) {
			return fmt.Errorf("unrecognized method number for actor %s: %d", actorcode, methodnum)
		} else {
			methodname = m[abi.MethodNum(methodnum)].Name
		}

		// exitcode string representations are of kind ErrType(0); strip out
		// the number portion.
		exitcodename := strings.Split(exitcode.ExitCode(exit).String(), "(")[0]
		// replace the slashes in the actor code name with underscores.
		actorcodename := strings.ReplaceAll(actorcode, "/", "_")

		// Compute the ID of the vector.
		id := fmt.Sprintf("ext-%s-%s-%s-%s-%s", extractManyFlags.batchId, actorcodename, methodname, exitcodename, seq)
		// Vector filename, using a base of outdir.
		file := filepath.Join(outdir, actorcodename, methodname, exitcodename, id) + ".json"

		log.Println(color.YellowString("processing message cid with 'sender' precursor mode: %s", id))

		opts := extractOpts{
			id:        id,
			block:     block,
			class:     "message",
			cid:       mcid,
			file:      file,
			retain:    "accessed-cids",
			precursor: PrecursorSelectSender,
		}

		if err := doExtract(opts); err != nil {
			log.Println(color.RedString("failed to extract vector for message %s: %s; queuing for 'all' precursor selection", mcid, err))
			retry = append(retry, opts)
			continue
		}

		log.Println(color.MagentaString("generated file: %s", file))

		generated = append(generated, file)
	}

	log.Printf("extractions to try with canonical precursor selection mode: %d", len(retry))

	for _, r := range retry {
		log.Printf("retrying %s: %s", r.cid, r.id)

		r.precursor = PrecursorSelectAll
		if err := doExtract(r); err != nil {
			merr = multierror.Append(merr, fmt.Errorf("failed to extract vector for message %s: %w", r.cid, err))
			continue
		}

		log.Println(color.MagentaString("generated file: %s", r.file))
		generated = append(generated, r.file)
	}

	if len(generated) == 0 {
		log.Println("no files generated")
	} else {
		log.Println("files generated:")
		for _, g := range generated {
			log.Println(g)
		}
	}

	if merr.ErrorOrNil() != nil {
		log.Println(color.YellowString("done processing with errors: %v", merr))
	} else {
		log.Println(color.GreenString("done processing with no errors"))
	}

	return merr.ErrorOrNil()
}
