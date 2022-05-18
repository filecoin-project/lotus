FROM golang:1.17.9-buster AS builder-deps
MAINTAINER Lotus Development Team

RUN apt-get update && apt-get install -y ca-certificates build-essential clang ocl-icd-opencl-dev ocl-icd-libopencl1 jq libhwloc-dev

ARG RUST_VERSION=nightly
ENV XDG_CACHE_HOME="/tmp"

ENV RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    PATH=/usr/local/cargo/bin:$PATH

RUN wget "https://static.rust-lang.org/rustup/dist/x86_64-unknown-linux-gnu/rustup-init"; \
    chmod +x rustup-init; \
    ./rustup-init -y --no-modify-path --profile minimal --default-toolchain $RUST_VERSION; \
    rm rustup-init; \
    chmod -R a+w $RUSTUP_HOME $CARGO_HOME; \
    rustup --version; \
    cargo --version; \
    rustc --version;


FROM builder-deps AS builder-local
MAINTAINER Lotus Development Team

COPY ./ /opt/filecoin
WORKDIR /opt/filecoin
RUN make clean deps


FROM builder-local AS builder-test
MAINTAINER Lotus Development Team

WORKDIR /opt/filecoin

RUN make debug


FROM builder-local AS builder
MAINTAINER Lotus Development Team

WORKDIR /opt/filecoin

ARG RUSTFLAGS=""
ARG GOFLAGS=""

RUN make lotus lotus-miner lotus-worker lotus-shed lotus-wallet lotus-gateway lotus-stats


FROM ubuntu:20.04 AS base
MAINTAINER Lotus Development Team

# Base resources
COPY --from=builder /etc/ssl/certs                           /etc/ssl/certs
COPY --from=builder /lib/x86_64-linux-gnu/libdl.so.2         /lib/
COPY --from=builder /lib/x86_64-linux-gnu/librt.so.1         /lib/
COPY --from=builder /lib/x86_64-linux-gnu/libgcc_s.so.1      /lib/
COPY --from=builder /lib/x86_64-linux-gnu/libutil.so.1       /lib/
COPY --from=builder /usr/lib/x86_64-linux-gnu/libltdl.so.7   /lib/
COPY --from=builder /usr/lib/x86_64-linux-gnu/libnuma.so.1   /lib/
COPY --from=builder /usr/lib/x86_64-linux-gnu/libhwloc.so.5  /lib/
COPY --from=builder /usr/lib/x86_64-linux-gnu/libOpenCL.so.1 /lib/

RUN useradd -r -u 532 -U fc \
 && mkdir -p /etc/OpenCL/vendors \
 && echo "libnvidia-opencl.so.1" > /etc/OpenCL/vendors/nvidia.icd

###
FROM base AS lotus
MAINTAINER Lotus Development Team

COPY --from=builder /opt/filecoin/lotus /usr/local/bin/
COPY --from=builder /opt/filecoin/lotus-shed /usr/local/bin/
COPY scripts/docker-lotus-entrypoint.sh /

ENV FILECOIN_PARAMETER_CACHE /var/tmp/filecoin-proof-parameters
ENV LOTUS_PATH /var/lib/lotus
ENV DOCKER_LOTUS_IMPORT_SNAPSHOT https://fil-chain-snapshots-fallback.s3.amazonaws.com/mainnet/minimal_finality_stateroots_latest.car
ENV DOCKER_LOTUS_IMPORT_WALLET ""

RUN mkdir /var/lib/lotus /var/tmp/filecoin-proof-parameters
RUN chown fc: /var/lib/lotus /var/tmp/filecoin-proof-parameters

VOLUME /var/lib/lotus
VOLUME /var/tmp/filecoin-proof-parameters

USER fc

EXPOSE 1234

ENTRYPOINT ["/docker-lotus-entrypoint.sh"]

CMD ["-help"]

###
FROM base AS lotus-wallet
MAINTAINER Lotus Development Team

COPY --from=builder /opt/filecoin/lotus-wallet /usr/local/bin/

ENV WALLET_PATH /var/lib/lotus-wallet

RUN mkdir /var/lib/lotus-wallet
RUN chown fc: /var/lib/lotus-wallet

VOLUME /var/lib/lotus-wallet

USER fc

EXPOSE 1777

ENTRYPOINT ["/usr/local/bin/lotus-wallet"]

CMD ["-help"]

###
FROM base AS lotus-gateway
MAINTAINER Lotus Development Team

COPY --from=builder /opt/filecoin/lotus-gateway /usr/local/bin/

USER fc

EXPOSE 1234

ENTRYPOINT ["/usr/local/bin/lotus-gateway"]

CMD ["-help"]


###
FROM base AS lotus-miner
MAINTAINER Lotus Development Team

COPY --from=builder /opt/filecoin/lotus-miner /usr/local/bin/
COPY scripts/docker-lotus-miner-entrypoint.sh /

ENV FILECOIN_PARAMETER_CACHE /var/tmp/filecoin-proof-parameters
ENV LOTUS_MINER_PATH /var/lib/lotus-miner

RUN mkdir /var/lib/lotus-miner /var/tmp/filecoin-proof-parameters
RUN chown fc: /var/lib/lotus-miner /var/tmp/filecoin-proof-parameters

VOLUME /var/lib/lotus-miner
VOLUME /var/tmp/filecoin-proof-parameters

USER fc

EXPOSE 2345

ENTRYPOINT ["/docker-lotus-miner-entrypoint.sh"]

CMD ["-help"]


###
FROM base AS lotus-worker
MAINTAINER Lotus Development Team

COPY --from=builder /opt/filecoin/lotus-worker /usr/local/bin/

ENV FILECOIN_PARAMETER_CACHE /var/tmp/filecoin-proof-parameters
ENV LOTUS_WORKER_PATH /var/lib/lotus-worker

RUN mkdir /var/lib/lotus-worker
RUN chown fc: /var/lib/lotus-worker

VOLUME /var/lib/lotus-worker

USER fc

EXPOSE 3456

ENTRYPOINT ["/usr/local/bin/lotus-worker"]

CMD ["-help"]


###
from base as lotus-all-in-one

ENV FILECOIN_PARAMETER_CACHE /var/tmp/filecoin-proof-parameters
ENV LOTUS_MINER_PATH /var/lib/lotus-miner
ENV LOTUS_PATH /var/lib/lotus
ENV LOTUS_WORKER_PATH /var/lib/lotus-worker
ENV WALLET_PATH /var/lib/lotus-wallet
ENV DOCKER_LOTUS_IMPORT_SNAPSHOT https://fil-chain-snapshots-fallback.s3.amazonaws.com/mainnet/minimal_finality_stateroots_latest.car

COPY --from=builder /opt/filecoin/lotus      /usr/local/bin/
COPY --from=builder /opt/filecoin/lotus-shed /usr/local/bin/
COPY --from=builder /opt/filecoin/lotus-wallet /usr/local/bin/
COPY --from=builder /opt/filecoin/lotus-gateway /usr/local/bin/
COPY --from=builder /opt/filecoin/lotus-miner /usr/local/bin/
COPY --from=builder /opt/filecoin/lotus-worker /usr/local/bin/
COPY --from=builder /opt/filecoin/lotus-stats /usr/local/bin/

RUN mkdir /var/tmp/filecoin-proof-parameters
RUN mkdir /var/lib/lotus
RUN mkdir /var/lib/lotus-miner
RUN mkdir /var/lib/lotus-worker
RUN mkdir /var/lib/lotus-wallet
RUN chown fc: /var/tmp/filecoin-proof-parameters
RUN chown fc: /var/lib/lotus
RUN chown fc: /var/lib/lotus-miner
RUN chown fc: /var/lib/lotus-worker
RUN chown fc: /var/lib/lotus-wallet


VOLUME /var/tmp/filecoin-proof-parameters
VOLUME /var/lib/lotus
VOLUME /var/lib/lotus-miner
VOLUME /var/lib/lotus-worker
VOLUME /var/lib/lotus-wallet

EXPOSE 1234
EXPOSE 2345
EXPOSE 3456
EXPOSE 1777

###
from base as lotus-test

ENV FILECOIN_PARAMETER_CACHE /var/tmp/filecoin-proof-parameters
ENV LOTUS_MINER_PATH /var/lib/lotus-miner
ENV LOTUS_PATH /var/lib/lotus
ENV LOTUS_WORKER_PATH /var/lib/lotus-worker
ENV WALLET_PATH /var/lib/lotus-wallet

COPY --from=builder-test /opt/filecoin/lotus      /usr/local/bin/
COPY --from=builder-test /opt/filecoin/lotus-miner /usr/local/bin/
COPY --from=builder-test /opt/filecoin/lotus-worker /usr/local/bin/
COPY --from=builder-test /opt/filecoin/lotus-seed /usr/local/bin/

RUN mkdir /var/tmp/filecoin-proof-parameters
RUN mkdir /var/lib/lotus
RUN mkdir /var/lib/lotus-miner
RUN mkdir /var/lib/lotus-worker
RUN mkdir /var/lib/lotus-wallet
RUN chown fc: /var/tmp/filecoin-proof-parameters
RUN chown fc: /var/lib/lotus
RUN chown fc: /var/lib/lotus-miner
RUN chown fc: /var/lib/lotus-worker
RUN chown fc: /var/lib/lotus-wallet


VOLUME /var/tmp/filecoin-proof-parameters
VOLUME /var/lib/lotus
VOLUME /var/lib/lotus-miner
VOLUME /var/lib/lotus-worker
VOLUME /var/lib/lotus-wallet

EXPOSE 1234
EXPOSE 2345
EXPOSE 3456
EXPOSE 1777

