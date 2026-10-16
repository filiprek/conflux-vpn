# Conflux relay container. Build context: a directory containing the linux/amd64
# conflux-relay binary and entrypoint.sh (see .github/workflows/release.yml).
FROM ubuntu:24.04@sha256:534baea6a22c03a63003dbc8dbe78fe34bc0d7e595d9a9dc9834884ff530eb55
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends iproute2 nftables ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY conflux-relay /usr/local/bin/conflux-relay
COPY entrypoint.sh /usr/local/bin/conflux-relay-entrypoint
VOLUME /etc/conflux
EXPOSE 4433/udp
HEALTHCHECK --interval=60s --timeout=15s --start-period=20s \
  CMD conflux-relay --config /etc/conflux/relay.json --health >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/conflux-relay-entrypoint"]
LABEL org.opencontainers.image.source="https://github.com/filiprek/conflux-vpn" \
      org.opencontainers.image.description="Conflux multipath VPN relay" \
      org.opencontainers.image.licenses="Apache-2.0"
