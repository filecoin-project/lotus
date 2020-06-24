module github.com/filecoin-project/oni/lotus-soup

go 1.14

require (
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/lotus v0.4.1-0.20200623104442-68d38eff33e4
	github.com/filecoin-project/specs-actors v0.6.2-0.20200617175406-de392ca14121
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-log/v2 v2.1.2-0.20200609205458-f8d20c392cb7
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/testground/sdk-go v0.2.3-0.20200617132925-2e4d69f9ba38
)

// This will work in all build modes: docker:go, exec:go, and local go build.
// On docker:go and exec:go, it maps to /extra/filecoin-ffi, as it's picked up
// as an "extra source" in the manifest.
replace github.com/filecoin-project/filecoin-ffi => ../extra/filecoin-ffi
