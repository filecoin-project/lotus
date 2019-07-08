all: build

blssigs: lib/bls-signatures/include/libbls_signatures.h

lib/bls-signatures/include/libbls_signatures.h: lib/bls-signatures/bls-signatures ;
	./scripts/install-bls-signatures.sh

deps: blssigs

build: deps
	go build -o lotus ./cmd/lotus

.PHONY: all build deps
