ARG CURIO_TEST_IMAGE=curio/curio-all-in-one:latest
#############################################################################
FROM ${CURIO_TEST_IMAGE}

ARG BUILD_VERSION=0.1

LABEL org.opencontainers.image.version=$BUILD_VERSION \
      org.opencontainers.image.authors="Curio Dev Team" \
      name="lotus-dev" \
      maintainer="Curio Dev Team" \
      vendor="Curio Dev Team" \
      version=$BUILD_VERSION \
      release=$BUILD_VERSION \
      summary="This image is used to host the curio dev service" \
      description="This image is used to host the curio dev service"

EXPOSE 12300 4701 32100

VOLUME /var/tmp/filecoin-proof-parameters
VOLUME /var/lib/genesis
VOLUME /var/lib/builtin-actors

WORKDIR /app
RUN mkdir -p /app

COPY entrypoint.sh /app

USER root

ENTRYPOINT ["./entrypoint.sh"]
