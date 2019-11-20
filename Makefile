all: build
.PHONY: all

GOVERSION:=$(shell go version | cut -d' ' -f 3 | cut -d. -f 2)
ifeq ($(shell expr $(GOVERSION) \< 13), 1)
$(warning Your Golang version is go 1.$(GOVERSION))
$(error Update Golang to version $(shell grep '^go' go.mod))
endif

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

CLEAN+=build/.update-modules

deps: $(BUILD_DEPS)
.PHONY: deps

lotus: $(BUILD_DEPS)
	rm -f lotus
	go build -o lotus ./cmd/lotus
	go run github.com/GeertJohan/go.rice/rice append --exec lotus -i ./build

.PHONY: lotus
CLEAN+=lotus

lotus-storage-miner: $(BUILD_DEPS)
	rm -f lotus-storage-miner
	go build -o lotus-storage-miner ./cmd/lotus-storage-miner
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-storage-miner -i ./build

.PHONY: lotus-storage-miner

CLEAN+=lotus-storage-miner

build: lotus lotus-storage-miner

.PHONY: build

install:
	install -C ./lotus /usr/local/bin/lotus
	install -C ./lotus-storage-miner /usr/local/bin/lotus-storage-miner

benchmarks:
	go run github.com/whyrusleeping/bencher ./... > bench.json
	@echo Submitting results
	@curl -X POST 'http://benchmark.kittyhawk.wtf/benchmark' -d '@bench.json' -u "${benchmark_http_cred}"
.PHONY: benchmarks

pond: build
	go build -o pond ./lotuspond
	(cd lotuspond/front && npm i && npm run build)
.PHONY: pond

townhall:
	rm -f townhall
	go build -o townhall ./cmd/lotus-townhall
	(cd ./cmd/lotus-townhall/townhall && npm i && npm run build)
	go run github.com/GeertJohan/go.rice/rice append --exec townhall -i ./cmd/lotus-townhall -i ./build
.PHONY: townhall

fountain:
	rm -f fountain
	go build -o fountain ./cmd/lotus-fountain
	go run github.com/GeertJohan/go.rice/rice append --exec fountain -i ./cmd/lotus-fountain
.PHONY: fountain

chainwatch:
	rm -f chainwatch
	go build -o chainwatch ./cmd/lotus-chainwatch
	go run github.com/GeertJohan/go.rice/rice append --exec chainwatch -i ./cmd/lotus-chainwatch
.PHONY: chainwatch

stats:
	rm -f stats
	go build -o stats ./tools/stats
.PHONY: stats

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
