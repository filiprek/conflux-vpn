FROM ubuntu:24.04@sha256:534baea6a22c03a63003dbc8dbe78fe34bc0d7e595d9a9dc9834884ff530eb55
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates curl iproute2 nftables && rm -rf /var/lib/apt/lists/*
COPY .build/conflux-relay /usr/local/bin/conflux-relay
COPY .build/conflux-bench /usr/local/bin/conflux-bench
COPY .build/conflux-lab-shaper /usr/local/bin/conflux-lab-shaper
COPY .build/conflux-lab-client /usr/local/bin/conflux-lab-client
COPY .build/sing-box /usr/local/bin/sing-box
COPY relay-entry.sh client-entry.sh shape.sh /lab/

# Keep normal TLS validation while trusting the current environment egress CA.
COPY .build/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
