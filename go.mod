module github.com/filecoin-project/sector-storage

go 1.13

require (
	github.com/elastic/go-sysinfo v1.3.0
	github.com/filecoin-project/filecoin-ffi v0.0.0-20200326153646-e899cc1dd072
	github.com/filecoin-project/go-fil-commcid v0.0.0-20200208005934-2b8bd03caca5
	github.com/filecoin-project/go-paramfetch v0.0.1
	github.com/filecoin-project/lotus v0.2.10
	github.com/filecoin-project/specs-actors v1.0.0
	github.com/filecoin-project/specs-storage v0.0.0-20200417134612-61b2d91a6102
	github.com/gorilla/mux v1.7.4
	github.com/hashicorp/go-multierror v1.0.0
	github.com/ipfs/go-cid v0.0.5
	github.com/ipfs/go-ipfs-files v0.0.7
	github.com/ipfs/go-log v1.0.3
	github.com/ipfs/go-log/v2 v2.0.3
	github.com/mitchellh/go-homedir v1.1.0
	go.opencensus.io v0.22.3
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
