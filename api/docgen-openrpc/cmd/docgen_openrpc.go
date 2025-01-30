package main

import (
	"fmt"
	"os"

	"github.com/filecoin-project/lotus/api/docgen"
	docgen_openrpc "github.com/filecoin-project/lotus/api/docgen-openrpc"
)

/*
main defines a small program that writes an OpenRPC document describing
a Lotus API to stdout.

If the first argument is "miner", the document will describe the StorageMiner API.
If not (no, or any other args), the document will describe the Full API.

Use:

		go run ./api/openrpc/cmd ["api/api_full.go"|"api/api_storage.go"|"api/api_worker.go"] ["FullNode"|"StorageMiner"|"Worker"]

	With gzip compression: a '-gzip' flag is made available as an optional third argument. Note that position matters.

		go run ./api/openrpc/cmd ["api/api_full.go"|"api/api_storage.go"|"api/api_worker.go"] ["FullNode"|"StorageMiner"|"Worker"] -gzip

*/

func main() {
	// Use os.Args to handle a somewhat hacky flag for the gzip option.
	// Could use flags package to handle this more cleanly, but that requires changes elsewhere
	// the scope of which just isn't warranted by this one use case which will usually be run
	// programmatically anyway.
	var (
		apiFile = os.Args[1]
		iface   = os.Args[2]
		pkg     = os.Args[3]
		dir     = os.Args[4]
		outGzip = len(os.Args) > 5 && os.Args[5] == "-gzip"
	)
	if ainfo, err := docgen.ParseApiASTInfo(apiFile, iface, pkg, dir); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to parse API AST info: %v\n", err)
		os.Exit(1)
	} else if err := docgen_openrpc.Generate(os.Stdout, iface, pkg, ainfo, outGzip); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to generate OpenRPC docs: %v\n", err)
		os.Exit(1)
	}
}
