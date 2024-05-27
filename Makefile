SHELL=/usr/bin/env bash

all: build
.PHONY: all

unexport GOFLAGS

GOCC?=go

GOVERSION:=$(shell $(GOCC) version | tr ' ' '\n' | grep go1 | sed 's/^go//' | awk -F. '{printf "%d%03d%03d", $$1, $$2, $$3}')
GOVERSIONMIN:=$(shell cat GO_VERSION_MIN | awk -F. '{printf "%d%03d%03d", $$1, $$2, $$3}')

ifeq ($(shell expr $(GOVERSION) \< $(GOVERSIONMIN)), 1)
$(warning Your Golang version is go$(shell expr $(GOVERSION) / 1000000).$(shell expr $(GOVERSION) % 1000000 / 1000).$(shell expr $(GOVERSION) % 1000))
$(error Update Golang to version to at least $(shell cat GO_VERSION_MIN))
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

build-devnets: build lotus-seed lotus-shed curio sptool
.PHONY: build-devnets

debug: GOFLAGS+=-tags=debug
debug: build-devnets

2k: GOFLAGS+=-tags=2k
2k: build-devnets

calibnet: GOFLAGS+=-tags=calibnet
calibnet: build-devnets

butterflynet: GOFLAGS+=-tags=butterflynet
butterflynet: build-devnets

interopnet: GOFLAGS+=-tags=interopnet
interopnet: build-devnets

lotus: $(BUILD_DEPS)
	rm -f lotus
	$(GOCC) build $(GOFLAGS) -o lotus ./cmd/lotus

.PHONY: lotus
BINS+=lotus

lotus-miner: $(BUILD_DEPS)
	rm -f lotus-miner
	$(GOCC) build $(GOFLAGS) -o lotus-miner ./cmd/lotus-miner
.PHONY: lotus-miner
BINS+=lotus-miner

curio: $(BUILD_DEPS)
	rm -f curio
	$(GOCC) build $(GOFLAGS) -o curio -ldflags " \
	-X github.com/filecoin-project/lotus/curiosrc/build.IsOpencl=$(FFI_USE_OPENCL) \
	-X github.com/filecoin-project/lotus/curiosrc/build.Commit=`git log -1 --format=%h_%cI`" \
	./cmd/curio 
.PHONY: curio
BINS+=curio

cu2k: GOFLAGS+=-tags=2k
cu2k: curio

sptool: $(BUILD_DEPS)
	rm -f sptool
	$(GOCC) build $(GOFLAGS) -o sptool ./cmd/sptool
.PHONY: sptool
BINS+=sptool

lotus-worker: $(BUILD_DEPS)
	rm -f lotus-worker
	$(GOCC) build $(GOFLAGS) -o lotus-worker ./cmd/lotus-worker
.PHONY: lotus-worker
BINS+=lotus-worker

lotus-shed: $(BUILD_DEPS)
	rm -f lotus-shed
	$(GOCC) build $(GOFLAGS) -o lotus-shed ./cmd/lotus-shed
.PHONY: lotus-shed
BINS+=lotus-shed

lotus-gateway: $(BUILD_DEPS)
	rm -f lotus-gateway
	$(GOCC) build $(GOFLAGS) -o lotus-gateway ./cmd/lotus-gateway
.PHONY: lotus-gateway
BINS+=lotus-gateway

build: lotus lotus-miner lotus-worker curio sptool
	@[[ $$(type -P "lotus") ]] && echo "Caution: you have \
an existing lotus binary in your PATH. This may cause problems if you don't run 'sudo make install'" || true

.PHONY: build

install: install-daemon install-miner install-worker install-curio install-sptool

install-daemon:
	install -C ./lotus /usr/local/bin/lotus

install-miner:
	install -C ./lotus-miner /usr/local/bin/lotus-miner

install-curio:
	install -C ./curio /usr/local/bin/curio

install-sptool:
	install -C ./sptool /usr/local/bin/sptool

install-worker:
	install -C ./lotus-worker /usr/local/bin/lotus-worker

install-app:
	install -C ./$(APP) /usr/local/bin/$(APP)

uninstall: uninstall-daemon uninstall-miner uninstall-worker
.PHONY: uninstall

uninstall-daemon:
	rm -f /usr/local/bin/lotus

uninstall-miner:
	rm -f /usr/local/bin/lotus-miner

uninstall-curio:
	rm -f /usr/local/bin/curio

uninstall-sptool:
	rm -f /usr/local/bin/sptool

uninstall-worker:
	rm -f /usr/local/bin/lotus-worker

# TOOLS

lotus-seed: $(BUILD_DEPS)
	rm -f lotus-seed
	$(GOCC) build $(GOFLAGS) -o lotus-seed ./cmd/lotus-seed

.PHONY: lotus-seed
BINS+=lotus-seed

benchmarks:
	$(GOCC) run github.com/whyrusleeping/bencher ./... > bench.json
	@echo Submitting results
	@curl -X POST 'http://benchmark.kittyhawk.wtf/benchmark' -d '@bench.json' -u "${benchmark_http_cred}"
.PHONY: benchmarks

lotus-fountain:
	rm -f lotus-fountain
	$(GOCC) build $(GOFLAGS) -o lotus-fountain ./cmd/lotus-fountain
	$(GOCC) run github.com/GeertJohan/go.rice/rice append --exec lotus-fountain -i ./cmd/lotus-fountain -i ./build
.PHONY: lotus-fountain
BINS+=lotus-fountain

lotus-bench:
	rm -f lotus-bench
	$(GOCC) build $(GOFLAGS) -o lotus-bench ./cmd/lotus-bench
.PHONY: lotus-bench
BINS+=lotus-bench

lotus-stats:
	rm -f lotus-stats
	$(GOCC) build $(GOFLAGS) -o lotus-stats ./cmd/lotus-stats
.PHONY: lotus-stats
BINS+=lotus-stats

lotus-pcr:
	rm -f lotus-pcr
	$(GOCC) build $(GOFLAGS) -o lotus-pcr ./cmd/lotus-pcr
.PHONY: lotus-pcr
BINS+=lotus-pcr

lotus-health:
	rm -f lotus-health
	$(GOCC) build -o lotus-health ./cmd/lotus-health
.PHONY: lotus-health
BINS+=lotus-health

lotus-wallet: $(BUILD_DEPS)
	rm -f lotus-wallet
	$(GOCC) build $(GOFLAGS) -o lotus-wallet ./cmd/lotus-wallet
.PHONY: lotus-wallet
BINS+=lotus-wallet

lotus-keygen:
	rm -f lotus-keygen
	$(GOCC) build -o lotus-keygen ./cmd/lotus-keygen
.PHONY: lotus-keygen
BINS+=lotus-keygen

testground:
	$(GOCC) build -tags testground -o /dev/null ./cmd/lotus
.PHONY: testground
BINS+=testground


tvx:
	rm -f tvx
	$(GOCC) build -o tvx ./cmd/tvx
.PHONY: tvx
BINS+=tvx

lotus-sim: $(BUILD_DEPS)
	rm -f lotus-sim
	$(GOCC) build $(GOFLAGS) -o lotus-sim ./cmd/lotus-sim
.PHONY: lotus-sim
BINS+=lotus-sim

# SYSTEMD

install-daemon-service: install-daemon
	mkdir -p /etc/systemd/system
	mkdir -p /var/log/lotus
	install -C -m 0644 ./scripts/lotus-daemon.service /etc/systemd/system/lotus-daemon.service
	systemctl daemon-reload
	@echo
	@echo "lotus-daemon service installed."
	@echo "To start the service, run: 'sudo systemctl start lotus-daemon'"
	@echo "To enable the service on startup, run: 'sudo systemctl enable lotus-daemon'"

install-miner-service: install-miner install-daemon-service
	mkdir -p /etc/systemd/system
	mkdir -p /var/log/lotus
	install -C -m 0644 ./scripts/lotus-miner.service /etc/systemd/system/lotus-miner.service
	systemctl daemon-reload
	@echo
	@echo "lotus-miner service installed."
	@echo "To start the service, run: 'sudo systemctl start lotus-miner'"
	@echo "To enable the service on startup, run: 'sudo systemctl enable lotus-miner'"

install-curio-service: install-curio install-sptool install-daemon-service
	mkdir -p /etc/systemd/system
	mkdir -p /var/log/lotus
	install -C -m 0644 ./scripts/curio.service /etc/systemd/system/curio.service
	systemctl daemon-reload
	@echo
	@echo "Curio service installed. Don't forget to run 'sudo systemctl start curio' to start it and 'sudo systemctl enable curio' for it to be enabled on startup."

install-main-services: install-miner-service

install-all-services: install-main-services

install-services: install-main-services

clean-daemon-service: clean-miner-service
	-systemctl stop lotus-daemon
	-systemctl disable lotus-daemon
	rm -f /etc/systemd/system/lotus-daemon.service
	systemctl daemon-reload

clean-miner-service:
	-systemctl stop lotus-miner
	-systemctl disable lotus-miner
	rm -f /etc/systemd/system/lotus-miner.service
	systemctl daemon-reload

clean-curio-service:
	-systemctl stop curio
	-systemctl disable curio
	rm -f /etc/systemd/system/curio.service
	systemctl daemon-reload

clean-main-services: clean-daemon-service

clean-all-services: clean-main-services

clean-services: clean-all-services

# MISC

buildall: $(BINS)

install-completions:
	mkdir -p /usr/share/bash-completion/completions /usr/local/share/zsh/site-functions/
	install -C ./scripts/bash-completion/lotus /usr/share/bash-completion/completions/lotus
	install -C ./scripts/zsh-completion/lotus /usr/local/share/zsh/site-functions/_lotus

unittests:
	@$(GOCC) test $(shell go list ./... | grep -v /lotus/itests)
.PHONY: unittests

clean:
	rm -rf $(CLEAN) $(BINS)
	-$(MAKE) -C $(FFI_PATH) clean
.PHONY: clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean

type-gen: api-gen
	$(GOCC) run ./gen/main.go
	$(GOCC) generate -x ./...
	goimports -w api/

actors-code-gen:
	$(GOCC) run ./gen/inline-gen . gen/inlinegen-data.json
	$(GOCC) run ./chain/actors/agen
	$(GOCC) fmt ./...

actors-gen: actors-code-gen
	$(GOCC) run ./scripts/fiximports
.PHONY: actors-gen

bundle-gen:
	$(GOCC) run ./gen/bundle $(VERSION) $(RELEASE) $(RELEASE_OVERRIDES)
	$(GOCC) fmt ./build/...
.PHONY: bundle-gen


api-gen:
	$(GOCC) run ./gen/api
	goimports -w api
	goimports -w api
.PHONY: api-gen

cfgdoc-gen:
	$(GOCC) run ./node/config/cfgdocgen > ./node/config/doc_gen.go

appimage: lotus
	rm -rf appimage-builder-cache || true
	rm AppDir/io.filecoin.lotus.desktop || true
	rm AppDir/icon.svg || true
	rm Appdir/AppRun || true
	mkdir -p AppDir/usr/bin
	cp ./lotus AppDir/usr/bin/
	appimage-builder

docsgen: docsgen-md docsgen-openrpc fiximports

docsgen-md-bin: api-gen actors-gen
	$(GOCC) build $(GOFLAGS) -o docgen-md ./api/docgen/cmd
docsgen-openrpc-bin: api-gen actors-gen
	$(GOCC) build $(GOFLAGS) -o docgen-openrpc ./api/docgen-openrpc/cmd

docsgen-md: docsgen-md-full docsgen-md-storage docsgen-md-worker docsgen-md-curio

docsgen-md-full: docsgen-md-bin
	./docgen-md "api/api_full.go" "FullNode" "api" "./api" > documentation/en/api-v1-unstable-methods.md
	./docgen-md "api/v0api/full.go" "FullNode" "v0api" "./api/v0api" > documentation/en/api-v0-methods.md
docsgen-md-storage: docsgen-md-bin
	./docgen-md "api/api_storage.go" "StorageMiner" "api" "./api" > documentation/en/api-v0-methods-miner.md
docsgen-md-worker: docsgen-md-bin
	./docgen-md "api/api_worker.go" "Worker" "api" "./api" > documentation/en/api-v0-methods-worker.md
docsgen-md-curio: docsgen-md-bin
	./docgen-md "api/api_curio.go" "Curio" "api" "./api" > documentation/en/api-v0-methods-curio.md

docsgen-openrpc: docsgen-openrpc-full docsgen-openrpc-storage docsgen-openrpc-worker docsgen-openrpc-gateway

docsgen-openrpc-full: docsgen-openrpc-bin
	./docgen-openrpc "api/api_full.go" "FullNode" "api" "./api" > build/openrpc/full.json
docsgen-openrpc-storage: docsgen-openrpc-bin
	./docgen-openrpc "api/api_storage.go" "StorageMiner" "api" "./api" > build/openrpc/miner.json
docsgen-openrpc-worker: docsgen-openrpc-bin
	./docgen-openrpc "api/api_worker.go" "Worker" "api" "./api" > build/openrpc/worker.json
docsgen-openrpc-gateway: docsgen-openrpc-bin
	./docgen-openrpc "api/api_gateway.go" "Gateway" "api" "./api" > build/openrpc/gateway.json

.PHONY: docsgen docsgen-md-bin docsgen-openrpc-bin

fiximports:
	$(GOCC) run ./scripts/fiximports

gen: actors-code-gen type-gen cfgdoc-gen docsgen api-gen
	$(GOCC) run ./scripts/fiximports
	@echo ">>> IF YOU'VE MODIFIED THE CLI OR CONFIG, REMEMBER TO ALSO RUN 'make docsgen-cli'"
.PHONY: gen

jen: gen

snap: lotus lotus-miner lotus-worker curio sptool
	snapcraft
	# snapcraft upload ./lotus_*.snap

# separate from gen because it needs binaries
docsgen-cli: lotus lotus-miner lotus-worker curio sptool
	python3 ./scripts/generate-lotus-cli.py
	./lotus config default > documentation/en/default-lotus-config.toml
	./lotus-miner config default > documentation/en/default-lotus-miner-config.toml
	./curio config default > documentation/en/default-curio-config.toml
.PHONY: docsgen-cli

print-%:
	@echo $*=$($*)

### Curio devnet images
curio_docker_user?=curio
curio_base_image=$(curio_docker_user)/curio-all-in-one:latest-debug
ffi_from_source?=0

curio-devnet: lotus lotus-miner lotus-shed lotus-seed curio sptool
.PHONY: curio-devnet

curio_docker_build_cmd=docker build --build-arg CURIO_TEST_IMAGE=$(curio_base_image) \
	--build-arg FFI_BUILD_FROM_SOURCE=$(ffi_from_source) $(docker_args)

docker/curio-all-in-one:
	$(curio_docker_build_cmd) -f Dockerfile.curio --target curio-all-in-one \
		-t $(curio_base_image) --build-arg GOFLAGS=-tags=debug .
.PHONY: docker/curio-all-in-one

docker/%:
	cd curiosrc/docker/$* && DOCKER_BUILDKIT=1 $(curio_docker_build_cmd) -t $(curio_docker_user)/$*-dev:dev \
		--build-arg BUILD_VERSION=dev .

docker/curio-devnet: $(lotus_build_cmd) \
	docker/curio-all-in-one docker/lotus docker/lotus-miner docker/curio docker/yugabyte
.PHONY: docker/curio-devnet

curio-devnet/up:
	rm -rf ./curiosrc/docker/data && docker compose -f ./curiosrc/docker/docker-compose.yaml up -d

curio-devnet/down:
	docker compose -f ./curiosrc/docker/docker-compose.yaml down --rmi=local && sleep 2 && rm -rf ./curiosrc/docker/data
