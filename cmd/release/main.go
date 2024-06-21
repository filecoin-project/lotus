package main

import (
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/filecoin-project/lotus/build"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "release",
		Usage: "Lotus release tool",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Format output as JSON",
			},
		},
		Before: func(c *cli.Context) error {
			if c.Bool("json") {
				log.SetFormatter(&log.JSONFormatter{})
			} else {
				log.SetFormatter(&log.TextFormatter{
					TimestampFormat: "2006-01-02 15:04:05",
					FullTimestamp:   true,
				})
			}
			log.SetOutput(os.Stdout)
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "node",
				Usage: "Commands related to the Lotus Node",
				Subcommands: []*cli.Command{
					{
						Name:  "version",
						Usage: "Print the Lotus Node version",
						Action: func(c *cli.Context) error {
							log.Info(build.NodeUserVersion())
							return nil
						},
					},
				},
			},
			{
				Name:  "miner",
				Usage: "Commands related to the Lotus Miner",
				Subcommands: []*cli.Command{
					{
						Name:  "version",
						Usage: "Print the Lotus Miner version",
						Action: func(c *cli.Context) error {
							log.Info(build.MinerUserVersion())
							return nil
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
