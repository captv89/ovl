# SPDX-License-Identifier: AGPL-3.0-only
#
# Builds ovl-office as a single container image (architecture 12.1:
# "ovl-office ... Docker Compose"). Build context is the repo root, not
# this directory — office/main.go's `//go:embed all:webdist` needs
# web/office's Vite build products copied into office/webdist before the
# Go build runs, and the npm workspace (package.json at repo root) needs
# the whole web/ tree. Referenced by .goreleaser.yaml's `dockers_v2`
# entry and by deploy/office/docker-compose.yml.
#
#   docker build -f office/Dockerfile -t ovl-office .

FROM node:22-slim AS web-builder
WORKDIR /src
COPY package.json package-lock.json ./
COPY web/office/package.json web/office/package.json
COPY web/vessel/package.json web/vessel/package.json
RUN npm ci
COPY web/office web/office
RUN npm run build --workspace=web/office

FROM golang:1.26 AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /src/office/webdist office/webdist
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/ovl-office ./office

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go-builder /out/ovl-office /usr/local/bin/ovl-office
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ovl-office"]
