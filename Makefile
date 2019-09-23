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

$(MODULES): build/.update-modules ;

# dummy file that marks the last time modules were updated
build/.update-modules:
	git submodule update --init --recursive
	touch $@

# end git modules

## PROOFS

bin/ipget:
	./bin/dist_get /ipns/dist.ipfs.io ipget bin/ipget v0.4.0

PARAM_SECTOR_SIZES:=1024 16777216 268435456 1073741824
PARAM_SECTOR_SIZES:=$(addprefix build/.params-,$(PARAM_SECTOR_SIZES))

$(PARAM_SECTOR_SIZES): build/proof-params/parameters.json bin/ipget
	./build/proof-params/paramfetch.sh
	touch $@

BUILD_DEPS+=build/.params-1024
CLEAN+=build/.params-1024

CLEAN+=build/.update-modules

deps: $(BUILD_DEPS)
.PHONY: deps

build: $(BUILD_DEPS)
	go build -o lotus ./cmd/lotus
	go build -o lotus-storage-miner ./cmd/lotus-storage-miner
.PHONY: build

benchmarks:
	go run github.com/whyrusleeping/bencher ./... > bench.json
	@echo Submitting results
	@curl -X POST 'http://benchmark.kittyhawk.wtf/benchmark' -d '@bench.json' -u "${benchmark_http_cred}"
.PHONY: benchmarks

pond: build
	go build -o pond ./lotuspond
	(cd lotuspond/front && npm i && npm run build)
.PHONY: pond

clean:
	rm -rf $(CLEAN)
	-$(MAKE) -C $(BLS_PATH) clean
	-$(MAKE) -C $(SECTOR_BUILDER_PATH) clean
.PHONY: clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean

type-gen:
	go run ./gen/main.go

print-%:
	@echo $*=$($*)
