# syntax=docker/dockerfile:1.7
#
# Images for axx's agent evals. Build context: the repository root. Built by
# `go run ./cmd/evals images`:
#
#   --target base      axx-evals-base: the agent's environment. A released axx,
#                      as users install it, with the packs of the starting
#                      projects prepared ahead (so a trial never waits for axx to
#                      prepare them), the parcels service, Go, and the starting
#                      projects (/opt/evals/workspace, and the README of the one
#                      without axx in /opt/evals/workspace-plain).
#   --target verifier  axx-evals-verifier: base plus the verifier. Only the
#                      separate verifier container uses it, so the agent never
#                      sees the checks or the list of deliberate bugs.

ARG GO_IMAGE=golang:1.27
ARG NODE_IMAGE=node:22-bookworm-slim
ARG UV_IMAGE=ghcr.io/astral-sh/uv:0.9.11

FROM ${GO_IMAGE} AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/parcels ./app \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/verifier/evals-verify ./cmd/evals-verify

# The axx under test: a release, checked against its checksum (images/base/axx.env,
# which `go run ./cmd/evals images` passes in).
FROM ${GO_IMAGE} AS axx
ARG AXX_VERSION
ARG AXX_SHA256_AMD64
ARG AXX_SHA256_ARM64
ARG TARGETARCH
RUN set -eu; \
    case "$TARGETARCH" in \
      amd64) sum="$AXX_SHA256_AMD64" ;; \
      arm64) sum="$AXX_SHA256_ARM64" ;; \
      *) echo "no axx release for $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    test -n "$AXX_VERSION" && test -n "$sum" || { echo "AXX_VERSION and its checksums are build arguments (images/base/axx.env)" >&2; exit 1; }; \
    tarball="axx_${AXX_VERSION}_linux_${TARGETARCH}.tar.gz"; \
    curl -fsSL -o "/tmp/$tarball" "https://github.com/nimbusxr/axx/releases/download/v${AXX_VERSION}/$tarball"; \
    echo "$sum  /tmp/$tarball" | sha256sum -c -; \
    mkdir -p /out && tar -xzf "/tmp/$tarball" -C /out axx

FROM ${UV_IMAGE} AS uv

FROM ${NODE_IMAGE} AS base
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       bash ca-certificates curl git jq less procps python3 python3-pip python3-venv \
    && rm -rf /var/lib/apt/lists/*
COPY --from=uv /uv /uvx /usr/local/bin/
COPY --from=axx /out/axx /usr/local/bin/
COPY --from=go /out/parcels /usr/local/bin/
# Go for tests without axx (the plain condition), as Node.js and Python are.
COPY --from=go /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH
RUN git config --system user.name "Parcels team" \
    && git config --system user.email "parcels-team@example.com" \
    && git config --system init.defaultBranch main \
    && git config --system --add safe.directory '*' \
    && useradd --create-home --uid 1001 parcels \
    && useradd --create-home --uid 1002 tester
# One cache for every user (the agent, and the verifier's tester), so the packs
# prepared here serve them all.
ENV XDG_CACHE_HOME=/var/cache/evals
COPY workspace /opt/evals/workspace
# Prepare the packs of the starting projects: the common one's, and those of
# `axx init`, for the tasks that start from nothing.
RUN set -eu; \
    mkdir -p "$XDG_CACHE_HOME"; \
    (cd /opt/evals/workspace && axx steps --json >/dev/null); \
    scratch=$(mktemp -d); (cd "$scratch" && axx init --no-ci --no-agents >/dev/null && axx steps --json >/dev/null); rm -rf "$scratch"; \
    chmod -R a+rwX "$XDG_CACHE_HOME"
COPY workspace-plain /opt/evals/workspace-plain
COPY images/base/bin/ /usr/local/bin/
WORKDIR /app

FROM base AS verifier
COPY --from=go /out/verifier/ /opt/evals/bin/
