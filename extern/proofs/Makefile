DEPS:=bin/paramcache bin/paramfetch misc/parameters.json

all: $(DEPS)
.PHONY: all

$(DEPS): .install-fil-proofs  ;

.install-fil-proofs: bls-signatures
	./install-proofs
	@touch $@

clean:
	rm -rf $(DEPS) .install-bls
.PHONY: clean
