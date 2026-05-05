# Build Stage: using Go 1.24 image
FROM registry.access.redhat.com/ubi9/go-toolset:9.7@sha256:a5c9eaea7dd305d0c79a0ff5c620c1c7a0cff87335684ae216205faca70458f3 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/llm-d-routing-sidecar/main.go cmd/cmd.go
# COPY pkg/ pkg/
COPY internal/ internal/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make image-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o bin/llm-d-routing-sidecar cmd/cmd.go

FROM registry.access.redhat.com/ubi9/ubi:9.7@sha256:039095faabf1edde946ff528b3b6906efa046ee129f3e33fd933280bb6936221

WORKDIR /
COPY --from=builder /workspace/bin/llm-d-routing-sidecar /app/llm-d-routing-sidecar
USER 65532:65532

ENTRYPOINT ["/app/llm-d-routing-sidecar"]
