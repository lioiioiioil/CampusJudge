# Build from repository root:
# docker build -t campusjudge-runner:local -f judge/Dockerfile .
FROM golang:1.25-bookworm AS supervisor
WORKDIR /build
COPY go.mod ./
COPY internal/judge/protocol ./internal/judge/protocol
COPY internal/judge/sandbox ./internal/judge/sandbox
RUN CGO_ENABLED=0 go test ./internal/judge/protocol ./internal/judge/sandbox \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /sandbox ./internal/judge/sandbox

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends gcc g++ libc6-dev python3 nodejs default-jdk-headless ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=supervisor /usr/local/go /usr/local/go
COPY --from=supervisor /sandbox /usr/local/bin/campusjudge-sandbox
ENV PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
# Only the trusted root supervisor keeps SETUID/SETGID/KILL to launch uid/gid
# 65534 without groups/capabilities and enforce timeouts. The engine supplies
# resource limits and tmpfs mounts. Submitted code never executes on the API host.
ENTRYPOINT ["/usr/local/bin/campusjudge-sandbox"]
