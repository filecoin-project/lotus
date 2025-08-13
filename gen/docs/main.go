package main

import (
	"fmt"
	"os"

	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/lotus/api/docgen"
	docgen_openrpc "github.com/filecoin-project/lotus/api/docgen-openrpc"
)

// Documentation output file constants
const (
	// v0 API - Deprecated
	apiMethodsV0File     = "documentation/en/api-methods-v0-deprecated.md"
	openRPCV0GatewayFile = "build/openrpc/v0/gateway.json"

	// v1 API - Stable
	apiMethodsV1File   = "documentation/en/api-methods-v1-stable.md"
	openRPCFullFile    = "build/openrpc/full.json"
	openRPCGatewayFile = "build/openrpc/gateway.json"

	// v2 API - Experimental
	apiMethodsV2File     = "documentation/en/api-methods-v2-experimental.md"
	openRPCV2FullFile    = "build/openrpc/v2/full.json"
	openRPCV2GatewayFile = "build/openrpc/v2/gateway.json"

	// Worker and Miner API - Stable
	apiMethodsWorkerFile = "documentation/en/api-methods-worker.md"
	apiMethodsMinerFile  = "documentation/en/api-methods-miner.md"
	openRPCWorkerFile    = "build/openrpc/worker.json"
	openRPCMinerFile     = "build/openrpc/miner.json"
)

func main() {
	var lets errgroup.Group
	lets.SetLimit(1) // TODO: Investigate why this can't run in parallel.
	lets.Go(generateApiV0Methods)
	lets.Go(generateOpenRpcGatewayV0)
	lets.Go(generateApiV1Methods)
	lets.Go(generateOpenRpcGatewayV1)
	lets.Go(generateApiV2Methods)
	lets.Go(generateOpenRpcGatewayV2)
	lets.Go(generateStorage)
	lets.Go(generateWorker)
	if err := lets.Wait(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println("All documentations generated successfully.")
}

func generateApiV0Methods() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/v0api/full.go", "FullNode", "v0api", "./api/v0api"); err != nil {
		return err
	} else {
		return generateMarkdown(apiMethodsV0File, "FullNode", "v0api", ainfo)
	}
}

func generateOpenRpcGatewayV0() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/v0api/gateway.go", "Gateway", "v0api", "./api/v0api"); err != nil {
		return err
	} else {
		return generateOpenRpc(openRPCV0GatewayFile, "Gateway", "v0api", ainfo)
	}
}

func generateApiV1Methods() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/api_full.go", "FullNode", "api", "./api"); err != nil {
		return err
	} else if err := generateMarkdown(apiMethodsV1File, "FullNode", "api", ainfo); err != nil {
		return err
	} else {
		return generateOpenRpc(openRPCFullFile, "FullNode", "api", ainfo)
	}
}

func generateOpenRpcGatewayV1() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/api_gateway.go", "Gateway", "api", "./api"); err != nil {
		return err
	} else {
		return generateOpenRpc(openRPCGatewayFile, "Gateway", "api", ainfo)
	}
}

func generateApiV2Methods() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/v2api/full.go", "FullNode", "v2api", "./api/v2api"); err != nil {
		return err
	} else if err := generateMarkdown(apiMethodsV2File, "FullNode", "v2api", ainfo); err != nil {
		return err
	} else {
		return generateOpenRpc(openRPCV2FullFile, "FullNode", "v2api", ainfo)
	}
}

func generateOpenRpcGatewayV2() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/v2api/gateway.go", "Gateway", "v2api", "./api/v2api"); err != nil {
		return err
	} else {
		return generateOpenRpc(openRPCV2GatewayFile, "Gateway", "v2api", ainfo)
	}
}

func generateWorker() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/api_worker.go", "Worker", "api", "./api"); err != nil {
		return err
	} else if err := generateMarkdown(apiMethodsWorkerFile, "Worker", "api", ainfo); err != nil {
		return err
	} else {
		return generateOpenRpc(openRPCWorkerFile, "Worker", "api", ainfo)
	}
}

func generateStorage() error {
	if ainfo, err := docgen.ParseApiASTInfo("api/api_storage.go", "StorageMiner", "api", "./api"); err != nil {
		return err
	} else if err := generateMarkdown(apiMethodsMinerFile, "StorageMiner", "api", ainfo); err != nil {
		return err
	} else {
		return generateOpenRpc(openRPCMinerFile, "StorageMiner", "api", ainfo)
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
