# Interlinea team server, with the web app.
#
#   docker build -t interlinea-server .
#   docker run -p 8080:8080 -v interlinea:/data interlinea-server
#
# See compose.yaml for PostgreSQL and the settings.

# The spelling dictionaries that come with Interlinea, fetched from the
# LibreOffice dictionaries once, on the machine that builds, for every
# platform of the image.
FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS dictionaries
WORKDIR /src
COPY go.mod go.sum Makefile ./
COPY langtag/ langtag/
COPY internal/unzip/ internal/unzip/
COPY spell/ spell/
RUN make dictionaries DICT_DIR=/out/dictionaries

FROM golang:1.24-bookworm AS core
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/interlinea-server ./cmd/interlinea-server \
 && mkdir -p /out/core \
 && GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o /out/core/interlinea.wasm ./cmd/interlinea-wasm \
 && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" /out/core/ \
 && mkdir -p /out/core/fonts && cp typeset/fonts/*.ttf typeset/fonts/OFL.txt /out/core/fonts/ \
 && mkdir -p /out/data

FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
COPY --from=core /out/core/ public/core/
COPY --from=dictionaries /out/dictionaries/ public/dictionaries/
RUN npm run build

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=core /out/interlinea-server /usr/local/bin/interlinea-server
COPY --from=web /src/web/dist /web
COPY LICENSE NOTICE /licenses/
COPY --from=core --chown=nonroot:nonroot /out/data /data
LABEL org.opencontainers.image.licenses="AGPL-3.0-only"
ENV INTERLINEA_ADDR=:8080 \
    INTERLINEA_DATA=/data \
    INTERLINEA_WEB=/web \
    INTERLINEA_LOG_JSON=true
VOLUME /data
EXPOSE 8080
USER nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/usr/local/bin/interlinea-server", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/interlinea-server"]
