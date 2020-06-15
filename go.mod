module github.com/filecoin-project/storage-fsm

go 1.13

require (
	github.com/filecoin-project/go-address v0.0.2-0.20200218010043-eb9bb40ed5be
	github.com/filecoin-project/go-cbor-util v0.0.0-20191219014500-08c40a1e63a2
	github.com/filecoin-project/go-fil-commcid v0.0.0-20200208005934-2b8bd03caca5
	github.com/filecoin-project/go-padreader v0.0.0-20200210211231-548257017ca6
	github.com/filecoin-project/go-paramfetch v0.0.2-0.20200218225740-47c639bab663 // indirect
	github.com/filecoin-project/go-statemachine v0.0.0-20200226041606-2074af6d51d9
	github.com/filecoin-project/sector-storage v0.0.0-20200615154852-728a47ab99d6
	github.com/filecoin-project/specs-actors v0.6.0
	github.com/filecoin-project/specs-storage v0.1.0
	github.com/ipfs/go-cid v0.0.5
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-hamt-ipld v0.0.15-0.20200204200533-99b8553ef242 // indirect
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200204214505-252690b78669 // indirect
	github.com/ipfs/go-log/v2 v2.0.3
	github.com/stretchr/testify v1.4.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20200414195334-429a0b5e922e
	go.uber.org/zap v1.14.1 // indirect
	golang.org/x/crypto v0.0.0-20200317142112-1b76d66859c6 // indirect
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b // indirect
	golang.org/x/sys v0.0.0-20200317113312-5766fd39f98d // indirect
	golang.org/x/tools v0.0.0-20200318150045-ba25ddc85566 // indirect
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
	gotest.tools v2.2.0+incompatible
	honnef.co/go/tools v0.0.1-2020.1.3 // indirect
)

replace github.com/golangci/golangci-lint => github.com/golangci/golangci-lint v1.18.0

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
