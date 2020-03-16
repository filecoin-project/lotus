DOCKER=docker
DOCKER_IMAGE=filecoin-devnet
DOCKER_CONTAINER=filecoin-devnet
PROOF_VOLUME=filecoin-proof-params

.PHONY: all build run kill login stop clean clean-all

all: build run login

.ONESHELL:
build:
	$(DOCKER) build -t "$(DOCKER_IMAGE)" \
	  -f ./Dockerfile \
	  ../../../../

run:
	@echo "Creating proof params volume: $(PROOF_VOLUME)"
	-$(DOCKER) volume create "$(PROOF_VOLUME)"
	@echo "Starting container $(DOCKER_CONTAINER)"
	$(DOCKER) run -d \
	  --name "$(DOCKER_CONTAINER)" \
	  -p "1234:1234" \
	  -p "5678:5678" \
	  -p "2345:2345" \
	  -v "$(PROOF_VOLUME):/var/tmp/filecoin-proof-parameters" \
	  "$(DOCKER_IMAGE)"

login:
	$(DOCKER) exec "$(DOCKER_CONTAINER)" \
	  /lotus/tools/dockers/docker-examples/devnet/docker_scripts/login.sh

restart:
	$(DOCKER) restart "$(DOCKER_CONTAINER)"

stop:
	$(DOCKER) stop "$(DOCKER_CONTAINER)"

kill:
	-$(DOCKER) kill "$(DOCKER_CONTAINER)"

clean: kill
	-$(DOCKER) rm "$(DOCKER_CONTAINER)"

clean-all: clean
	-$(DOCKER) volume rm "$(PROOF_VOLUME)"
	-$(DOCKER) image rm "$(DOCKER_IMAGE)"
