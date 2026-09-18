FROM node:24-alpine AS frontend
WORKDIR /src/web
RUN npm install --global pnpm@9.12.2
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/*.go web/
COPY --from=frontend /src/web/dist web/dist
ARG VERSION=0.1.0-dev
RUN CGO_ENABLED=0 go build -tags production -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /sublane ./cmd/sublane

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S sublane && adduser -S -G sublane sublane && mkdir /data && chown sublane:sublane /data
WORKDIR /app
COPY --from=backend /sublane /usr/local/bin/sublane
COPY LICENSE THIRD_PARTY_NOTICES.md ./
COPY licenses/ ./licenses/
USER sublane
ENV SUBLANE_ADDR=0.0.0.0:8080 SUBLANE_DATA_DIR=/data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s CMD wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1
ENTRYPOINT ["/usr/local/bin/sublane"]
