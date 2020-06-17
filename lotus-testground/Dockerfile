#:::
#::: BUILD CONTAINER
#:::

# GO_VERSION is the golang version this image will be built against.
ARG GO_VERSION=1.14.2

# This Dockerfile performs a multi-stage build and RUNTIME_IMAGE is the image
# onto which to copy the resulting binary. 
# Picking a different runtime base image from the build image allows us to
# slim down the deployable considerably.
#
# The user can override the runtime image by passing in the appropriate builder
# configuration option.
#ARG RUNTIME_IMAGE=busybox:1.31.1-glibc

# Dynamically select the golang version.
FROM golang:${GO_VERSION}-buster AS builder

RUN apt-get update && apt-get install -y ca-certificates llvm clang mesa-opencl-icd ocl-icd-opencl-dev jq gcc git bzr pkg-config
ARG LOTUS_VERSION=master
RUN git clone https://github.com/filecoin-project/lotus.git ${PLAN_DIR}/../lotus && cd ${PLAN_DIR}/../lotus && git checkout ${LOTUS_VERSION} && git submodule update --init && make 2k
RUN cd ${PLAN_DIR}/../lotus/extern/filecoin-ffi \
    && make \
    && mkdir /tmp/filecoin-ffi \
    && cp -R ${PLAN_DIR}/../lotus/extern/filecoin-ffi /tmp

# TESTPLAN_EXEC_PKG is the executable package of the testplan to build.
# The image will build that package only.
ARG TESTPLAN_EXEC_PKG="."

# GO_PROXY is the go proxy that will be used, or direct by default.
ARG GO_PROXY=http://testground-goproxy:8081

# BUILD_TAGS is either nothing, or when expanded, it expands to "-tags <comma-separated build tags>"
ARG BUILD_TAGS

ENV TESTPLAN_EXEC_PKG ${TESTPLAN_EXEC_PKG}

# PLAN_DIR is the location containing the plan source inside the container.
ENV PLAN_DIR /plan

# Copy only go.mod files and download deps, in order to leverage Docker caching.
COPY /plan/go.mod ${PLAN_DIR}/go.mod

# Download deps.
RUN echo "Using go proxy: ${GO_PROXY}" \
    && cd ${PLAN_DIR} \
    && go env -w GOPROXY="${GO_PROXY}" \
    && go mod download

# Now copy the rest of the source and run the build.
COPY . /

RUN cd ${PLAN_DIR} \
    && go env -w GOPROXY="${GO_PROXY}" \
    && GOOS=linux GOARCH=amd64 go build -o testplan ${BUILD_TAGS} ${TESTPLAN_EXEC_PKG}

# Store module dependencies
RUN cd ${PLAN_DIR} \
  && go list -m all > /testground_dep_list

#:::
#::: RUNTIME CONTAINER
#:::

#FROM ${RUNTIME_IMAGE} AS binary

#COPY /usr/lib/x86_64-linux-gnu/* /lib/x86_64-linux-gnu/* /usr/lib/
#COPY /testground_dep_list /
RUN cp /plan/testplan /

EXPOSE 6060
ENTRYPOINT [ "/testplan"]

