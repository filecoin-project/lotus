module github.com/filecoin-project/oni/lotus-soup

go 1.14

require (
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/codeskyblue/go-sh v0.0.0-20200712050446-30169cf553fe
	github.com/davecgh/go-spew v1.1.1
	github.com/drand/drand v1.0.3-0.20200714175734-29705eaf09d4
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/go-fil-markets v0.5.2
	github.com/filecoin-project/go-jsonrpc v0.1.1-0.20200602181149-522144ab4e24
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/lotus v0.4.3-0.20200729013254-5df0ee7935e0
	github.com/filecoin-project/sector-storage v0.0.0-20200727112136-9377cb376d25
	github.com/filecoin-project/specs-actors v0.8.1-0.20200728182452-1476088f645b
	github.com/filecoin-project/storage-fsm v0.0.0-20200728185042-33f96f051f20
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.4
	github.com/hashicorp/go-multierror v1.1.0
	github.com/influxdata/influxdb v1.8.0 // indirect
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log/v2 v2.1.2-0.20200626104915-0016c0b4b3e4
	github.com/ipfs/go-merkledag v0.3.1
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipld/go-car v0.1.1-0.20200526133713-1c7508d55aae
	github.com/kpacha/opencensus-influxdb v0.0.0-20181102202715-663e2683a27c
	github.com/libp2p/go-libp2p v0.10.0
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/libp2p/go-libp2p-pubsub-tracer v0.0.0-20200626141350-e730b32bf1e6
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multiaddr-net v0.1.5
	github.com/testground/sdk-go v0.2.3-0.20200706132230-6a65ddac2d8c
	go.opencensus.io v0.22.4
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
)

// This will work in all build modes: docker:go, exec:go, and local go build.
// On docker:go and exec:go, it maps to /extra/filecoin-ffi, as it's picked up
// as an "extra source" in the manifest.
replace github.com/filecoin-project/filecoin-ffi => ../extra/filecoin-ffi
