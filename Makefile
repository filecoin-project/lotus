SHELL=/usr/bin/env bash

all: build  ## Build all main binaries (default target)

.PHONY: all

unexport GOFLAGS

GOCC?=go

GOVERSION:=$(shell $(GOCC) version | tr ' ' '\n' | grep go1 | sed 's/^go//' | awk -F. '{printf "%d%03d%03d", $$1, $$2, $$3}')
GOVERSIONMIN:=$(shell cat GO_VERSION_MIN | awk -F. '{printf "%d%03d%03d", $$1, $$2, $$3}')

ifeq ($(shell expr $(GOVERSION) \< $(GOVERSIONMIN)), 1)
$(warning Your Golang version is go$(shell expr $(GOVERSION) / 1000000).$(shell expr $(GOVERSION) % 1000000 / 1000).$(shell expr $(GOVERSION) % 1000))
$(error Update Golang to version to at least $(shell cat GO_VERSION_MIN))
endif

GOLANGCI_LINT_VERSION=v1.64.8

# git modules that need to be loaded
MODULES:=

CLEAN:=
BINS:=

ldflags=-X=github.com/filecoin-project/lotus/build.CurrentCommit=+git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))
ifneq ($(strip $(LDFLAGS)),)
	ldflags+=-extldflags=$(LDFLAGS)
endif

GOFLAGS+=-ldflags='$(ldflags)'

FIX_IMPORTS = $(GOCC) run ./scripts/fiximports

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

ffi-version-check:  ## Check FFI version compatibility
	@[[ "$$(awk '/const Version/{print $$5}' extern/filecoin-ffi/version.go)" -eq 3 ]] || (echo "FFI version mismatch, update submodules"; exit 1)
BUILD_DEPS+=ffi-version-check

.PHONY: ffi-version-check

$(MODULES): build/.update-modules ;  ## Update git submodules
# dummy file that marks the last time modules were updated
build/.update-modules:
	git submodule update --init --recursive
	touch $@

# end git modules

## MAIN BINARIES

CLEAN+=build/.update-modules

deps: $(BUILD_DEPS)  ## Install build dependencies
.PHONY: deps

# Network-specific flags
DEBUG_FLAGS=-tags=debug
TWOK_FLAGS=-tags=2k
CALIBNET_FLAGS=-tags=calibnet
BUTTERFLYNET_FLAGS=-tags=butterflynet
INTEROPNET_FLAGS=-tags=interopnet

# Network-specific pattern rules
debug-%:
	$(MAKE) $* GOFLAGS='$(GOFLAGS) $(DEBUG_FLAGS)'
2k-%:
	$(MAKE) $* GOFLAGS='$(GOFLAGS) $(TWOK_FLAGS)'
calibnet-%:
	$(MAKE) $* GOFLAGS='$(GOFLAGS) $(CALIBNET_FLAGS)'
butterflynet-%:
	$(MAKE) $* GOFLAGS='$(GOFLAGS) $(BUTTERFLYNET_FLAGS)'
interopnet-%:
	$(MAKE) $* GOFLAGS='$(GOFLAGS) $(INTEROPNET_FLAGS)'

build-devnets: build lotus-seed lotus-shed
.PHONY: build-devnets

# For backward compatibility
debug:
	@printf "\033[33m'make debug' builds all devnet binaries. Use 'make debug-<binary>' targets for individual binaries.\033[0m\n"
	@printf "Example: make debug-lotus debug-lotus-miner\n\n"
	$(MAKE) debug-lotus debug-lotus-miner debug-lotus-worker debug-lotus-seed debug-lotus-shed
.PHONY: debug

2k:
	@printf "\033[33m'make 2k' builds all devnet binaries. Use 'make 2k-<binary>' targets for individual binaries.\033[0m\n"
	@printf "Example: make 2k-lotus 2k-lotus-miner\n\n"
	$(MAKE) 2k-lotus 2k-lotus-miner 2k-lotus-worker 2k-lotus-seed 2k-lotus-shed
.PHONY: 2k

calibnet:
	@printf "\033[33m'make calibnet' builds all devnet binaries. Use 'make calibnet-<binary>' targets for individual binaries.\033[0m\n"
	@printf "Example: make calibnet-lotus calibnet-lotus-miner\n\n"
	$(MAKE) calibnet-lotus calibnet-lotus-miner calibnet-lotus-worker calibnet-lotus-seed calibnet-lotus-shed
.PHONY: calibnet

butterflynet:
	@printf "\033[33m'make butterflynet' builds all devnet binaries. Use 'make butterflynet-<binary>' targets for individual binaries.\033[0m\n"
	@printf "Example: make butterflynet-lotus butterflynet-lotus-miner\n\n"
	$(MAKE) butterflynet-lotus butterflynet-lotus-miner butterflynet-lotus-worker butterflynet-lotus-seed butterflynet-lotus-shed
.PHONY: butterflynet

interopnet:
	@printf "\033[33m'make interopnet' builds all devnet binaries. Use 'make interopnet-<binary>' targets for individual binaries.\033[0m\n"
	@printf "Example: make interopnet-lotus interopnet-lotus-miner\n\n"
	$(MAKE) interopnet-lotus interopnet-lotus-miner interopnet-lotus-worker interopnet-lotus-seed interopnet-lotus-shed
.PHONY: interopnet

lotus: $(BUILD_DEPS)  ## Build the main Lotus binary
	rm -f lotus
	$(GOCC) build $(GOFLAGS) -o lotus ./cmd/lotus

.PHONY: lotus
BINS+=lotus

lotus-miner: $(BUILD_DEPS)  ## Build the Lotus miner binary
	rm -f lotus-miner
	$(GOCC) build $(GOFLAGS) -o lotus-miner ./cmd/lotus-miner
.PHONY: lotus-miner
BINS+=lotus-miner

lotus-worker: $(BUILD_DEPS)  ## Build the Lotus worker binary
	rm -f lotus-worker
	$(GOCC) build $(GOFLAGS) -o lotus-worker ./cmd/lotus-worker
.PHONY: lotus-worker
BINS+=lotus-worker

lotus-shed: $(BUILD_DEPS)  ## Build the Lotus shed tool
	rm -f lotus-shed
	$(GOCC) build $(GOFLAGS) -o lotus-shed ./cmd/lotus-shed
.PHONY: lotus-shed
BINS+=lotus-shed

lotus-gateway: $(BUILD_DEPS)  ## Build the Lotus gateway
	rm -f lotus-gateway
	$(GOCC) build $(GOFLAGS) -o lotus-gateway ./cmd/lotus-gateway
.PHONY: lotus-gateway
BINS+=lotus-gateway

build: lotus lotus-miner lotus-worker  ## Build all main binaries
	@[[ $$(type -P "lotus") ]] && echo "Caution: you have \
an existing lotus binary in your PATH. This may cause problems if you don't run 'sudo make install'" || true

.PHONY: build

install: install-daemon install-miner install-worker  ## Install all binaries

install-daemon:  ## Install the Lotus daemon
	install -C ./lotus /usr/local/bin/lotus

install-miner:  ## Install the Lotus miner
	install -C ./lotus-miner /usr/local/bin/lotus-miner

install-worker:  ## Install the Lotus worker
	install -C ./lotus-worker /usr/local/bin/lotus-worker

install-app:  ## Install a specified app
	install -C ./$(APP) /usr/local/bin/$(APP)

uninstall: uninstall-daemon uninstall-miner uninstall-worker  ## Uninstall all binaries
.PHONY: uninstall

uninstall-daemon:  ## Uninstall the Lotus daemon
	rm -f /usr/local/bin/lotus

uninstall-miner:  ## Uninstall the Lotus miner
	rm -f /usr/local/bin/lotus-miner

uninstall-worker:  ## Uninstall the Lotus worker
	rm -f /usr/local/bin/lotus-worker

# TOOLS

lotus-seed: $(BUILD_DEPS) ## Build the Lotus seed tool
	rm -f lotus-seed
	$(GOCC) build $(GOFLAGS) -o lotus-seed ./cmd/lotus-seed

.PHONY: lotus-seed
BINS+=lotus-seed

benchmarks:  ## Run benchmarks and submit results
	$(GOCC) run github.com/whyrusleeping/bencher ./... > bench.json
	@echo Submitting results
	@curl -X POST 'http://benchmark.kittyhawk.wtf/benchmark' -d '@bench.json' -u "${benchmark_http_cred}"
.PHONY: benchmarks

lotus-fountain:  ## Build the Lotus fountain tool
	rm -f lotus-fountain
	$(GOCC) build $(GOFLAGS) -o lotus-fountain ./cmd/lotus-fountain
	$(GOCC) run github.com/GeertJohan/go.rice/rice append --exec lotus-fountain -i ./cmd/lotus-fountain -i ./build
.PHONY: lotus-fountain
BINS+=lotus-fountain

lotus-bench:  ## Build the Lotus bench tool
	rm -f lotus-bench
	$(GOCC) build $(GOFLAGS) -o lotus-bench ./cmd/lotus-bench
.PHONY: lotus-bench
BINS+=lotus-bench

lotus-stats:  ## Build the Lotus stats tool
	rm -f lotus-stats
	$(GOCC) build $(GOFLAGS) -o lotus-stats ./cmd/lotus-stats
.PHONY: lotus-stats
BINS+=lotus-stats

lotus-pcr:  ## Build the Lotus PCR tool
	rm -f lotus-pcr
	$(GOCC) build $(GOFLAGS) -o lotus-pcr ./cmd/lotus-pcr
.PHONY: lotus-pcr
BINS+=lotus-pcr

lotus-health:  ## Build the Lotus health tool
	rm -f lotus-health
	$(GOCC) build -o lotus-health ./cmd/lotus-health
.PHONY: lotus-health
BINS+=lotus-health

lotus-wallet: $(BUILD_DEPS)  ## Build the Lotus wallet tool
	rm -f lotus-wallet
	$(GOCC) build $(GOFLAGS) -o lotus-wallet ./cmd/lotus-wallet
.PHONY: lotus-wallet
BINS+=lotus-wallet

lotus-keygen:  ## Build the Lotus keygen tool
	rm -f lotus-keygen
	$(GOCC) build -o lotus-keygen ./cmd/lotus-keygen
.PHONY: lotus-keygen
BINS+=lotus-keygen

testground:  ## Build for testground
	$(GOCC) build -tags testground -o /dev/null ./cmd/lotus
.PHONY: testground
BINS+=testground


tvx:  ## Build the TVX tool
	rm -f tvx
	$(GOCC) build -o tvx ./cmd/tvx
.PHONY: tvx
BINS+=tvx

lotus-sim: $(BUILD_DEPS)  ## Build the Lotus simulator
	rm -f lotus-sim
	$(GOCC) build $(GOFLAGS) -o lotus-sim ./cmd/lotus-sim
.PHONY: lotus-sim
BINS+=lotus-sim

# SYSTEMD

install-daemon-service: install-daemon  ## Install systemd service for Lotus daemon
	mkdir -p /etc/systemd/system
	mkdir -p /var/log/lotus
	install -C -m 0644 ./scripts/lotus-daemon.service /etc/systemd/system/lotus-daemon.service
	systemctl daemon-reload
	@echo
	@echo "lotus-daemon service installed."
	@echo "To start the service, run: 'sudo systemctl start lotus-daemon'"
	@echo "To enable the service on startup, run: 'sudo systemctl enable lotus-daemon'"

install-miner-service: install-miner install-daemon-service  ## Install systemd service for Lotus miner
	mkdir -p /etc/systemd/system
	mkdir -p /var/log/lotus
	install -C -m 0644 ./scripts/lotus-miner.service /etc/systemd/system/lotus-miner.service
	systemctl daemon-reload
	@echo
	@echo "lotus-miner service installed."
	@echo "To start the service, run: 'sudo systemctl start lotus-miner'"
	@echo "To enable the service on startup, run: 'sudo systemctl enable lotus-miner'"

install-main-services: install-miner-service  ## Install main systemd services

install-all-services: install-main-services  ## Install all systemd services

install-services: install-main-services  ## Alias for installing main services

clean-daemon-service: clean-miner-service  ## Clean systemd service for Lotus daemon
	-systemctl stop lotus-daemon
	-systemctl disable lotus-daemon
	rm -f /etc/systemd/system/lotus-daemon.service
	systemctl daemon-reload

clean-miner-service:  ## Clean systemd service for Lotus miner
	-systemctl stop lotus-miner
	-systemctl disable lotus-miner
	rm -f /etc/systemd/system/lotus-miner.service
	systemctl daemon-reload

clean-main-services: clean-daemon-service  ## Clean main systemd services

clean-all-services: clean-main-services  ## Clean all systemd services

clean-services: clean-all-services  ## Alias for cleaning all services

# MISC
buildall: $(BINS)  ## Build all binaries

install-completions:  ## Install shell completions
	mkdir -p /usr/share/bash-completion/completions /usr/local/share/zsh/site-functions/
	install -C ./scripts/bash-completion/lotus /usr/share/bash-completion/completions/lotus
	install -C ./scripts/zsh-completion/lotus /usr/local/share/zsh/site-functions/_lotus

unittests:  ## Run unit tests
	@$(GOCC) test $(shell go list ./... | grep -v /lotus/itests)
.PHONY: unittests

lint:
	go mod tidy
	go vet ./...
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --timeout 10m --concurrency 4
.PHONY: lint

clean:  ## Clean build artifacts
	rm -rf $(CLEAN) $(BINS)
	-$(MAKE) -C $(FFI_PATH) clean
.PHONY: clean

dist-clean:  ## Thoroughly clean, including git submodules
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean

type-gen: api-gen  ## Generate type information
	$(GOCC) run ./gen/main.go
	$(GOCC) generate -x ./...
	$(FIX_IMPORTS)

actors-code-gen:  ## Generate actor code
	$(GOCC) run ./gen/inline-gen . gen/inlinegen-data.json
	$(GOCC) run ./chain/actors/agen
	$(GOCC) fmt ./...

actors-gen: actors-code-gen  ## Generate actors
	$(GOCC) run ./scripts/fiximports
.PHONY: actors-gen

bundle-gen:  ## Generate bundle
	$(GOCC) run ./gen/bundle $(VERSION) $(RELEASE) $(RELEASE_OVERRIDES)
	$(GOCC) fmt ./build/...
.PHONY: bundle-gen

api-gen:  ## Generate API
	$(GOCC) run ./gen/api
	$(FIX_IMPORTS)
.PHONY: api-gen

cfgdoc-gen:  ## Generate configuration documentation
	$(GOCC) run ./node/config/cfgdocgen > ./node/config/doc_gen.go

appimage: lotus  ## Build AppImage
	rm -rf appimage-builder-cache || true
	rm AppDir/io.filecoin.lotus.desktop || true
	rm AppDir/icon.svg || true
	rm Appdir/AppRun || true
	mkdir -p AppDir/usr/bin
	cp ./lotus AppDir/usr/bin/
	appimage-builder

docsgen: fiximports  ## Generate documentation
	$(GOCC) run ./gen/docs
.PHONY: docsgen

fiximports:  ## Fix imports
	$(FIX_IMPORTS)
.PHONY: fiximports

gen: actors-code-gen type-gen cfgdoc-gen docsgen api-gen  ## Run all generation tasks
	$(GOCC) run ./scripts/fiximports
	@echo ">>> IF YOU'VE MODIFIED THE CLI OR CONFIG, REMEMBER TO ALSO RUN 'make docsgen-cli'"
.PHONY: gen

jen: gen  ## Alias for gen

snap: lotus lotus-miner lotus-worker  ## Build snap package
	snapcraft
	# snapcraft upload ./lotus_*.snap

docsgen-cli:  ## Generate CLI documentation
	$(GOCC) run ./scripts/docsgen-cli
.PHONY: docsgen-cli

print-%:  ## Print variable value
	@echo $*=$($*)

help:  ## Display this help message
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort
	@echo
	@printf "Network-specific builds:\n"
	@printf "  \033[36mcalibnet-<binary>\033[0m         Build <binary> with Calibration network support\n"
	@printf "  \033[36mbutterflynet-<binary>\033[0m     Build <binary> with Butterfly network support\n"
	@printf "  \033[36minteropnet-<binary>\033[0m       Build <binary> with Interop network support\n"
	@printf "  \033[36m2k-<binary>\033[0m               Build <binary> with 2k network support\n"
	@printf "  \033[36mdebug-<binary>\033[0m            Build <binary> with Debug tags\n"
	@echo
	@printf "Examples:\n"
	@printf "  \033[36mmake calibnet-lotus\033[0m              Build the Lotus daemon for Calibnet\n"
	@printf "  \033[36mmake calibnet-lotus-miner\033[0m        Build the Lotus miner for Calibnet\n"
	@printf "  \033[36mmake butterflynet-lotus-gateway\033[0m  Build the Lotus gateway for Butterflynet\n"