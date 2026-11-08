# Build stage — cross-compile from the build host's arch to $TARGETARCH so a
# multi-arch `IMAGE_PLATFORMS` builds without QEMU emulating the compiler.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:87c65a688975c0b1d4585d204874c45ca78354670e3a5f11368c5a492303d696 AS builder

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
FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

COPY --from=builder /build/webhook /webhook

ENTRYPOINT ["/webhook"]
