package main

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/filecoin-project/lotus/chain/consensus/mir"
	"github.com/urfave/cli/v2"
)

var initCmd = &cli.Command{
	Name:  "init",
	Usage: "Initialize the config for a mir-validator",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "overwrite",
			Usage:   "Overwrites existing validator config from repo. Use with caution!",
			Aliases: []string{"f"},
			Value:   false,
		},
		&cli.StringFlag{
			Name:  "membership",
			Usage: "Pass membership config with the list of validator in the validator set",
		},
	},
	Action: func(cctx *cli.Context) error {
		// check if repo initialized
		if err := repoInitialized(context.Background(), cctx); err != nil {
			return err
		}
		if cctx.Bool("overwrite") {
			cleanConfig(cctx.String("repo"))
		} else {
			// check if validator has been initialized.
			isCfg, err := isConfigured(cctx.String("repo"))
			if err == nil {
				return fmt.Errorf("validator already configured. Run `./mir-validator config init -f` if you want to overwrite the current config")
			}
			if isCfg && err != nil {
				return fmt.Errorf("validator configured and config corrupted: %v. Backup the config files you want to keep and run `./mir-validator init -f`", err)
			}
		}

		// TODO: Allow passing libp2p ID config as a parameter.
		_, err := newLp2pHost(cctx.String("repo"))
		if err != nil {
			return fmt.Errorf("couldn't initialize libp2p config: %s", err)
		}

		// TODO: Pass validator set for initialization
		mp := path.Join(cctx.String("repo"), MembershipCfgPath)
		if cctx.String("membership") != "" {
			validators, err := mir.GetValidatorsFromCfg(cctx.String("membership"))
			if err != nil {
				return fmt.Errorf("error importing membership config specified: %s", err)
			}
			// persist validator config in the right path.
			if err := mir.ValidatorsToCfg(validators, mp); err != nil {
				return fmt.Errorf("error exporting membership config: %s", err)
			}
		} else {
			log.Infof("Creating empty membership cfg at %s. Remember to run ./mir-validator add-validator to add more membership validators", mp)
			if _, err := os.Create(mp); err != nil {
				return fmt.Errorf("error creating empty membership config in %s", mp)
			}
		}

		log.Infow("Initialized mir validator. Run ./mir-validator run to start validator process")
		return nil
	},
}
