#####################################
FROM golang:1.18.1-buster AS lotus-builder
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

COPY ./ /opt/filecoin
WORKDIR /opt/filecoin
RUN make clean deps

ARG RUSTFLAGS=""
ARG GOFLAGS=""

RUN make buildall

#####################################
FROM ubuntu:20.04 AS lotus-base
MAINTAINER Lotus Development Team

# Base resources
COPY --from=lotus-builder /etc/ssl/certs                           /etc/ssl/certs
COPY --from=lotus-builder /lib/x86_64-linux-gnu/libdl.so.2         /lib/
COPY --from=lotus-builder /lib/x86_64-linux-gnu/librt.so.1         /lib/
COPY --from=lotus-builder /lib/x86_64-linux-gnu/libgcc_s.so.1      /lib/
COPY --from=lotus-builder /lib/x86_64-linux-gnu/libutil.so.1       /lib/
COPY --from=lotus-builder /usr/lib/x86_64-linux-gnu/libltdl.so.7   /lib/
COPY --from=lotus-builder /usr/lib/x86_64-linux-gnu/libnuma.so.1   /lib/
COPY --from=lotus-builder /usr/lib/x86_64-linux-gnu/libhwloc.so.5  /lib/
COPY --from=lotus-builder /usr/lib/x86_64-linux-gnu/libOpenCL.so.1 /lib/

RUN useradd -r -u 532 -U fc \
 && mkdir -p /etc/OpenCL/vendors \
 && echo "libnvidia-opencl.so.1" > /etc/OpenCL/vendors/nvidia.icd

#####################################
FROM lotus-base AS lotus
MAINTAINER Lotus Development Team

COPY --from=lotus-builder /opt/filecoin/lotus /usr/local/bin/
COPY --from=lotus-builder /opt/filecoin/lotus-shed /usr/local/bin/
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

#####################################
FROM lotus-base AS lotus-all-in-one

ENV FILECOIN_PARAMETER_CACHE /var/tmp/filecoin-proof-parameters
ENV LOTUS_MINER_PATH /var/lib/lotus-miner
ENV LOTUS_PATH /var/lib/lotus
ENV LOTUS_WORKER_PATH /var/lib/lotus-worker
ENV WALLET_PATH /var/lib/lotus-wallet

COPY --from=lotus-builder /opt/filecoin/lotus          /usr/local/bin/
COPY --from=lotus-builder /opt/filecoin/lotus-seed     /usr/local/bin/
COPY --from=lotus-builder /opt/filecoin/lotus-shed     /usr/local/bin/
COPY --from=lotus-builder /opt/filecoin/lotus-wallet   /usr/local/bin/
COPY --from=lotus-builder /opt/filecoin/lotus-gateway  /usr/local/bin/
COPY --from=lotus-builder /opt/filecoin/lotus-miner    /usr/local/bin/
COPY --from=lotus-builder /opt/filecoin/lotus-worker   /usr/local/bin/
COPY --from=lotus-builder /opt/filecoin/lotus-stats    /usr/local/bin/
COPY --from=lotus-builder /opt/filecoin/lotus-fountain /usr/local/bin/

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
