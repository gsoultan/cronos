# cronos, as an image.
#
# Three stages, because three different things need building and only one of
# them needs to be in the result: the portal is static files, the server is a
# binary, and the typesetter is somebody else's binary this depends on.

# --- The portal -------------------------------------------------------------
FROM oven/bun:1-alpine AS portal
WORKDIR /src

# The lockfile alone first, so a change to a component does not reinstall
# every dependency. --frozen-lockfile because a build that resolves a version
# for itself is a build that ships something nobody chose.
COPY apps/portal/package.json apps/portal/bun.lock* ./apps/portal/
RUN cd apps/portal && bun install --frozen-lockfile

COPY apps/portal ./apps/portal
# Baked in at build time, which is what Vite does with these: an image is per
# deployment, and a deployment knows its own API. The token is deliberately
# not among them — a token in an image is a credential in a registry.
ARG VITE_CRONOS_API=""
RUN cd apps/portal && bun run build

# --- The server -------------------------------------------------------------
# Pinned to the patch, not the minor.
#
# `golang:1.26-alpine` is whatever 1.26.x Docker Hub last built, which is the
# right thing right up until it is behind: six standard-library advisories were
# open against 1.26.5, and a tag that floats gives no way to say which one an
# image was built with. go.mod names the same version, so the two cannot drift.
FROM golang:1.26.6-alpine AS server
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO off, so the result runs on a distroless base with no libc. That is also
# what excludes the duckdb federation build, which needs cgo — a deployment
# that federates builds its own image with -tags duckdb and a base that has
# the runtime.
ENV CGO_ENABLED=0
# Trimmed and stripped: the paths of the machine that built it are not
# information anybody deploying this needs, and they are information about us.
RUN go build -trimpath -ldflags="-s -w" -o /out/cronosd ./cmd/cronosd && \
    go build -trimpath -ldflags="-s -w" -o /out/cronos-token ./cmd/cronos-token && \
    go build -trimpath -ldflags="-s -w" -o /out/cronos-user ./cmd/cronos-user

# --- The typesetter ---------------------------------------------------------
#
# Paginated output shells out to `typst`. Without it in the image, PDF fails at
# six in the morning on the first of the month and nowhere earlier — every
# other path works, so nothing before then goes wrong.
FROM alpine:3.20 AS typst
ARG TYPST_VERSION=0.12.0
RUN apk add --no-cache curl tar xz && \
    arch="$(uname -m)" && \
    case "$arch" in \
      x86_64) target=x86_64-unknown-linux-musl ;; \
      aarch64) target=aarch64-unknown-linux-musl ;; \
      *) echo "no typst build for $arch" >&2; exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/typst-${target}.tar.xz" \
      | tar -xJ -C /tmp && \
    mv "/tmp/typst-${target}/typst" /usr/local/bin/typst && \
    chmod +x /usr/local/bin/typst && \
    typst --version

# --- The result -------------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    # A user, because a report engine that reads somebody else's warehouse
    # should not be doing it as root.
    adduser -D -u 10001 cronos

COPY --from=server /out/cronosd /usr/local/bin/cronosd
COPY --from=server /out/cronos-token /usr/local/bin/cronos-token
COPY --from=server /out/cronos-user /usr/local/bin/cronos-user
COPY --from=typst /usr/local/bin/typst /usr/local/bin/typst
COPY --from=portal /src/apps/portal/dist /srv/portal

# Timezones, because a schedule's "first of the month at six" is a local claim
# and a container with no zoneinfo resolves every timezone to UTC — which is a
# statement dated an hour early in the wrong month, and a support ticket.
ENV TZ=UTC

USER cronos
EXPOSE 8787

# Readiness rather than liveness: an orchestrator's own healthcheck decides
# whether to send traffic, and that is the question /v1/ready answers.
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8787/v1/ready >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/cronosd"]
