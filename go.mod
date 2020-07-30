module github.com/filecoin-project/sector-storage

go 1.13

require (
	github.com/detailyang/go-fallocate v0.0.0-20180908115635-432fa640bd2e
	github.com/elastic/go-sysinfo v1.3.0
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200716204036-cddc56607e1d
	github.com/filecoin-project/go-bitfield v0.1.2
	github.com/filecoin-project/go-fil-commcid v0.0.0-20200716160307-8f644712406f
	github.com/filecoin-project/go-paramfetch v0.0.2-0.20200218225740-47c639bab663
	github.com/filecoin-project/specs-actors v0.8.2
	github.com/filecoin-project/specs-storage v0.1.1-0.20200622113353-88a9704877ea
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.4
	github.com/hashicorp/go-multierror v1.0.0
	github.com/ipfs/go-cid v0.0.6
	github.com/ipfs/go-ipfs-files v0.0.7
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200204214505-252690b78669 // indirect
	github.com/ipfs/go-log v1.0.4
	github.com/ipfs/go-log/v2 v2.0.5
	github.com/mattn/go-isatty v0.0.9 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/stretchr/testify v1.6.1
	go.opencensus.io v0.22.3
	golang.org/x/crypto v0.0.0-20200317142112-1b76d66859c6 // indirect
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b // indirect
	golang.org/x/sys v0.0.0-20200317113312-5766fd39f98d // indirect
	golang.org/x/tools v0.0.0-20200318150045-ba25ddc85566 // indirect
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
	honnef.co/go/tools v0.0.1-2020.1.3 // indirect
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
