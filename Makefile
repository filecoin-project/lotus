all: build
.PHONY: all

BUILD_DEPS+=lib/sectorbuilder/include/sector_builder_ffi.h

# git modules that need to be loaded
MODULES:=

CLEAN:=

BLS_PATH:=extern/go-bls-sigs/
BLS_DEPS:=libbls_signatures.a libbls_signatures.pc libbls_signatures.h
BLS_DEPS:=$(addprefix $(BLS_PATH),$(BLS_DEPS))

$(BLS_DEPS): build/.bls-install ;

build/.bls-install: $(BLS_PATH)
	$(MAKE) -C $(BLS_PATH) $(BLS_DEPS:$(BLS_PATH)%=%)
	@touch $@

MODULES+=$(BLS_PATH)
BUILD_DEPS+=build/.bls-install
CLEAN+=build/.bls-install


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

CLEAN+=build/.update-modules

deps: $(BUILD_DEPS)
.PHONY: deps

build: $(BUILD_DEPS)
	go build -o lotus ./cmd/lotus
.PHONY: build

clean:
	rm -rf $(CLEAN)
	-$(MAKE) -C $(BLS_PATH) clean
.PHONY: clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean


print-%:
	@echo $*=$($*)
