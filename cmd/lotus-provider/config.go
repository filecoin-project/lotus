package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

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
		configRmCmd,
		configMigrateCmd,
	},
}

var configDefaultCmd = &cli.Command{
	Name:    "default",
	Aliases: []string{"defaults"},
	Usage:   "Print default node config",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-comment",
			Usage: "don't comment default values",
		},
	},
	Action: func(cctx *cli.Context) error {
		comment := !cctx.Bool("no-comment")
		cfg, err := getDefaultConfig(comment)
		if err != nil {
			return err
		}
		fmt.Print(cfg)

		return nil
	},
}

func getDefaultConfig(comment bool) (string, error) {
	c := config.DefaultLotusProvider()
	cb, err := config.ConfigUpdate(c, nil, config.Commented(comment), config.DefaultKeepUncommented(), config.NoEnv())
	if err != nil {
		return "", err
	}
	return string(cb), nil
}

var configSetCmd = &cli.Command{
	Name:      "set",
	Aliases:   []string{"add", "update", "create"},
	Usage:     "Set a config layer or the base by providing a filename or stdin.",
	ArgsUsage: "a layer's file name",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "title",
			Usage: "title of the config layer (req'd for stdin)",
		},
	},
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()

		db, err := makeDB(cctx)
		if err != nil {
			return err
		}

		name := cctx.String("title")
		var stream io.Reader = os.Stdin
		if args.Len() != 1 {
			if cctx.String("title") == "" {
				return errors.New("must have a title for stdin, or a file name")
			}
		} else {
			stream, err = os.Open(args.First())
			if err != nil {
				return fmt.Errorf("cannot open file %s: %w", args.First(), err)
			}
			if name == "" {
				name = strings.Split(path.Base(args.First()), ".")[0]
			}
		}
		bytes, err := io.ReadAll(stream)
		if err != nil {
			return fmt.Errorf("cannot read stream/file %w", err)
		}

		lp := config.DefaultLotusProvider() // ensure it's toml
		_, err = toml.Decode(string(bytes), lp)
		if err != nil {
			return fmt.Errorf("cannot decode file: %w", err)
		}
		_ = lp

		_, err = db.Exec(context.Background(),
			`INSERT INTO harmony_config (title, config) VALUES ($1, $2) 
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
	Aliases:   []string{"cat", "show"},
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
		err = db.QueryRow(context.Background(), `SELECT config FROM harmony_config WHERE title=$1`, args.First()).Scan(&cfg)
		if err != nil {
			return err
		}
		fmt.Println(cfg)

		return nil
	},
}

var configListCmd = &cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "List config layers you can get.",
	Flags:   []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		db, err := makeDB(cctx)
		if err != nil {
			return err
		}
		var res []string
		err = db.Select(context.Background(), &res, `SELECT title FROM harmony_config ORDER BY title`)
		if err != nil {
			return fmt.Errorf("unable to read from db: %w", err)
		}
		for _, r := range res {
			fmt.Println(r)
		}

		return nil
	},
}

var configRmCmd = &cli.Command{
	Name:    "remove",
	Aliases: []string{"rm", "del", "delete"},
	Usage:   "Remove a named config layer.",
	Flags:   []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() != 1 {
			return errors.New("must have exactly 1 arg for the layer name")
		}
		db, err := makeDB(cctx)
		if err != nil {
			return err
		}
		ct, err := db.Exec(context.Background(), `DELETE FROM harmony_config WHERE title=$1`, args.First())
		if err != nil {
			return fmt.Errorf("unable to read from db: %w", err)
		}
		if ct == 0 {
			return fmt.Errorf("no layer named %s", args.First())
		}

		return nil
	},
}
var configViewCmd = &cli.Command{
	Name:      "interpret",
	Aliases:   []string{"view", "stacked", "stack"},
	Usage:     "Interpret stacked config layers by this version of lotus-provider, with system-generated comments.",
	ArgsUsage: "a list of layers to be interpreted as the final config",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:     "layers",
			Usage:    "comma or space separated list of layers to be interpreted",
			Value:    cli.NewStringSlice("base"),
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		db, err := makeDB(cctx)
		if err != nil {
			return err
		}
		lp, err := getConfig(cctx, db)
		if err != nil {
			return err
		}
		cb, err := config.ConfigUpdate(lp, config.DefaultLotusProvider(), config.Commented(true), config.DefaultKeepUncommented(), config.NoEnv())
		if err != nil {
			return xerrors.Errorf("cannot interpret config: %w", err)
		}
		fmt.Println(string(cb))
		return nil
	},
}

func getConfig(cctx *cli.Context, db *harmonydb.DB) (*config.LotusProviderConfig, error) {
	lp := config.DefaultLotusProvider()
	have := []string{}
	layers := cctx.StringSlice("layers")
	for _, layer := range layers {
		text := ""
		err := db.QueryRow(cctx.Context, `SELECT config FROM harmony_config WHERE title=$1`, layer).Scan(&text)
		if err != nil {
			if strings.Contains(err.Error(), sql.ErrNoRows.Error()) {
				return nil, fmt.Errorf("missing layer '%s' ", layer)
			}
			if layer == "base" {
				return nil, errors.New(`lotus-provider defaults to a layer named 'base'. 
				Either use 'migrate' command or edit a base.toml and upload it with: lotus-provider config set base.toml`)
			}
			return nil, fmt.Errorf("could not read layer '%s': %w", layer, err)
		}
		meta, err := toml.Decode(text, &lp)
		if err != nil {
			return nil, fmt.Errorf("could not read layer, bad toml %s: %w", layer, err)
		}
		for _, k := range meta.Keys() {
			have = append(have, strings.Join(k, " "))
		}
	}
	_ = have // FUTURE: verify that required fields are here.
	// If config includes 3rd-party config, consider JSONSchema as a way that
	// 3rd-parties can dynamically include config requirements and we can
	// validate the config. Because of layering, we must validate @ startup.
	return lp, nil
}
