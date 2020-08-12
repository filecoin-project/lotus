module github.com/filecoin-project/storage-fsm

go 1.13

require (
	github.com/filecoin-project/go-address v0.0.3
	github.com/filecoin-project/go-cbor-util v0.0.0-20191219014500-08c40a1e63a2
	github.com/filecoin-project/go-fil-commcid v0.0.0-20200716160307-8f644712406f
	github.com/filecoin-project/go-padreader v0.0.0-20200210211231-548257017ca6
	github.com/filecoin-project/go-paramfetch v0.0.2-0.20200701152213-3e0f0afdc261 // indirect
	github.com/filecoin-project/go-statemachine v0.0.0-20200730031800-c3336614d2a7
	github.com/filecoin-project/sector-storage v0.0.0-20200810171746-eac70842d8e0
	github.com/filecoin-project/specs-actors v0.8.7-0.20200811223639-8db91253c07a
	github.com/filecoin-project/specs-storage v0.1.1-0.20200730063404-f7db367e9401
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-log/v2 v2.0.5
	github.com/stretchr/testify v1.6.1
	github.com/whyrusleeping/cbor-gen v0.0.0-20200811225321-4fed70922d45
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	gotest.tools v2.2.0+incompatible
)

replace github.com/golangci/golangci-lint => github.com/golangci/golangci-lint v1.18.0

replace github.com/filecoin-project/sector-storage => ../sector-storage

replace github.com/filecoin-project/filecoin-ffi => ../filecoin-ffi
