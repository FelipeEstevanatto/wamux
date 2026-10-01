# ---- Manager (React frontend) ----
# Builds the SPA from the vendored source in wamux-manager/, using the
# committed package-lock.json (which pins @evoapi/design-system to 0.0.5).
# Bun is used because it installs straight from package-lock.json and is far
# faster than npm; the toolchain only exists in this stage.
FROM oven/bun:1-alpine AS manager

WORKDIR /manager
COPY wamux-manager/package.json wamux-manager/bun.lock ./
RUN bun install --frozen-lockfile
COPY wamux-manager/ ./
RUN bun run build

FROM golang:1.26.8-alpine AS build

RUN apk update && apk add --no-cache git build-base libjpeg-turbo-dev libwebp-dev

WORKDIR /build

# Copiar apenas arquivos de dependências primeiro para cachear o download
COPY go.mod go.sum ./

# whatsmeow agora vem do proxy oficial (go.mau.fi/whatsmeow, sem replace local) —
# não há mais submódulo whatsmeow-lib para copiar.
RUN go mod download

# Copiar o restante do código
COPY . .

# Use the freshly built manager assets instead of the committed ones. Fork-only
# files under manager/dist (e.g. dashboard.html) are not produced by the Vite
# build, so they come from the repo copy above and are preserved.
RUN rm -rf manager/dist/assets manager/dist/index.html
COPY --from=manager /manager/dist/assets ./manager/dist/assets
COPY --from=manager /manager/dist/index.html ./manager/dist/index.html

ARG VERSION=dev
# go_json selects the fast JSON encoder (goccy/go-json) via pkg/jsonx. It is a
# drop-in and part of the standard build for this fork; drop the tag to fall back
# to encoding/json.
ARG GO_JSON_TAG=go_json
RUN CGO_ENABLED=1 go build -tags "${GO_JSON_TAG}" -ldflags "-X main.version=${VERSION}" -o server ./cmd/wamux

# Runtime base is kept on the same Alpine major.minor as the build stage
# (golang:1.26.8-alpine is Alpine 3.24.x). The CGO binary links dynamically against
# musl, so building on 3.24 and running on 3.24 avoids a cross-version libc
# mismatch, and 3.24 carries current ffmpeg/poppler/libjpeg/libwebp security
# fixes that 3.19.1 no longer receives.
FROM alpine:3.24 AS final

# Image metadata. This is an independent build; the labels mark it as such.
# The CI workflow adds source/revision/version on top of these.
LABEL org.opencontainers.image.title="WaMux" \
      org.opencontainers.image.description="WaMux — independent, self-hosted WhatsApp API (originally forked from Evolution Go). Not affiliated with, endorsed by, or an official release of Evolution Foundation." \
      org.opencontainers.image.url="https://github.com/FelipeEstevanatto/wamux" \
      org.opencontainers.image.source="https://github.com/FelipeEstevanatto/wamux" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.vendor="FelipeEstevanatto (community fork, not Evolution Foundation)"

# poppler-utils provides pdftoppm, used to rasterize PDF page 1 for /send/media document thumbnails
RUN apk update && apk add --no-cache tzdata ffmpeg libjpeg-turbo libwebp poppler-utils

WORKDIR /app

COPY --from=build /build/server .
COPY --from=build /build/manager/dist ./manager/dist
COPY --from=build /build/VERSION ./VERSION

# Apache-2.0 §4(a): ship the license text with the Object form. NOTICE and
# TRADEMARKS carry the additional conditions, and FORK_NOTES explains what this
# fork changed (Apache-2.0 §4(b)).
COPY --from=build /build/LICENSE /build/NOTICE /build/TRADEMARKS.md /build/FORK_NOTES.md ./

# Run as an unprivileged user, not root. A non-root UID limits the blast radius
# of any compromise: no writes outside the mounted data volume, no ability to
# ptrace/modify other processes. The data and media directories are owned by the
# same UID so the app can still write logs, the sqlite fallback and stored media.
ARG UID=10001
RUN addgroup -S -g ${UID} wamux \
    && adduser -S -u ${UID} -G wamux -h /app -s /sbin/nologin wamux \
    && mkdir -p /app/data/logs /app/data/media \
    && chown -R wamux:wamux /app

USER wamux

ENV TZ=America/Sao_Paulo
ENV LOG_DIRECTORY=/app/data/logs

ENTRYPOINT ["/app/server"]
