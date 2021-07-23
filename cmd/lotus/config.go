package main

import (
	"bytes"
	"fmt"
	"github.com/filecoin-project/lotus/node/repo"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/config"
)

var configCmd = &cli.Command{
	Name:  "config",
	Usage: "Manage node config",
	Subcommands: []*cli.Command{
		configDefaultCmd,
		configUpdateCmd,
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
		c := config.DefaultFullNode()

		if cctx.Bool("no-comment") {
			buf := new(bytes.Buffer)
			_, _ = buf.WriteString("# Default config:\n")
			e := toml.NewEncoder(buf)
			if err := e.Encode(c); err != nil {
				return xerrors.Errorf("encoding default config: %w", err)
			}

			fmt.Println(buf.String())
			return nil
		}

		cb, err := config.ConfigComment(c)
		if err != nil {
			return err
		}

		fmt.Println(string(cb))

		return nil
	},
}

var configUpdateCmd = &cli.Command{
	Name:  "updated",
	Usage: "Print updated node config",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-comment",
			Usage: "don't comment default values",
		},
	},
	Action: func(cctx *cli.Context) error {
		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}

		if !ok {
			return xerrors.Errorf("repo not initialized")
		}

		lr, err := r.LockRO(repo.FullNode)
		if err != nil {
			return xerrors.Errorf("locking repo: %w", err)
		}

		cfgNode, err := lr.Config()
		if err != nil {
			_ = lr.Close()
			return xerrors.Errorf("getting node config: %w", err)
		}

		if err := lr.Close(); err != nil {
			return err
		}

		cfgDef := config.DefaultFullNode()

		var nodeStr, defStr string
		{
			buf := new(bytes.Buffer)
			e := toml.NewEncoder(buf)
			if err := e.Encode(cfgDef); err != nil {
				return xerrors.Errorf("encoding default config: %w", err)
			}

			defStr = buf.String()
		}

		{
			buf := new(bytes.Buffer)
			e := toml.NewEncoder(buf)
			if err := e.Encode(cfgNode); err != nil {
				return xerrors.Errorf("encoding node config: %w", err)
			}

			nodeStr = buf.String()
		}

		if !cctx.Bool("no-comment") {
			defLines := strings.Split(defStr, "\n")
			defaults := map[string]struct{}{}
			for i := range defLines {
				l := strings.TrimSpace(defLines[i])
				if len(l) == 0 {
					continue
				}
				if l[0] == '#' || l[0] == '[' {
					continue
				}
				defaults[l] = struct{}{}
			}

			nodeLines := strings.Split(nodeStr, "\n")
			for i := range nodeLines {
				if _, found := defaults[strings.TrimSpace(nodeLines[i])]; found {
					nodeLines[i] = "#" + nodeLines[i]
				}
			}

			nodeStr = strings.Join(nodeLines, "\n")
		}

		// sanity-check that the updated config parses the same way as the current one
		{
			cfgUpdated, err := config.FromReader(strings.NewReader(nodeStr), config.DefaultFullNode())
			if err != nil {
				return xerrors.Errorf("parsing updated config: %w", err)
			}

			if !reflect.DeepEqual(cfgNode, cfgUpdated) {
				return xerrors.Errorf("updated config didn't match current config")
			}
		}

		fmt.Println(nodeStr)
		return nil
	},
}
