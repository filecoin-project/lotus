SHELL=/usr/bin/env bash

%:
	@echo -e "### ⚠️⚠️⚠️ NOTICE ⚠️⚠️⚠️ ###\n" \
	         "The 'releases' branch has been deprecated with the 202408 split of 'Lotus Node' and 'Lotus Miner'.\n" \
		     "See https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md for more info and alternatives.\n" \
		     "----------------------" | tee /dev/stderr
	@exit 1

.PHONY: % 