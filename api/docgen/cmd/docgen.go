package main

import (
	"fmt"
	"os"

	"github.com/filecoin-project/lotus/api/docgen"
)

func main() {
	var (
		apiFile = os.Args[1]
		iface   = os.Args[2]
		pkg     = os.Args[3]
		dir     = os.Args[4]
	)
	if ainfo, err := docgen.ParseApiASTInfo(apiFile, iface, pkg, dir); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to parse API AST info: %v\n", err)
		os.Exit(1)
	} else if err := docgen.Generate(os.Stdout, iface, pkg, ainfo); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to generate docs: %v\n", err)
		os.Exit(1)
	}
}
