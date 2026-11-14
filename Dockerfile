# Build the manager binary
FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG GIT_COMMIT=unknown

WORKDIR /workspace

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
FROM gcr.io/distroless/static-debian13:nonroot AS cabase

FROM --platform=$BUILDPLATFORM golang:1.24 AS cafilter
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
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=cafilter /tmp/out.crt /etc/ssl/certs/ca-certificates.crt

LABEL org.opencontainers.image.title="CNPG Storage Manager"
LABEL org.opencontainers.image.description="Automated storage management for CloudNativePG clusters"
LABEL org.opencontainers.image.source="https://github.com/supporttools/cnpg-storage-manager"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
