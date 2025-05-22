FROM --platform=$BUILDPLATFORM golang:1.24.3 AS build

LABEL org.opencontainers.image.source=https://github.com/danroux/sk8l-api
LABEL org.opencontainers.image.description="sk8l-api image"
LABEL org.opencontainers.image.licenses=MIT

WORKDIR /src/

ARG TARGETOS TARGETARCH
RUN go env GOCACHE
ENV GOMODCACHE=/go/pkg/mod
ENV GOCACHE=/root/.cache/go-build
RUN go env GOCACHE

# COPY go.mod go.sum ./
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download -x

COPY *.go .
COPY protos/ ./protos
COPY Makefile .
COPY annotations.tmpl .

RUN --mount=type=cache,target=/root/.cache/go-build make go-out

COPY . .

FROM alpine:3.21.3

WORKDIR /app/

USER 1001

EXPOSE 8585

COPY --from=build /src/sk8l /app

CMD ["/app/sk8l"]
