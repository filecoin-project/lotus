all: build
.PHONY: all


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

SECTOR_BUILDER_PATH:=extern/go-sectorbuilder/
SECTOR_BUILDER_DEPS:=libsector_builder_ffi.a sector_builder_ffi.pc sector_builder_ffi.h
SECTOR_BUILDER_DEPS:=$(addprefix $(SECTOR_BUILDER_PATH),$(SECTOR_BUILDER_DEPS))

$(SECTOR_BUILDER_DEPS): build/.sector-builder-install ;

build/.sector-builder-install: $(SECTOR_BUILDER_PATH)
	$(MAKE) -C $(SECTOR_BUILDER_PATH) $(SECTOR_BUILDER_DEPS:$(SECTOR_BUILDER_PATH)%=%)
	@touch $@

MODULES+=$(SECTOR_BUILDER_PATH)
BUILD_DEPS+=build/.sector-builder-install
CLEAN+=build/.sector-builder-install


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
	go build -o lotus-storage-miner ./cmd/lotus-storage-miner
.PHONY: build

pond: build
	go build -o pond ./lotuspond
	(cd lotuspond/front && npm i && npm run build)
.PHONY: pond

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
