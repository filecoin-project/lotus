package main

import (
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/lotus"
)

func main() {
	app := lotus.App()
	lcli.RunApp(app)
}
