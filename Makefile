all: build

blssigs: lib/bls-signatures/include/libbls_signatures.h

lib/bls-signatures/include/libbls_signatures.h: lib/bls-signatures/bls-signatures ;
	./scripts/install-bls-signatures.sh

sectorbuilder: lib/sectorbuilder/include/sector_builder_ffi.h

lib/sectorbuilder/include/sector_builder_ffi.h: lib/sectorbuilder/rust-fil-sector-builder ;
	./lib/sectorbuilder/build.sh

deps: blssigs sectorbuilder

build: deps
	go build -o lotus ./cmd/lotus

.PHONY: all build deps blssigs
