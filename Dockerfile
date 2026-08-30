# syntax=docker/dockerfile:1

FROM alpine:latest

ARG TARGETARCH=amd64

RUN addgroup -S -g 10001 bluescale \
    && adduser -S -D -H -u 10001 -G bluescale bluescale \
    && mkdir -p /data \
    && chown bluescale:bluescale /data

COPY --chmod=0755 dist/bluescale-linux-${TARGETARCH} /usr/local/bin/bluescale

ENV BLUESCALE_ADDR=:8080 \
    BLUESCALE_DATA_DIR=/data

WORKDIR /app
VOLUME ["/data"]
EXPOSE 8080

USER 10001:10001
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/bluescale"]
