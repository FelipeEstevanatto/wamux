# ---- Manager (React frontend) ----
# Builds the SPA from the vendored source in evolution-go-manager/, using the
# committed package-lock.json (which pins @evoapi/design-system to 0.0.5).
# Bun is used because it installs straight from package-lock.json and is far
# faster than npm; the toolchain only exists in this stage.
FROM oven/bun:1-alpine AS manager

WORKDIR /manager
COPY evolution-go-manager/package.json evolution-go-manager/bun.lock ./
RUN bun install --frozen-lockfile
COPY evolution-go-manager/ ./
RUN bun run build

FROM golang:1.26-alpine AS build

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
RUN CGO_ENABLED=1 go build -ldflags "-X main.version=${VERSION}" -o server ./cmd/evolution-go

# Runtime base is kept on the same Alpine major.minor as the build stage
# (golang:1.26-alpine is Alpine 3.24.x). The CGO binary links dynamically against
# musl, so building on 3.24 and running on 3.24 avoids a cross-version libc
# mismatch, and 3.24 carries current ffmpeg/poppler/libjpeg/libwebp security
# fixes that 3.19.1 no longer receives.
FROM alpine:3.24 AS final

# Image metadata. This is an UNOFFICIAL community build: the labels say so, so
# nobody mistakes it for the official evoapicloud/evolution-go image. The CI
# workflow adds source/revision/version on top of these.
LABEL org.opencontainers.image.title="Evolution Go (community fork)" \
      org.opencontainers.image.description="Unofficial community build of Evolution Go. Not affiliated with, endorsed by, or an official release of Evolution Foundation." \
      org.opencontainers.image.url="https://github.com/FelipeEstevanatto/evo-gofork" \
      org.opencontainers.image.source="https://github.com/FelipeEstevanatto/evo-gofork" \
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

ENV TZ=America/Sao_Paulo

ENTRYPOINT ["/app/server"]
