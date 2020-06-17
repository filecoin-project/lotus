module github.com/testground/testground/plans/network

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/lotus v0.3.1-0.20200518172415-1ed618334471
	github.com/filecoin-project/sector-storage v0.0.0-20200522011946-a59ca7536a95
	github.com/filecoin-project/specs-actors v0.5.4-0.20200521014528-0df536f7e461
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-ipfs-blockstore v1.0.0
	github.com/labstack/gommon v0.3.0
	github.com/libp2p/go-libp2p v0.9.4
	github.com/libp2p/go-libp2p-core v0.5.7
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/testground/sdk-go v0.2.0
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
)

replace github.com/filecoin-project/filecoin-ffi => ../lotus/extern/filecoin-ffi

replace github.com/filecoin-project/lotus => ../lotus
