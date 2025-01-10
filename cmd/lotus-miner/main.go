package main

import (
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/miner"
)

func main() {
	app := miner.App()
	lcli.RunApp(app)
}
