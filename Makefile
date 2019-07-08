blssigs:
	./scripts/install-bls-signatures.sh

deps: blssigs
	go mod download

build: deps
	go build -o lotus ./cmd/lotus

.PHONY: build deps blssigs