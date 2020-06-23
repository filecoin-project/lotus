module github.com/filecoin-project/oni/lotus-testground

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/lotus v0.4.1-0.20200622193952-c33d9e966837
	github.com/filecoin-project/sector-storage v0.0.0-20200618073200-d9de9b7cb4b4
	github.com/filecoin-project/specs-actors v0.6.2-0.20200617175406-de392ca14121
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-ipfs-blockstore v1.0.0
	github.com/libp2p/go-libp2p v0.9.4
	github.com/libp2p/go-libp2p-core v0.5.7
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/testground/sdk-go v0.2.3-0.20200617132925-2e4d69f9ba38
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
)

// This will work in all build modes: docker:go, exec:go, and local go build.
// On docker:go and exec:go, it maps to /extra/filecoin-ffi, as it's picked up
// as an "extra source" in the manifest.
replace github.com/filecoin-project/filecoin-ffi => ../extra/filecoin-ffi
