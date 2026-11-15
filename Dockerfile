# Base images come through the Harbor proxy cache (dockerhub and gcr projects), pinned by
# digest, so a base-image republish cannot silently change the toolchain or the CA bundle
# (cluster-services-1401). The digests are the multi-arch index digests, identical to upstream
# docker.io/library/golang:1.24 and gcr.io/distroless/static-debian13:nonroot on 2026-11-15.
# CI's Harbor robot holds pull on both proxy projects. To bump: `docker buildx imagetools
# inspect <ref-without-digest>` and copy the top-level Digest.
ARG GOLANG_IMAGE=harbor.support.tools/dockerhub/library/golang:1.24@sha256:d2d2bc1c84f7e60d7d2438a3836ae7d0c847f4888464e7ec9ba3a1339a1ee804
ARG DISTROLESS_IMAGE=harbor.support.tools/gcr/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931

# Build the manager binary
FROM ${GOLANG_IMAGE} AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG GIT_COMMIT=unknown

WORKDIR /workspace

# Go modules through the in-house Athens proxy, falling back to proxy.golang.org on error.
ARG GOPROXY=https://goproxy.support.tools|https://proxy.golang.org
ENV GOPROXY=${GOPROXY}

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the Go source (relies on .dockerignore to filter)
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/
COPY pkg/ pkg/

# Build with version information
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_COMMIT}" \
    -a -o manager cmd/main.go

# Fleet CA standard (cluster-services-1375/1381): no eMudhra emSign roots at all
# (ECC G3, ECC C3, Root C1, Root G1). static-debian13 drops ECC G3 but still ships
# C3, C1 and G1, so strip every emSign root from its bundle and fail the build if
# one survives. No extra fetch: it reuses the two images this build already pulls.
FROM ${DISTROLESS_IMAGE} AS cabase

FROM --platform=$BUILDPLATFORM ${GOLANG_IMAGE} AS cafilter
COPY --from=cabase /etc/ssl/certs/ca-certificates.crt /tmp/in.crt
RUN set -eu; \
    mkdir /tmp/split && cd /tmp/split; \
    awk '/-----BEGIN CERTIFICATE-----/{n++} n>0 {print > sprintf("c%04d.pem", n)}' /tmp/in.crt; \
    : > /tmp/out.crt; kept=0; \
    for f in c*.pem; do \
      subj="$(openssl x509 -in "$f" -noout -subject)"; \
      case "$subj" in \
        *[eE][mM][sS]ign*) echo "stripped: $subj" ;; \
        *) cat "$f" >> /tmp/out.crt; kept=$((kept+1)) ;; \
      esac; \
    done; \
    echo "kept $kept certs"; \
    test "$kept" -ge 100; \
    mkdir /tmp/check && cd /tmp/check; \
    awk '/-----BEGIN CERTIFICATE-----/{n++} n>0 {print > sprintf("c%04d.pem", n)}' /tmp/out.crt; \
    if for f in c*.pem; do openssl x509 -in "$f" -noout -subject; done | grep -i emsign; then \
      echo "emSign root survived" >&2; exit 1; \
    fi

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM ${DISTROLESS_IMAGE}
COPY --from=cafilter /tmp/out.crt /etc/ssl/certs/ca-certificates.crt

LABEL org.opencontainers.image.title="CNPG Storage Manager"
LABEL org.opencontainers.image.description="Automated storage management for CloudNativePG clusters"
LABEL org.opencontainers.image.source="https://github.com/supporttools/cnpg-storage-manager"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
