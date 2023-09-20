package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/kr/pretty"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
)

var configCmd = &cli.Command{
	Name:  "config",
	Usage: "Manage node config by layers. The layer 'base' will always be applied. ",
	Subcommands: []*cli.Command{
		configDefaultCmd,
		configSetCmd,
		configGetCmd,
		configListCmd,
		configViewCmd,
	},
}

var configDefaultCmd = &cli.Command{
	Name:  "default",
	Usage: "Print default node config",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-comment",
			Usage: "don't comment default values",
		},
	},
	Action: func(cctx *cli.Context) error {
		c := config.DefaultLotusProvider()

		cb, err := config.ConfigUpdate(c, nil, config.Commented(!cctx.Bool("no-comment")), config.DefaultKeepUncommented(), config.NoEnv())
		if err != nil {
			return err
		}

		fmt.Print(string(cb))

		return nil
	},
}

var configSetCmd = &cli.Command{
	Name:      "set",
	Usage:     "Set a config layer or the base",
	ArgsUsage: "a layer's name",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() != 1 {
			return errors.New("must have exactly 1 arg for the file name")
		}
		db, err := makeDB(cctx)
		if err != nil {
			return err
		}

		fn := args.First()
		bytes, err := os.ReadFile(fn)
		if err != nil {
			return fmt.Errorf("cannot read file %w", err)
		}

		lp := config.DefaultLotusProvider() // ensure it's toml
		_, err = toml.Decode(string(bytes), lp)
		if err != nil {
			return fmt.Errorf("cannot decode file: %w", err)
		}
		_ = lp

		name := strings.Split(fn, ".")[0]
		_, err = db.Exec(context.Background(),
			`INSERT INTO harmony_config (title, config) VALUES (?,?) 
			ON CONFLICT (title) DO UPDATE SET config = excluded.config`, name, string(bytes))
		if err != nil {
			return fmt.Errorf("unable to save config layer: %w", err)
		}

		fmt.Println("Layer " + name + " created/updated")
		return nil
	},
}

var configGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "Get a config layer by name. You may want to pipe the output to a file, or use 'less'",
	ArgsUsage: "layer name",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() != 1 {
			return fmt.Errorf("want 1 layer arg, got %d", args.Len())
		}
		db, err := makeDB(cctx)
		if err != nil {
			return err
		}

		var cfg string
		err = db.QueryRow(context.Background(), `SELECT config FROM harmony_config WHERE title=?`, args.First()).Scan(&cfg)
		if err != nil {
			return err
		}
		fmt.Println(cfg)

		return nil
	},
}

var configListCmd = &cli.Command{
	Name:  "list",
	Usage: "List config layers you can get.",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		db, err := makeDB(cctx)
		if err != nil {
			return err
		}
		var res []string
		err = db.Select(context.Background(), &res, `SELECT title FROM harmony_confg ORDER BY title`)
		if err != nil {
			return fmt.Errorf("unable to read from db: %w", err)
		}
		for _, r := range res {
			fmt.Println(r)
		}

		return nil
	},
}

var configViewCmd = &cli.Command{
	Name:      "view",
	Usage:     "View stacked config layers as it will be interpreted.",
	ArgsUsage: "a list of layers to be interpreted as the final config",
	Action: func(cctx *cli.Context) error {
		db, err := makeDB(cctx)
		if err != nil {
			return err
		}
		lp, err := getConfig(cctx, db)
		if err != nil {
			return err
		}

		fmt.Println(pretty.Sprint(lp))

		return nil
	},
}

func getConfig(cctx *cli.Context, db *harmonydb.DB) (*config.LotusProviderConfig, error) {
	lp := config.DefaultLotusProvider()
	have := []string{}
	for _, layer := range regexp.MustCompile("[ |,]").Split(cctx.String("layers"), -1) {
		text := ""
		err := db.QueryRow(cctx.Context, `SELECT config FROM harmony_config WHERE title=?`, layer).Scan(&text)
		if err != nil {
			return nil, fmt.Errorf("could not read layer %s: %w", layer, err)
		}
		meta, err := toml.Decode(text, &lp)
		if err != nil {
			return nil, fmt.Errorf("could not read layer, bad toml %s: %w", layer, err)
		}
		for _, k := range meta.Keys() {
			have = append(have, strings.Join(k, " "))
		}
	}
	_ = have // TODO: verify that required fields are here.
	return lp, nil
}
