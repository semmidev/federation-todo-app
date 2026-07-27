# One-shot composer: waits for both subgraphs (via docker-compose
# depends_on/healthcheck), fetches each subgraph's live federation SDL, and
# composes the supergraph schema for the Apollo Router.
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN curl -sSL https://rover.apollo.dev/nix/latest | sh
ENV PATH="/root/.rover/bin:${PATH}"
ENV APOLLO_ELV2_LICENSE=accept

WORKDIR /workspace
COPY supergraph-config.yaml .

ENTRYPOINT ["sh", "-c", "rover supergraph compose --config supergraph-config.yaml --output /output/supergraph.graphql"]
