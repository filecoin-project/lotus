package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
)

var configCmd = &cli.Command{
	Name:  "config",
	Usage: "Manage node config by layers. The layer 'base' will always be applied at Curio start-up.",
	Subcommands: []*cli.Command{
		configDefaultCmd,
		configSetCmd,
		configGetCmd,
		configListCmd,
		configViewCmd,
		configRmCmd,
		configEditCmd,
		configNewCmd,
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
		cfg, err := deps.GetDefaultConfig(comment)
		if err != nil {
			return err
		}
		fmt.Print(cfg)

		return nil
	},
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

		db, err := deps.MakeDB(cctx)
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

		curioConfig := config.DefaultCurioConfig() // ensure it's toml
		_, err = deps.LoadConfigWithUpgrades(string(bytes), curioConfig)
		if err != nil {
			return fmt.Errorf("cannot decode file: %w", err)
		}
		_ = curioConfig

		err = setConfig(db, name, string(bytes))

		if err != nil {
			return fmt.Errorf("unable to save config layer: %w", err)
		}

		fmt.Println("Layer " + name + " created/updated")
		return nil
	},
}

func setConfig(db *harmonydb.DB, name, config string) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO harmony_config (title, config) VALUES ($1, $2) 
			ON CONFLICT (title) DO UPDATE SET config = excluded.config`, name, config)
	return err
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
		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		cfg, err := getConfig(db, args.First())
		if err != nil {
			return err
		}
		fmt.Println(cfg)

		return nil
	},
}

func getConfig(db *harmonydb.DB, layer string) (string, error) {
	var cfg string
	err := db.QueryRow(context.Background(), `SELECT config FROM harmony_config WHERE title=$1`, layer).Scan(&cfg)
	if err != nil {
		return "", err
	}
	return cfg, nil
}

var configListCmd = &cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "List config layers present in the DB.",
	Flags:   []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		db, err := deps.MakeDB(cctx)
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
		db, err := deps.MakeDB(cctx)
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
	Usage:     "Interpret stacked config layers by this version of curio, with system-generated comments.",
	ArgsUsage: "a list of layers to be interpreted as the final config",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:     "layers",
			Usage:    "comma or space separated list of layers to be interpreted (base is always applied)",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}
		layers := cctx.StringSlice("layers")
		curioConfig, err := deps.GetConfig(cctx.Context, layers, db)
		if err != nil {
			return err
		}
		cb, err := config.ConfigUpdate(curioConfig, config.DefaultCurioConfig(), config.Commented(true), config.DefaultKeepUncommented(), config.NoEnv())
		if err != nil {
			return xerrors.Errorf("cannot interpret config: %w", err)
		}
		fmt.Println(string(cb))
		return nil
	},
}

var configEditCmd = &cli.Command{
	Name:      "edit",
	Usage:     "edit a config layer",
	ArgsUsage: "[layer name]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "editor",
			Usage:   "editor to use",
			Value:   "vim",
			EnvVars: []string{"EDITOR"},
		},
		&cli.StringFlag{
			Name:        "source",
			Usage:       "source config layer",
			DefaultText: "<edited layer>",
		},
		&cli.BoolFlag{
			Name:  "allow-overwrite",
			Usage: "allow overwrite of existing layer if source is a different layer",
		},
		&cli.BoolFlag{
			Name:  "no-source-diff",
			Usage: "save the whole config into the layer, not just the diff",
		},
		&cli.BoolFlag{
			Name:        "no-interpret-source",
			Usage:       "do not interpret source layer",
			DefaultText: "true if --source is set",
		},
	},
	Action: func(cctx *cli.Context) error {
		layer := cctx.Args().First()
		if layer == "" {
			return errors.New("layer name is required")
		}

		source := layer
		if cctx.IsSet("source") {
			source = cctx.String("source")

			if source == layer && !cctx.Bool("allow-owerwrite") {
				return errors.New("source and target layers are the same")
			}
		}

		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		sourceConfig, err := getConfig(db, source)
		if err != nil {
			return xerrors.Errorf("getting source config: %w", err)
		}

		if cctx.IsSet("source") && source != layer && !cctx.Bool("no-interpret-source") {
			curioCfg := config.DefaultCurioConfig()
			if _, err := toml.Decode(sourceConfig, curioCfg); err != nil {
				return xerrors.Errorf("parsing source config: %w", err)
			}

			cb, err := config.ConfigUpdate(curioCfg, config.DefaultCurioConfig(), config.Commented(true), config.DefaultKeepUncommented(), config.NoEnv())
			if err != nil {
				return xerrors.Errorf("interpreting source config: %w", err)
			}
			sourceConfig = string(cb)
		}

		editor := cctx.String("editor")
		newConfig, err := edit(editor, sourceConfig)
		if err != nil {
			return xerrors.Errorf("editing config: %w", err)
		}

		toWrite := newConfig

		if cctx.IsSet("source") && !cctx.Bool("no-source-diff") {
			updated, err := diff(sourceConfig, newConfig)
			if err != nil {
				return xerrors.Errorf("computing diff: %w", err)
			}

			{
				fmt.Printf("%s will write changes as the layer because %s is not set\n", color.YellowString(">"), color.GreenString("--no-source-diff"))
				fmt.Println(updated)
				fmt.Printf("%s Confirm [y]: ", color.YellowString(">"))

				for {
					var confirmBuf [16]byte
					n, err := os.Stdin.Read(confirmBuf[:])
					if err != nil {
						return xerrors.Errorf("reading confirmation: %w", err)
					}
					confirm := strings.TrimSpace(string(confirmBuf[:n]))

					if confirm == "" {
						confirm = "y"
					}

					if confirm[:1] == "y" {
						break
					}
					if confirm[:1] == "n" {
						return nil
					}

					fmt.Printf("%s Confirm [y]:\n", color.YellowString(">"))
				}
			}

			toWrite = updated
		}

		fmt.Printf("%s Writing config for layer %s\n", color.YellowString(">"), color.GreenString(layer))

		return setConfig(db, layer, toWrite)
	},
}

func diff(sourceConf, newConf string) (string, error) {
	fromSrc := config.DefaultCurioConfig()
	fromNew := config.DefaultCurioConfig()

	_, err := toml.Decode(sourceConf, fromSrc)
	if err != nil {
		return "", xerrors.Errorf("decoding source config: %w", err)
	}

	_, err = toml.Decode(newConf, fromNew)
	if err != nil {
		return "", xerrors.Errorf("decoding new config: %w", err)
	}

	cb, err := config.ConfigUpdate(fromNew, fromSrc, config.Commented(true), config.NoEnv())
	if err != nil {
		return "", xerrors.Errorf("interpreting source config: %w", err)
	}

	lines := strings.Split(string(cb), "\n")
	var outLines []string
	var categoryBuf string

	for _, line := range lines {
		// drop empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}
		// drop lines starting with '#'
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		// if starting with [, it's a category
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			categoryBuf = line
			continue
		}

		if categoryBuf != "" {
			outLines = append(outLines, categoryBuf)
			categoryBuf = ""
		}

		outLines = append(outLines, line)
	}

	return strings.Join(outLines, "\n"), nil
}

func edit(editor, cfg string) (string, error) {
	file, err := os.CreateTemp("", "curio-config-*.toml")
	if err != nil {
		return "", err
	}

	_, err = file.WriteString(cfg)
	if err != nil {
		return "", err
	}

	filePath := file.Name()

	if err := file.Close(); err != nil {
		return "", err
	}

	defer func() {
		_ = os.Remove(filePath)
	}()

	cmd := exec.Command(editor, filePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(data), err
}
