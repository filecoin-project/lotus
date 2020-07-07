SHELL = /bin/bash

.DEFAULT_GOAL := download-proofs

download-proofs:
	go run github.com/filecoin-project/go-paramfetch/paramfetch 2048 ./docker-images/proof-parameters.json
.PHONY: download-proofs