SHELL = /bin/bash

.DEFAULT_GOAL := download-proofs

download-proofs:
	go run github.com/filecoin-project/go-paramfetch/paramfetch 2048 ./docker-images/proof-parameters.json

build-images:
	docker build -t "iptestground/oni-buildbase:v11-lotus" -f "docker-images/Dockerfile.oni-buildbase" "docker-images"
	docker build -t "iptestground/oni-runtime:v5" -f "docker-images/Dockerfile.oni-runtime" "docker-images"

push-images:
	docker push iptestground/oni-buildbase:v11-lotus
	docker push iptestground/oni-runtime:v5

pull-images:
	docker pull iptestground/oni-buildbase:v11-lotus
	docker pull iptestground/oni-runtime:v5

.PHONY: download-proofs build-images push-images pull-images
