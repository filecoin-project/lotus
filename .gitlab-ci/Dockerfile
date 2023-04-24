FROM registry.sxxfuture.net/lib/docker/lotus-builder/debian:230224-002040 as builder
FROM ubuntu:20.04

COPY --from=builder /etc/ssl/certs                           /etc/ssl/certs
COPY --from=builder /lib/x86_64-linux-gnu/libdl.so.2         /lib/
COPY --from=builder /lib/x86_64-linux-gnu/librt.so.1         /lib/
COPY --from=builder /lib/x86_64-linux-gnu/libgcc_s.so.1      /lib/
COPY --from=builder /lib/x86_64-linux-gnu/libutil.so.1       /lib/
COPY --from=builder /usr/lib/x86_64-linux-gnu/libltdl.so.7   /lib/
COPY --from=builder /usr/lib/x86_64-linux-gnu/libnuma.so.1   /lib/
COPY --from=builder /usr/lib/x86_64-linux-gnu/libhwloc.so.15 /lib/
COPY --from=builder /usr/lib/x86_64-linux-gnu/libOpenCL.so.1 /lib/

RUN mkdir -p /etc/OpenCL/vendors \
 && echo "libnvidia-opencl.so.1" > /etc/OpenCL/vendors/nvidia.icd

COPY lotus lotus-* /usr/local/bin/

