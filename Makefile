all: build
.PHONY: all

BUILD_DEPS:=lib/bls-signatures/include/libbls_signatures.h
BUILD_DEPS+=lib/sectorbuilder/include/sector_builder_ffi.h

# git modules that need to be loaded
MODULES:=

lib/bls-signatures/include/libbls_signatures.h: lib/bls-signatures/bls-signatures
	./scripts/install-bls-signatures.sh
	@touch $@

MODULES+=lib/bls-signatures/bls-signatures


lib/sectorbuilder/include/sector_builder_ffi.h: lib/sectorbuilder lib/sectorbuilder/rust-fil-sector-builder
	./lib/sectorbuilder/build.sh
	@touch $@

MODULES+=lib/sectorbuilder
MODULES+=lib/sectorbuilder/rust-fil-sector-builder

$(MODULES): build/.update-modules ;

# dummy file that marks the last time modules were updated
build/.update-modules:
	git submodule update --init --recursive
	touch $@

deps: $(BUILD_DEPS)
.PHONY: deps

build: $(BUILD_DEPS)
	go build -o lotus ./cmd/lotus
.PHONY: build

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean
