package main

import (
	"os"

	logging "github.com/ipfs/go-log/v2"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/build"
)

var log = logging.Logger("lotus-shed")

func main() {
	logging.SetLogLevel("*", "INFO")

	local := []*cli.Command{
		base32Cmd,
		base16Cmd,
		keyinfoCmd,
		peerkeyCmd,
		noncefix,
	}

	app := &cli.App{
		Name:     "lotus-shed",
		Usage:    "A place for all the lotus tools",
		Version:  build.BuildVersion,
		Commands: local,
	}

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		os.Exit(1)
		return
	}
}
