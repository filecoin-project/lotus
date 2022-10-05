package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/filecoin-project/lotus/chain/consensus/mir"
	"github.com/urfave/cli/v2"
)

// TODO: Make these config files configurable.
const (
	PrivKeyPath       = "mir.key"
	MaddrPath         = "mir.maddr"
	MembershipCfgPath = "mir.validators"
)

var configFiles = []string{PrivKeyPath, MaddrPath, MembershipCfgPath}

var cfgCmd = &cli.Command{
	Name:  "config",
	Usage: "Interact Mir validator config",
	Subcommands: []*cli.Command{
		initCmd,
		addValidatorCmd,
	},
}

var addValidatorCmd = &cli.Command{
	Name:  "add-validator",
	Usage: "Append validator to mir membership configuration",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return fmt.Errorf("expected validator address as input")
		}
		// check if repo initialized
		if err := repoInitialized(context.Background(), cctx); err != nil {
			return err
		}

		// check if validator has been initialized.
		if err := initCheck(cctx.String("repo")); err != nil {
			return err
		}

		mp := path.Join(cctx.String("repo"), MembershipCfgPath)
		val, err := mir.ValidatorFromString(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("error parsing validator from string: %s. Use the following format: <wallet id>@<multiaddr>", err)
		}
		// persist validator config in the right path.
		if err := mir.ValidatorsToCfg(mir.NewValidatorSet([]mir.Validator{val}), mp); err != nil {
			return fmt.Errorf("error exporting membership config: %s", err)
		}

		log.Infow("Mir validator appended to membership config file")
		return nil
	},
}

func cleanConfig(repo string) {
	log.Infow("Cleaning mir config files from repo")
	for _, s := range configFiles {
		p := filepath.Join(repo, s)
		os.Remove(p)
	}
}
