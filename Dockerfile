# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/web
RUN npm install --global pnpm@9.12.2 --no-audit --no-fund
COPY web/package.json web/pnpm-lock.yaml ./
RUN --mount=type=cache,id=sublane-pnpm,target=/pnpm/store \
    pnpm install --frozen-lockfile --store-dir=/pnpm/store
COPY web/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/*.go web/
COPY --from=frontend /src/web/dist web/dist
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=0.1.0-dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -tags production -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.version=${VERSION}" -o /out/sublane ./cmd/sublane

# Prepare architecture-independent runtime files natively; the final stage needs no emulation.
# Keep the existing Alpine service account allocation so existing named volumes stay writable.
FROM --platform=$BUILDPLATFORM alpine:3.23 AS runtime-files
RUN test -s /etc/ssl/certs/ca-certificates.crt \
    && addgroup -S sublane && adduser -S -G sublane sublane \
    && mkdir -m 0700 /data && chown sublane:sublane /data

FROM alpine:3.23 AS runtime
ARG VERSION=0.1.0-dev
ARG REVISION=unknown
ARG SOURCE_URL=https://github.com/murongg/SubLane
LABEL org.opencontainers.image.title="SubLane" \
      org.opencontainers.image.description="A minimal subscription gateway for internal teams" \
      org.opencontainers.image.source="${SOURCE_URL}" \
      org.opencontainers.image.url="${SOURCE_URL}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.licenses="AGPL-3.0-only"
COPY --from=runtime-files /etc/passwd /etc/passwd
COPY --from=runtime-files /etc/group /etc/group
COPY --from=runtime-files /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=runtime-files --chown=sublane:sublane /data /data
WORKDIR /app
COPY --from=backend /out/sublane /usr/local/bin/sublane
COPY LICENSE THIRD_PARTY_NOTICES.md ./
COPY licenses/ ./licenses/
USER sublane:sublane
ENV SUBLANE_ADDR=0.0.0.0:8080 SUBLANE_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --retries=3 \
    CMD ["wget", "-q", "-T", "2", "-O", "/dev/null", "http://127.0.0.1:8080/readyz"]
ENTRYPOINT ["/usr/local/bin/sublane"]
