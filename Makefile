all: build
.PHONY: all


# git modules that need to be loaded
MODULES:=

CLEAN:=

## BLS

BLS_PATH:=extern/go-bls-sigs/
BLS_DEPS:=libbls_signatures.a libbls_signatures.pc libbls_signatures.h
BLS_DEPS:=$(addprefix $(BLS_PATH),$(BLS_DEPS))

$(BLS_DEPS): build/.bls-install ;

build/.bls-install: $(BLS_PATH)
	$(MAKE) -C $(BLS_PATH) $(BLS_DEPS:$(BLS_PATH)%=%)
	@touch $@

MODULES+=$(BLS_PATH)
BUILD_DEPS+=build/.bls-install
CLEAN+=build/.bls-install

## SECTOR BUILDER

SECTOR_BUILDER_PATH:=extern/go-sectorbuilder/
SECTOR_BUILDER_DEPS:=libsector_builder_ffi.a sector_builder_ffi.pc sector_builder_ffi.h
SECTOR_BUILDER_DEPS:=$(addprefix $(SECTOR_BUILDER_PATH),$(SECTOR_BUILDER_DEPS))

$(SECTOR_BUILDER_DEPS): build/.sector-builder-install ;

build/.sector-builder-install: $(SECTOR_BUILDER_PATH)
	$(MAKE) -C $(SECTOR_BUILDER_PATH) $(SECTOR_BUILDER_DEPS:$(SECTOR_BUILDER_PATH)%=%)
	@touch $@

MODULES+=$(SECTOR_BUILDER_PATH)
BUILD_DEPS+=build/.sector-builder-install
CLEAN+=build/.sector-builder-install

## PROOFS

PROOFS_PATH:=extern/go-fil-proofs/
PROOFS_DEPS:=bin/paramcache bin/paramfetch misc/parameters.json
PROOFS_DEPS:=$(addprefix $(PROOFS_PATH),$(PROOFS_DEPS))

$(PROOFS_DEPS): build/.proofs-install ;

build/.proofs-install: $(PROOFS_PATH)
	$(MAKE) -C $(PROOFS_PATH) $(PROOFS_DEPS:$(PROOFS_PATH)%=%)
	@touch $@

MODULES+=$(PROOFS_PATH)
BUILD_DEPS+=build/.proofs-install
CLEAN+=build/.proofs-install

PARAM_SECTOR_SIZES:=1024 268435456
PARAM_SECTOR_SIZES:=$(addprefix params-,$(PARAM_SECTOR_SIZES))

$(PARAM_SECTOR_SIZES): extern/go-fil-proofs/bin/paramfetch extern/go-fil-proofs/misc/parameters.json
	IPFS_LOGGING=info ./extern/go-fil-proofs/bin/paramfetch -z $(subst params-,,$@) --verbose --json=./extern/go-fil-proofs/misc/parameters.json --ipget-args="--node spawn -p /ip4/138.201.67.219/tcp/4002/ws/ipfs/QmUd6zHcbkbcs7SMxwLs48qZVX3vpcM8errYS7xEczwRMA -p /ip4/138.201.67.218/tcp/4002/ws/ipfs/QmbVWZQhCGrS7DhgLqWbgvdmKN7JueKCREVanfnVpgyq8x -p /ip4/94.130.135.167/tcp/4002/ws/ipfs/QmUEMvxS2e7iDrereVYc5SWPauXPyNwxcy9BXZrC1QTcHE -p /ip4/138.201.68.74/tcp/4001/ipfs/QmdnXwLrC8p1ueiq2Qya8joNvk3TVVDAut7PrikmZwubtR -p /ip4/138.201.67.220/tcp/4001/ipfs/QmNSYxZAiJHeLdkBg38roksAR9So7Y5eojks1yjEcUtZ7i" --ipget-version=v0.4.0
.PHONY: $(PARAM_SECTOR_SIZES)

BUILD_DEPS+=params-1024

paramcache: extern/go-fil-proofs/bin/paramcache
	RUST_LOG=info ./extern/go-fil-proofs/bin/paramcache

# end git modules

$(MODULES): build/.update-modules ;

# dummy file that marks the last time modules were updated
build/.update-modules:
	git submodule update --init --recursive
	touch $@

CLEAN+=build/.update-modules

deps: $(BUILD_DEPS)
.PHONY: deps

build: $(BUILD_DEPS)
	go build -o lotus ./cmd/lotus
	go build -o lotus-storage-miner ./cmd/lotus-storage-miner
.PHONY: build

benchmarks:
	go run github.com/whyrusleeping/bencher ./... > bench.json
	curl -X POST 'http://benchmark.kittyhawk.wtf/benchmark' -d '@bench.json' -u "${benchmark_http_cred}"
.PHONY: benchmarks

pond: build
	go build -o pond ./lotuspond
	(cd lotuspond/front && npm i && npm run build)
.PHONY: pond

clean:
	rm -rf $(CLEAN)
	-$(MAKE) -C $(BLS_PATH) clean
	-$(MAKE) -C $(SECTOR_BUILDER_PATH) clean
	-$(MAKE) -C $(PROOFS_PATH) clean
.PHONY: clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean

type-gen:
	rm -f ./chain/types/cbor_gen.go
	go run ./gen/main.go

print-%:
	@echo $*=$($*)
