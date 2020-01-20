SHELL=/usr/bin/env bash

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
BINS:=
GOFLAGS+=-ldflags='-X="github.com/filecoin-project/lotus/build".CurrentCommit=+git$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))'

## FFI

FFI_PATH:=extern/filecoin-ffi/
FFI_DEPS:=libfilecoin.a filecoin.pc filecoin.h
FFI_DEPS:=$(addprefix $(FFI_PATH),$(FFI_DEPS))

$(FFI_DEPS): build/.filecoin-install ;

build/.filecoin-install: $(FFI_PATH)
	$(MAKE) -C $(FFI_PATH) $(FFI_DEPS:$(FFI_PATH)%=%)
	@touch $@

MODULES+=$(FFI_PATH)
BUILD_DEPS+=build/.filecoin-install
CLEAN+=build/.filecoin-install

$(MODULES): build/.update-modules ;

# dummy file that marks the last time modules were updated
build/.update-modules:
	git submodule update --init --recursive
	touch $@

# end git modules

## MAIN BINARIES

CLEAN+=build/.update-modules

deps: $(BUILD_DEPS)
.PHONY: deps

debug: GOFLAGS+=-tags=debug
debug: lotus lotus-storage-miner lotus-seal-worker lotus-seed

lotus: $(BUILD_DEPS)
	rm -f lotus
	go build $(GOFLAGS) -o lotus ./cmd/lotus
	go run github.com/GeertJohan/go.rice/rice append --exec lotus -i ./build

.PHONY: lotus
BINS+=lotus

lotus-storage-miner: $(BUILD_DEPS)
	rm -f lotus-storage-miner
	go build $(GOFLAGS) -o lotus-storage-miner ./cmd/lotus-storage-miner
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-storage-miner -i ./build
.PHONY: lotus-storage-miner
BINS+=lotus-storage-miner

lotus-seal-worker: $(BUILD_DEPS)
	rm -f lotus-seal-worker
	go build $(GOFLAGS) -o lotus-seal-worker ./cmd/lotus-seal-worker
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-seal-worker -i ./build
.PHONY: lotus-seal-worker
BINS+=lotus-seal-worker

lotus-shed: $(BUILD_DEPS)
	rm -f lotus-shed
	go build $(GOFLAGS) -o lotus-shed ./cmd/lotus-shed
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-shed -i ./build
.PHONY: lotus-seal-worker
BINS+=lotus-seal-worker

build: lotus lotus-storage-miner lotus-seal-worker
	@[[ $$(type -P "lotus") ]] && echo "Caution: you have \
an existing lotus binary in your PATH. This may cause problems if you don't run 'sudo make install'" || true

.PHONY: build

install:
	install -C ./lotus /usr/local/bin/lotus
	install -C ./lotus-storage-miner /usr/local/bin/lotus-storage-miner
	install -C ./lotus-seal-worker /usr/local/bin/lotus-seal-worker

# TOOLS

lotus-seed: $(BUILD_DEPS)
	rm -f lotus-seed
	go build $(GOFLAGS) -o lotus-seed ./cmd/lotus-seed
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-seed -i ./build

.PHONY: lotus-seed
BINS+=lotus-seed

benchmarks:
	go run github.com/whyrusleeping/bencher ./... > bench.json
	@echo Submitting results
	@curl -X POST 'http://benchmark.kittyhawk.wtf/benchmark' -d '@bench.json' -u "${benchmark_http_cred}"
.PHONY: benchmarks

pond: build
	go build -o pond ./lotuspond
	(cd lotuspond/front && npm i && CI=false npm run build)
.PHONY: pond
BINS+=pond

townhall:
	rm -f townhall
	go build -o townhall ./cmd/lotus-townhall
	(cd ./cmd/lotus-townhall/townhall && npm i && npm run build)
	go run github.com/GeertJohan/go.rice/rice append --exec townhall -i ./cmd/lotus-townhall -i ./build
.PHONY: townhall
BINS+=townhall

fountain:
	rm -f fountain
	go build -o fountain ./cmd/lotus-fountain
	go run github.com/GeertJohan/go.rice/rice append --exec fountain -i ./cmd/lotus-fountain -i ./build
.PHONY: fountain
BINS+=fountain

chainwatch:
	rm -f chainwatch
	go build -o chainwatch ./cmd/lotus-chainwatch
	go run github.com/GeertJohan/go.rice/rice append --exec chainwatch -i ./cmd/lotus-chainwatch -i ./build
.PHONY: chainwatch
BINS+=chainwatch

bench:
	rm -f bench
	go build -o bench ./cmd/lotus-bench
	go run github.com/GeertJohan/go.rice/rice append --exec bench -i ./build
.PHONY: bench
BINS+=bench

stats:
	rm -f stats
	go build -o stats ./tools/stats
	go run github.com/GeertJohan/go.rice/rice append --exec stats -i ./build
.PHONY: stats
BINS+=stats

health:
	rm -f lotus-health
	go build -o lotus-health ./cmd/lotus-health
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-health -i ./build

.PHONY: health
BINS+=health

# MISC

buildall: $(BINS)

clean:
	rm -rf $(CLEAN) $(BINS)
	-$(MAKE) -C $(FFI_PATH) clean
.PHONY: clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean

type-gen:
	go run ./gen/main.go

print-%:
	@echo $*=$($*)
