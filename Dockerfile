FROM golang:1.24.2 AS build

LABEL org.opencontainers.image.source=https://github.com/danroux/sk8l-api
LABEL org.opencontainers.image.description="sk8l-api image"
LABEL org.opencontainers.image.licenses=MIT

WORKDIR /src/

ARG TARGETOS TARGETARCH
RUN go env GOCACHE
ENV GOCACHE=/gocache
ENV GOMODCACHE=/gomodcache
RUN mkdir /gocache /gomodcache
RUN go env GOCACHE

# COPY go.mod go.sum ./
COPY go.mod .
COPY go.sum .
COPY *.go .
COPY protos/ ./protos
COPY Makefile .
COPY annotations.tmpl .

RUN --mount=type=cache,target=/gomodcache go mod download -x
RUN --mount=type=cache,target=/gocache make go-out

COPY . .

FROM alpine:3.21.3

WORKDIR /app/

USER 1001

EXPOSE 8585

COPY --from=build /src/sk8l /app

CMD ["/app/sk8l"]
