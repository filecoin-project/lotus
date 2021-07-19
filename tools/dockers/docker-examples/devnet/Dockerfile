FROM ubuntu:19.10
MAINTAINER Richard Patel <me@terorie.dev>

RUN apt-get update \
 && apt-get install -y \
    s6 ca-certificates curl git bzr tmux \
    llvm clang \
    mesa-opencl-icd ocl-icd-opencl-dev \
    golang-1.13-go \
 && ln -s /usr/lib/go-1.13/bin/go /usr/bin/go

RUN curl -sSf https://sh.rustup.rs | sh -s -- -y

# Download packages first so they can be cached.
COPY go.mod go.sum /lotus/
COPY extern/ /lotus/extern/
RUN cd /lotus \
 && go mod download

COPY Makefile /lotus

# Because extern/filecoin-ffi building script need to get version number from git
COPY .git/ /lotus/.git/
COPY .gitmodules /lotus/

# Download dependencies first
RUN cd /lotus \
  && mkdir /lotus/build \
  && . $HOME/.cargo/env \
  && make clean \
  && make deps


COPY . /lotus

ARG MAKE_TARGET=debug

# Build the thing.
RUN cd /lotus \
  && . $HOME/.cargo/env \
  && make $MAKE_TARGET \
  && ln -s -t /usr/local/bin/ /lotus/lotus /lotus/lotus-*

# Symlink devnet environment
RUN ln -s /lotus/tools/dockers/docker-examples/devnet /devnet
WORKDIR /devnet

ENTRYPOINT ["/bin/sh"]
CMD ["/devnet/docker_scripts/start.sh"]

VOLUME ["/var/tmp/filecoin-proof-parameters/"]

EXPOSE \
    # WS port
    1234/tcp \
    # Miner port
    2345/tcp \
    # P2P port
    5678/tcp
