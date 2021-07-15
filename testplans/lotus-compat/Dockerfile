ARG GO_VERSION=1.16.5



FROM golang:${GO_VERSION}-buster AS compat-base
RUN apt update && apt install -y mesa-opencl-icd ocl-icd-opencl-dev gcc git bzr jq pkg-config mesa-opencl-icd ocl-icd-opencl-dev libhwloc-dev bzr
RUN ldconfig



FROM compat-base AS compat
ARG LOTUS_OLD_VERSION=d920b47e7ef8945cdcdbae96a2ab56cecacd1414
ARG LOTUS_NEW_VERSION=f0c9e4e8f2b66e14cba3c47823b72e5f7c6be6ae
RUN git clone https://github.com/filecoin-project/lotus.git lotus-old && cd lotus-old && git checkout ${LOTUS_OLD_VERSION} && make clean && make debug
RUN git clone https://github.com/filecoin-project/lotus.git lotus-new && cd lotus-new && git checkout ${LOTUS_NEW_VERSION} && make clean && make debug
### Fetch proof params
RUN /go/lotus-new/lotus fetch-params 2KiB
RUN /go/lotus-new/lotus fetch-params 8MiB
RUN mv /go/lotus-old /lotus-old/
RUN mv /go/lotus-new /lotus-new/



FROM compat-base AS testplan-builder
ARG TESTPLAN_EXEC_PKG="."
ARG GO_PROXY=http://localhost:8081
ARG BUILD_TAGS
ENV TESTPLAN_EXEC_PKG ${TESTPLAN_EXEC_PKG}
ENV PLAN_DIR /plan/
COPY /plan/go.mod ${PLAN_DIR}
### Copy Lotus source
RUN echo "Using go proxy: ${GO_PROXY}" \
    && cd ${PLAN_DIR} \
    && go env -w GOPROXY="${GO_PROXY}" \
    && go mod download
COPY . /
RUN cd ${PLAN_DIR} \
    && go env -w GOPROXY="${GO_PROXY}" \
    && GOOS=linux GOARCH=amd64 go build -o testplan ${BUILD_TAGS} ${TESTPLAN_EXEC_PKG}
RUN cd ${PLAN_DIR} \
  && go list -m all > /testground_dep_list



FROM compat-base
### Copy proof params
COPY --from=compat /var/tmp/filecoin-proof-parameters /var/tmp/filecoin-proof-parameters
### Copy Lotus source and binaries
COPY --from=compat /lotus-old /lotus-old
COPY --from=compat /lotus-new /lotus-new
### Copy sample txt files
COPY --from=testplan-builder /plan/quick-brown-fox-10.txt /qbf10.txt
COPY --from=testplan-builder /plan/quick-brown-fox-8.txt /qbf8.txt
### Copy testplan
COPY --from=testplan-builder /testground_dep_list /
COPY --from=testplan-builder /plan/testplan /
EXPOSE 6060
ENTRYPOINT [ "/testplan"]
