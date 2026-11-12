# Build stage — cross-compile from the build host's arch to $TARGETARCH so a
# multi-arch `IMAGE_PLATFORMS` builds without QEMU emulating the compiler.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:eb61c3873c97416fff0116553fff65d67a3549f72e11d9758849a689cdd8b523 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY src/ ./src/
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -trimpath -buildvcs=false \
    -ldflags="-w -s -buildid=" \
    -o webhook \
    ./src/

# Final stage
FROM gcr.io/distroless/static:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931

COPY --from=builder /build/webhook /webhook

ENTRYPOINT ["/webhook"]
