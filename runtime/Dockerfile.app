# syntax=docker/dockerfile:1
ARG ARTEX_IMAGE=ghcr.io/jiwoochris/artex-global:latest

FROM node:22-bookworm-slim AS frontend
WORKDIR /src/web
ENV NEXT_TELEMETRY_DISABLED=1
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --include=dev
COPY web/ ./
RUN npm run build:static

FROM golang:1.26 AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=frontend /src/web/out ./server/webui/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -mod=readonly -tags embedui -trimpath -o /artex ./cmd/artex

FROM ${ARTEX_IMAGE}
COPY --from=backend /artex /app/artex
