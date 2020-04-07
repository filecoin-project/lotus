SHELL=/usr/bin/env bash

all: build
.PHONY: all

# git submodules that need to be loaded
SUBMODULES:=

# things to clean up, e.g. libfilecoin.a
CLEAN:=

FFI_PATH:=extern/filecoin-ffi/
FFI_DEPS:=libfilcrypto.a filcrypto.pc filcrypto.h
FFI_DEPS:=$(addprefix $(FFI_PATH),$(FFI_DEPS))

$(FFI_DEPS): build/.filecoin-ffi-install ;

# dummy file that marks the last time the filecoin-ffi project was built
build/.filecoin-ffi-install: $(FFI_PATH)
	$(MAKE) -C $(FFI_PATH) $(FFI_DEPS:$(FFI_PATH)%=%)
	@touch $@

SUBMODULES+=$(FFI_PATH)
BUILD_DEPS+=build/.filecoin-ffi-install
CLEAN+=build/.filecoin-ffi-install

$(SUBMODULES): build/.update-submodules ;

# dummy file that marks the last time submodules were updated
build/.update-submodules:
	git submodule update --init --recursive
	touch $@

CLEAN+=build/.update-submodules

# build and install any upstream dependencies, e.g. filecoin-ffi
deps: $(BUILD_DEPS)
.PHONY: deps

test: $(BUILD_DEPS)
	RUST_LOG=info go test -race -count 1 -v -timeout 120m ./...
.PHONY: test

lint: $(BUILD_DEPS)
	golangci-lint run -v --concurrency 2 --new-from-rev origin/master
.PHONY: lint

build: $(BUILD_DEPS)
	go build -v $(GOFLAGS) ./...
.PHONY: build

clean:
	rm -rf $(CLEAN)
	-$(MAKE) -C $(FFI_PATH) clean
.PHONY: clean

type-gen:
	go run ./gen/main.go
.PHONY: type-gen
