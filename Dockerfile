# syntax=docker/dockerfile:1

FROM golang:1.26.6-bookworm AS build

WORKDIR /src

# Dependencies first, so editing a handler does not re-download the module graph.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# The release workflow passes the git tag. Left at dev for a local or CI build, which
# is exactly what an unversioned binary should call itself.
ARG VERSION=dev

# CGO off because the runtime image has no libc to link against. -trimpath keeps the
# build directory out of the binary, which is both smaller and one less thing leaked in
# a panic trace.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/bot ./cmd/bot

# distroless static carries CA certificates and nothing else: no shell, no package
# manager, no libc. The bot talks to Discord and to raider-mate-service over TLS and
# writes nothing to disk, so that is the whole runtime.
FROM gcr.io/distroless/static:nonroot

# 65532 is distroless's nonroot user. Stated numerically rather than by name so that a
# Kubernetes runAsNonRoot check can verify it without resolving /etc/passwd.
USER 65532:65532

COPY --from=build --chown=65532:65532 /out/bot /bot

ENTRYPOINT ["/bot"]
