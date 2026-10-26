FROM --platform=$BUILDPLATFORM golang:1.27.3 AS build

WORKDIR /src/

ARG TARGETOS TARGETARCH
ENV GOMODCACHE=/go/pkg/mod
ENV GOCACHE=/root/.cache/go-build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,id=gomodcache go mod download -x

COPY *.go .
COPY protos/ ./protos
COPY internal/ ./internal
COPY Makefile .
COPY dashboards/ ./dashboards

RUN --mount=type=cache,target=/go/pkg/mod,id=gomodcache \
    --mount=type=cache,target=/root/.cache/go-build,id=gocache \
    make go-out

FROM alpine:3.24.3

LABEL org.opencontainers.image.source=https://github.com/danroux/sk8l-api
LABEL org.opencontainers.image.description="sk8l-api image"
LABEL org.opencontainers.image.licenses=MIT

WORKDIR /app/

USER 1001

EXPOSE 8585

COPY --from=build /src/sk8l /app

CMD ["/app/sk8l"]
