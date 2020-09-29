# FROM golang:1.12-alpine as build

# for using the go race detector tool
FROM golang:1.15-alpine as build

# Install SSL certificates
RUN apk update && apk add --no-cache git gcc musl-dev

# Build static mariner binary
RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .
# original
# RUN go build -ldflags "-linkmode external -extldflags -static" -o bin/mariner

# DEBUG - including race detector into build
RUN go build -race -ldflags "-linkmode external -extldflags -static" -o bin/mariner

FROM alpine
COPY --from=build /go/src/github.com/uc-cdis/mariner/bin/mariner /go/src/github.com/uc-cdis/mariner/Docker/engine/engineDockerrun.sh /
RUN apk update && apk add --no-cache ca-certificates

ENTRYPOINT [ "/bin/sh", "/engineDockerrun.sh" ]