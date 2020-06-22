module github.com/filecoin-project/oni/lotus-testground

go 1.14

require (
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/lotus v0.3.1-0.20200518172415-1ed618334471
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

replace github.com/filecoin-project/filecoin-ffi => ../lotus/extern/filecoin-ffi

replace github.com/filecoin-project/lotus => ../lotus
