FROM oven/bun:1@sha256:0733e50325078969732ebe3b15ce4c4be5082f18c4ac1a0f0ca4839c2e4e42a7 AS builder

WORKDIR /build/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY ./web ./
COPY ./VERSION /build/VERSION
ARG MYAPI_BRAND_NAME=MyAPI
ARG MYAPI_BRAND_LOGO=/myapi-logo-v1.png
RUN DISABLE_ESLINT_PLUGIN='true' \
    VITE_SELF_USE_MINIMAL='true' \
    VITE_BRAND_NAME="${MYAPI_BRAND_NAME}" \
    VITE_BRAND_LOGO="${MYAPI_BRAND_LOGO}" \
    VITE_REACT_APP_VERSION=$(cat /build/VERSION) \
    bun run build

FROM golang:1.26.1-alpine@sha256:2389ebfa5b7f43eeafbd6be0c3700cc46690ef842ad962f6c5bd6be49ed82039 AS builder2
ENV GO111MODULE=on CGO_ENABLED=0 GOWORK=off

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

ADD go.mod go.sum ./
# relaykit is a local submodule referenced via replace; its go.mod must be
# present for go mod download to resolve the main module graph.
ADD relaykit/go.mod ./relaykit/go.mod
RUN go mod download

COPY . .
COPY --from=builder /build/web/dist ./web/dist
RUN module_path="$(go list -m)" && \
    go build -ldflags "-s -w -X ${module_path}/common.Version=$(cat VERSION)" -o my-api

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder2 /build/my-api /
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/my-api"]
