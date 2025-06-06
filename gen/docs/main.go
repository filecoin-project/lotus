package main

import (
	"fmt"
	"os"

	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/lotus/api/docgen"
	docgen_openrpc "github.com/filecoin-project/lotus/api/docgen-openrpc"
)

func main() {
	var lets errgroup.Group
	lets.SetLimit(1) // TODO: Investigate why this can't run in parallel.
	lets.Go(generateApiFull)
	lets.Go(generateApiV0Methods)
	lets.Go(generateApiV2Methods)
	lets.Go(generateStorage)
	lets.Go(generateWorker)
	lets.Go(generateOpenRpcGatewayV0)
	lets.Go(generateOpenRpcGatewayV1)
	lets.Go(generateOpenRpcGatewayV2)
	if err := lets.Wait(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println("All documentations generated successfully.")
}

func generateWorker() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/api_worker.go", "Worker", "api", "./api"); err != nil {
		return err
	} else if err := generateMarkdown("documentation/en/api-v0-methods-worker.md", "Worker", "api", ainfo); err != nil {
		return err
	} else {
		return generateOpenRpc("build/openrpc/worker.json", "Worker", "api", ainfo)
	}
}

func generateStorage() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/api_storage.go", "StorageMiner", "api", "./api"); err != nil {
		return err
	} else if err := generateMarkdown("documentation/en/api-v0-methods-miner.md", "StorageMiner", "api", ainfo); err != nil {
		return err
	} else {
		return generateOpenRpc("build/openrpc/miner.json", "StorageMiner", "api", ainfo)
	}
}

func generateApiV0Methods() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/v0api/full.go", "FullNode", "v0api", "./api/v0api"); err != nil {
		return err
	} else {
		return generateMarkdown("documentation/en/api-v0-methods.md", "FullNode", "v0api", ainfo)
	}
}

func generateApiV2Methods() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/v2api/full.go", "FullNode", "v2api", "./api/v2api"); err != nil {
		return err
	} else if err := generateMarkdown("documentation/en/api-v2-unstable-methods.md", "FullNode", "v2api", ainfo); err != nil {
		return err
	} else {
		return generateOpenRpc("build/openrpc/v2/full.json", "FullNode", "v2api", ainfo)
	}
}

func generateApiFull() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/api_full.go", "FullNode", "api", "./api"); err != nil {
		return err
	} else if err := generateMarkdown("documentation/en/api-v1-unstable-methods.md", "FullNode", "api", ainfo); err != nil {
		return err
	} else {
		return generateOpenRpc("build/openrpc/full.json", "FullNode", "api", ainfo)
	}
}

func generateOpenRpcGatewayV0() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/v0api/gateway.go", "Gateway", "v0api", "./api/v0api"); err != nil {
		return err
	} else {
		return generateOpenRpc("build/openrpc/v0/gateway.json", "Gateway", "v0api", ainfo)
	}
}

func generateOpenRpcGatewayV1() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/api_gateway.go", "Gateway", "api", "./api"); err != nil {
		return err
	} else {
		return generateOpenRpc("build/openrpc/gateway.json", "Gateway", "api", ainfo)
	}
}

func generateOpenRpcGatewayV2() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/v2api/gateway.go", "Gateway", "v2api", "./api/v2api"); err != nil {
		return err
	} else {
		return generateOpenRpc("build/openrpc/v2/gateway.json", "Gateway", "v2api", ainfo)
	}
}

func generateMarkdown(path, iface, pkg string, info docgen.ApiASTInfo) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	return docgen.Generate(out, iface, pkg, info)
}

func generateOpenRpc(path, iface, pkg string, info docgen.ApiASTInfo) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	return docgen_openrpc.Generate(out, iface, pkg, info, false)
}
