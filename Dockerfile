# https://docs.docker.com/build/building/multi-stage/
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
# https://docs.docker.com/build/building/multi-platform/#cross-compilation
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/campusjudge ./cmd/campusjudge

# The CLI talks to the host engine through a socket mounted at runtime. This
# image does not run its own Docker daemon or execute student code directly.
FROM docker:29-cli
WORKDIR /app
COPY --from=build /out/campusjudge /usr/local/bin/campusjudge
EXPOSE 8080
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=15s --timeout=5s --start-period=30s --retries=3 CMD ["/usr/local/bin/campusjudge", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/campusjudge"]
CMD ["-addr", "0.0.0.0:8080"]
