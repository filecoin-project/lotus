all: build
.PHONY: all

GOVERSION:=$(shell go version | cut -d' ' -f 3 | cut -d. -f 2)
ifeq ($(shell expr $(GOVERSION) \< 13), 1)
$(warning Your Golang version is go 1.$(GOVERSION))
$(error Update Golang to version $(shell grep '^go' go.mod))
endif

build:
	go build ./...

test:
	go test ./...

type-gen:
	go run ./cbor-gen/main.go
