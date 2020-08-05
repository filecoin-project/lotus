package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/oni/tvx/state"
)

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
	}
	return s
}

var examineFlags struct {
	file string
	pre  bool
	post bool
}

var examineCmd = &cli.Command{
	Name:        "examine",
	Description: "examine an exported state root as represented in a test vector",
	Action:      runExamineCmd,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "file",
			Usage:       "test vector file",
			Required:    true,
			Destination: &examineFlags.file,
		},
		&cli.BoolFlag{
			Name:        "pre",
			Usage:       "examine the precondition state tree",
			Destination: &examineFlags.pre,
		},
		&cli.BoolFlag{
			Name:        "post",
			Usage:       "examine the postcondition state tree",
			Destination: &examineFlags.post,
		},
	},
}

func runExamineCmd(_ *cli.Context) error {
	file, err := os.Open(examineFlags.file)
	if err != nil {
		return err
	}

	var tv TestVector
	if err := json.NewDecoder(file).Decode(&tv); err != nil {
		return err
	}

	examine := func(encoded []byte) error {
		tree, err := state.RecoverStateTree(context.TODO(), encoded)
		if err != nil {
			return err
		}

		initActor, err := tree.GetActor(builtin.InitActorAddr)
		if err != nil {
			return fmt.Errorf("cannot recover init actor: %w", err)
		}

		var ias init_.State
		if err := tree.Store.Get(context.TODO(), initActor.Head, &ias); err != nil {
			return err
		}

		adtStore := adt.WrapStore(context.TODO(), tree.Store)
		m, err := adt.AsMap(adtStore, ias.AddressMap)
		if err != nil {
			return err
		}
		actors, err := m.CollectKeys()
		for _, actor := range actors {
			fmt.Printf("%s\n", actor)
		}
		return nil
	}

	if examineFlags.pre {
		log.Print("examining precondition tree")
		if err := examine(tv.Pre.StateTree.CAR); err != nil {
			return err
		}
	}

	if examineFlags.post {
		log.Print("examining postcondition tree")
		if err := examine(tv.Post.StateTree.CAR); err != nil {
			return err
		}
	}

	return nil
}
