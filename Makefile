SHELL=/usr/bin/env bash

all: build
.PHONY: all

unexport GOFLAGS

GOVERSION:=$(shell go version | cut -d' ' -f 3 | sed 's/^go//' | awk -F. '{printf "%d%03d%03d", $$1, $$2, $$3}')
ifeq ($(shell expr $(GOVERSION) \< 1015005), 1)
$(warning Your Golang version is go$(shell expr $(GOVERSION) / 1000000).$(shell expr $(GOVERSION) % 1000000 / 1000).$(shell expr $(GOVERSION) % 1000))
$(error Update Golang to version to at least 1.15.5)
endif

# git modules that need to be loaded
MODULES:=

CLEAN:=
BINS:=

ldflags=-X=github.com/filecoin-project/lotus/build.CurrentCommit=+git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))
ifneq ($(strip $(LDFLAGS)),)
	ldflags+=-extldflags=$(LDFLAGS)
endif

GOFLAGS+=-ldflags="$(ldflags)"


## FFI

FFI_PATH:=extern/filecoin-ffi/
FFI_DEPS:=.install-filcrypto
FFI_DEPS:=$(addprefix $(FFI_PATH),$(FFI_DEPS))

$(FFI_DEPS): build/.filecoin-install ;

build/.filecoin-install: $(FFI_PATH)
	$(MAKE) -C $(FFI_PATH) $(FFI_DEPS:$(FFI_PATH)%=%)
	@touch $@

MODULES+=$(FFI_PATH)
BUILD_DEPS+=build/.filecoin-install
CLEAN+=build/.filecoin-install

ffi-version-check:
	@[[ "$$(awk '/const Version/{print $$5}' extern/filecoin-ffi/version.go)" -eq 3 ]] || (echo "FFI version mismatch, update submodules"; exit 1)
BUILD_DEPS+=ffi-version-check

.PHONY: ffi-version-check


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

build-devnets: build lotus-seed lotus-shed lotus-wallet lotus-gateway
.PHONY: build-devnets

debug: GOFLAGS+=-tags=debug
debug: build-devnets

2k: GOFLAGS+=-tags=2k
2k: build-devnets

calibnet: GOFLAGS+=-tags=calibnet
calibnet: build-devnets

nerpanet: GOFLAGS+=-tags=nerpanet
nerpanet: build-devnets

butterflynet: GOFLAGS+=-tags=butterflynet
butterflynet: build-devnets

lotus: $(BUILD_DEPS)
	rm -f lotus
	go build $(GOFLAGS) -o lotus ./cmd/lotus
	go run github.com/GeertJohan/go.rice/rice append --exec lotus -i ./build

.PHONY: lotus
BINS+=lotus

lotus-miner: $(BUILD_DEPS)
	rm -f lotus-miner
	go build $(GOFLAGS) -o lotus-miner ./cmd/lotus-storage-miner
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-miner -i ./build
.PHONY: lotus-miner
BINS+=lotus-miner

lotus-worker: $(BUILD_DEPS)
	rm -f lotus-worker
	go build $(GOFLAGS) -o lotus-worker ./cmd/lotus-seal-worker
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-worker -i ./build
.PHONY: lotus-worker
BINS+=lotus-worker

lotus-shed: $(BUILD_DEPS)
	rm -f lotus-shed
	go build $(GOFLAGS) -o lotus-shed ./cmd/lotus-shed
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-shed -i ./build
.PHONY: lotus-shed
BINS+=lotus-shed

lotus-gateway: $(BUILD_DEPS)
	rm -f lotus-gateway
	go build $(GOFLAGS) -o lotus-gateway ./cmd/lotus-gateway
.PHONY: lotus-gateway
BINS+=lotus-gateway

build: lotus lotus-miner lotus-worker
	@[[ $$(type -P "lotus") ]] && echo "Caution: you have \
an existing lotus binary in your PATH. This may cause problems if you don't run 'sudo make install'" || true

.PHONY: build

install: install-daemon install-miner install-worker

install-daemon:
	install -C ./lotus /usr/local/bin/lotus

install-miner:
	install -C ./lotus-miner /usr/local/bin/lotus-miner

install-worker:
	install -C ./lotus-worker /usr/local/bin/lotus-worker

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

lotus-pond: 2k
	go build -o lotus-pond ./lotuspond
.PHONY: lotus-pond
BINS+=lotus-pond

lotus-pond-front:
	(cd lotuspond/front && npm i && CI=false npm run build)
.PHONY: lotus-pond-front

lotus-pond-app: lotus-pond-front lotus-pond
.PHONY: lotus-pond-app

lotus-townhall:
	rm -f lotus-townhall
	go build -o lotus-townhall ./cmd/lotus-townhall
.PHONY: lotus-townhall
BINS+=lotus-townhall

lotus-townhall-front:
	(cd ./cmd/lotus-townhall/townhall && npm i && npm run build)
.PHONY: lotus-townhall-front

lotus-townhall-app: lotus-touch lotus-townhall-front
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-townhall -i ./cmd/lotus-townhall -i ./build
.PHONY: lotus-townhall-app

lotus-fountain:
	rm -f lotus-fountain
	go build -o lotus-fountain ./cmd/lotus-fountain
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-fountain -i ./cmd/lotus-fountain -i ./build
.PHONY: lotus-fountain
BINS+=lotus-fountain

lotus-chainwatch:
	rm -f lotus-chainwatch
	go build $(GOFLAGS) -o lotus-chainwatch ./cmd/lotus-chainwatch
.PHONY: lotus-chainwatch
BINS+=lotus-chainwatch

lotus-bench:
	rm -f lotus-bench
	go build -o lotus-bench ./cmd/lotus-bench
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-bench -i ./build
.PHONY: lotus-bench
BINS+=lotus-bench

lotus-stats:
	rm -f lotus-stats
	go build $(GOFLAGS) -o lotus-stats ./cmd/lotus-stats
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-stats -i ./build
.PHONY: lotus-stats
BINS+=lotus-stats

lotus-pcr:
	rm -f lotus-pcr
	go build $(GOFLAGS) -o lotus-pcr ./cmd/lotus-pcr
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-pcr -i ./build
.PHONY: lotus-pcr
BINS+=lotus-pcr

lotus-health:
	rm -f lotus-health
	go build -o lotus-health ./cmd/lotus-health
	go run github.com/GeertJohan/go.rice/rice append --exec lotus-health -i ./build
.PHONY: lotus-health
BINS+=lotus-health

lotus-wallet:
	rm -f lotus-wallet
	go build -o lotus-wallet ./cmd/lotus-wallet
.PHONY: lotus-wallet
BINS+=lotus-wallet

lotus-keygen:
	rm -f lotus-keygen
	go build -o lotus-keygen ./cmd/lotus-keygen
.PHONY: lotus-keygen
BINS+=lotus-keygen

testground:
	go build -tags testground -o /dev/null ./cmd/lotus
.PHONY: testground
BINS+=testground


tvx:
	rm -f tvx
	go build -o tvx ./cmd/tvx
.PHONY: tvx
BINS+=tvx

install-chainwatch: lotus-chainwatch
	install -C ./lotus-chainwatch /usr/local/bin/lotus-chainwatch

# SYSTEMD

install-daemon-service: install-daemon
	mkdir -p /etc/systemd/system
	mkdir -p /var/log/lotus
	install -C -m 0644 ./scripts/lotus-daemon.service /etc/systemd/system/lotus-daemon.service
	systemctl daemon-reload
	@echo
	@echo "lotus-daemon service installed. Don't forget to run 'sudo systemctl start lotus-daemon' to start it and 'sudo systemctl enable lotus-daemon' for it to be enabled on startup."

install-miner-service: install-miner install-daemon-service
	mkdir -p /etc/systemd/system
	mkdir -p /var/log/lotus
	install -C -m 0644 ./scripts/lotus-miner.service /etc/systemd/system/lotus-miner.service
	systemctl daemon-reload
	@echo
	@echo "lotus-miner service installed. Don't forget to run 'sudo systemctl start lotus-miner' to start it and 'sudo systemctl enable lotus-miner' for it to be enabled on startup."

install-chainwatch-service: install-chainwatch install-daemon-service
	mkdir -p /etc/systemd/system
	mkdir -p /var/log/lotus
	install -C -m 0644 ./scripts/lotus-chainwatch.service /etc/systemd/system/lotus-chainwatch.service
	systemctl daemon-reload
	@echo
	@echo "chainwatch service installed. Don't forget to run 'sudo systemctl start lotus-chainwatch' to start it and 'sudo systemctl enable lotus-chainwatch' for it to be enabled on startup."

install-main-services: install-miner-service

install-all-services: install-main-services install-chainwatch-service

install-services: install-main-services

clean-daemon-service: clean-miner-service clean-chainwatch-service
	-systemctl stop lotus-daemon
	-systemctl disable lotus-daemon
	rm -f /etc/systemd/system/lotus-daemon.service
	systemctl daemon-reload

clean-miner-service:
	-systemctl stop lotus-miner
	-systemctl disable lotus-miner
	rm -f /etc/systemd/system/lotus-miner.service
	systemctl daemon-reload

clean-chainwatch-service:
	-systemctl stop lotus-chainwatch
	-systemctl disable lotus-chainwatch
	rm -f /etc/systemd/system/lotus-chainwatch.service
	systemctl daemon-reload

clean-main-services: clean-daemon-service

clean-all-services: clean-main-services

clean-services: clean-all-services

# MISC

buildall: $(BINS)

completions:
	./scripts/make-completions.sh lotus
	./scripts/make-completions.sh lotus-miner
.PHONY: completions

install-completions:
	mkdir -p /usr/share/bash-completion/completions /usr/local/share/zsh/site-functions/
	install -C ./scripts/bash-completion/lotus /usr/share/bash-completion/completions/lotus
	install -C ./scripts/bash-completion/lotus-miner /usr/share/bash-completion/completions/lotus-miner
	install -C ./scripts/zsh-completion/lotus /usr/local/share/zsh/site-functions/_lotus
	install -C ./scripts/zsh-completion/lotus-miner /usr/local/share/zsh/site-functions/_lotus-miner

clean:
	rm -rf $(CLEAN) $(BINS)
	-$(MAKE) -C $(FFI_PATH) clean
.PHONY: clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean

type-gen: api-gen
	go run ./gen/main.go
	go generate -x ./...
	goimports -w api/

method-gen: api-gen
	(cd ./lotuspond/front/src/chain && go run ./methodgen.go)

actors-gen:
	go run ./chain/actors/agen
	go fmt ./...

api-gen:
	go run ./gen/api
	goimports -w api
	goimports -w api
.PHONY: api-gen

appimage: $(BUILD_DEPS)
	rm -rf appimage-builder-cache || true
	rm AppDir/io.filecoin.lotus.desktop || true
	rm AppDir/icon.svg || true
	rm Appdir/AppRun || true
	mkdir -p AppDir/usr/bin
	rm -rf lotus
	go run github.com/GeertJohan/go.rice/rice embed-go -i ./build
	go build $(GOFLAGS) -o lotus ./cmd/lotus
	cp ./lotus AppDir/usr/bin/
	appimage-builder
docsgen: docsgen-md docsgen-openrpc

docsgen-md-bin: api-gen actors-gen
	go build $(GOFLAGS) -o docgen-md ./api/docgen/cmd
docsgen-openrpc-bin: api-gen actors-gen
	go build $(GOFLAGS) -o docgen-openrpc ./api/docgen-openrpc/cmd

docsgen-md: docsgen-md-full docsgen-md-storage docsgen-md-worker

docsgen-md-full: docsgen-md-bin
	./docgen-md "api/api_full.go" "FullNode" "api" "./api" > documentation/en/api-v1-unstable-methods.md
	./docgen-md "api/v0api/full.go" "FullNode" "v0api" "./api/v0api" > documentation/en/api-v0-methods.md
docsgen-md-storage: docsgen-md-bin
	./docgen-md "api/api_storage.go" "StorageMiner" "api" "./api" > documentation/en/api-v0-methods-miner.md
docsgen-md-worker: docsgen-md-bin
	./docgen-md "api/api_worker.go" "Worker" "api" "./api" > documentation/en/api-v0-methods-worker.md

docsgen-openrpc: docsgen-openrpc-full docsgen-openrpc-storage docsgen-openrpc-worker

docsgen-openrpc-full: docsgen-openrpc-bin
	./docgen-openrpc "api/api_full.go" "FullNode" "api" "./api" -gzip > build/openrpc/full.json.gz
docsgen-openrpc-storage: docsgen-openrpc-bin
	./docgen-openrpc "api/api_storage.go" "StorageMiner" "api" "./api" -gzip > build/openrpc/miner.json.gz
docsgen-openrpc-worker: docsgen-openrpc-bin
	./docgen-openrpc "api/api_worker.go" "Worker" "api" "./api" -gzip > build/openrpc/worker.json.gz

.PHONY: docsgen docsgen-md-bin docsgen-openrpc-bin

gen: actors-gen type-gen method-gen docsgen api-gen
.PHONY: gen

print-%:
	@echo $*=$($*)
