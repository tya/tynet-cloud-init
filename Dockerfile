# Multi-stage build: produce a tiny image with just the static binary.
# Target arch is set by buildx (--platform linux/arm64) in CI.
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w' -o /out/tynet-cloud-init .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tynet-cloud-init /usr/local/bin/tynet-cloud-init
EXPOSE 8443
ENTRYPOINT ["/usr/local/bin/tynet-cloud-init"]
