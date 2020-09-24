SHELL = /bin/bash

.DEFAULT_GOAL := download-proofs

download-proofs:
	go run github.com/filecoin-project/go-paramfetch/paramfetch 2048 ./docker-images/proof-parameters.json

build-images:
	docker build -t "iptestground/oni-buildbase:v8" -f "docker-images/Dockerfile.oni-buildbase" "docker-images"
	docker build -t "iptestground/oni-runtime:v3" -f "docker-images/Dockerfile.oni-runtime" "docker-images"
	docker build -t "iptestground/oni-runtime:v4-debug" -f "docker-images/Dockerfile.oni-runtime-debug" "docker-images"

push-images:
	docker push iptestground/oni-buildbase:v8
	docker push iptestground/oni-runtime:v3
	docker push iptestground/oni-runtime:v4-debug

pull-images:
	docker pull iptestground/oni-buildbase:v8
	docker pull iptestground/oni-runtime:v3
	docker pull iptestground/oni-runtime:v4-debug

.PHONY: download-proofs build-images push-images pull-images
