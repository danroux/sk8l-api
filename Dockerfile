FROM golang:1.22.0 AS build

WORKDIR /src/

RUN go env GOCACHE
ENV GOCACHE /gocache
ENV GOMODCACHE /gomodcache
RUN mkdir /gocache /gomodcache
RUN go env GOCACHE

# COPY go.mod go.sum ./
COPY go.mod .
COPY go.sum .
COPY *.go .
COPY protos/ ./protos
COPY Makefile .

RUN --mount=type=cache,target=/gomodcache go mod download -x
RUN --mount=type=cache,target=/gocache make go-out

COPY . .

FROM alpine:3.18.3

WORKDIR /app/

USER 1001

EXPOSE 8585

COPY --from=build /src/sk8l /app

CMD ["/app/sk8l"]
