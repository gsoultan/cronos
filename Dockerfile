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

COPY . .
# CGO off, so the result runs on a distroless base with no libc. That is also
# what excludes the duckdb federation build, which needs cgo — a deployment
# that federates builds its own image with -tags duckdb and a base that has
# the runtime.
ENV CGO_ENABLED=0

# There is no `go mod download` layer, and its absence is the point.
#
# `go mod download` fetches the whole module graph: 326 modules, 1249MB. The
# four binaries below compile 40 of them, 460MB, and none of the nine duckdb
# modules — federation is a build tag and CGO is off, so that code is not in
# any import graph here. `go build` fetches what it compiles, so deleting the
# step removes 789MB from every build of this image.
#
# What it cost was a cached layer, and that layer was worth less than it looked.
# It only survives while go.mod and go.sum are untouched, and it is worth
# nothing at all on a fresh runner — which is what builds this: the image job in
# .github/workflows/check.yml runs a plain `docker build` on ubuntu-latest with
# no layer cache and no buildx, so on every push that layer downloaded 789MB
# nobody could use. Locally the trade is real but small: `make image` after a
# source change now re-fetches 460MB instead of none.
#
# The alternative that keeps both is a BuildKit cache mount on the build below.
# It is deliberately not used, because it turns every builder without BuildKit
# into a parse error, and this file is built by docker, podman and Apple's
# container across three architectures.

# Which build this is, passed in rather than read from the repository.
#
# `go build` stamps the commit by itself — but only when .git is there, and
# .dockerignore excludes it deliberately: it is a large directory that
# invalidates a layer on every commit and is not something the build needs.
# So an image built without this reports "unknown", which is exactly the answer
# nobody wants from the one copy of cronos that runs in somebody else's
# cluster. Pass it:
#
#   docker build --build-arg CRONOS_VERSION="$(git describe --tags --always --dirty)" .
#
# The image job in .github/workflows/check.yml builds it with this set and
# refuses an "unknown" in the result.
ARG CRONOS_VERSION=unknown
# Trimmed and stripped: the paths of the machine that built it are not
# information anybody deploying this needs, and they are information about us.
RUN LDFLAGS="-s -w -X github.com/gsoultan/cronos/internal/platform/build.version=${CRONOS_VERSION}" && \
    go build -trimpath -ldflags="$LDFLAGS" -o /out/cronosd ./cmd/cronosd && \
    go build -trimpath -ldflags="$LDFLAGS" -o /out/cronos-token ./cmd/cronos-token && \
    go build -trimpath -ldflags="$LDFLAGS" -o /out/cronos-user ./cmd/cronos-user && \
    go build -trimpath -ldflags="$LDFLAGS" -o /out/cronos-import ./cmd/cronos-import

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
# The migration tool ships in the image because the person holding four hundred
# .jrxml files is running a container, not a Go toolchain — and they need it
# before the deployment is worth keeping, not after.
COPY --from=server /out/cronos-import /usr/local/bin/cronos-import
COPY --from=typst /usr/local/bin/typst /usr/local/bin/typst
COPY --from=portal /src/apps/portal/dist /srv/portal

# Timezones, because a schedule's "first of the month at six" is a local claim.
#
# A container with no zoneinfo does not quietly resolve to UTC, which is what
# this said and is worth correcting: time.LoadLocation returns an error, the
# schedule will not arm, and the instance says so. Run in a bare alpine, cronosd
# stopped with `unknown time zone Europe/Berlin` before a listener opened.
#
# The binary carries its own copy now (time/tzdata, about 450KB), so it runs on
# whatever base somebody else builds on. This stays because Go reads the system
# database first where there is one: a host that updates tzdata for a DST law
# change should win over a copy frozen at build time.
ENV TZ=UTC

USER cronos
EXPOSE 8787

# Readiness rather than liveness: an orchestrator's own healthcheck decides
# whether to send traffic, and that is the question /v1/ready answers.
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8787/v1/ready >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/cronosd"]
