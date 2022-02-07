module github.com/filecoin-project/lotus/testplans/lotus-soup

go 1.16

require (
	contrib.go.opencensus.io/exporter/prometheus v0.4.0
	github.com/codeskyblue/go-sh v0.0.0-20200712050446-30169cf553fe
	github.com/davecgh/go-spew v1.1.1
	github.com/drand/drand v1.2.1
	github.com/filecoin-project/go-address v0.0.6
	github.com/filecoin-project/go-data-transfer v1.11.4
	github.com/filecoin-project/go-fil-markets v1.13.4
	github.com/filecoin-project/go-jsonrpc v0.1.5
	github.com/filecoin-project/go-state-types v0.1.3
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/lotus v0.0.0-00010101000000-000000000000
	github.com/filecoin-project/specs-actors v0.9.14
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/influxdata/influxdb v1.9.4 // indirect
	github.com/ipfs/go-cid v0.1.0
	github.com/ipfs/go-datastore v0.4.6
	github.com/ipfs/go-ipfs-files v0.0.9
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.3.0
	github.com/ipfs/go-merkledag v0.4.1
	github.com/ipfs/go-unixfs v0.2.6
	github.com/ipld/go-car v0.3.2-0.20211001225732-32d0d9933823
	github.com/kpacha/opencensus-influxdb v0.0.0-20181102202715-663e2683a27c
	github.com/libp2p/go-libp2p v0.15.0
	github.com/libp2p/go-libp2p-core v0.9.0
	github.com/libp2p/go-libp2p-pubsub-tracer v0.0.0-20200626141350-e730b32bf1e6
	github.com/multiformats/go-multiaddr v0.4.1
	github.com/testground/sdk-go v0.2.6
	go.opencensus.io v0.23.0
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
)

// This will work in all build modes: docker:go, exec:go, and local go build.
// On docker:go and exec:go, it maps to /extra/filecoin-ffi, as it's picked up
// as an "extra source" in the manifest.
replace github.com/filecoin-project/filecoin-ffi => ../../extern/filecoin-ffi

replace github.com/filecoin-project/lotus => ../../
